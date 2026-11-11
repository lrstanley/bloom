// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Buffer holds the lines of text a [Viewport] shows, such as a log or a
// file, kept in the model and changed as the text changes. A viewport only
// lays out and draws the lines in sight, and the buffer keeps what it works
// out about its lines, such as how wide each is, so that appending a line
// to a buffer of a million costs about as much as appending one to a buffer
// of ten.
//
// Lines may hold ANSI styles and hyperlinks. A style left open at the end
// of a line carries on into the next, as it would in a terminal, so the
// output of a syntax highlighter that styles a comment over many lines is
// drawn as it would be printed. Tabs are drawn as spaces to the next stop,
// every four columns.
//
// The zero value is an empty buffer, ready to use. A Buffer is not safe for
// use by more than one goroutine at a time: change it in Update, as the rest
// of the model is.
type Buffer struct {
	lines []string
	// pens is the style each line starts in, carried on from the lines
	// before it, as an index into penTab plus one, or zero for none. It is
	// nil until a line starts in a style.
	pens   []int32
	penTab []pen
	end    pen // the style at the end of the last line

	rev   uint64 // counts changes
	reset uint64 // rev of the last change that was not an append

	measures [2]*bufMeasure // most recently used first
	indexes  [2]*wrapIndex
	rows     []int32      // scratch for counting the rows of lines
	long     [4]*longLine // the long lines last shown, most recent first
}

// longLine is what a buffer keeps of a line at least longLineBytes long
// that a viewport shows: its checkpoints, measured with method, as far
// along it as they were wanted, or to its end once done, and the cells its
// rows start at when last wrapped, to width with mode.
type longLine struct {
	line   int
	method ansi.Method
	cps    []checkpoint
	done   bool
	width  int
	mode   WrapMode
	starts []int
}

// reach adds checkpoints to l, of line s, until there is one within
// checkpointEvery cells before cell a, or the line ends, scanning on from
// the last one rather than from the line's start.
func (l *longLine) reach(s string, a int) {
	last := l.cps[len(l.cps)-1]
	if l.done || a < last.x+checkpointEvery {
		return
	}
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	sc := last.scanner(s, l.method, p)
	// A checkpoint starts at the first command before its glyph, if there
	// is one, so that a trusted viewport draws it with the glyph.
	sc.pass = true
	cmd := -1
	var cmdPen pen
	next := last.x + checkpointEvery
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		if k == glyphPass && cmd < 0 {
			cmd, cmdPen = sc.at, sc.pen
		}
		if k != glyphText {
			continue
		}
		if sc.x >= next && sc.w > 0 && !sc.tab {
			cp := checkpoint{x: sc.x, i: sc.at, pen: sc.pen}
			if cmd >= 0 {
				cp.i, cp.pen = cmd, cmdPen
			}
			l.cps = append(l.cps, cp)
			if a < sc.x+checkpointEvery {
				return
			}
			next = sc.x + checkpointEvery
		}
		if sc.w > 0 {
			cmd = -1
		}
	}
	l.done = true
}

// longLine returns what the buffer keeps of line i, a long line, measured
// with method, working it out the first time it is asked for.
func (b *Buffer) longLine(i int, method ansi.Method) *longLine {
	for k, l := range b.long {
		if l != nil && l.line == i && l.method == method {
			copy(b.long[1:k+1], b.long[:k])
			b.long[0] = l
			return l
		}
	}
	l := &longLine{line: i, method: method, cps: []checkpoint{{pen: b.penOf(i)}}}
	copy(b.long[1:], b.long[:len(b.long)-1])
	b.long[0] = l
	return l
}

// cutFrom returns where to scan line i from, measured with method, to reach
// cell a: its start, or the checkpoint before a of a long line.
func (b *Buffer) cutFrom(i int, method ansi.Method, a int) checkpoint {
	if len(b.lines[i]) < longLineBytes || a < checkpointEvery {
		return checkpoint{pen: b.penOf(i)}
	}
	l := b.longLine(i, method)
	l.reach(b.lines[i], a)
	k, _ := slices.BinarySearchFunc(l.cps, a+1, func(cp checkpoint, x int) int { return cp.x - x })
	return l.cps[max(k-1, 0)]
}

// rowStarts returns the cells the rows of line i start at, wrapped to w
// with mode and measured with method: worked out in scratch, or kept for a
// long line, which must not be changed.
func (b *Buffer) rowStarts(i int, method ansi.Method, w int, mode WrapMode, scratch *[]int) []int {
	s := b.lines[i]
	if len(s) < longLineBytes {
		_, *scratch = wrapLine(s, method, w, mode, (*scratch)[:0])
		return *scratch
	}
	l := b.longLine(i, method)
	if l.starts == nil || l.width != w || l.mode != mode {
		_, l.starts = wrapLine(s, method, w, mode, l.starts[:0])
		l.width, l.mode = w, mode
	}
	return l.starts
}

// pen is the style and hyperlink that text is drawn in at some point of a
// line.
type pen struct {
	style uv.Style
	link  uv.Link
}

func (p *pen) zero() bool { return styleIsZero(&p.style) && p.link.URL == "" }

func (p *pen) equal(o *pen) bool { return p.style.Equal(&o.style) && p.link == o.link }

func styleIsZero(s *uv.Style) bool {
	return s.Fg == nil && s.Bg == nil && s.UnderlineColor == nil && s.Underline == 0 && s.Attrs == 0
}

// NewBuffer returns a buffer holding s, as [Buffer.SetContent] sets it.
func NewBuffer(s string) *Buffer {
	b := &Buffer{}
	b.SetContent(s)
	return b
}

// SetContent replaces the buffer's lines with those of s, split at its
// newlines. A newline at the end of s ends its last line, rather than
// starting an empty one after it, and "" leaves the buffer empty.
func (b *Buffer) SetContent(s string) {
	b.clear()
	if s != "" {
		b.appendText(s)
	}
}

// SetLines replaces the buffer's lines with lines. A line holding newlines
// is split at them.
func (b *Buffer) SetLines(lines ...string) {
	b.clear()
	b.Append(lines...)
}

// Append adds lines to the end of the buffer. A line holding newlines is
// split at them, and a newline at the end of one ends it, so lines read
// from a program's output can be given as they come. "" adds an empty
// line.
func (b *Buffer) Append(lines ...string) {
	b.lines = slices.Grow(b.lines, len(lines))
	for _, l := range lines {
		b.appendText(l)
	}
}

// Clear removes every line.
func (b *Buffer) Clear() { b.clear() }

func (b *Buffer) clear() {
	clear(b.lines)
	b.lines, b.pens, b.penTab, b.end = b.lines[:0], nil, nil, pen{}
	b.long = [4]*longLine{}
	b.rev++
	b.reset = b.rev
}

// appendText adds the lines of s, split at its newlines, which is one
// empty line for "".
func (b *Buffer) appendText(s string) {
	b.rev++
	s = strings.TrimSuffix(s, "\n")
	for {
		line, rest, more := strings.Cut(s, "\n")
		b.appendLine(strings.TrimSuffix(line, "\r"))
		if !more {
			return
		}
		s = rest
	}
}

// appendLine adds one line, noting the style it starts in and the one it
// ends in, for the line after it.
func (b *Buffer) appendLine(s string) {
	if !b.end.zero() {
		if b.pens == nil {
			b.pens = make([]int32, len(b.lines), cap(b.lines))
		}
		if n := len(b.penTab); n == 0 || !b.penTab[n-1].equal(&b.end) {
			b.penTab = append(b.penTab, b.end)
		}
		b.pens = append(b.pens, int32(len(b.penTab))) //nolint:gosec // Bounded by the lines.
	} else if b.pens != nil {
		b.pens = append(b.pens, 0)
	}
	b.lines = append(b.lines, s)
	if strings.IndexByte(s, ansi.ESC) >= 0 {
		b.end = penAfter(s, b.end)
	}
}

// Len returns the number of lines.
func (b *Buffer) Len() int { return len(b.lines) }

// Line returns line i as it was given, with its styles, or "" when there is
// no such line.
func (b *Buffer) Line(i int) string {
	if i < 0 || i >= len(b.lines) {
		return ""
	}
	return b.lines[i]
}

// Text returns the text of line i without its styles, with its tabs, as the
// function given to [ViewportNode.Marks] is given it, or "" when there is no
// such line.
func (b *Buffer) Text(i int) string {
	if i < 0 || i >= len(b.lines) {
		return ""
	}
	return plainText(b.lines[i], ansi.GraphemeWidth)
}

// String returns the buffer's lines, joined with newlines.
func (b *Buffer) String() string { return strings.Join(b.lines, "\n") }

// Match is where a regular expression matched the text of a line, as
// [Buffer.Find] finds it: the bytes from Start to End, End exclusive, of
// [Buffer.Text] of the line.
type Match struct {
	Line, Start, End int
}

// Find returns the first match of re that starts at or after byte at of the
// text of line, looking through the lines after it in turn, and whether
// there is one. It does not wrap around to the first line. To find the
// match after m, give m.Line and m.End, or m.Start+1 for an empty match.
func (b *Buffer) Find(re *regexp.Regexp, line, at int) (Match, bool) {
	if re == nil {
		return Match{}, false
	}
	for i := max(line, 0); i < len(b.lines); i++ {
		text := b.Text(i)
		from := 0
		if i == line {
			from = min(max(at, 0), len(text))
		}
		if loc := re.FindStringIndex(text[from:]); loc != nil {
			return Match{Line: i, Start: from + loc[0], End: from + loc[1]}, true
		}
	}
	return Match{}, false
}

// FindBefore returns the last match of re that starts before byte at of the
// text of line, looking through the lines before it in turn, and whether
// there is one. To find the match before m, give m.Line and m.Start.
func (b *Buffer) FindBefore(re *regexp.Regexp, line, at int) (Match, bool) {
	if re == nil {
		return Match{}, false
	}
	for i := min(line, len(b.lines)-1); i >= 0; i-- {
		text := b.Text(i)
		limit := len(text) + 1
		if i == line {
			limit = min(max(at, 0), len(text)+1)
		}
		var last []int
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if loc[0] >= limit {
				break
			}
			last = loc
		}
		if last != nil {
			return Match{Line: i, Start: last[0], End: last[1]}, true
		}
	}
	return Match{}, false
}

// penOf returns the style line i starts in.
func (b *Buffer) penOf(i int) pen {
	if i >= len(b.pens) || b.pens[i] == 0 {
		return pen{}
	}
	return b.penTab[b.pens[i]-1]
}

// bufMeasure is how wide each line of a buffer is, measured with a width
// method, for the lines up to n, as they were since the buffer's last reset.
type bufMeasure struct {
	method ansi.Method
	reset  uint64
	// widths is how many cells each line takes, with what measuring it
	// told of it in the bits above, as [measureLine] returns it.
	widths []int32
	widest int
}

// measure returns how wide the buffer's lines are with method, measuring
// only the lines added since it last did.
func (b *Buffer) measure(method ansi.Method) *bufMeasure {
	m := b.measures[0]
	if m == nil || m.method != method {
		if o := b.measures[1]; o != nil && o.method == method {
			b.measures[0], b.measures[1] = o, m
			m = o
		} else {
			m = &bufMeasure{method: method}
			b.measures[0], b.measures[1] = m, b.measures[0]
		}
	}
	if m.reset != b.reset {
		m.reset, m.widths, m.widest = b.reset, m.widths[:0], 0
	}
	from := len(m.widths)
	if from == len(b.lines) {
		return m
	}
	m.widths = slices.Grow(m.widths, len(b.lines)-from)[:len(b.lines)]
	inParallel(len(b.lines)-from, func(lo, hi int) {
		measureLines(b.lines[from+lo:from+hi], method, m.widths[from+lo:from+hi])
	})
	for _, w := range m.widths[from:] {
		m.widest = max(m.widest, int(w&widthMask))
	}
	return m
}

// measureLines sets widths to how wide each of lines is, measured with
// method, as [measureLine] returns it.
func measureLines(lines []string, method ansi.Method, widths []int32) {
	for i, s := range lines {
		widths[i] = measureLine(s, method)
	}
}

// width returns how many cells line i takes.
func (m *bufMeasure) width(i int) int { return int(m.widths[i] & widthMask) }

// wrapIndex is how many rows each line of a buffer takes, wrapped to width
// with a mode, for the lines up to n, as they were since its last reset.
type wrapIndex struct {
	method ansi.Method
	mode   WrapMode
	width  int
	reset  uint64
	n      int
	// cum is how many rows the lines before each line take, cum[n] all of
	// them, or nil while every line is one row.
	cum []int
}

// rowsBefore returns how many rows the lines before line i take.
func (x *wrapIndex) rowsBefore(i int) int {
	if x.cum == nil {
		return i
	}
	return x.cum[i]
}

// rowsOf returns how many rows line i takes.
func (x *wrapIndex) rowsOf(i int) int {
	if x.cum == nil {
		return 1
	}
	return x.cum[i+1] - x.cum[i]
}

// total returns how many rows all the lines take.
func (x *wrapIndex) total() int { return x.rowsBefore(x.n) }

// lineAt returns the line that row is in, and how far into the line's rows
// it is, for a row of the lines.
func (x *wrapIndex) lineAt(row int) (line, sub int) {
	if x.cum == nil {
		return row, 0
	}
	// The last line whose rows start at or before row.
	line, _ = slices.BinarySearch(x.cum[:x.n+1], row+1)
	line--
	return line, row - x.cum[line]
}

// index returns how many rows the buffer's lines take, wrapped to width
// with mode and measured with method, working out only the lines added since
// it last did.
func (b *Buffer) index(method ansi.Method, mode WrapMode, width int) *wrapIndex {
	m := b.measure(method)
	if mode == WrapNone || width <= 0 {
		// One row a line, which needs nothing kept.
		return &wrapIndex{method: method, mode: WrapNone, width: width, reset: b.reset, n: len(b.lines)}
	}
	match := func(x *wrapIndex) bool {
		return x != nil && x.method == method && x.mode == mode && x.width == width
	}
	x := b.indexes[0]
	switch {
	case match(x):
	case match(b.indexes[1]):
		b.indexes[0], b.indexes[1] = b.indexes[1], x
		x = b.indexes[0]
	default:
		if o := b.indexes[1]; o != nil {
			// The oldest index's storage is reused for the new one.
			o.cum = o.cum[:0]
			*o = wrapIndex{cum: o.cum}
			x = o
		} else {
			x = &wrapIndex{}
		}
		x.method, x.mode, x.width, x.reset = method, mode, width, ^b.reset
		b.indexes[0], b.indexes[1] = x, b.indexes[0]
	}
	if x.reset != b.reset {
		x.reset, x.n = b.reset, 0
		if m.widest <= width {
			x.cum = nil
		} else {
			x.cum = append(x.cum[:0], 0)
		}
	}
	if x.n == len(b.lines) {
		return x
	}
	if x.cum == nil {
		// Lines added since might wrap: rows are counted from here on.
		widest := 0
		for i := x.n; i < len(b.lines); i++ {
			widest = max(widest, m.width(i))
		}
		if widest <= width {
			x.n = len(b.lines)
			return x
		}
		cum := slices.Grow([]int(nil), len(b.lines)+1)[:x.n+1]
		for i := range cum {
			cum[i] = i
		}
		x.cum = cum
	}
	from := x.n
	rows := b.rowScratch(len(b.lines) - from)
	countRows(b.lines[from:], m.widths[from:], method, width, mode, rows)
	for k, r := range rows {
		x.cum = append(x.cum, x.cum[from+k]+int(r))
	}
	x.n = len(b.lines)
	return x
}

// rowScratch returns n rows of scratch, reused from the last time.
func (b *Buffer) rowScratch(n int) []int32 {
	if cap(b.rows) < n {
		b.rows = make([]int32, n)
	}
	return b.rows[:n]
}

// parallelLines is how many lines there must be to measure, or to count
// the rows of, before the lines are shared between the processors, which is
// worth it for a buffer set or wrapped again at a new width, such as while
// the pane it is in animates, but not for a few lines appended.
const parallelLines = 16 << 10

// inParallel calls fn for spans of n lines, from lo up to hi, that cover all
// of them, sharing a lot of lines between the processors.
func inParallel(n int, fn func(lo, hi int)) {
	workers := min(runtime.GOMAXPROCS(0), n/(parallelLines/4))
	if n < parallelLines || workers < 2 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for lo := 0; lo < n; lo += chunk {
		hi := min(lo+chunk, n)
		wg.Go(func() { fn(lo, hi) })
	}
	wg.Wait()
}

// countRows sets rows to how many rows each of lines, as wide as widths
// says, takes wrapped to width with mode.
func countRows(lines []string, widths []int32, method ansi.Method, width int, mode WrapMode, rows []int32) {
	inParallel(len(lines), func(lo, hi int) {
		countRowsOf(lines[lo:hi], widths[lo:hi], method, width, mode, rows[lo:hi])
	})
}

func countRowsOf(lines []string, widths []int32, method ansi.Method, width int, mode WrapMode, rows []int32) {
	var starts []int
	for i, s := range lines {
		n := 1
		if w := widths[i]; int(w&widthMask) > width {
			if mode == WrapChars && w&wideLine == 0 {
				n = (int(w&widthMask) + width - 1) / width
			} else {
				n, starts = wrapLine(s, method, width, mode, starts[:0])
			}
		}
		rows[i] = int32(n) //nolint:gosec // A line has far fewer rows than 1<<31.
	}
}
