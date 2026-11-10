// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image/color"
	"reflect"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// tabStop is how many columns apart the stops a tab in a [Buffer] moves to
// are, as Lip Gloss draws a tab four spaces wide.
const tabStop = 4

// glyphKind is what [scanner.next] found.
type glyphKind uint8

const (
	glyphEnd  glyphKind = iota
	glyphText           // a glyph, or a space a tab is drawn as
	glyphSeq            // a style or hyperlink sequence
)

// scanner walks a line of a buffer as a viewport draws it: glyph by glyph,
// measured as the canvas measures them when it parses a string, with tabs
// drawn as spaces to the next stop, and control characters and escape
// sequences other than styles and hyperlinks dropped. With a parser, it
// follows the style text is drawn in, from the style the line starts in.
type scanner struct {
	s      string
	i      int
	state  byte
	method ansi.Method
	p      *ansi.Parser
	pen    pen
	adv    int // cells the last glyph took, to move past before the next
	tabs   int // spaces left of a tab

	// What next found: a glyph's text and width, starting at cell x and
	// byte at of s, or a sequence's text.
	g        string
	w, x, at int
	tab      bool // g is a space of a tab
	tabFirst bool // and the first of them
}

func newScanner(s string, method ansi.Method) scanner {
	return scanner{s: s, method: method}
}

// next finds the next glyph or sequence of the line.
func (sc *scanner) next() glyphKind {
	sc.x += sc.adv
	sc.adv = 0
	if sc.tabs > 0 {
		sc.tabs--
		sc.g, sc.w, sc.adv, sc.tab, sc.tabFirst = " ", 1, 1, true, false
		return glyphText
	}
	sc.tab, sc.tabFirst = false, false
	for sc.i < len(sc.s) {
		at := sc.i
		if c := sc.s[at]; sc.state == 0 && c >= 0x20 && c < 0x7f && (at+1 == len(sc.s) || sc.s[at+1] < 0x80) {
			sc.i++
			sc.g, sc.w, sc.adv, sc.at = sc.s[at:at+1], 1, 1, at
			return glyphText
		}
		seq, width, n, state := sc.method.DecodeSequenceInString(sc.s[at:], sc.state, sc.p)
		// The decoder's ASCII fast path does not look for combining marks
		// after the character, as the canvas does when it parses.
		if n == 1 && seq[0] > 0x1f && seq[0] < 0x7f && len(sc.s) > at+1 && sc.s[at+1] >= 0xc0 {
			if cluster, cw := ansi.FirstGraphemeCluster(sc.s[at:], sc.method); len(cluster) > 1 {
				seq, width, n = cluster, cw, len(cluster)
			}
		}
		if n <= 0 {
			n = 1
		}
		sc.i, sc.state = at+n, state
		switch {
		case width > 0:
			sc.g, sc.w, sc.adv, sc.at = seq, width, width, at
			return glyphText
		case seq == "\t":
			sc.tabs = tabStop - sc.x%tabStop - 1
			sc.g, sc.w, sc.adv, sc.at, sc.tab, sc.tabFirst = " ", 1, 1, at, true, true
			return glyphText
		case sc.p != nil && ansi.HasCsiPrefix(seq) && sc.p.Command() == 'm':
			uv.ReadStyle(sc.p.Params(), &sc.pen.style)
			sc.g, sc.at = seq, at
			return glyphSeq
		case sc.p != nil && ansi.HasOscPrefix(seq) && sc.p.Command() == 8:
			uv.ReadLink(sc.p.Data(), &sc.pen.link)
			sc.g, sc.at = seq, at
			return glyphSeq
		case seq != "" && seq[0] >= 0xc0:
			// A combining mark parted from its glyph by a sequence, which
			// takes no cells of its own.
			sc.g, sc.w, sc.at = seq, 0, at
			return glyphText
		}
	}
	return glyphEnd
}

// simpleLine reports whether s is only printable ASCII, so that each byte
// is a cell.
func simpleLine(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

// lineWidth32 returns how many cells line s takes, and whether a glyph of
// it is wider than one cell.
func lineWidth32(s string, method ansi.Method) (w int, wide bool) {
	if n, ok := asciiWidth(s); ok {
		return n, false
	}
	return scanWidth(s, method)
}

// scanWidth is [lineWidth32] for any line, a glyph at a time.
func scanWidth(s string, method ansi.Method) (w int, wide bool) {
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		wide = wide || sc.w > 1
	}
	return sc.x + sc.adv, wide
}

// What measuring a line tells of it, kept in the bits of its width above
// the cells it takes.
const (
	// wideLine marks a line with a glyph wider than one cell, which cannot
	// be wrapped by counting its cells alone.
	wideLine = 1 << 30
	// plainLine marks a line of only printable ASCII, a cell a byte.
	plainLine = 1 << 29
	// asciiLine marks a line of printable ASCII and whole control
	// sequences and commands, without tabs, which [asciiCut] cuts.
	asciiLine = 1 << 28
	// widthMask is the cells a line takes.
	widthMask = asciiLine - 1
)

// measureLine returns how many cells line s takes, measured with method,
// with what measuring it told of it in the bits above.
func measureLine(s string, method ansi.Method) int32 {
	w, wide := lineWidth32(s, method)
	flags := 0
	switch {
	case wide:
		flags = wideLine
	case simpleLine(s):
		flags = plainLine | asciiLine
	case strings.IndexByte(s, '\t') < 0:
		if _, ok := asciiWidth(s); ok {
			flags = asciiLine
		}
	}
	return int32(min(w, widthMask) | flags) //nolint:gosec // Bounded by the mask.
}

// longLineBytes is how long a line must be for a viewport to keep
// checkpoints of it, and the rows it wraps to, rather than scan it from
// its start for each row it shows of it, every frame.
const longLineBytes = 8 << 10

// checkpointEvery is how many cells apart the checkpoints of a long line
// are, so that cutting a row out of a line megabytes long scans little more
// than the row.
const checkpointEvery = 1024

// checkpoint is where a glyph of a line starts, and the style it is drawn
// in there, to start scanning the line from.
type checkpoint struct {
	x, i int // its cell, and its byte
	pen  pen
}

// scanner returns a scanner of line s from cp, following its style.
func (cp checkpoint) scanner(s string, method ansi.Method, p *ansi.Parser) scanner {
	return scanner{s: s, i: cp.i, x: cp.x, method: method, p: p, pen: cp.pen}
}

// skipSeq returns the index just past the escape sequence at s[i], an ESC,
// for a control sequence (CSI) or an operating system command (OSC) that is
// whole, or -1 for any other, which only [scanner] knows how to take.
func skipSeq(s string, i int) int {
	if i+1 >= len(s) {
		return -1
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			return j + 1
		}
	case ']':
		for j := i + 2; j < len(s); j++ {
			switch {
			case s[j] == 0x07:
				return j + 1
			case s[j] == ansi.ESC:
				// Only as the start of the terminator: an escape inside a
				// command starts a sequence of its own, which the cells may
				// pass through to the terminal, so it must be dropped.
				if j+1 < len(s) && s[j+1] == '\\' {
					return j + 2
				}
				return -1
			case s[j] < 0x20:
				return -1
			}
		}
	}
	return -1
}

// asciiWidth returns how many cells line s takes, and true, when it is only
// printable ASCII, tabs, and whole control sequences and commands, which
// take no cells, as most logs are; otherwise it returns false.
func asciiWidth(s string) (w int, ok bool) {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c >= 0x20 && c < 0x7f:
			w++
			i++
		case c == '\t':
			w += tabStop - w%tabStop
			i++
		case c == ansi.ESC:
			if i = skipSeq(s, i); i < 0 {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	return w, true
}

// penAfter returns the style text is drawn in at the end of line s, which
// starts in start. Only its escape sequences are read.
func penAfter(s string, start pen) pen {
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	at := start
	for i := strings.IndexByte(s, ansi.ESC); i >= 0 && i < len(s); {
		seq, _, n, _ := ansi.GraphemeWidth.DecodeSequenceInString(s[i:], 0, p)
		switch {
		case ansi.HasCsiPrefix(seq) && p.Command() == 'm':
			uv.ReadStyle(p.Params(), &at.style)
		case ansi.HasOscPrefix(seq) && p.Command() == 8:
			uv.ReadLink(p.Data(), &at.link)
		}
		i += max(n, 1)
		next := strings.IndexByte(s[i:], ansi.ESC)
		if next < 0 {
			break
		}
		i += next
	}
	return at
}

// plainText returns the text of line s without its styles, with its tabs.
func plainText(s string, method ansi.Method) string {
	if simpleLine(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		switch {
		case sc.tabFirst:
			b.WriteByte('\t')
		case !sc.tab:
			b.WriteString(sc.g)
		}
	}
	return b.String()
}

// WrapMode is how a [Viewport] fits lines wider than it is.
type WrapMode uint8

const (
	// WrapNone shows lines whole, cut off at the viewport's right edge,
	// and scrolled sideways with [ViewportPos.X]. It is the default.
	WrapNone WrapMode = iota
	// WrapWords wraps lines at the spaces between words, and after
	// hyphens, as Lip Gloss wraps a [Text], breaking a word only when it is
	// wider than the viewport.
	WrapWords
	// WrapChars wraps lines at the viewport's width, wherever it falls.
	WrapChars
)

// wrapLine appends to starts the cells line s's rows start at, wrapped to w
// cells with mode, the first at 0, and returns how many rows it takes.
//
// A row with words mode ends at the start of the last word that fits, or
// just after the last hyphen that does, and the spaces before that word are
// its own, hanging past its width, unseen.
// A glyph wider than w takes a row of its own.
func wrapLine(s string, method ansi.Method, w int, mode WrapMode, starts []int) (rows int, out []int) {
	starts = append(starts, 0)
	if w <= 0 {
		return 1, starts
	}
	if fast, ok := asciiWrap(s, w, mode, starts); ok {
		return len(fast), fast
	}
	starts = scanWrap(s, method, w, mode, starts)
	return len(starts), starts
}

// scanWrap is [wrapLine] for any line, a glyph at a time.
func scanWrap(s string, method ansi.Method, w int, mode WrapMode, starts []int) []int {
	words := mode == WrapWords
	// open is set where a row may start at the next glyph: after spaces, or
	// a hyphen.
	start, brk, open := 0, -1, false
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		x, gw := sc.x, sc.w
		if gw == 0 {
			continue
		}
		if words {
			if sc.g == " " {
				open = true
				continue
			}
			if open {
				brk = x
			}
			open = sc.g == "-"
		}
		if x+gw-start <= w || x == start {
			continue
		}
		if words && brk > start {
			start = brk
			starts = append(starts, start)
		}
		if x+gw-start > w && x > start {
			start = x
			starts = append(starts, start)
		}
	}
	return starts
}

// asciiWrap is [wrapLine] for a line that [asciiWidth] can measure, a cell
// a byte, which it wraps a word at a time rather than a glyph at a time. It
// returns false for any other line, with starts as it was given.
func asciiWrap(s string, w int, mode WrapMode, starts []int) ([]int, bool) {
	given := len(starts)
	if mode == WrapChars {
		// Every cell is as wide as the next: a row every w cells.
		width, ok := asciiWidth(s)
		if !ok {
			return starts, false
		}
		for at := w; at < width; at += w {
			starts = append(starts, at)
		}
		return starts, true
	}
	start := 0
	// fit breaks the rows of the run of cells from ws up to we, which a
	// space came before when spaced.
	fit := func(ws, we int, spaced bool) {
		if we-start <= w {
			return
		}
		if spaced && ws > start {
			start = ws
			starts = append(starts, start)
		}
		for we-start > w {
			start += w
			starts = append(starts, start)
		}
	}
	// ws is where the run of a word's cells being read starts, spaced is
	// set once a row may start at a run, and hyphen when the last run
	// ended with one, after which a row may start.
	x, ws, spaced, hyphen := 0, -1, false, false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c > ' ' && c < 0x7f:
			if hyphen && ws >= 0 {
				fit(ws, x, spaced)
				ws, spaced = -1, true
			}
			// A run of a word's characters, a cell each, up to a hyphen.
			j := i + 1
			for j < len(s) && s[j] > ' ' && s[j] < 0x7f && s[j-1] != '-' {
				j++
			}
			if ws < 0 {
				ws = x
			}
			x += j - i
			i = j
			hyphen = s[j-1] == '-'
			continue
		case c == ' ' || c == '\t':
			if ws >= 0 {
				fit(ws, x, spaced)
				ws = -1
			}
			hyphen = false
			if c == '\t' {
				x += tabStop - x%tabStop
			} else {
				x++
			}
			spaced = true
			i++
			continue
		case c == ansi.ESC:
			if i = skipSeq(s, i); i >= 0 {
				continue
			}
		}
		return starts[:given], false
	}
	if ws >= 0 {
		fit(ws, x, spaced)
	}
	return starts, true
}

// overlay is a style drawn over part of a row, such as a mark or the style
// of its line, as the escape sequence that sets it.
type overlay struct {
	start, end int // cells of the line, end exclusive
	sgr        string
}

// overlaySGR returns the escape sequence that sets the colors and
// attributes of style, or "" for a style that sets none.
func overlaySGR(style lipgloss.Style) string {
	return (*penSGRs)(nil).overlay(style)
}

// markCells returns marks, given in bytes of the text of line s as
// [plainText] returns it, as the cells of the line they cover, in order,
// dropping those that cover none or set no style, and those overlapping one
// before them. Their styles' escape sequences are kept in sgrs.
func markCells(s string, method ansi.Method, marks []Mark, sgrs *penSGRs, dst []overlay) []overlay {
	// The marks drawn, in bytes, which do not overlap, so that each glyph
	// is in one at most, found by walking both in order.
	at := len(dst)
	sorted := slices.SortedStableFunc(slices.Values(marks), func(a, b Mark) int { return a.Start - b.Start })
	for i := range sorted {
		m := &sorted[i]
		if m.End <= m.Start || (len(dst) > at && m.Start < dst[len(dst)-1].end) {
			continue
		}
		if sgr := sgrs.overlay(m.Style); sgr != "" {
			dst = append(dst, overlay{start: m.Start, end: m.End, sgr: sgr})
		}
	}
	bytes := slices.Clone(dst[at:])
	if len(bytes) == 0 {
		return dst
	}
	marked := dst[at:]
	for k := range marked {
		marked[k].start, marked[k].end = -1, -1
	}

	// Each glyph is at the byte of the text its first cell comes from.
	sc, plain, k := newScanner(s, method), 0, 0
	for sc.next() != glyphEnd {
		b := plain
		switch {
		case sc.tabFirst:
			plain++
		case sc.tab:
			b = plain - 1
		default:
			plain += len(sc.g)
		}
		for k < len(bytes) && b >= bytes[k].end {
			k++
		}
		if k == len(bytes) {
			break
		}
		if b >= bytes[k].start {
			o := &marked[k]
			if o.start < 0 {
				o.start = sc.x
			}
			o.end = sc.x + sc.w
		}
	}
	out := dst[:at]
	for _, o := range marked {
		if o.start >= 0 && o.end > o.start {
			out = append(out, o)
		}
	}
	return out
}

// rowStyle is what is drawn over a row of a line: line under the text's own
// styles, across all of the row, which it pads with spaces to fill cells,
// and marks over the cells they cover. pens keeps the escape sequences of
// the text's styles, which are drawn again over line after each of its own.
type rowStyle struct {
	line  string
	marks []overlay
	fill  int
	pens  *penSGRs
}

// penSGRs keeps the escape sequences of the styles that text was drawn in,
// as making one allocates, and the same few styles come up row after row,
// such as the colors of a log's levels.
type penSGRs struct {
	styles []uv.Style
	sgrs   []string
}

// overlay returns the escape sequence of the colors and attributes of style,
// or "" for a style that sets none, as [overlaySGR] does.
func (c *penSGRs) overlay(style lipgloss.Style) string {
	s := cellStyle(style)
	if styleIsZero(&s) {
		return ""
	}
	return c.of(&s)
}

// of returns the escape sequence of style s.
func (c *penSGRs) of(s *uv.Style) string {
	if c == nil {
		return s.String()
	}
	for i := range c.styles {
		if sameSGR(&c.styles[i], s) {
			return c.sgrs[i]
		}
	}
	if len(c.styles) >= 64 {
		c.styles, c.sgrs = c.styles[:0], c.sgrs[:0]
	}
	sgr := s.String()
	c.styles, c.sgrs = append(c.styles, *s), append(c.sgrs, sgr)
	return sgr
}

// sameSGR reports whether styles a and b are set with the same escape
// sequence: unlike [uv.Style.Equal], a color of the terminal's palette and
// the RGB it stands for differ, as the terminal may show them differently.
func sameSGR(a, b *uv.Style) bool {
	return a.Attrs == b.Attrs && a.Underline == b.Underline &&
		sameColorKind(a.Fg, b.Fg) && sameColorKind(a.Bg, b.Bg) && sameColorKind(a.UnderlineColor, b.UnderlineColor)
}

// sameColorKind reports whether a and b are the same color of the same type.
// The types Lip Gloss and escape sequences make colors of are compared
// directly; others are compared through reflection, which allocates, and
// those that cannot be compared, such as one holding a slice, are never the
// same.
func sameColorKind(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch x := a.(type) {
	case ansi.BasicColor:
		return sameAs(x, b)
	case ansi.IndexedColor:
		return sameAs(x, b)
	case ansi.RGBColor:
		return sameAs(x, b)
	case color.RGBA:
		return sameAs(x, b)
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	return va.Type() == vb.Type() && va.Comparable() && vb.Comparable() && a == b
}

// sameAs reports whether b is x, of the same type.
func sameAs[T comparable](x T, b color.Color) bool {
	y, ok := b.(T)
	return ok && x == y
}

// cutRow returns the cells from a up to b of line s as a string that draws
// them on its own: in the style the line is in at a, with st drawn over
// them, if not nil, and with its styles reset at the end. The line is
// scanned from from, its start or a checkpoint before a, and flags are what
// measuring it told of it. A glyph cut by a is drawn as spaces, and one cut
// by b is left out.
func cutRow(s string, from checkpoint, flags int32, method ansi.Method, a, b int, st *rowStyle) string {
	if st == nil {
		st = &rowStyle{}
	}
	if st.line == "" && len(st.marks) == 0 {
		if flags&plainLine != 0 && from.pen.zero() {
			return s[min(a, len(s)):min(max(b, a), len(s))]
		}
		if flags&asciiLine != 0 {
			return asciiCut(s, from, a, b)
		}
	}
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	w := rowWriter{sc: from.scanner(s, method, p), line: st.line, marks: st.marks, pens: st.pens, in: -1}
	w.out.Grow(min(len(s)-from.i, b-a) + 32)

scan:
	for k := w.sc.next(); k != glyphEnd; k = w.sc.next() {
		if k == glyphSeq {
			w.sequence()
			continue
		}
		x, gw := w.sc.x, w.sc.w
		switch {
		case gw == 0:
			// A combining mark goes with the glyph before it.
			if w.started && x <= b {
				w.out.WriteString(w.sc.g)
			}
			continue
		case x+gw <= a:
			continue
		case x+gw > b:
			break scan
		}
		w.glyph(x, a)
	}
	return w.finish(st.fill)
}

// asciiCut is [cutRow] with nothing drawn over the row, for a line of
// printable ASCII and whole control sequences and commands, without tabs,
// as asciiLine marks it, whose cells it cuts from the line as they are, with
// the style the line is in where they start.
func asciiCut(s string, cp checkpoint, a, b int) string {
	from, to, x, seqs := -1, len(s), cp.x, false
	for i := cp.i; i < len(s); {
		if s[i] == ansi.ESC {
			seqs = true
			if i = skipSeq(s, i); i < 0 {
				break
			}
			continue
		}
		if x == a && from < 0 {
			from = i
		}
		if x == b {
			to = i
			break
		}
		x++
		i++
	}
	if from < 0 {
		return ""
	}
	at := cp.pen
	if seqs {
		at = penAfter(s[cp.i:from], cp.pen)
	}
	if at.zero() && !seqs {
		return s[from:to]
	}
	var out strings.Builder
	out.Grow(to - from + 48)
	if !styleIsZero(&at.style) {
		out.WriteString(at.style.String())
	}
	if at.link.URL != "" {
		out.WriteString(ansi.SetHyperlink(at.link.URL, at.link.Params))
	}
	out.WriteString(s[from:to])
	out.WriteString(ansi.ResetStyle)
	if at.link.URL != "" || strings.Contains(s[from:to], "\x1b]8;") {
		out.WriteString(ansi.ResetHyperlink())
	}
	return out.String()
}

// rowWriter writes a row of a line as [cutRow] draws it.
type rowWriter struct {
	out     strings.Builder
	sc      scanner
	line    string    // the style of the line, drawn over all of it
	marks   []overlay // in order
	pens    *penSGRs
	mark    int  // the next mark to enter
	in      int  // the mark being drawn, or -1
	started bool // the row's first glyph was written
	styled  bool // a style was written, to reset at the end
	drawn   int  // cells written
}

// setPen writes the style drawn over the line, and the style the text is in
// over it.
func (w *rowWriter) setPen() {
	if w.line != "" {
		w.out.WriteString(w.line)
		w.styled = true
	}
	if !styleIsZero(&w.sc.pen.style) {
		w.out.WriteString(w.pens.of(&w.sc.pen.style))
		w.styled = true
	}
}

// begin starts the row in the style and hyperlink the line is in.
func (w *rowWriter) begin() {
	w.started = true
	w.setPen()
	if l := w.sc.pen.link; l.URL != "" {
		w.out.WriteString(ansi.SetHyperlink(l.URL, l.Params))
	}
}

// leave ends the mark being drawn.
func (w *rowWriter) leave() {
	w.out.WriteString(ansi.ResetStyle)
	w.setPen()
	w.in = -1
}

// moveTo leaves and enters marks for a glyph at cell x.
func (w *rowWriter) moveTo(x int) {
	if w.in >= 0 && w.marks[w.in].end <= x {
		w.leave()
	}
	for w.in < 0 && w.mark < len(w.marks) && w.marks[w.mark].start <= x {
		if w.marks[w.mark].end > x {
			w.out.WriteString(w.marks[w.mark].sgr)
			w.styled, w.in = true, w.mark
		}
		w.mark++
	}
}

// sequence writes the style or hyperlink sequence the scanner found, once
// the row has started. Under a line's style or a mark, a style the text
// sets is drawn again as the scanner followed it, between the two, as a
// reset in the text would otherwise drop them.
func (w *rowWriter) sequence() {
	if !w.started {
		return
	}
	w.styled = true
	if (w.line == "" && w.in < 0) || !ansi.HasCsiPrefix(w.sc.g) {
		w.out.WriteString(w.sc.g)
		return
	}
	w.out.WriteString(ansi.ResetStyle)
	w.setPen()
	if w.in >= 0 {
		w.out.WriteString(w.marks[w.in].sgr)
	}
}

// glyph writes the glyph the scanner found at cell x, in a row from cell a,
// as spaces when it starts before a.
func (w *rowWriter) glyph(x, a int) {
	if !w.started {
		w.begin()
	}
	w.moveTo(x)
	if x < a {
		cut := x + w.sc.w - a
		w.out.WriteString(strings.Repeat(" ", cut))
		w.drawn += cut
		return
	}
	w.out.WriteString(w.sc.g)
	w.drawn += w.sc.w
}

// finish pads the row to fill cells in the line's style, if it has one,
// resets its styles, and returns it.
func (w *rowWriter) finish(fill int) string {
	if w.line != "" && fill > w.drawn {
		if !w.started {
			w.begin()
		}
		if w.in >= 0 {
			w.leave()
		}
		w.out.WriteString(strings.Repeat(" ", fill-w.drawn))
	}
	if w.styled {
		w.out.WriteString(ansi.ResetStyle)
	}
	if w.started && w.sc.pen.link.URL != "" {
		w.out.WriteString(ansi.ResetHyperlink())
	}
	return w.out.String()
}

// cellText returns the text of the glyphs of line s that start in the cells
// from a to b, inclusive, without styles, with a tab once for the spaces of
// a tab, scanning it from from, its start or a checkpoint before a.
func cellText(s string, from checkpoint, method ansi.Method, a, b int) string {
	var out strings.Builder
	sc := from.scanner(s, method, nil)
	taken := false
	for sc.next() != glyphEnd {
		x := sc.x
		if x > b {
			break
		}
		if x < a {
			continue
		}
		switch {
		case sc.w == 0:
			if taken {
				out.WriteString(sc.g)
			}
		case sc.tab:
			if sc.tabFirst || !taken {
				out.WriteByte('\t')
			}
		default:
			out.WriteString(sc.g)
		}
		taken = true
	}
	return out.String()
}
