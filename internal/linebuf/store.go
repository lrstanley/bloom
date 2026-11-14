// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// Store holds the lines of a buffer, and what it works out about them, such
// as how wide each is, so that appending a line to a store of a million
// costs about as much as appending one to a store of ten. A style left open
// at the end of a line carries on into the next.
//
// The zero value is an empty store, ready to use. A Store is not safe for
// use by more than one goroutine at a time.
type Store struct {
	lines []string
	// pens is the style each line starts in, carried on from the lines
	// before it, as an index into penTab plus one, or zero for none. It is
	// nil until a line starts in a style.
	pens   []int32
	penTab []pen
	end    pen // the style at the end of the last line

	rev   uint64 // counts changes
	reset uint64 // rev of the last change that was not an append

	measures [2]*Measure // most recently used first
	indexes  [2]*Index
	rows     []int32      // scratch for counting the rows of lines
	long     [4]*longLine // the long lines last shown, most recent first
}

// longLine is what a buffer keeps of a line at least LongLineBytes long
// that a viewport shows: its checkpoints, measured with method, as far
// along it as they were wanted, or to its end once done, and the cells its
// rows start at when last wrapped, to width with mode.
type longLine struct {
	line   int
	method ansi.Method
	cps    []checkpoint
	done   bool
	width  int
	mode   Mode
	starts []int
}

// reach adds checkpoints to l, of line s, until there is one within
// CheckpointEvery cells before cell a, or the line ends, scanning on from
// the last one rather than from the line's start.
func (l *longLine) reach(s string, a int) {
	last := l.cps[len(l.cps)-1]
	if l.done || a < last.x+CheckpointEvery {
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
	next := last.x + CheckpointEvery
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
			if a < sc.x+CheckpointEvery {
				return
			}
			next = sc.x + CheckpointEvery
		}
		if sc.w > 0 {
			cmd = -1
		}
	}
	l.done = true
}

// longLine returns what the buffer keeps of line i, a long line, measured
// with method, working it out the first time it is asked for.
func (b *Store) longLine(i int, method ansi.Method) *longLine {
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
func (b *Store) cutFrom(i int, method ansi.Method, a int) checkpoint {
	if len(b.lines[i]) < LongLineBytes || a < CheckpointEvery {
		return checkpoint{pen: b.penOf(i)}
	}
	l := b.longLine(i, method)
	l.reach(b.lines[i], a)
	k, _ := slices.BinarySearchFunc(l.cps, a+1, func(cp checkpoint, x int) int { return cp.x - x })
	return l.cps[max(k-1, 0)]
}

// RowStarts returns the cells the rows of line i start at, wrapped to w
// with mode and measured with method: worked out in scratch, or kept for a
// long line, which must not be changed.
func (b *Store) RowStarts(i int, method ansi.Method, w int, mode Mode, scratch *[]int) []int {
	s := b.lines[i]
	if len(s) < LongLineBytes {
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

// Clear removes every line.
func (b *Store) Clear() {
	clear(b.lines)
	b.lines, b.pens, b.penTab, b.end = b.lines[:0], nil, nil, pen{}
	b.long = [4]*longLine{}
	b.rev++
	b.reset = b.rev
}

// AppendText adds the lines of s, split at its newlines, which is one
// empty line for "". A newline at the end of s ends its last line.
func (b *Store) AppendText(s string) {
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
func (b *Store) appendLine(s string) {
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

// Grow makes room for n more lines.
func (b *Store) Grow(n int) { b.lines = slices.Grow(b.lines, n) }

// Len returns the number of lines.
func (b *Store) Len() int { return len(b.lines) }

// Line returns line i as it was given, with its styles.
func (b *Store) Line(i int) string { return b.lines[i] }

// String returns the lines, joined with newlines.
func (b *Store) String() string { return strings.Join(b.lines, "\n") }

// Rev counts the changes to the store, for those that keep what they worked
// out from its lines.
func (b *Store) Rev() uint64 { return b.rev }

// CutRow returns the cells from a up to end of line i, measured with
// method, as a string that draws them on its own, as [cutRow] does.
func (b *Store) CutRow(i int, method ansi.Method, a, end int, st *RowStyle) string {
	m := b.Measure(method)
	return cutRow(b.lines[i], b.cutFrom(i, method, a), m.widths[i], method, a, end, st)
}

// CellText returns the text of the glyphs of line i that start in the cells
// from a to end, inclusive, as [cellText] does.
func (b *Store) CellText(i int, method ansi.Method, a, end int) string {
	return cellText(b.lines[i], b.cutFrom(i, method, a), method, a, end)
}

// penOf returns the style line i starts in.
func (b *Store) penOf(i int) pen {
	if i >= len(b.pens) || b.pens[i] == 0 {
		return pen{}
	}
	return b.penTab[b.pens[i]-1]
}

// Measure is how wide each line of a buffer is, measured with a width
// method, for the lines up to n, as they were since the buffer's last reset.
type Measure struct {
	method ansi.Method
	reset  uint64
	// widths is how many cells each line takes, with what measuring it
	// told of it in the bits above, as [measureLine] returns it.
	widths []int32
	Widest int
}

// Measure returns how wide the buffer's lines are with method, measuring
// only the lines added since it last did.
func (b *Store) Measure(method ansi.Method) *Measure {
	m := b.measures[0]
	if m == nil || m.method != method {
		if o := b.measures[1]; o != nil && o.method == method {
			b.measures[0], b.measures[1] = o, m
			m = o
		} else {
			m = &Measure{method: method}
			b.measures[0], b.measures[1] = m, b.measures[0]
		}
	}
	if m.reset != b.reset {
		m.reset, m.widths, m.Widest = b.reset, m.widths[:0], 0
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
		m.Widest = max(m.Widest, int(w&widthMask))
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

// Width returns how many cells line i takes.
func (m *Measure) Width(i int) int { return int(m.widths[i] & widthMask) }

// Index is how many rows each line of a buffer takes, wrapped to width
// with a mode, for the lines up to n, as they were since its last reset.
type Index struct {
	method ansi.Method
	mode   Mode
	width  int
	reset  uint64
	n      int
	// cum is how many rows the lines before each line take, cum[n] all of
	// them, or nil while every line is one row.
	cum []int
}

// RowsBefore returns how many rows the lines before line i take.
func (x *Index) RowsBefore(i int) int {
	if x.cum == nil {
		return i
	}
	return x.cum[i]
}

// RowsOf returns how many rows line i takes.
func (x *Index) RowsOf(i int) int {
	if x.cum == nil {
		return 1
	}
	return x.cum[i+1] - x.cum[i]
}

// Total returns how many rows all the lines take.
func (x *Index) Total() int { return x.RowsBefore(x.n) }

// Uniform reports whether every line takes one row.
func (x *Index) Uniform() bool { return x.cum == nil }

// LineAt returns the line that row is in, and how far into the line's rows
// it is, for a row of the lines.
func (x *Index) LineAt(row int) (line, sub int) {
	if x.cum == nil {
		return row, 0
	}
	// The last line whose rows start at or before row.
	line, _ = slices.BinarySearch(x.cum[:x.n+1], row+1)
	line--
	return line, row - x.cum[line]
}

// Index returns how many rows the buffer's lines take, wrapped to width
// with mode and measured with method, working out only the lines added since
// it last did.
func (b *Store) Index(method ansi.Method, mode Mode, width int) *Index {
	m := b.Measure(method)
	if mode == None || width <= 0 {
		// One row a line, which needs nothing kept.
		return &Index{method: method, mode: None, width: width, reset: b.reset, n: len(b.lines)}
	}
	match := func(x *Index) bool {
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
			*o = Index{cum: o.cum}
			x = o
		} else {
			x = &Index{}
		}
		x.method, x.mode, x.width, x.reset = method, mode, width, ^b.reset
		b.indexes[0], b.indexes[1] = x, b.indexes[0]
	}
	if x.reset != b.reset {
		x.reset, x.n = b.reset, 0
		if m.Widest <= width {
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
			widest = max(widest, m.Width(i))
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
func (b *Store) rowScratch(n int) []int32 {
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
func countRows(lines []string, widths []int32, method ansi.Method, width int, mode Mode, rows []int32) {
	inParallel(len(lines), func(lo, hi int) {
		countRowsOf(lines[lo:hi], widths[lo:hi], method, width, mode, rows[lo:hi])
	})
}

func countRowsOf(lines []string, widths []int32, method ansi.Method, width int, mode Mode, rows []int32) {
	var starts []int
	for i, s := range lines {
		n := 1
		if w := widths[i]; int(w&widthMask) > width {
			if mode == Chars && w&wideLine == 0 {
				n = (int(w&widthMask) + width - 1) / width
			} else {
				n, starts = wrapLine(s, method, width, mode, starts[:0])
			}
		}
		rows[i] = int32(n) //nolint:gosec // A line has far fewer rows than 1<<31.
	}
}
