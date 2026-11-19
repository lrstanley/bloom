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

	"bloomfx.dev/bloom/internal/linebuf"
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
	caret     *Range
	onCaret   func(sel Range) tea.Cmd
	cursor    *tea.Cursor
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

// WrapMode is how a [Viewport] fits lines wider than it is.
type WrapMode uint8

const (
	// WrapNone shows lines whole, cut off at the viewport's right edge,
	// and scrolled sideways with [ViewportPos.X]. It is the default.
	WrapNone = WrapMode(linebuf.None)
	// WrapWords wraps lines at the spaces between words, and after
	// hyphens, as Lip Gloss wraps a [Text], breaking a word only when it is
	// wider than the viewport.
	WrapWords = WrapMode(linebuf.Words)
	// WrapChars wraps lines at the viewport's width, wherever it falls.
	WrapChars = WrapMode(linebuf.Chars)
)

// styleSGR returns the escape sequence that sets the colors and attributes
// of style, kept in c when it is not nil, or "" for a style that sets none.
func styleSGR(c *linebuf.SGRs, style lipgloss.Style) string {
	s := cellStyle(style)
	if linebuf.ZeroStyle(&s) {
		return ""
	}
	return c.Of(&s)
}

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
	// Line is the line the row shows, or -1 for a row below the last line,
	// which shows none.
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
// The column runs the viewport's full height: fn is also called for the
// rows below the last line, with a Line of -1, such as to carry the
// gutter's background down, or draw a "~". Gutters are drawn left to right
// in the order given, and take their space from the lines.
func (n ViewportNode) Gutter(width int, fn func(row GutterRow) string) ViewportNode {
	if width <= 0 || fn == nil {
		return n
	}
	n.gutters = append(slices.Clip(n.gutters), gutter{width: width, fn: fn})
	return n
}

// LineNumbers draws the number of each line, from 1, in a gutter on the
// left as wide as the largest number and a space, drawn in style's colors
// and attributes. Rows that continue a wrapped line, and those below the
// last line, have none, but are drawn in style.
func (n ViewportNode) LineNumbers(style lipgloss.Style) ViewportNode {
	n.gutters = append(slices.Clip(n.gutters), gutter{numbers: styleSGR(nil, style), lines: true})
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
	n.style = styleSGR(nil, style)
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

// Caret makes the viewport an editing surface: sel is what is selected in
// its buffer, the caret at its Head, kept in the model. The viewport reads
// it whenever it is drawn, and [Root.Selection] whenever it is asked, so a
// range the program changes with the keyboard is seen in the same Update,
// and the mouse writes it, as it does the range of an [Editable]: a press
// puts the caret under the pointer, before the character there, and with
// shift held moves the Head there and keeps the Anchor; a drag moves the
// Head; a double click selects the word, found in the buffer's text as
// [WithWordChars] tells, and a triple click the line. A press past the end of a row its line wraps after
// leaves the caret at the end of that row, with [Range.EndOfRow] set, rather
// than at the start of the next. A range that holds text is drawn as the
// selection is, and is the one selection on the screen: [Root.CopySelection]
// copies it and leaves it, [Root.ClearSelection] collapses it to its Head,
// and [Root.SelectAll], given the viewport's ID, selects all of the buffer.
// The viewport is selectable without [ViewportNode.Selectable], and dragging
// past its edge scrolls it through [ViewportNode.OnScroll].
//
// So that the caret has a cell at the end of a line as wide as the
// viewport, the lines are laid out one column narrower than it, the last
// column kept for the caret, whether they wrap or scroll sideways. An empty
// buffer has the caret at its top-left, and takes edits as one empty line.
//
// Following the range through the edits to the buffer is the program's to
// do, with the [Change] each edit returns, such as to put the caret at
// [Change.End] after typing. A range out of the buffer's bounds is clamped
// where it is drawn and read. The methods of [ViewportView] move the caret
// by the rows the lines wrap to, to the ends of rows, and by pages, and
// keep it in sight.
func (n ViewportNode) Caret(sel *Range) ViewportNode { n.caret = sel; return n }

// OnCaret calls fn when the mouse moves the caret or changes what is
// selected, after the range given to [ViewportNode.Caret] is written, such
// as to forget the column kept while moving up and down. Its command is
// run as the mouse event's.
func (n ViewportNode) OnCaret(fn func(sel Range) tea.Cmd) ViewportNode {
	n.onCaret = fn
	return n
}

// Cursor shows the terminal cursor at the caret, the Head of the range given
// to [ViewportNode.Caret], drawn on the side of a wrap its EndOfRow tells,
// as [ViewportView.CaretCell] has it, while it is in sight, in c's shape and
// color, as [TextNode.Cursor] does at a cell: give it only while the
// viewport has focus. The position of c is not used. It does nothing without
// a caret.
func (n ViewportNode) Cursor(c *tea.Cursor) ViewportNode { n.cursor = c; return n }

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
	if row.Continued || row.Line < 0 {
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
	// gutters, and besides the column kept for the caret of a viewport
	// given [ViewportNode.Caret].
	Width int
	// Widest is how many columns the widest line took.
	Widest int

	buf    *Buffer
	method ansi.Method
	wrap   WrapMode
	follow bool
	gutter int  // how many columns the gutters took
	caret  bool // a column past Width is kept for the caret
}

// index returns the rows of the view's buffer at its width.
func (v ViewportView) index() linebuf.Index {
	return v.buf.s.Index(v.method, linebuf.Mode(v.wrap), v.Width)
}

// AtRow returns the position with row at the top, counting the rows of all
// of the lines, wrapped, from 0, as a [Scrollbar] gives its offset.
func (v ViewportView) AtRow(row int) ViewportPos {
	if v.buf == nil || v.buf.Len() == 0 {
		return ViewportPos{}
	}
	x := v.index()
	bottom := max(x.Total()-v.Rows, 0)
	row = min(max(row, 0), bottom)
	if v.follow && row == bottom {
		return ViewportPos{Line: ViewportEnd.Line, X: v.Pos.X}
	}
	line, sub := x.LineAt(row)
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
	start, rows := x.RowsBefore(line), x.RowsOf(line)
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

// The methods below find positions in the buffer, and move the caret of a
// range, by the rows its lines are laid out in, as the keys of an editor
// move it, and take and give cells relative to the viewport's top-left, its
// gutters included, as the mouse does. Those that move a caret find the row
// it is on with its side, [Range.EndOfRow], and give it the side it needs
// where they move it to; the Anchor is left as it is, for the program to
// collapse the range to the caret unless shift is held.

// top returns the row at the top of the viewport, with x the index of the
// rows of its buffer as it is now, as a frame would lay it out from Pos.
func (v ViewportView) top(x linebuf.Index) int {
	bottom := max(x.Total()-v.Rows, 0)
	switch p := v.Pos; {
	case p.Line >= v.buf.Len():
		return bottom
	case p.Line >= 0:
		return min(x.RowsBefore(p.Line)+min(max(p.Row, 0), x.RowsOf(p.Line)-1), bottom)
	}
	return 0
}

// cols returns how many columns of text the viewport shows: its width, and
// the caret's column.
func (v ViewportView) cols() int {
	if v.caret {
		return v.Width + 1
	}
	return v.Width
}

// PosAt returns the position in the buffer of the character drawn at cell,
// relative to the viewport's top-left, as the mouse finds it to put a caret
// there: before the character, the end of the row for a cell past its
// text, the start of the buffer for a cell above its first row, and its end
// for one below its last. A row a line wraps after ends where the next row
// starts, past the spaces that hang off a row wrapped at words. The cells
// of the gutters are columns before the first in sight.
func (v ViewportView) PosAt(cell image.Point) Pos {
	return v.CaretAt(cell).Head
}

// CaretAt returns the empty range at cell, relative to the viewport's
// top-left, as a click puts the caret there: at the position
// [ViewportView.PosAt] gives, with EndOfRow set when the cell is past the
// text of a row its line wraps after, so that the caret stays on that row
// rather than going to the start of the next.
func (v ViewportView) CaretAt(cell image.Point) Range {
	if v.buf == nil || v.buf.Len() == 0 {
		return Range{}
	}
	rs := v.index()
	var scratch []int
	row := cell.Y + v.top(rs)
	p := rs.PosAt(row, cell.X-v.gutter+v.Pos.X, &scratch)
	return Range{Anchor: Pos(p), Head: Pos(p), EndOfRow: rs.EndsRow(p, row, &scratch)}
}

// CellOf returns the cell, relative to the viewport's top-left, that p,
// clamped to the buffer, is drawn at, and whether that is in sight. A
// position where a line wraps is drawn at the start of the row after, and
// one among the spaces that hang off the end of a row wrapped at words, at
// the column past its text. [ViewportView.CaretCell] finds where a caret
// is drawn, which may be at the end of the row before.
func (v ViewportView) CellOf(p Pos) (image.Point, bool) { return v.cellAt(p, false) }

// CaretCell returns the cell, relative to the viewport's top-left, that the
// caret of sel, at its Head, is drawn at, and whether that is in sight: as
// [ViewportView.CellOf] has it, but at a place its line wraps with
// EndOfRow set, past the cells of the row before, in the caret's column at
// the latest.
func (v ViewportView) CaretCell(sel Range) (image.Point, bool) {
	return v.cellAt(sel.Head, sel.EndOfRow)
}

// cellAt is CellOf, and with end, CaretCell.
func (v ViewportView) cellAt(p Pos, end bool) (image.Point, bool) {
	if v.buf == nil || v.buf.Len() == 0 {
		return image.Pt(v.gutter, 0), v.Rows > 0 && v.cols() > 0
	}
	rs := v.index()
	var scratch []int
	row, cell := rs.CaretIn(linebuf.Pos(p), end, &scratch)
	at := image.Pt(v.gutter+cell-v.Pos.X, row-v.top(rs))
	in := at.Y >= 0 && at.Y < v.Rows && cell >= v.Pos.X && cell-v.Pos.X < v.cols()
	return at, in
}

// Column returns the column of its row that the caret of sel is drawn at,
// counting from the start of the row rather than from where the viewport
// is scrolled to, to keep while moving up and down with
// [ViewportView.MoveRows].
func (v ViewportView) Column(sel Range) int {
	if v.buf == nil || v.buf.Len() == 0 {
		return 0
	}
	var scratch []int
	_, cell := v.index().CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	return cell
}

// MoveRows returns sel with its Head moved n rows down from the row its
// caret is drawn on, or up for a negative n, counting the rows the lines
// wrap to, to column of that row, as an arrow key moves a caret: to the
// character boundary nearest the column, never inside a wide character or
// a tab, and to the end of the row, as [ViewportView.RowEnd] has it, for a
// column at or past the row's cells. A move above the first row goes to the
// start of the text, and one below the last row to its end. The Anchor is
// kept, and EndOfRow is set for the Head at the end of a row its line wraps
// after.
func (v ViewportView) MoveRows(sel Range, n, column int) Range {
	if v.buf == nil || v.buf.Len() == 0 {
		return sel.WithHead(Pos{})
	}
	rs := v.index()
	var scratch []int
	row, _ := rs.CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	switch row += n; {
	case row < 0:
		return sel.WithHead(Pos{})
	case row >= rs.Total():
		return sel.WithHead(v.buf.End())
	}
	p, end := rs.AtColumn(row, column, &scratch)
	sel = sel.WithHead(Pos(p))
	sel.EndOfRow = end
	return sel
}

// ScrollRows returns sel with its Head moved n rows as [ViewportView.MoveRows]
// moves it, and the position scrolled n rows the same way, so that the
// caret stays on the row of the viewport it was on, as page up and page
// down move it. Near the top or the bottom, the viewport scrolls as far as
// it can, and the caret still moves. The position has the caret in sight,
// scrolled as [ViewportView.Show] scrolls it when it would not be, such as
// for a caret that was out of sight.
func (v ViewportView) ScrollRows(sel Range, n, column int) (Range, ViewportPos) {
	sel = v.MoveRows(sel, n, column)
	if v.buf == nil || v.buf.Len() == 0 {
		return sel, v.Pos
	}
	rs := v.index()
	top := min(max(v.top(rs)+n, 0), max(rs.Total()-v.Rows, 0))
	var scratch []int
	row, cell := rs.CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	return sel, v.show(rs, v.AtRow(top), top, row, cell)
}

// RowStart returns sel with its Head moved to the start of the row its
// caret is drawn on, as the home key moves a caret, its Anchor kept.
func (v ViewportView) RowStart(sel Range) Range {
	if v.buf == nil || v.buf.Len() == 0 {
		return sel.WithHead(Pos{})
	}
	rs := v.index()
	var scratch []int
	row, _ := rs.CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	return sel.WithHead(Pos(rs.PosAt(row, 0, &scratch)))
}

// RowEnd returns sel with its Head moved to the end of the row its caret
// is drawn on, as the end key moves a caret, its Anchor kept: the end of
// the line on its last row, and on a row the line wraps after, the place
// it wraps, where the next row starts, after any spaces that hang off a
// row wrapped at words, with EndOfRow set so that the caret is drawn at
// the end of the row rather than at the start of the next.
func (v ViewportView) RowEnd(sel Range) Range {
	if v.buf == nil || v.buf.Len() == 0 {
		return sel.WithHead(Pos{})
	}
	rs := v.index()
	var scratch []int
	row, _ := rs.CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	p, end := rs.RowEnd(row, &scratch)
	sel = sel.WithHead(Pos(p))
	sel.EndOfRow = end
	return sel
}

// Show returns the position scrolled as little as it takes for the caret
// of sel to be in sight, at the cell [ViewportView.CaretCell] gives, down
// or up, and while the lines do not wrap, sideways, such as to keep a caret
// in sight as it moves. It is Pos when the caret is in sight.
func (v ViewportView) Show(sel Range) ViewportPos {
	if v.buf == nil || v.buf.Len() == 0 {
		return v.Pos
	}
	rs := v.index()
	var scratch []int
	row, cell := rs.CaretIn(linebuf.Pos(sel.Head), sel.EndOfRow, &scratch)
	return v.show(rs, v.Pos, v.top(rs), row, cell)
}

// show returns pos, with top the row at its top, scrolled as little as it
// takes for cell of row to be in sight.
func (v ViewportView) show(x linebuf.Index, pos ViewportPos, top, row, cell int) ViewportPos {
	switch {
	case row < top:
		pos = v.AtRow(row)
	case row >= top+max(v.Rows, 1):
		pos = v.AtRow(row - max(v.Rows, 1) + 1)
	}
	if x.Uniform() {
		switch cols := max(v.cols(), 1); {
		case cell < pos.X:
			pos.X = cell
		case cell >= pos.X+cols:
			pos.X = cell - cols + 1
		}
	}
	return pos
}

// vpLayout is a viewport as laid out in a frame: where it was scrolled to,
// where its lines go, and the rows in sight, a span of the engine's vpRows.
type vpLayout struct {
	view    ViewportView
	content image.Rectangle
	rows    [2]int
	past    int // how many rows at the bottom are below the last line
}

// measureViewport returns the natural size of viewport n within avail: as
// wide as its widest line, besides its gutters, and as tall as its lines,
// wrapped to that width, no more than avail.
func (e *engine) measureViewport(n ViewportNode, avail Size) Size {
	// A caret has a column of its own, past the lines.
	caret := 0
	if n.caret != nil {
		caret = 1
	}
	if n.buf == nil || n.buf.Len() == 0 {
		if caret > 0 {
			return Size{W: gutterWidth(n.gutters, 0) + caret, H: 1}
		}
		return Size{}
	}
	lines := n.buf.Len()
	g := gutterWidth(n.gutters, lines) + caret
	m := n.buf.s.Measure(e.method)
	w := m.Widest
	if n.wrap != WrapNone {
		w = min(w, max(avail.W-g, 0))
	}
	if w <= 0 {
		// Empty lines, or no room to wrap in: a row a line.
		return Size{W: g, H: lines}
	}
	if n.wrap == WrapNone || m.Widest <= w {
		return Size{W: w + g, H: lines}
	}
	if avail.H >= unbounded {
		return Size{W: w + g, H: n.buf.s.Index(e.method, linebuf.Mode(n.wrap), w).Total()}
	}
	// Only whether the lines fill avail matters, so they are counted until
	// they do.
	return Size{W: w + g, H: n.buf.s.CountRows(e.method, linebuf.Mode(n.wrap), w, avail.H)}
}

// layoutViewport works out where viewport n, box i, laid out in r, is
// scrolled to and which rows are in sight.
func (e *engine) layoutViewport(i int, n ViewportNode, r image.Rectangle) {
	l := vpLayout{content: r}
	l.rows[0] = len(e.vpRows)
	l.view = ViewportView{buf: n.buf, method: e.method, wrap: n.wrap, follow: n.follow, Rows: r.Dy(), caret: n.caret != nil}
	if n.buf != nil {
		lines := n.buf.Len()
		l.view.gutter = min(gutterWidth(n.gutters, lines), r.Dx())
		l.content.Min.X += l.view.gutter
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
	if n.caret != nil {
		// The last column is the caret's, past the end of a line as wide as
		// the lines are laid out.
		w = max(w-1, 0)
	}
	m := n.buf.s.Measure(e.method)
	x := n.buf.s.Index(e.method, linebuf.Mode(n.wrap), w)
	v.Lines, v.Width, v.Widest, v.TotalRows = lines, w, m.Widest, x.Total()
	switch {
	case lines == 0:
		l.past = h
		return
	case w <= 0:
		// No room for the lines, so which rows have one is not worked out.
		return
	}

	bottom := max(v.TotalRows-h, 0)
	p := n.pos
	row := 0
	switch {
	case p.Line >= lines:
		row = bottom
	case p.Line >= 0:
		row = x.RowsBefore(p.Line) + min(max(p.Row, 0), x.RowsOf(p.Line)-1)
	}
	row = min(row, bottom)
	line, sub := x.LineAt(row)
	col := 0
	if x.Uniform() {
		col = min(max(p.X, 0), max(m.Widest-w, 0))
	}
	v.Row, v.First, v.Pos = row, line, ViewportPos{Line: line, Row: sub, X: col}
	if n.follow && row == bottom {
		v.Pos = ViewportPos{Line: ViewportEnd.Line, X: col}
	}

	e.vpRows = x.InSight(line, sub, h, col, e.vpRows)
	l.past = h - (len(e.vpRows) - l.rows[0])
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
// in, or in the frame drawn last when that still shows the same buffer.
func (e *engine) viewportScroll(n ViewportNode, key nodeKey) on.Handler {
	screen, lastFrame, fn := e.screen, e.lastFrame, n.onScroll
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
		// A step goes from the frame drawn last, which has what the model made
		// of the events before this one: a terminal can send several wheel
		// events before the screen is redrawn, and each would otherwise step
		// from the same place. The buffer must match, as a node without an ID
		// is found by where it is, which another viewport may have taken.
		if l := lastFrame(); l != nil && l != f {
			if j := l.find(key); j >= 0 && l.entries[j].isVP && l.entries[j].vp.buf == v.buf {
				v = l.entries[j].vp
			}
		}
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
	if r.rooted && n.buf != nil {
		// A buffer with no word characters of its own finds words as a
		// double click here does.
		n.buf.shownWith(r.words.Chars)
	}
	// Rows are only laid out for a buffer.
	buf, rows := n.buf, r.vpRows[l.rows[0]:l.rows[1]]
	content := l.content
	lineSGR, marks, markLine := "", r.vpMarks[:0], -1
	for k, row := range rows {
		y := content.Min.Y + k
		r.paintGutters(i, n, y, GutterRow{Line: row.Line, Continued: row.Sub > 0, Lines: buf.Len()}, vis)

		if row.Line != markLine {
			markLine, lineSGR, marks = row.Line, n.style, marks[:0]
			if n.lineStyle != nil {
				lineSGR += styleSGR(&r.vpPens, n.lineStyle(row.Line))
			}
			if n.marks != nil {
				s := buf.s.Line(row.Line)
				spans := r.vpSpans[:0]
				given := n.marks(row.Line, linebuf.PlainText(s, r.method))
				for k := range given {
					m := &given[k]
					spans = append(spans, linebuf.Overlay{Start: m.Start, End: m.End, SGR: styleSGR(&r.vpPens, m.Style)})
				}
				marks, r.vpSpans = linebuf.MarkCells(s, r.method, spans, marks), spans[:0]
			}
		}
		st := linebuf.RowStyle{Line: lineSGR, Marks: marks, Pens: &r.vpPens, Pass: n.trusted}
		if lineSGR != "" {
			st.Fill = content.Dx()
		}
		text := buf.s.CutRow(row.Line, r.method, row.A, row.B, &st)
		r.blitText(text, n.trusted, image.Rect(content.Min.X, y, content.Max.X, y+1), vis)
	}
	r.vpMarks = marks[:0]
	for y := content.Max.Y - l.past; y < content.Max.Y; y++ {
		r.paintGutters(i, n, y, GutterRow{Line: -1, Lines: l.view.Lines}, vis)
	}
	if n.style != "" && len(rows) < content.Dy() {
		blank := n.style + strings.Repeat(" ", content.Dx()) + ansi.ResetStyle
		for y := content.Min.Y + len(rows); y < content.Max.Y; y++ {
			r.blit(blank, image.Rect(content.Min.X, y, content.Max.X, y+1), vis)
		}
	}
}

// paintGutters adds the ops that draw the gutters of viewport n, box i,
// beside row, at y, inside vis.
func (r *renderer) paintGutters(i int, n ViewportNode, y int, row GutterRow, vis image.Rectangle) {
	end := r.viewportOf(i).content.Min.X
	gx := r.boxes[i].rect.Min.X
	for g := range n.gutters {
		gt := &n.gutters[g]
		w := gt.size(row.Lines)
		r.blitText(gt.text(row, w), n.trusted, image.Rect(gx, y, gx+w, y+1), vis.Intersect(image.Rect(gx, y, end, y+1)))
		gx += w
	}
}

// paintCaret notes box i, viewport n given a caret, drawn at vis, whose
// range is kept in the model, adds the op that draws the range over it, and
// shows the cursor at its Head.
func (r *renderer) paintCaret(i int, n ViewportNode, vis image.Rectangle) {
	l := r.viewportOf(i)
	if l == nil {
		return
	}
	b := &r.boxes[i]
	if n.cursor != nil {
		if at, ok := l.view.CaretCell(*n.caret); ok {
			c := *n.cursor
			c.X, c.Y = at.X, at.Y
			r.placeCursor(&c, b.rect.Min, vis)
		}
	}
	if !r.parse {
		return
	}
	buf, sel, fn := n.buf, n.caret, n.onCaret
	rect, shown := viewportLeaf(l, vis)
	h := heldLeaf{
		leaf: selLeaf{key: b.key(), box: i, rect: rect, clip: shown, vp: l},
		get:  func() Range { return clampRange(buf, *sel) },
		set: func(rg Range) tea.Cmd {
			*sel = clampRange(buf, rg)
			if fn != nil {
				return fn(*sel)
			}
			return nil
		},
	}
	if buf != nil {
		h.leaf.rev = buf.Rev()
	}
	r.addHeld(h, b.rect, vis)
}

// clampRange returns rg with its ends clamped to the text of buf, and
// EndOfRow kept.
func clampRange(buf *Buffer, rg Range) Range {
	if buf == nil {
		return Range{}
	}
	return Range{Anchor: buf.Clamp(rg.Anchor), Head: buf.Clamp(rg.Head), EndOfRow: rg.EndOfRow}
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
// reading the cells of a selection asks for each cell of a row in turn.
func (r *renderer) vpRowCells(l *vpLayout, y int) (cells string) {
	v := &l.view
	if y < 0 || v.buf == nil {
		return ""
	}
	// The rows are those the lines take now, which an edit since the frame
	// may have left fewer of.
	rows := v.index()
	if y >= rows.Total() {
		return ""
	}
	return rows.RowCells(y, &r.vpCell)
}

// vpWord returns the word of the buffer of the viewport laid out as l that
// the character at cell at of its lines is in, and whether there is one:
// none for a cell with no character, past the text of its row. It is found
// in the buffer's text, rather than in the cells of the row, so that a word
// wrapped across rows is found whole, with the buffer's word characters, or
// the Root's when it was given none.
func (r *renderer) vpWord(l *vpLayout, at image.Point) (from, to Pos, ok bool) {
	v := &l.view
	if v.buf == nil || v.buf.Len() == 0 || at.X < 0 {
		return Pos{}, Pos{}, false
	}
	p, ok := v.index().CharAt(at.Y, at.X, &r.vpScratch)
	if !ok {
		return Pos{}, Pos{}, false
	}
	return v.buf.wordAt(Pos(p), v.buf.chars(r.words.Chars), false)
}

// vpCells calls fn with each cell on screen inside clip, of the rows in
// sight of the viewport laid out as l, that the text from from up to to
// takes. A row the selection runs on past is taken to the viewport's right
// edge, for the line break or the wrap after it.
func (r *renderer) vpCells(l *vpLayout, from, to Pos, clip image.Rectangle, fn func(p image.Point)) {
	v := &l.view
	if v.buf == nil {
		return
	}
	rows := v.index()
	for k, row := range r.vpRows[l.rows[0]:l.rows[1]] {
		y := l.content.Min.Y + k
		if y < clip.Min.Y || y >= clip.Max.Y {
			continue
		}
		lo, hi, ok := rows.Covered(row, linebuf.Pos(from), linebuf.Pos(to))
		if !ok {
			continue
		}
		x0, x1 := l.content.Min.X+lo-row.A, l.content.Max.X
		if hi != math.MaxInt {
			x1 = min(x1, l.content.Min.X+hi-row.A)
		}
		for px := max(x0, clip.Min.X); px < min(x1, clip.Max.X); px++ {
			fn(image.Pt(px, y))
		}
	}
}

// vpBlockText returns the text of the cells of the lines of the viewport
// laid out as l in the rectangle with corners from and to, inclusive, as
// laid out, a line to a row.
func vpBlockText(l *vpLayout, from, to image.Point) string {
	v := &l.view
	if v.buf == nil || v.buf.Len() == 0 {
		return ""
	}
	rows := v.index()
	return rows.BlockText(from.X, from.Y, to.X, min(to.Y, rows.Total()-1))
}
