// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// wheelStep is how many rows, or columns, a step of the mouse wheel
// scrolls a viewport given [ViewportNode.OnScroll].
const wheelStep = 3

// ViewportNode shows the lines of a [Buffer], as many as fit, from a
// position kept in the model. Build one with [Viewport].
type ViewportNode struct {
	meta

	buf       *Buffer
	pos       ViewportPos
	wrap      WrapMode
	follow    bool
	gutters   []gutter
	marks     func(line int, text string) []Mark
	lineStyle func(line int) lipgloss.Style
	style     string // the escape sequence of Style's colors and attributes
	onScroll  func(pos ViewportPos) tea.Cmd
	trusted   bool
}

// Viewport returns a viewport showing the lines of buf, from the start
// until it is given a position with [ViewportNode.At]. It lays out and draws
// only the lines in sight, however many the buffer holds, so a log of a
// million lines costs no more to draw than one of a screenful, and costs
// little more to append to.
//
// It is as wide as its widest line and as tall as its lines, wrapped, and
// is usually placed where it fills the space it is given, such as in a
// [Fill] slot or a [Box]. [Frame.Viewport] tells where a frame showed it
// scrolled to, and moves it from there, [ScrollbarOf] its ID makes a
// scrollbar for it, and [ViewportNode.OnScroll] scrolls it with the mouse.
func Viewport(buf *Buffer) ViewportNode {
	return ViewportNode{buf: buf}
}

// ViewportPos is where a [Viewport] is scrolled to: the line at its top,
// and with wrapping, how many of that line's rows are above the top, so
// that the same text stays at the top as its width changes. Keep it in the
// model, and set it from what [Frame.Viewport] and [ViewportNode.OnScroll]
// give, which have it clamped to the lines there are.
type ViewportPos struct {
	// Line is the line at the top. One past the last line or more, as with
	// [ViewportEnd], shows the last of them.
	Line int
	// Row is how many rows of Line are above the top, while it wraps.
	Row int
	// X is how many columns the lines are scrolled sideways, while they do
	// not wrap.
	X int
}

// ViewportEnd is the position of a [Viewport] scrolled to its end, showing
// its last line at its bottom, which stays at the end as lines are
// appended. See [ViewportNode.Follow].
var ViewportEnd = ViewportPos{Line: math.MaxInt}

// AtEnd reports whether p is the end of a viewport, as [ViewportEnd] is,
// rather than a line of it.
func (p ViewportPos) AtEnd() bool { return p.Line == math.MaxInt }

// At sets where the viewport is scrolled to. A position past the end shows
// the last of the lines, and the viewport never leaves space below its last
// line while there are lines above its top to fill it.
func (n ViewportNode) At(pos ViewportPos) ViewportNode { n.pos = pos; return n }

// Wrap sets how lines wider than the viewport are fitted to it. The default
// is [WrapNone].
func (n ViewportNode) Wrap(mode WrapMode) ViewportNode { n.wrap = mode; return n }

// Follow keeps the viewport at its end while it is there, as a terminal or
// tail -f does: a position scrolled to its end, by the wheel or with the
// methods of [ViewportView], is given as [ViewportEnd], which stays at the
// end as lines are appended. Scrolled away from the end, it stays where it
// is. Start it at ViewportEnd to follow from the start.
func (n ViewportNode) Follow() ViewportNode { n.follow = true; return n }

// GutterRow is a row of a [Viewport] that a gutter is drawn beside.
type GutterRow struct {
	// Line is the line the row shows.
	Line int
	// Continued is set for a row after the first of a line that wraps.
	Continued bool
	// Lines is how many lines the buffer has, such as to size line numbers
	// by.
	Lines int
}

// gutter is a column drawn beside a viewport's lines.
type gutter struct {
	width   int
	fn      func(GutterRow) string
	numbers string // the style of line numbers, as escape sequences, when lines is set
	lines   bool
}

// Gutter draws a column width cells wide on the left of the lines, beside
// them, with what fn returns for each row in sight, such as a marker for a
// changed line or a breakpoint. Text wider than the gutter is cut off.
// Gutters are drawn left to right in the order given, and take their space
// from the lines.
func (n ViewportNode) Gutter(width int, fn func(row GutterRow) string) ViewportNode {
	if width <= 0 || fn == nil {
		return n
	}
	n.gutters = append(slices.Clip(n.gutters), gutter{width: width, fn: fn})
	return n
}

// LineNumbers draws the number of each line, from 1, in a gutter on the
// left as wide as the largest number and a space, drawn in style's colors
// and attributes. Rows that continue a wrapped line have none.
func (n ViewportNode) LineNumbers(style lipgloss.Style) ViewportNode {
	n.gutters = append(slices.Clip(n.gutters), gutter{numbers: overlaySGR(style), lines: true})
	return n
}

// Mark is part of a line drawn in a style of its own, such as a match of a
// search, as a function given to [ViewportNode.Marks] returns it.
type Mark struct {
	// Start and End are the bytes of the line's text that are marked, End
	// exclusive, as [regexp.Regexp.FindAllStringIndex] gives them for the
	// text the function is given.
	Start, End int
	// Style's colors and attributes are drawn over the text marked.
	Style lipgloss.Style
}

// Marks draws parts of lines in styles of their own, such as the matches of
// a search, as fn returns them. It is given each line in sight as it is
// drawn, with the line's text without styles, as [Buffer.Text] gives it,
// so that it costs nothing for lines out of sight. A mark that overlaps
// one before it is not drawn.
func (n ViewportNode) Marks(fn func(line int, text string) []Mark) ViewportNode {
	n.marks = fn
	return n
}

// MarkMatches returns a function for [ViewportNode.Marks] that marks every
// match of re in style. A nil re marks nothing.
func MarkMatches(re *regexp.Regexp, style lipgloss.Style) func(line int, text string) []Mark {
	if re == nil {
		return nil
	}
	return func(_ int, text string) []Mark {
		locs := re.FindAllStringIndex(text, -1)
		if len(locs) == 0 {
			return nil
		}
		marks := make([]Mark, len(locs))
		for i, l := range locs {
			marks[i] = Mark{Start: l[0], End: l[1], Style: style}
		}
		return marks
	}
}

// Style draws the viewport's lines, and its rows below them, in the colors
// and attributes of style, under their own styles, those of
// [ViewportNode.LineStyle], and their marks. Its padding, borders, and
// margins are not drawn: put the viewport in a [Box] for those.
func (n ViewportNode) Style(style lipgloss.Style) ViewportNode {
	n.style = overlaySGR(style)
	return n
}

// LineStyle draws each line in sight in the colors and attributes of the
// style fn returns for it, under the line's own and its marks, across the
// whole width of the viewport, such as to show the line under a cursor. It
// is called only for lines in sight.
func (n ViewportNode) LineStyle(fn func(line int) lipgloss.Style) ViewportNode {
	n.lineStyle = fn
	return n
}

// Trusted keeps the escape sequences in the buffer's lines, and in the
// text of its gutters, that are written to the terminal as they are rather
// than drawn, as [TextNode.Trusted] does for a [Text]: APC, DCS, SOS and PM
// commands, such as images. Each is drawn with the row holding the glyph
// after it, or the last glyph of its line. Mark the viewport trusted only
// when the program made all of what it shows.
func (n ViewportNode) Trusted() ViewportNode { n.trusted = true; return n }

// OnScroll scrolls the viewport with the mouse wheel, three rows a step, or
// three columns sideways, running fn with the position to scroll to, to
// keep in the model for [ViewportNode.At]. A [ScrollbarOf] the viewport
// without an OnScroll of its own drags it with fn too, and selecting text
// in it scrolls it while the pointer is held past its edge.
func (n ViewportNode) OnScroll(fn func(pos ViewportPos) tea.Cmd) ViewportNode {
	n.onScroll = fn
	return n
}

// ID sets the ID that [Frame.Rect], [Frame.Viewport], and [ScrollbarOf]
// refer to the viewport by.
func (n ViewportNode) ID[K comparable](id K) ViewportNode { n.id = id; return n }

// On adds mouse handlers to the viewport, made by package [on].
func (n ViewportNode) On(handlers ...on.Handler) ViewportNode {
	n.handlers = with(n.handlers, handlers)
	return n
}

// Selectable lets the left button select text in the viewport by dragging
// over it, as a terminal selects text, but kept inside the viewport.
// Holding alt as the drag starts selects a rectangle of cells instead. See
// [Root.Selection] for the text selected, which takes the lines scrolled out
// of sight as well, with the rows of a wrapped line joined again, and
// without the gutters.
func (n ViewportNode) Selectable() ViewportNode { n.selectable = true; return n }

// View implements [Child], drawing the node at its natural size.
func (n ViewportNode) View() string { return Render(0, 0, n) }

// gutterWidth returns how many cells gutters take, beside a buffer of lines
// lines.
func gutterWidth(gutters []gutter, lines int) int {
	w := 0
	for i := range gutters {
		w += gutters[i].size(lines)
	}
	return w
}

func (g *gutter) size(lines int) int {
	if g.lines {
		return len(strconv.Itoa(max(lines, 1))) + 1
	}
	return g.width
}

// text returns what gutter g shows beside row, w cells wide.
func (g *gutter) text(row GutterRow, w int) string {
	if !g.lines {
		return g.fn(row)
	}
	var b strings.Builder
	b.Grow(w + len(g.numbers) + len(ansi.ResetStyle))
	b.WriteString(g.numbers)
	if row.Continued {
		b.WriteString(strings.Repeat(" ", w))
	} else {
		num := strconv.Itoa(row.Line + 1)
		b.WriteString(strings.Repeat(" ", max(w-1-len(num), 0)))
		b.WriteString(num)
		b.WriteByte(' ')
	}
	if g.numbers != "" {
		b.WriteString(ansi.ResetStyle)
	}
	return b.String()
}

// ViewportView is where a [Viewport] was scrolled to in a frame, as
// [Frame.Viewport] reports it, and how big it was. Its methods return the
// positions to scroll to from there, to give to [ViewportNode.At]:
//
//	case "pgdown":
//		v, _ := m.ui.Frame().Viewport(paneLog)
//		m.pos = v.PageDown()
//
// They work out the rows of the buffer as it is when they are called, so a
// view kept from before lines were appended still scrolls through them.
type ViewportView struct {
	// Pos is where the viewport was, once clamped to its lines, or
	// [ViewportEnd] while it followed its end. Keep it as the position to
	// scroll from.
	Pos ViewportPos
	// First is the line at the top, which Pos does not tell while it is
	// ViewportEnd.
	First int
	// Row is how many rows were above the top: the offset of the
	// viewport, in rows.
	Row int
	// Rows is how many rows tall the viewport was.
	Rows int
	// TotalRows is how many rows all of the lines took, wrapped.
	TotalRows int
	// Lines is how many lines the buffer had.
	Lines int
	// Width is how many columns wide the lines were shown in, besides the
	// gutters.
	Width int
	// Widest is how many columns the widest line took.
	Widest int

	buf    *Buffer
	method ansi.Method
	wrap   WrapMode
	follow bool
}

// index returns the rows of the view's buffer at its width.
func (v ViewportView) index() *wrapIndex {
	return v.buf.index(v.method, v.wrap, v.Width)
}

// AtRow returns the position with row at the top, counting the rows of all
// of the lines, wrapped, from 0, as a [Scrollbar] gives its offset.
func (v ViewportView) AtRow(row int) ViewportPos {
	if v.buf == nil || v.buf.Len() == 0 {
		return ViewportPos{}
	}
	x := v.index()
	bottom := max(x.total()-v.Rows, 0)
	row = min(max(row, 0), bottom)
	if v.follow && row == bottom {
		return ViewportPos{Line: ViewportEnd.Line, X: v.Pos.X}
	}
	line, sub := x.lineAt(row)
	return ViewportPos{Line: line, Row: sub, X: v.Pos.X}
}

// Down returns the position n rows further down, or up for a negative n.
func (v ViewportView) Down(n int) ViewportPos { return v.AtRow(v.Row + n) }

// Up returns the position n rows further up.
func (v ViewportView) Up(n int) ViewportPos { return v.AtRow(v.Row - n) }

// PageDown returns the position a viewport's height further down.
func (v ViewportView) PageDown() ViewportPos { return v.Down(max(v.Rows, 1)) }

// PageUp returns the position a viewport's height further up.
func (v ViewportView) PageUp() ViewportPos { return v.Up(max(v.Rows, 1)) }

// HalfPageDown returns the position half a viewport's height further down.
func (v ViewportView) HalfPageDown() ViewportPos { return v.Down(max(v.Rows/2, 1)) }

// HalfPageUp returns the position half a viewport's height further up.
func (v ViewportView) HalfPageUp() ViewportPos { return v.Up(max(v.Rows/2, 1)) }

// Top returns the position at the first line.
func (v ViewportView) Top() ViewportPos { return v.AtRow(0) }

// Bottom returns the position showing the last line at the bottom.
func (v ViewportView) Bottom() ViewportPos { return v.AtRow(math.MaxInt) }

// Right returns the position scrolled n columns further right, or left for
// a negative n, while the lines do not wrap.
func (v ViewportView) Right(n int) ViewportPos {
	p := v.Pos
	if v.wrap != WrapNone && v.Widest > v.Width {
		return p
	}
	p.X = min(max(p.X+n, 0), max(v.Widest-v.Width, 0))
	return p
}

// Left returns the position scrolled n columns further left.
func (v ViewportView) Left(n int) ViewportPos { return v.Right(-n) }

// Reveal returns the position that scrolls as little as it can for line to
// be in sight, whole if it fits, or from its start if it does not.
func (v ViewportView) Reveal(line int) ViewportPos {
	if v.buf == nil || v.buf.Len() == 0 {
		return v.Pos
	}
	x := v.index()
	line = min(max(line, 0), v.buf.Len()-1)
	start, rows := x.rowsBefore(line), x.rowsOf(line)
	switch {
	case start < v.Row:
		return v.AtRow(start)
	case start+rows > v.Row+v.Rows:
		return v.AtRow(min(start+rows-v.Rows, start))
	}
	return v.Pos
}

// AtTop reports whether the first line was at the top.
func (v ViewportView) AtTop() bool { return v.Row <= 0 }

// AtBottom reports whether the last line was in sight at the bottom.
func (v ViewportView) AtBottom() bool { return v.Row >= v.TotalRows-v.Rows }

// Percent returns how far down the viewport was scrolled, from 0 at the top
// to 1 at the bottom, or 1 when all of it was in sight.
func (v ViewportView) Percent() float64 {
	if scrollable := v.TotalRows - v.Rows; scrollable > 0 {
		return min(max(float64(v.Row)/float64(scrollable), 0), 1)
	}
	return 1
}

// vpLayout is a viewport as laid out in a frame: where it was scrolled to,
// where its lines go, and the rows in sight, a span of the engine's vpRows.
type vpLayout struct {
	view    ViewportView
	content image.Rectangle
	rows    [2]int
}

// vpRow is a row of a viewport in sight: the cells from a up to b of a
// line, and which of the line's rows it is.
type vpRow struct {
	line, sub, a, b int
}

// measureViewport returns the natural size of viewport n within avail: as
// wide as its widest line, besides its gutters, and as tall as its lines,
// wrapped to that width, no more than avail.
func (e *engine) measureViewport(n ViewportNode, avail Size) Size {
	if n.buf == nil || n.buf.Len() == 0 {
		return Size{}
	}
	lines := n.buf.Len()
	g := gutterWidth(n.gutters, lines)
	m := n.buf.measure(e.method)
	w := m.widest
	if n.wrap != WrapNone {
		w = min(w, max(avail.W-g, 0))
	}
	if w <= 0 {
		// Empty lines, or no room to wrap in: a row a line.
		return Size{W: g, H: lines}
	}
	if n.wrap == WrapNone || m.widest <= w {
		return Size{W: w + g, H: lines}
	}
	if avail.H >= unbounded {
		return Size{W: w + g, H: n.buf.index(e.method, n.wrap, w).total()}
	}
	// Only whether the lines fill avail matters, so they are counted until
	// they do.
	h := 0
	var starts []int
	for i := 0; i < lines && h < avail.H; i++ {
		if m.width(i) <= w {
			h++
			continue
		}
		h += len(n.buf.rowStarts(i, e.method, w, n.wrap, &starts))
	}
	return Size{W: w + g, H: h}
}

// layoutViewport works out where viewport n, box i, laid out in r, is
// scrolled to and which rows are in sight.
func (e *engine) layoutViewport(i int, n ViewportNode, r image.Rectangle) {
	l := vpLayout{content: r}
	l.rows[0] = len(e.vpRows)
	l.view = ViewportView{buf: n.buf, method: e.method, wrap: n.wrap, follow: n.follow, Rows: r.Dy()}
	if n.buf != nil {
		lines := n.buf.Len()
		l.content.Min.X += min(gutterWidth(n.gutters, lines), r.Dx())
		e.viewportRows(&l, n, lines)
	}
	l.rows[1] = len(e.vpRows)
	e.vps = append(e.vps, l)
	e.boxes[i].vp = len(e.vps)
}

// viewportRows clamps the position of viewport n, laid out as l, and notes
// the rows in sight.
func (e *engine) viewportRows(l *vpLayout, n ViewportNode, lines int) {
	v := &l.view
	w, h := l.content.Dx(), l.content.Dy()
	m := n.buf.measure(e.method)
	x := n.buf.index(e.method, n.wrap, w)
	v.Lines, v.Width, v.Widest, v.TotalRows = lines, w, m.widest, x.total()
	if lines == 0 || w <= 0 {
		return
	}

	bottom := max(v.TotalRows-h, 0)
	p := n.pos
	row := 0
	switch {
	case p.Line >= lines:
		row = bottom
	case p.Line >= 0:
		row = x.rowsBefore(p.Line) + min(max(p.Row, 0), x.rowsOf(p.Line)-1)
	}
	row = min(row, bottom)
	line, sub := x.lineAt(row)
	col := 0
	if x.cum == nil {
		col = min(max(p.X, 0), max(m.widest-w, 0))
	}
	v.Row, v.First, v.Pos = row, line, ViewportPos{Line: line, Row: sub, X: col}
	if n.follow && row == bottom {
		v.Pos = ViewportPos{Line: ViewportEnd.Line, X: col}
	}

	var scratch []int
	for y := 0; y < h && line < lines; line++ {
		if x.cum == nil || x.rowsOf(line) == 1 {
			e.vpRows = append(e.vpRows, vpRow{line: line, a: col, b: col + w})
			y++
			sub = 0
			continue
		}
		starts := n.buf.rowStarts(line, e.method, w, n.wrap, &scratch)
		end := m.width(line)
		for k := sub; k < len(starts) && y < h; k++ {
			rowEnd := end
			if k+1 < len(starts) {
				rowEnd = starts[k+1]
			}
			e.vpRows = append(e.vpRows, vpRow{line: line, sub: k, a: starts[k], b: min(rowEnd, starts[k]+w)})
			y++
		}
		sub = 0
	}
}

// viewportOf returns how box i, a viewport, was laid out.
func (e *engine) viewportOf(i int) *vpLayout {
	if k := e.boxes[i].vp; k > 0 {
		return &e.vps[k-1]
	}
	return nil
}

// viewportScroll returns the wheel handler of viewport n, the box at path
// with key, which finds where it was scrolled to in the frame the event is
// in.
func (e *engine) viewportScroll(n ViewportNode, key nodeKey) on.Handler {
	screen, fn := e.screen, n.onScroll
	return on.Scroll(func(dx, dy int) tea.Cmd {
		if screen == nil {
			return nil
		}
		f := screen()
		k := f.find(key)
		if k < 0 || !f.entries[k].isVP {
			return nil
		}
		v := f.entries[k].vp
		p := v.Pos
		switch {
		case dy != 0:
			p = v.Down(dy * wheelStep)
		case dx != 0:
			p = v.Right(dx * wheelStep)
		}
		if p == v.Pos {
			return nil
		}
		return fn(p)
	})
}

// paintViewport adds the ops that draw the rows of viewport n, box i, in
// sight inside vis.
func (r *renderer) paintViewport(i int, n ViewportNode, vis image.Rectangle) {
	l := r.viewportOf(i)
	if l == nil || !r.parse {
		return
	}
	// Rows are only laid out for a buffer.
	buf, rows := n.buf, r.vpRows[l.rows[0]:l.rows[1]]
	var m *bufMeasure
	if len(rows) > 0 {
		m = buf.measure(r.method)
	}
	content := l.content
	lineSGR, marks, markLine := "", r.vpMarks[:0], -1
	for k, row := range rows {
		y := content.Min.Y + k
		gx := r.boxes[i].rect.Min.X
		for g := range n.gutters {
			gt := &n.gutters[g]
			w := gt.size(buf.Len())
			text := gt.text(GutterRow{Line: row.line, Continued: row.sub > 0, Lines: buf.Len()}, w)
			r.blitText(text, n.trusted, image.Rect(gx, y, gx+w, y+1), vis.Intersect(image.Rect(gx, y, content.Min.X, y+1)))
			gx += w
		}

		s := buf.lines[row.line]
		if row.line != markLine {
			markLine, lineSGR, marks = row.line, n.style, marks[:0]
			if n.lineStyle != nil {
				lineSGR += r.vpPens.overlay(n.lineStyle(row.line))
			}
			if n.marks != nil {
				marks = markCells(s, r.method, n.marks(row.line, plainText(s, r.method)), &r.vpPens, marks)
			}
		}
		st := rowStyle{line: lineSGR, marks: marks, pens: &r.vpPens, pass: n.trusted}
		if lineSGR != "" {
			st.fill = content.Dx()
		}
		text := cutRow(s, buf.cutFrom(row.line, r.method, row.a), m.widths[row.line], r.method, row.a, row.b, &st)
		r.blitText(text, n.trusted, image.Rect(content.Min.X, y, content.Max.X, y+1), vis)
	}
	r.vpMarks = marks[:0]
	if n.style != "" && len(rows) < content.Dy() {
		blank := n.style + strings.Repeat(" ", content.Dx()) + ansi.ResetStyle
		for y := content.Min.Y + len(rows); y < content.Max.Y; y++ {
			r.blit(blank, image.Rect(content.Min.X, y, content.Max.X, y+1), vis)
		}
	}
}

// viewportLeaf returns the text leaf of viewport box i laid out as l,
// inside clip: all of its lines, as laid out from where it was scrolled to,
// of which those in sight are inside clip.
func viewportLeaf(l *vpLayout, clip image.Rectangle) (rect, vis image.Rectangle) {
	v := &l.view
	w := v.Width
	if v.wrap == WrapNone || v.Widest <= v.Width {
		w = max(v.Widest, v.Width)
	}
	at := image.Pt(l.content.Min.X-v.Pos.X, l.content.Min.Y-v.Row)
	rect = image.Rectangle{Min: at, Max: at.Add(image.Pt(w, v.TotalRows))}
	return rect, l.content.Intersect(clip)
}

// vpRowCells returns the cells of row y of the lines of the viewport laid
// out as l, all of it from its first cell, kept for the next call, as
// selecting a word asks for each cell of a row in turn.
func (r *renderer) vpRowCells(l *vpLayout, y int) (cells string) {
	v := &l.view
	if y < 0 || y >= v.TotalRows || v.buf == nil {
		return ""
	}
	if m := &r.vpCell; m.buf == v.buf && m.rev == v.buf.rev && m.y == y && m.width == v.Width && m.wrap == v.wrap {
		return m.cells
	}
	var scratch []int
	line, a, b := v.rowAt(v.index(), y, &scratch)
	s := v.buf.lines[line]
	m := v.buf.measure(v.method)
	cells = cutRow(s, v.buf.cutFrom(line, v.method, a), m.widths[line], v.method, a, b, nil)
	r.vpCell = vpCellMemo{buf: v.buf, rev: v.buf.rev, y: y, width: v.Width, wrap: v.wrap, cells: cells}
	return cells
}

// rowAt returns the line that row y of the lines of v shows, with index x,
// and the cells of it the row takes, from a up to end, which for its last
// row is past the end of the line. Wrapped rows are worked out in scratch.
func (v ViewportView) rowAt(x *wrapIndex, y int, scratch *[]int) (line, a, end int) {
	line, sub := x.lineAt(y)
	if x.cum == nil || x.rowsOf(line) == 1 {
		return line, 0, math.MaxInt
	}
	starts := v.buf.rowStarts(line, v.method, v.Width, v.wrap, scratch)
	a, end = starts[sub], math.MaxInt
	if sub+1 < len(starts) {
		end = starts[sub+1]
	}
	return line, a, end
}

// vpCellMemo is the row of a viewport whose cells were last asked for.
type vpCellMemo struct {
	buf      *Buffer
	rev      uint64
	y, width int
	wrap     WrapMode
	cells    string
}

// vpText returns the text of the lines of the viewport laid out as l from
// cell from to cell to, in reading order, as laid out, with the rows of a
// wrapped line joined.
func vpText(l *vpLayout, from, to image.Point, block bool) string {
	v := &l.view
	if v.buf == nil || v.buf.Len() == 0 {
		return ""
	}
	x := v.index()
	var b strings.Builder
	var scratch []int
	rowCells := func(y int) (line, a, end int) { return v.rowAt(x, y, &scratch) }
	// text returns the text of the cells of line from lo to hi.
	text := func(line, lo, hi int) string {
		s := v.buf.lines[line]
		return strings.TrimRight(cellText(s, v.buf.cutFrom(line, v.method, lo), v.method, lo, hi), " ")
	}
	to.Y = min(to.Y, v.TotalRows-1)
	if block {
		for y := max(from.Y, 0); y <= to.Y; y++ {
			if y > from.Y {
				b.WriteByte('\n')
			}
			line, a, end := rowCells(y)
			lo, hi := a+from.X, min(a+to.X, end-1)
			b.WriteString(text(line, lo, hi))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	for y := max(from.Y, 0); y <= to.Y; {
		line, a, end := rowCells(y)
		lo := a
		if y == from.Y {
			lo = min(a+from.X, end)
		}
		// The rows of the line, from this one, up to the last selected.
		last := min(to.Y, x.rowsBefore(line)+x.rowsOf(line)-1)
		hi := math.MaxInt
		if last == to.Y {
			_, la, lend := rowCells(last)
			hi = min(la+to.X, lend-1)
		}
		if y > from.Y {
			b.WriteByte('\n')
		}
		b.WriteString(text(line, lo, hi))
		y = last + 1
	}
	return b.String()
}
