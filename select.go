// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"math"
	"slices"
	"time"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/internal/linebuf"
	"bloomfx.dev/bloom/internal/textsel"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// SelectionMsg is sent when text is selected with the mouse in a node made
// selectable: when a drag that selected some ends, or once a double click
// can no longer become a triple click. The selection stays until the next
// click, [Root.ClearSelection], or [Root.CopySelection].
type SelectionMsg struct {
	// ID is the ID of the node the text was selected in, or of its nearest
	// ancestor with one, such as the slot a component is placed in.
	ID any
	// Text is the text selected.
	Text string
}

// CopyMsg is sent once [Root.CopySelection] has copied the text selected.
type CopyMsg struct {
	// ID is the ID of the node the text was selected in, as in
	// [SelectionMsg].
	ID any
	// Text is the text copied.
	Text string
	// Err is what the function given with [WithClipboard] returned, such as
	// when the system clipboard did not take the text. The text was sent to
	// the terminal's clipboard with OSC 52 either way, which terminals do not
	// answer, so a nil Err cannot promise the text arrived there.
	Err error
}

// TextSource is a component that knows its own text, which selecting text
// in it with the mouse reads instead of its cells as drawn: such as to copy
// a secret it shows masked, or to leave out what it draws around its text.
// A component that implements it is selectable without being made so, as
// with [SlotNode.Selectable].
type TextSource interface {
	Child
	// SelectedText returns the text of the component in span.
	SelectedText(span Span) string
}

// TextMap is a component that shows part of its text, or lays it out
// itself, and tells where in that text its cells are, such as one that
// scrolls its own lines or wraps them, or draws a gutter beside them. A
// selection in it is kept as positions in its text rather than as cells, so
// that it stays on the text as the component scrolls, and takes the text
// scrolled out of sight. The root keeps the selection, draws it over the
// cells Cells reports, after the component draws, in the selection's style
// (see [WithSelectionStyle]), and reads the text copied from Text.
//
// A component that implements it is selectable without being made so, as
// with [SlotNode.Selectable], and may also be inside a larger node made
// selectable, which a selection then runs into and through, as it does
// through a [Text]. Dragging in it selects the character under the pointer,
// as a terminal does; a double click selects the word there, as drawn, and
// a triple click the whole line of the text. Holding alt as the drag starts
// selects as a drag does, since a rectangle of cells is more than the
// methods tell. Dragging past its edge scrolls it through the [on.Scroll]
// handler under the pointer, as the wheel would, for as long as the pointer
// is held there, since the root cannot know how much text is out of sight.
//
// A selection in it ends when it leaves the frame. When its text changes
// under a selection, clearing the selection, with [Root.ClearSelection], is
// the program's to do.
//
// It is for a component that draws its own cells, with View. One that is a
// [Layouter] is drawn by the nodes it lays out, whose text the root reads
// from them, and is not taken as a TextMap, or as an [Editable].
type TextMap interface {
	Child
	// PosAt returns the position in the text of the character drawn at
	// cell, relative to the component's top-left, laid out at size: the end
	// of the row for a cell past its text, including one just past the
	// component's right edge, the start of the row for one before its text,
	// such as in a gutter, and the start of the text for a cell above it, and
	// its end for one below it.
	PosAt(cell image.Point, size Size) Pos
	// Cells calls row for each row in sight, laid out at size, that the text
	// from from up to to covers, with the cells of it it takes, from x0 up to
	// x1, relative to the component's top-left. Either position may be past
	// the end of its line, or of the text, which the root gives for "to the
	// end": the component clamps them.
	Cells(from, to Pos, size Size, row func(y, x0, x1 int))
	// Text returns the text from from up to to, which may be past the end of
	// their lines or of the text, as for Cells.
	Text(from, to Pos) string
}

// Editable is a [TextMap] that keeps its own selection, such as an editor
// or an input, whose caret is the selection's Head. It keeps the range
// itself because its caret outlives the selection and is wanted while it
// handles a key, which may make or move the selection as often as the mouse
// does. The root does everything else: it sets the range from the mouse,
// draws it, copies it, and keeps one selection at a time across the screen.
//
// A press puts the caret under the pointer, and with shift held moves the
// Head there and keeps the Anchor. The Root leaves [Range.EndOfRow] unset in
// the ranges it gives, since the methods of a TextMap tell positions in the
// text, not which side of a wrap a caret is drawn on. A drag moves the Head,
// running the range from where the caret went at the press to where it is
// under the pointer. A double click selects the word, and a triple click the
// line. Holding alt as the drag starts selects as a drag does. The positions
// given to Select may be past the end of a line or of the text: the
// component clamps them.
//
// Every frame the root reads Selection and draws a range that holds text
// over the cells Cells reports, as it draws its own selection: the component
// draws no highlight of its own. A selection started anywhere else collapses
// the range to its Head. A range the component makes itself, such as with
// shift and an arrow key, becomes the selection as soon as the root sees it,
// in [Root.Selection] or [Root.CopySelection] in the same Update, or in the
// next frame, and any other is cleared. [SelectionMsg] is sent for what the
// mouse selects in it, and not for what the component selects.
//
// [Root.CopySelection] copies Text of the range and leaves the range as it
// is, [Root.ClearSelection] collapses it to its Head, and [Root.SelectAll],
// given the ID of the node the component is in, selects all of it.
//
// The component must be placed in the tree by pointer, such as
// bloom.Fixed(1, &m.input), so that Select reaches the model's copy of it.
//
// To edit the lines of a [Buffer], give a [Viewport] a caret with
// [ViewportNode.Caret] rather than implementing Editable: it behaves as one.
type Editable interface {
	TextMap
	// Selection returns what is selected, an empty range at the caret when
	// nothing is.
	Selection() Range
	// Select sets what is selected, from the mouse.
	Select(r Range)
}

// Span is the part of a node selected with the mouse, in cells relative to
// its top-left as laid out. Inside a [Scroll], that is the top-left of all
// of it, not of what is in sight.
type Span struct {
	// Start and End are the first and last cells selected, inclusive: every
	// cell from Start to End in reading order, or with Block, the rectangle
	// with them at its top-left and bottom-right corners. A selection that
	// takes a row to its end ends at the node's last column, which may be
	// past the end of the row's text.
	Start, End image.Point
	Block      bool
	// Size is the size the node was laid out at, such as for a component
	// that wraps its text to its width, to wrap it the same way again.
	Size Size
}

// WithSelectionStyle sets how text selected with the mouse is drawn: with
// the style's foreground and background, and its bold, italic, faint,
// underline, strikethrough, and reverse. A color the style leaves unset is
// kept from each cell, such as a background alone to keep the colors of
// highlighted code; a foreground kept that would be hard to read on the
// background, such as a dim one, takes the terminal's default foreground
// or background instead, whichever reads better.
//
// Without it, selected cells get a background between the terminal's
// default background and foreground, and keep their own foregrounds where
// they can be read on it.
func WithSelectionStyle(style lipgloss.Style) Option {
	return func(c *config) { c.selStyle, c.selStyled = style, true }
}

// WithWordChars sets the characters other than letters and digits that a
// double click takes into a word. Words are found as Unicode text
// segmentation (UAX #29) finds them, and are joined across these
// characters, so that the default, "@-./_~?&=%+#", keeps a path, a URL, or
// an email address one word. With "", words are as Unicode finds them.
//
// A double click in a [Viewport] finds the word in its buffer's text, so
// that a word its lines wrap across rows is selected whole, with the
// characters the buffer was given with [Buffer.SetWordChars], and these
// when it was given none: such a buffer takes these for [Buffer.WordAt],
// [Buffer.NextWord], and [Buffer.PrevWord] too, once it is shown. Elsewhere,
// such as in a [Text] or a [TextMap], it finds the word in the cells drawn
// on the row, with these.
func WithWordChars(chars string) Option {
	return func(c *config) { c.wordChars = &chars }
}

// WithClipboard sets the function [Root.CopySelection] writes text to the
// system clipboard with, such as one that runs xclip or wl-copy. It runs
// first, and the text then goes to the terminal's clipboard with OSC 52,
// which also reaches a terminal over SSH; what it returns is sent in
// [CopyMsg]. A function that cannot reach a system clipboard at all, such as
// over SSH, should return nil, leaving the copy to OSC 52.
func WithClipboard(write func(text string) error) Option {
	return func(c *config) { c.clipboard = write }
}

// WithCopyFlash sets whether [Root.CopySelection] sweeps a band of the
// selection's color across the text it copied, to show what went. It is on
// by default, and the band only sweeps while [anim.Motion] is on.
func WithCopyFlash(enabled bool) Option {
	return func(c *config) { c.noFlash = !enabled }
}

// WithClickDelay sets whether a click inside a node made selectable waits
// until it can no longer become a double click before its click handlers
// run, so that double clicking to select a word does not also click what
// it is in. A double or triple click there selects rather than clicks. It
// is on by default; off, clicks run at once, and a double click is two.
func WithClickDelay(enabled bool) Option {
	return func(c *config) { c.noClickDelay = !enabled }
}

const (
	// defaultWordChars are those of kitty, so that a path is one word.
	defaultWordChars = "@-./_~?&=%+#"
	// clickSlop is how far, in columns on the same row, a click may land
	// from the last one and still be part of a double or triple click.
	clickSlop = 1
	// scrollEvery is how often the node text is selected in scrolls while
	// the pointer is held past its edge.
	scrollEvery = 50 * time.Millisecond
	// flashFor is how long the band [Root.CopySelection] sweeps takes.
	flashFor = 450 * time.Millisecond
)

// The ticks a Root sends itself while selecting.
type (
	// selScrollMsg scrolls the text being selected.
	selScrollMsg struct{ root *Root }
	// selDoneMsg ends a double click selecting.
	selDoneMsg struct {
		root *Root
		seq  int
	}
	// clickMsg runs a click held back.
	clickMsg struct {
		root *Root
		seq  int
	}
	// flashMsg draws the next frame of the copy flash.
	flashMsg struct{ root *Root }
)

// selPos is where a selection starts or ends: a cell of a text leaf, found
// from the pointer once the frame is laid out, and followed from frame to
// frame by the leaf's key, so that it stays on the text as the leaf moves,
// such as inside a [Scroll]. In a leaf of text, a viewport or a [TextMap],
// it is the text there instead, which stays the same as the leaf scrolls
// and wraps it differently. In an item of a [List], it also keeps the item,
// so that it is kept while the list does not build the item.
type selPos struct {
	// Place is the list items the leaf is in, or the start or end of all of
	// the text of the node selected in, as [Root.SelectAll] selects it,
	// whatever leaves it has.
	textsel.Place

	leaf nodeKey
	at   image.Point // the cell, in a leaf of cells
	// In a leaf of text, the text the end takes whole, from from up to to:
	// the character, word, or line, or a caret, where from is to.
	from, to Pos
	// In a viewport, the row of the cell, by where the row starts, and its
	// column, for a corner of a block, and the buffer the positions are in,
	// at revision rev, to follow them through its edits.
	corner Pos
	col    int
	buf    *Buffer
	rev    uint64
	ok     bool
}

// selection is what a Root keeps of the text being selected, in the node
// with key, which bounds it.
type selection struct {
	ok      bool
	key     nodeKey
	id      any
	seq     int         // counts selections, for the ticks of one to know it
	press   image.Point // where the drag started, until anchor is found there
	pointer image.Point // where the pointer is while dragging, which head follows
	anchor  selPos
	head    selPos
	// headFirst is whether head came before anchor when that could last be
	// told, for when it cannot, as with both in an item that is not built.
	headFirst bool
	unit      textsel.Unit
	block     bool // a rectangle of cells, rather than a run of text
	// held is set for a selection whose range is kept outside the Root, by
	// the [Editable] with key, which anchor and head only follow while the
	// mouse selects in it.
	held bool
	// active is set once something is selected: when the pointer moves
	// after the press, or the press is a double or triple click. A click
	// alone selects nothing.
	active    bool
	moved     bool // the pointer moved since the press, so its release is no click
	dragging  bool
	scrolling bool // a selScrollMsg is due
}

// selLeaf is a node inside the selected node that has text: a [Text], a
// component, or a [TextSource], drawn at clip, which is empty when it is
// out of sight.
type selLeaf struct {
	key        nodeKey
	rect, clip image.Rectangle
	box        int
	src        *canvas.Canvas // its cells, all of them; nil for a TextSource drawn by its layout
	lazy       bool           // src is parsed only once wanted, for a leaf out of sight
	view       string         // what a component out of sight showed this frame, to parse then
	text       string         // the string of a styled text node, to find where it wrapped, or of one cut to fit
	truncated  bool           // text was cut to fit, with "…"
	source     TextSource
	inner      bool      // cells drawn by a TextSource's layout, which are not text of their own
	vp         *vpLayout // the layout of a viewport, whose rows are drawn from its buffer when wanted
	tm         TextMap   // a component that tells where in its text its cells are
	rev        uint64    // the revision of the viewport's buffer when the leaf was noted
}

// selEnd is an end of a selection and where it is in the leaves noted.
type selEnd struct {
	pos  selPos
	stop textsel.Stop
}

// selRange is a selection as laid out in a frame: from leaf a at pa to
// leaf b at pb, in the order the leaves were drawn, or with block, the
// corners of a rectangle. In a leaf of text, it starts at qa in leaf a, and
// ends before qb in leaf b. An end in an item of a list that the list did
// not build is at the start of the first leaf after it, or the end of the
// last before it, and a is past b when no leaf is between the ends. It is
// comparable, so a frame whose selection did not change is not composited
// again.
type selRange struct {
	a, b   int
	pa, pb image.Point
	qa, qb Pos
	block  bool
	show   bool
}

// cellStyle returns the colors and attributes of a Lip Gloss style, as a
// cell is drawn in them.
func cellStyle(style lipgloss.Style) uv.Style {
	var s uv.Style
	s.Fg, s.Bg = colorOf(style.GetForeground()), colorOf(style.GetBackground())
	attr := func(on bool, a uint8) {
		if on {
			s.Attrs |= a
		}
	}
	attr(style.GetBold(), uv.AttrBold)
	attr(style.GetItalic(), uv.AttrItalic)
	attr(style.GetFaint(), uv.AttrFaint)
	attr(style.GetStrikethrough(), uv.AttrStrikethrough)
	attr(style.GetBlink(), uv.AttrBlink)
	attr(style.GetReverse(), uv.AttrReverse)
	if style.GetUnderline() {
		s.Underline = uv.UnderlineSingle
	}
	return s
}

// colorOf returns c, or nil for a color that is not set.
func colorOf(c color.Color) color.Color {
	if _, none := c.(lipgloss.NoColor); none {
		return nil
	}
	return c
}

// selected reports whether box b is the node the Root is selecting in.
func (r *renderer) selected(b *box) bool {
	return r.sel.ok && !r.sel.held && b.meta.selectable && !b.inert && b.key() == r.sel.key
}

// noteLeaves notes the text leaves of box i, the node selected in, at path,
// clipped to clip, and the lists among them, in place of those noted
// before.
func (r *renderer) noteLeaves(i int, clip image.Rectangle, path uint64) {
	r.leaves = r.leaves[:0]
	r.outline.Reset()
	r.collectLeaves(i, clip, path, false)
}

// collectLeaves notes the text leaves of box i, the selected node or a box
// inside it at path, clipped to clip: every [Text], component, and
// [TextSource] in it, including those out of sight, such as scrolled away,
// so that a selection takes them too. What a [BoxNode] draws on its edges
// is decoration, and the layout of a TextSource draws cells but no text of
// its own, which inner marks. A [List] has only the items it built, and is
// noted with marks around them, for those it did not.
func (r *renderer) collectLeaves(i int, clip image.Rectangle, path uint64, inner bool) {
	if i < 0 || r.boxes[i].hidden {
		return
	}
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	kid := func(k int, inner bool) {
		if k < 0 {
			return
		}
		in := vis
		if r.boxes[k].float {
			// A layer that floats is clipped to the screen, not to what it is in.
			in = r.area
		}
		r.collectLeaves(k, in, childPath(path, r.boxes[k].slot), inner)
	}
	leaf := func(cells bool) *selLeaf {
		// A leaf is known by where it is in the selected node, rather than
		// in the tree, so that the selection stays on it as the node moves.
		key := nodeKey{path: path}
		if b.meta.id != nil {
			key = nodeKey{id: b.meta.id}
		}
		l := selLeaf{key: key, box: i, rect: b.rect, clip: vis, inner: inner}
		switch {
		case cells && !vis.Empty():
			// Drawn this frame, so its cells are already parsed.
			l.src = r.leafCanvas(i)
		case cells:
			l.lazy = true
			// A component may change before its cells are wanted, so what
			// it shows is taken now, as paint would, and parsed then.
			if _, text := b.child.(TextNode); !text {
				l.view = r.viewOf(i)
			}
		}
		switch c := b.child.(type) {
		case TextMap:
			l.tm = c
		case TextSource:
			l.source = c
		}
		if inner {
			l.tm, l.source = nil, nil
		}
		r.leaves = append(r.leaves, l)
		r.outline.AddLeaf(textsel.Leaf{Rect: l.rect, Clip: l.clip, Inner: inner, Open: l.tm != nil})
		return &r.leaves[len(r.leaves)-1]
	}
	switch n := b.child.(type) {
	case TextNode:
		l := leaf(true)
		if n.styled || n.truncate {
			l.text, l.truncated = n.s, n.truncate
		}
	case BoxNode:
		kid(r.boxContent(i, n), inner)
	case ListNode:
		r.noteList(i, n, path, inner, kid)
	case SlotNode, FlexNode, GridNode, WrapNode, ScrollNode, OverlayNode, LayerNode, CenterNode, TransitionNode:
		for _, k := range r.kidsOf(i) {
			kid(k, inner)
		}
	case ViewportNode:
		if l := r.viewportOf(i); l != nil && !inner {
			key := nodeKey{path: path}
			if b.meta.id != nil {
				key = nodeKey{id: b.meta.id}
			}
			rect, shown := viewportLeaf(l, vis)
			leaf := selLeaf{key: key, box: i, rect: rect, clip: shown, vp: l}
			if l.view.buf != nil {
				leaf.rev = l.view.buf.Rev()
			}
			r.leaves = append(r.leaves, leaf)
			r.outline.AddLeaf(textsel.Leaf{Rect: rect, Clip: shown})
		}
	case DrawableNode, ScrollbarNode, TooltipSinkNode:
	case Layouter:
		_, src := b.child.(TextSource)
		if src && !inner {
			leaf(false)
		}
		kid(b.expanded, inner || src)
	default:
		leaf(true)
	}
}

// noteList notes list n, box i at path, and the leaves of the items it
// built, with kid, between its marks. Inside the layout of a [TextSource],
// whose cells are not text of their own, it notes the leaves alone.
func (r *renderer) noteList(i int, n ListNode, path uint64, inner bool, kid func(k int, inner bool)) {
	if inner {
		for _, k := range r.kidsOf(i) {
			kid(k, true)
		}
		return
	}
	b := &r.boxes[i]
	l := textsel.List{Path: path, Box: i, Rect: b.rect, Count: n.count, First: b.list.First}
	l.Last = l.First - 1
	kids := r.kidsOf(i)
	if len(kids) > 0 {
		// Each item is built with its index as its slot.
		l.First, l.Last = r.boxes[kids[0]].slot, r.boxes[kids[len(kids)-1]].slot
	}
	k := r.outline.OpenList(l)
	for _, kd := range kids {
		r.outline.InItem(k, r.boxes[kd].slot)
		kid(kd, false)
	}
	r.outline.CloseList(k)
}

// leafCanvas returns the cells of box i, a text node or a component, as
// paint draws them.
func (r *renderer) leafCanvas(i int) *canvas.Canvas {
	b := &r.boxes[i]
	n, ok := b.child.(TextNode)
	if !ok {
		return r.cache.Get(r.viewOf(i))
	}
	if n.hover != nil && r.isHovered(b) {
		n.style, n.styled = *n.hover, true
	}
	text, _ := r.textAt(i, n, b.rect.Dx(), b.rect.Dy())
	if n.trusted {
		return r.cache.Get(text)
	}
	return r.cache.Sanitized(text)
}

// cells returns the cells of leaf l, parsing those of a leaf out of sight
// the first time they are wanted, rather than every frame.
func (r *renderer) cells(l *selLeaf) *canvas.Canvas {
	if l.lazy {
		if _, text := r.boxes[l.box].child.(TextNode); text {
			l.src = r.leafCanvas(l.box)
		} else {
			l.src = r.cache.Get(l.view)
		}
		l.lazy = false
	}
	return l.src
}

// findLeaf returns the index of the leaf with key k, or -1.
func (r *renderer) findLeaf(k nodeKey) int {
	for i := range r.leaves {
		if !r.leaves[i].inner && r.leaves[i].key == k {
			return i
		}
	}
	return -1
}

// leafAt returns the cell of a leaf that the pointer at p selects to: the
// one under it, or else the end of the nearest before it in reading order,
// preferring one above or below it, or else the start of the first.
func (r *renderer) leafAt(p image.Point) selPos {
	i, at := r.outline.LeafAt(p)
	if i < 0 {
		return selPos{}
	}
	return r.posIn(i, at)
}

// posIn returns the cell of leaf i nearest at: its start above it, its end
// below it, and the nearest of the row otherwise. In a leaf of text, it is
// the text there, as the selection's unit takes it.
func (r *renderer) posIn(i int, at image.Point) selPos {
	l := &r.leaves[i]
	var e selPos
	if l.isText() {
		e = r.textEnd(l, at, r.sel.unit, false)
	} else {
		w, h := l.rect.Dx(), l.rect.Dy()
		switch {
		case at.Y < 0:
			at = image.Point{}
		case at.Y >= h:
			at = image.Pt(w-1, h-1)
		default:
			at.X = min(max(at.X, 0), w-1)
		}
		e = selPos{leaf: l.key, at: at, ok: true}
	}
	r.outline.PlaceOf(i, &e.Place)
	return e
}

// stopOf returns where end e is in the leaves noted, from level of its
// items on, as [textsel.Outline.StopOf] finds it.
func (r *renderer) stopOf(e *selPos, level int) textsel.Stop {
	leaf := -1
	if e.Edge == 0 {
		leaf = r.findLeaf(e.leaf)
	}
	return r.outline.StopOf(&e.Place, leaf, level)
}

// before reports whether end p, at stop a, comes before end q, at stop b,
// and whether that can be told, as [textsel.Outline.Before] tells it, and
// for two ends in the same leaf, by where they are in it.
func (r *renderer) before(p *selPos, a textsel.Stop, q *selPos, b textsel.Stop) (before, known bool) {
	if a.Kind == textsel.StopLeaf && b.Kind == textsel.StopLeaf && a.I == b.I {
		return r.endBefore(a.I, p, q), true
	}
	return r.outline.Before(&p.Place, a, &q.Place, b)
}

// clampAt returns at inside leaf i.
func (r *renderer) clampAt(i int, at image.Point) image.Point {
	l := &r.leaves[i]
	return image.Pt(min(max(at.X, 0), l.rect.Dx()-1), min(max(at.Y, 0), l.rect.Dy()-1))
}

// resolveSel finds the selection in the leaves noted for this frame, from
// where the pointer pressed and is now, and widens it to the words or
// lines it takes whole. The text of a leaf of text was taken whole as its
// ends were found; positions in a viewport follow the edits to its buffer,
// and when they cannot, the selection is lost. An end in an item of a list
// that the list did not build is at the list's mark before or after the
// items it built, so that what is in sight between the ends is drawn, and
// the text of the items it did not build is read there.
func (r *renderer) resolveSel() (selRange, bool) {
	s := &r.sel
	if len(r.leaves) == 0 {
		return selRange{}, false
	}
	if !s.anchor.ok {
		s.anchor = r.leafAt(s.press)
	}
	if s.dragging || !s.head.ok {
		s.head = r.leafAt(s.pointer)
	}
	a, b := r.stopOf(&s.anchor, 0), r.stopOf(&s.head, 0)
	if a.Kind == textsel.StopLost || b.Kind == textsel.StopLost {
		r.selLost = true
		return selRange{}, false
	}
	if a.Kind == textsel.StopNone || b.Kind == textsel.StopNone {
		return selRange{}, false
	}
	if a.Kind == textsel.StopLeaf && !follow(&r.leaves[a.I], &s.anchor) || b.Kind == textsel.StopLeaf && !follow(&r.leaves[b.I], &s.head) {
		r.selLost = true
		return selRange{}, false
	}
	if s.block {
		// A rectangle is of cells in sight.
		if a.Kind != textsel.StopLeaf || b.Kind != textsel.StopLeaf {
			return selRange{}, false
		}
		// A rectangle of cells is more than a TextMap tells of.
		if r.leaves[a.I].tm == nil && r.leaves[b.I].tm == nil {
			g := selRange{a: a.I, b: b.I, block: true, show: s.active}
			g.pa, g.pb = r.corner(a.I, &s.anchor), r.corner(b.I, &s.head)
			return g, true
		}
	}
	ea, eb := selEnd{pos: s.anchor, stop: a}, selEnd{pos: s.head, stop: b}
	if first, known := r.before(&s.head, b, &s.anchor, a); known {
		s.headFirst = first
	}
	if s.headFirst {
		ea, eb = eb, ea
	}
	g := r.rangeOf(&ea, &eb, s.unit)
	g.show = g.show && s.active
	r.ends = [2]selEnd{ea, eb}
	return g, true
}

// rangeOf returns the selection from end ea to end eb, which does not come
// before it, as laid out in the leaves noted: from the first leaf at or
// after ea to the last at or before eb, which take from where an end is in
// them, as unit takes text there, and are otherwise taken whole. When no
// leaf is between them, its a is past its b.
func (r *renderer) rangeOf(ea, eb *selEnd, unit textsel.Unit) selRange {
	g := selRange{a: r.outline.FirstAfter(ea.stop), b: r.outline.LastBefore(eb.stop), show: true}
	if g.a > g.b {
		return g
	}
	atA, atB := ea.stop.Kind == textsel.StopLeaf, eb.stop.Kind == textsel.StopLeaf
	la, lb := &r.leaves[g.a], &r.leaves[g.b]
	if atA {
		if la.isText() {
			g.qa = ea.pos.from
		} else {
			g.pa = r.clampAt(g.a, ea.pos.at)
		}
	}
	switch {
	case !atB && lb.isText():
		g.qb = textEndPos(lb)
	case !atB:
		g.pb = image.Pt(lb.rect.Dx()-1, lb.rect.Dy()-1)
	case lb.isText():
		g.qb = eb.pos.to
		if atA && g.a == g.b && eb.pos.to.Before(ea.pos.to) {
			g.qb = ea.pos.to
		}
	default:
		g.pb = r.clampAt(g.b, eb.pos.at)
	}
	switch unit {
	case textsel.UnitLine:
		if atA && !la.isText() {
			g.pa.X = 0
		}
		if atB && !lb.isText() {
			g.pb.X = lb.rect.Dx() - 1
		}
	case textsel.UnitWord:
		// An end in a leaf of text took its word as it was found.
		okA := false
		if atA && !la.isText() {
			var x0 int
			if x0, _, okA = r.wordAt(g.a, g.pa); okA {
				g.pa.X = x0
			}
		}
		if atB && !lb.isText() {
			if _, x1, ok := r.wordAt(g.b, g.pb); ok {
				g.pb.X = x1
			}
		}
		if atA && atB && g.a == g.b && !la.isText() && g.pa == g.pb && !okA {
			// A double click on a blank selects nothing.
			g.show = false
		}
	case textsel.UnitCell:
	}
	if atA && atB && la.isText() && g.a == g.b && !g.qa.Before(g.qb) {
		// Nothing of the text is taken, such as past the end of a row.
		g.show = false
	}
	return g
}

// endBefore reports whether end p of a selection in leaf i comes before
// end q in it.
func (r *renderer) endBefore(i int, p, q *selPos) bool {
	if r.leaves[i].isText() {
		return p.from.Before(q.from)
	}
	pa, pb := r.clampAt(i, q.at), r.clampAt(i, p.at)
	return pb.Y < pa.Y || pb.Y == pa.Y && pb.X < pa.X
}

// corner returns the cell of leaf i that end e of a block selection is at,
// as laid out: in a viewport, the column of the row that e's row starts in
// is wrapped to now.
func (r *renderer) corner(i int, e *selPos) image.Point {
	l := &r.leaves[i]
	if l.vp == nil {
		return r.clampAt(i, e.at)
	}
	row := 0
	if v := &l.vp.view; v.buf != nil {
		row, _ = v.index().CellOf(linebuf.Pos(e.corner), false, &r.vpScratch)
	}
	return image.Pt(max(e.col, 0), row)
}

// leafCell returns the cell at x, y of leaf i, or nil where it has none.
func (r *renderer) leafCell(i, x, y int) *uv.Cell { return r.leafCellOf(&r.leaves[i], x, y) }

// leafCellOf returns the cell at x, y of leaf l, or nil where it has none.
func (r *renderer) leafCellOf(l *selLeaf, x, y int) *uv.Cell {
	if l.vp != nil {
		return r.cache.Get(r.vpRowCells(l.vp, y)).CellAt(x, 0)
	}
	if src := r.cells(l); src != nil {
		return src.CellAt(x, y)
	}
	return r.cellAt(l.rect.Min.Add(image.Pt(x, y)))
}

// cellAt returns the cell of the leaves at p, as laid out, or nil.
func (r *renderer) cellAt(p image.Point) *uv.Cell {
	for i := len(r.leaves) - 1; i >= 0; i-- {
		if l := &r.leaves[i]; p.In(l.rect) {
			if l.vp != nil {
				return r.leafCell(i, p.X-l.rect.Min.X, p.Y-l.rect.Min.Y)
			}
			if src := r.cells(l); src != nil {
				return src.CellAt(p.X-l.rect.Min.X, p.Y-l.rect.Min.Y)
			}
		}
	}
	return nil
}

// wordAt returns the columns of the word at p in leaf i, inclusive, and
// whether there is one there.
func (r *renderer) wordAt(i int, p image.Point) (x0, x1 int, ok bool) {
	return r.wordIn(&r.leaves[i], p)
}

// paintSelection finds the selection in the leaves of box i, the selected
// node, clipped to clip, and adds the op that draws it over them.
func (r *renderer) paintSelection(i int, clip image.Rectangle) {
	if !r.parse {
		return
	}
	r.noteLeaves(i, clip, 0)
	g, ok := r.settle()
	if !ok {
		return
	}
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	if b.hidden || vis.Empty() || !g.show || g.a > g.b {
		// Nothing selected is in sight, such as when both ends are in items
		// of a list scrolled away on the same side.
		return
	}
	// The cells its positions are drawn at may change with the text of a
	// viewport though nothing drawn of it does, as when a tab becomes spaces.
	var rev uint64
	for k := g.a; k <= g.b; k++ {
		rev += r.leaves[k].rev
	}
	r.ops = append(r.ops, op{kind: opSelect, rect: b.rect, clip: vis, sel: g, t: r.flashT, rev: rev})
}

// settle finds the selection in the leaves noted as the range that is drawn
// and copied for it, until the next frame.
func (r *renderer) settle() (selRange, bool) {
	r.selMemo.Reset()
	g, ok := r.resolveSel()
	r.rng, r.rngSeq = g, -1
	if ok {
		r.rngSeq = r.sel.seq
	}
	return g, ok
}

// selectAll selects as s all the text of its node, selectable and shown at
// vis in the last frame, and reports whether it has any. It runs from the
// start of the node's text to its end, rather than from its first leaf to
// its last, so that it takes every item of a list in it, and costs no more
// for a list of many items: their text is read only when it is wanted.
func (r *renderer) selectAll(s *selection, vis image.Rectangle) bool {
	r.rngSeq = -1
	box := -1
	for k := range r.boxes {
		// A box never laid out, such as one built for a side its layer did not go
		// on, is not the node on screen.
		if b := &r.boxes[k]; b.meta.selectable && !b.hidden && !b.rect.Empty() && b.key() == s.key {
			box = k
			break
		}
	}
	if box < 0 {
		return false
	}
	r.noteLeaves(box, vis, 0)
	if !slices.ContainsFunc(r.leaves, func(l selLeaf) bool { return !l.inner }) {
		return false
	}
	s.anchor, s.head = selPos{Edge: -1, ok: true}, selPos{Edge: 1, ok: true}
	r.sel = *s
	_, ok := r.settle()
	return ok
}

// executeSelect draws the selection over the cells of t it covers.
func (r *renderer) executeSelect(o *op, t target) {
	if o.t < 0 {
		r.eachSelected(o, func(p image.Point) {
			if c := t.c.CellAt(p.X-t.origin.X, p.Y-t.origin.Y); c != nil {
				r.selPaint.Highlight(c, &r.env, &r.mixer)
			}
		})
		return
	}
	// The flash crosses the cells copied, so it first finds their bounds.
	var area image.Rectangle
	r.eachSelected(o, func(p image.Point) {
		area = area.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
	})
	r.eachSelected(o, func(p image.Point) {
		if c := t.c.CellAt(p.X-t.origin.X, p.Y-t.origin.Y); c != nil {
			r.selPaint.Flash(c, p, o.t, area, &r.env, &r.mixer)
		}
	})
}

// eachSelected calls fn with each cell on screen that the selection of op
// o covers.
func (r *renderer) eachSelected(o *op, fn func(p image.Point)) {
	// Only what is in sight is visited, however much is selected.
	g := &o.sel
	if o.held > 0 {
		l := &r.helds[o.held-1].leaf
		r.textCells(l, g.qa, g.qb, l.clip.Intersect(o.clip), fn)
		return
	}
	if g.block {
		area := r.blockRect(g).Intersect(o.clip)
		for i := range r.leaves {
			if l := &r.leaves[i]; !l.inner {
				in := l.clip.Intersect(area)
				for y := in.Min.Y; y < in.Max.Y; y++ {
					for x := in.Min.X; x < in.Max.X; x++ {
						fn(image.Pt(x, y))
					}
				}
			}
		}
		return
	}
	for i := g.a; i <= g.b; i++ {
		l := &r.leaves[i]
		in := l.clip.Intersect(o.clip)
		if l.inner || in.Empty() {
			continue
		}
		if l.isText() {
			from, to := r.textSpan(g, i)
			r.textCells(l, from, to, in, fn)
			continue
		}
		in = in.Sub(l.rect.Min)
		from, to := r.leafSpan(g, i)
		for y := max(from.Y, in.Min.Y); y <= min(to.Y, in.Max.Y-1); y++ {
			x0, x1 := textsel.RowSpan(from, to, y, l.rect.Dx())
			for x := max(x0, in.Min.X); x <= min(x1, in.Max.X-1); x++ {
				fn(l.rect.Min.Add(image.Pt(x, y)))
			}
		}
	}
}

// leafSpan returns the first and last cells of leaf i that g selects.
func (r *renderer) leafSpan(g *selRange, i int) (from, to image.Point) {
	l := &r.leaves[i]
	to = image.Pt(l.rect.Dx()-1, l.rect.Dy()-1)
	if i == g.a {
		from = g.pa
	}
	if i == g.b {
		to = g.pb
	}
	return from, to
}

// textSpan returns where the text of leaf i, a leaf of text, that g selects
// starts and ends.
func (r *renderer) textSpan(g *selRange, i int) (from, to Pos) {
	l := &r.leaves[i]
	to = textEndPos(l)
	if i == g.a {
		from = g.qa
	}
	if i == g.b {
		to = g.qb
	}
	return from, to
}

// blockRect returns the rectangle of a block selection, as laid out.
func (r *renderer) blockRect(g *selRange) image.Rectangle {
	return textsel.Corners(r.leaves[g.a].rect.Min.Add(g.pa), r.leaves[g.b].rect.Min.Add(g.pb))
}

// selectedText returns the text of the selection found in the last frame,
// and whether it is still there: the positions in a viewport are followed
// through the edits made to its buffer since, which may have lost them.
// The items of lists that the frame did not build are built now, apart from
// it, for their text.
//
// The text is kept, for reading it again, until the next frame or the
// selection is found again, while the buffers it was read from are not
// edited.
func (r *renderer) selectedText() (string, bool) {
	m, key := &r.selMemo, textKey{seq: r.rngSeq}
	if text, ok := m.Get(key); ok {
		return text, true
	}
	g := &r.rng
	t := selText{unit: r.sel.unit, revs: m.Read()}
	var text string
	if g.block {
		var ok bool
		if text, ok = r.blockText(&t, g); !ok {
			m.Done(key, "", t.revs, false)
			return "", false
		}
	} else {
		ea, eb := r.ends[0], r.ends[1]
		// The edits since the frame may have left nothing selected.
		now, ok := r.rangeNow(&ea, &eb)
		ok = ok && now.show && r.writeSel(&t, &now, &ea, &eb)
		r.aside = nil
		if !ok {
			m.Done(key, "", t.revs, false)
			return "", false
		}
		text = t.String()
	}
	m.Done(key, text, t.revs, !t.live)
	return text, true
}

// textKey is what the text of a selection is kept by: the selection, and
// for one kept outside the Root, its range.
type textKey struct {
	seq int
	rg  Range
}

// selText is the text of a selection being read, leaf by leaf, as unit
// takes it, and what it was read from.
type selText struct {
	textsel.Joiner

	unit textsel.Unit
	// revs are the buffers of the viewports read, and live is set once a
	// [TextMap] or [TextSource] is, whose text may change without the Root
	// knowing.
	revs []textsel.StoreRev
	live bool
}

// from notes that the text of leaf l is read.
func (t *selText) from(l *selLeaf) {
	switch {
	case l.vp != nil:
		if buf := l.vp.view.buf; buf != nil {
			t.revs = textsel.NoteStore(t.revs, &buf.s)
		}
	case l.tm != nil, l.source != nil:
		t.live = true
	}
}

// writeSel adds to t the text of the selection from end ea to end eb, in
// order, whose range in the leaves noted is g: of the leaves between them,
// and, at the marks between them, of the items selected that the lists did
// not build. It reports false when the text cannot be read: when an end is
// not in the item built to find it in.
func (r *renderer) writeSel(t *selText, g *selRange, ea, eb *selEnd) bool {
	return r.outline.Walk(ea.stop, eb.stop,
		func(i int) { r.writeLeaf(t, g, i) },
		func(m int) bool { return r.writeItems(t, m, ea, eb) })
}

// writeLeaf adds to t the text of leaf i that g selects.
func (r *renderer) writeLeaf(t *selText, g *selRange, i int) {
	l := &r.leaves[i]
	if l.inner {
		return
	}
	t.from(l)
	if l.isText() {
		from, to := r.textSpan(g, i)
		t.Add(leafTextOf(l, from, to), l.rect.Min.Y, l.rect.Max.Y-1)
		return
	}
	from, to := r.leafSpan(g, i)
	t.Add(r.leafText(l, from, to), l.rect.Min.Y+from.Y, l.rect.Min.Y+to.Y)
}

// writeItems adds to t the text of the items selected at mark m of a list
// that it did not build: those before the first it built, or after the
// last, from the item ea is in when ea is at m, and up to the one eb is in
// when eb is.
func (r *renderer) writeItems(t *selText, m int, ea, eb *selEnd) bool {
	l := &r.outline.Lists[r.outline.Marks[m].List]
	from, to, in, out := r.outline.Unbuilt(m, ea.stop, &ea.pos.Place, eb.stop, &eb.pos.Place)
	n, _ := r.boxes[l.Box].child.(ListNode)
	for j := from; j <= to; j++ {
		var a, b *selEnd
		if in && j == from {
			a = ea
		}
		if out && j == to {
			b = eb
		}
		if a == nil && b == nil && n.itemText != nil {
			t.Apart = true
			t.Add(itemApart(n.count, j, n.itemText), 0, 0)
			t.Apart = true
			continue
		}
		if !r.writeItem(t, l, n, j, a, b) {
			return false
		}
	}
	return true
}

// writeItem adds to t the text of item j of list n, noted as l, which it
// did not build, built now as the list would build it: all of it, or from
// end ea, and up to end eb, of those given. Each item is on rows of its own.
func (r *renderer) writeItem(t *selText, l *textsel.List, n ListNode, j int, ea, eb *selEnd) bool {
	c := itemApart(n.count, j, n.item)
	if c == nil || absent(c) {
		return true
	}
	a := r.asideOf()
	root := a.buildAside(l.Rect.Dx(), c)
	defer a.cache.EndFrame(a.area.Dx() * a.area.Dy())
	a.noteLeaves(root, image.Rectangle{}, childPath(l.Path, j))
	start, finish := selEnd{stop: textsel.Stop{Kind: textsel.StopBegin}}, selEnd{stop: textsel.Stop{Kind: textsel.StopEnd}}
	if ea != nil && !a.endIn(ea, &start) || eb != nil && !a.endIn(eb, &finish) {
		return false
	}
	if ea != nil && eb != nil {
		if first, known := a.before(&finish.pos, finish.stop, &start.pos, start.stop); known && first {
			start, finish = finish, start
		}
	}
	g := a.rangeOf(&start, &finish, t.unit)
	t.Apart = true
	ok := a.writeSel(t, &g, &start, &finish)
	t.Apart = true
	return ok
}

// endIn finds end e, which is in an item that r built apart from the
// frame, in the leaves r noted of it, as at, and reports whether it is
// there.
func (r *renderer) endIn(e, at *selEnd) bool {
	at.pos = e.pos
	at.stop = r.stopOf(&at.pos, e.stop.Level+1)
	switch at.stop.Kind {
	case textsel.StopLeaf:
		return follow(&r.leaves[at.stop.I], &at.pos)
	case textsel.StopMark:
		return true
	case textsel.StopNone, textsel.StopBegin, textsel.StopEnd, textsel.StopLost:
	}
	return false
}

// asideOf returns the renderer that builds the items of lists that r did
// not build, made the first time it is wanted.
func (r *renderer) asideOf() *renderer {
	if r.aside == nil {
		a := newRenderer(r.method)
		a.screen, a.words.Chars, a.apart = r.screen, r.words.Chars, true
		r.aside = a
	}
	return r.aside
}

// buildAside lays out c, an item of a list w cells wide, as the list lays
// out the items it builds, as wide as the list and as tall as it wants, in
// place of what r built before. It is for reading the text of an item that
// a frame did not build, apart from the frame: r is not the frame's
// renderer, keeps no state for the frame, and animates nothing, and what it
// builds is not drawn.
func (r *renderer) buildAside(w int, c Child) int {
	clear(r.boxes)
	clear(r.memo)
	r.boxes, r.kids = r.boxes[:0], r.kids[:0]
	r.vps, r.vpRows = r.vps[:0], r.vpRows[:0]
	defer r.frameMemos.Retire()
	defer r.textMemos.Retire()
	defer r.styled.Retire()
	root := r.build(c, -1, 0)
	r.area = image.Rect(0, 0, w, r.measure(root, Size{W: w, H: unbounded}).H)
	r.arrange(root, r.area)
	return root
}

// rangeNow returns the range of the selection from end ea to end eb as it
// is now, rather than as the last frame found it: an end in a viewport is
// followed through the edits made to its buffer since, and the range found
// again from the ends, as the next frame would find it, so that the text
// read before that frame is the text read after it. It reports false when
// an end could not be followed.
func (r *renderer) rangeNow(ea, eb *selEnd) (selRange, bool) {
	for _, e := range [2]*selEnd{ea, eb} {
		if e.stop.Kind == textsel.StopLeaf && !follow(&r.leaves[e.stop.I], &e.pos) {
			return selRange{}, false
		}
	}
	return r.rangeOf(ea, eb, r.sel.unit), true
}

// leafText returns the text of leaf l, a leaf of cells, from from to to, in
// reading order.
func (r *renderer) leafText(l *selLeaf, from, to image.Point) string {
	if l.source != nil {
		return l.source.SelectedText(Span{Start: from, End: to, Size: Size{W: l.rect.Dx(), H: l.rect.Dy()}})
	}
	src := r.cells(l)
	if src == nil {
		return ""
	}
	if l.truncated && from.Y == to.Y {
		if s, ok := textsel.CutText(src, lineBreaks.Replace(ansi.Strip(l.text)), from.Y, from.X, to.X); ok {
			return s
		}
	}
	var joins []textsel.Join
	if l.text != "" && !l.truncated && to.Y > from.Y {
		joins = textsel.WrapJoins(src, l.text)
	}
	return textsel.SpanText(src.CellAt, l.rect.Dx(), from, to, joins)
}

// blockText returns the text of block selection g, a line to a row, noting
// in t what it is read from, and whether it is still there: the corners of
// a rectangle in a viewport are followed through the edits made to its
// buffer since the frame, which may have lost them.
func (r *renderer) blockText(t *selText, g *selRange) (string, bool) {
	a, b := r.sel.anchor, r.sel.head
	if !follow(&r.leaves[g.a], &a) || !follow(&r.leaves[g.b], &b) {
		return "", false
	}
	area := textsel.Corners(r.leaves[g.a].rect.Min.Add(r.corner(g.a, &a)), r.leaves[g.b].rect.Min.Add(r.corner(g.b, &b)))
	la, lb := &r.leaves[g.a], &r.leaves[g.b]
	t.from(la)
	t.from(lb)
	switch {
	case g.a == g.b && la.vp != nil:
		// A viewport's own rows are read as they are now.
		return vpBlockText(la.vp, area.Min.Sub(la.rect.Min), area.Max.Sub(la.rect.Min).Sub(image.Pt(1, 1))), true
	case g.a == g.b && la.source != nil:
		return la.source.SelectedText(Span{
			Start: area.Min.Sub(la.rect.Min), End: area.Max.Sub(la.rect.Min).Sub(image.Pt(1, 1)), Block: true,
			Size: Size{W: la.rect.Dx(), H: la.rect.Dy()},
		}), true
	}
	// The rest is read from cells as the last frame laid them out, which an
	// edit since to a viewport the rectangle has a corner in, or runs over,
	// has left out of date.
	edited := func(l *selLeaf) bool { return l.vp != nil && l.vp.view.buf != nil && l.vp.view.buf.Rev() != l.rev }
	if edited(la) || edited(lb) {
		return "", false
	}
	for i := range r.leaves {
		if l := &r.leaves[i]; !l.inner && l.rect.Overlaps(area) {
			t.from(l)
			if edited(l) {
				return "", false
			}
		}
	}
	return textsel.RectText(func(x, y int) *uv.Cell { return r.cellAt(image.Pt(x, y)) }, area), true
}

// heldLeaf is a selection whose range is kept outside the Root, such as by
// an [Editable]: read each frame with get, and set from the mouse with set,
// whose command the Root returns. It is drawn over its leaf, the text it is
// a selection of, which is where the mouse finds positions in it.
type heldLeaf struct {
	leaf selLeaf
	get  func() Range
	set  func(Range) tea.Cmd
	// last is the range as the Root last saw it, for a range made outside
	// the Root to become the selection when it sees it change.
	last Range
}

// paintHeld notes box i, an [Editable] drawn at vis, which keeps its own
// selection, and adds the op that draws its range over it.
func (r *renderer) paintHeld(i int, ed Editable, vis image.Rectangle) {
	b := &r.boxes[i]
	r.addHeld(heldLeaf{
		leaf: selLeaf{key: b.key(), box: i, rect: b.rect, clip: vis, tm: ed, lazy: true, view: r.viewOf(i)},
		get:  ed.Selection,
		set:  func(rg Range) tea.Cmd { ed.Select(rg); return nil },
	}, b.rect, vis)
}

// addHeld notes h, the selection kept outside the Root of a node laid out
// at rect and drawn at vis, and adds the op that draws its range over it,
// as the copy flash while that runs over it, when the range holds text.
func (r *renderer) addHeld(h heldLeaf, rect, vis image.Rectangle) {
	for k := range r.prevHelds {
		if r.prevHelds[k].leaf.key == h.leaf.key {
			h.last = r.prevHelds[k].last
			break
		}
	}
	r.helds = append(r.helds, h)
	rg := h.get()
	if rg.Empty() {
		return
	}
	t := -1.0
	if r.heldFlashT >= 0 && r.heldFlash == h.leaf.key {
		t = r.heldFlashT
	}
	var buf *Buffer
	if h.leaf.vp != nil {
		buf = h.leaf.vp.view.buf
	}
	r.ops = append(r.ops, op{
		kind: opSelect, rect: rect, clip: vis, held: len(r.helds), t: t, rev: h.leaf.rev, buf: buf,
		sel: selRange{qa: rg.From(), qb: rg.To(), show: true},
	})
}

// isText reports whether leaf l keeps its selection as positions in its
// text, rather than as cells: a viewport, or a [TextMap].
func (l *selLeaf) isText() bool { return l.vp != nil || l.tm != nil }

func (l *selLeaf) size() Size { return Size{W: l.rect.Dx(), H: l.rect.Dy()} }

// textPosAt returns the position in the text of leaf l, a leaf of text, of
// the character at cell at, relative to its top-left.
func (r *renderer) textPosAt(l *selLeaf, at image.Point) Pos {
	if l.vp != nil {
		if v := &l.vp.view; v.buf != nil {
			return Pos(v.index().PosAt(at.Y, at.X, &r.vpScratch))
		}
		return Pos{}
	}
	return l.tm.PosAt(at, l.size())
}

// textAfter returns the position after the character at cell at of leaf l:
// the next that the cells to its right are at, which steps over a wide
// character, or its own for a cell past the text of its row.
func (r *renderer) textAfter(l *selLeaf, at image.Point) Pos {
	if l.vp != nil {
		if v := &l.vp.view; v.buf != nil {
			return Pos(v.index().After(at.Y, at.X, &r.vpScratch))
		}
		return Pos{}
	}
	size := l.size()
	p := l.tm.PosAt(at, size)
	for x := at.X + 1; x <= size.W; x++ {
		if q := l.tm.PosAt(image.Pt(x, at.Y), size); q != p {
			return q
		}
	}
	return p
}

// textLine returns the start and end of line of the text of leaf l.
func textLine(l *selLeaf, line int) (from, to Pos) {
	if l.vp != nil && l.vp.view.buf != nil {
		buf := l.vp.view.buf
		return buf.Clamp(Pos{Line: line}), buf.Clamp(Pos{Line: line, Col: math.MaxInt})
	}
	return Pos{Line: line}, Pos{Line: line, Col: math.MaxInt}
}

// textEnd returns where leaf l, a leaf of text, has an end of a selection
// at cell at, relative to its top-left, as unit takes text there: the
// character, or with caret, the place before it, or the word or line. A
// cell above the leaf is at its start, one below it at its end, and one
// past either side of a row at that end of the row.
func (r *renderer) textEnd(l *selLeaf, at image.Point, unit textsel.Unit, caret bool) selPos {
	w, h := l.rect.Dx(), l.rect.Dy()
	switch {
	case at.Y < 0:
		at = image.Point{}
	case at.Y >= h:
		at = image.Pt(w, h-1)
	default:
		at.X = min(max(at.X, 0), w)
	}
	p := r.textPosAt(l, at)
	e := selPos{leaf: l.key, at: image.Pt(min(at.X, w-1), at.Y), from: p, to: p, ok: true}
	switch unit {
	case textsel.UnitWord:
		if l.vp != nil {
			if from, to, ok := r.vpWord(l.vp, at); ok {
				e.from, e.to = from, to
			}
		} else if x0, x1, ok := r.wordIn(l, at); ok {
			e.from, e.to = r.textPosAt(l, image.Pt(x0, at.Y)), r.textAfter(l, image.Pt(x1, at.Y))
		}
	case textsel.UnitLine:
		e.from, e.to = textLine(l, p.Line)
	case textsel.UnitCell:
		if !caret {
			e.to = r.textAfter(l, at)
		}
	}
	if l.vp != nil {
		// A corner of a block is kept as the row it is on, by where the row
		// starts, and its column.
		e.corner, e.col = r.textPosAt(l, image.Pt(0, at.Y)), at.X
		e.buf = l.vp.view.buf
		if e.buf != nil {
			e.rev = e.buf.Rev()
		}
	}
	return e
}

// textEndPos returns the end of the text of leaf l: that of a viewport's
// buffer, or for a [TextMap], a position past any, which it clamps.
func textEndPos(l *selLeaf) Pos {
	if l.vp != nil {
		if l.vp.view.buf == nil {
			return Pos{}
		}
		return l.vp.view.buf.End()
	}
	return Pos{Line: math.MaxInt, Col: math.MaxInt}
}

// follow moves the positions of e, an end of a selection in leaf l, through
// the changes made to the text of l since they were found, and reports
// whether they could be: not when the text was set again, or the leaf shows
// another buffer.
func follow(l *selLeaf, e *selPos) bool {
	if l.vp == nil {
		return true
	}
	buf := l.vp.view.buf
	if buf != e.buf {
		return false
	}
	if buf == nil || buf.Rev() == e.rev {
		return true
	}
	cs, ok := buf.changesSince(e.rev)
	if !ok {
		return false
	}
	for _, c := range cs {
		from, to := c.MapSpan(linebuf.Pos(e.from), linebuf.Pos(e.to))
		e.from, e.to, e.corner = Pos(from), Pos(to), Pos(c.Map(linebuf.Pos(e.corner), false))
	}
	e.rev = buf.Rev()
	return true
}

// heldRange returns the range an [Editable] is given for a selection from
// end a to end b: from where the caret goes at a to where it goes at b,
// taking in all of what a and b take whole, such as words.
func heldRange(a, b selPos) Range {
	if b.from.Before(a.from) {
		return Range{Anchor: a.to, Head: b.from}
	}
	return Range{Anchor: a.from, Head: b.to}
}

// heldCaret returns the range h is given for a selection from end a to end
// b, as heldRange has it. In a viewport, its caret is drawn on the row the
// pointer at b is on: EndOfRow is set when the Head is the place its line
// wraps after that row. A [TextMap] tells positions only, so the range of
// an [Editable] has EndOfRow unset.
func (r *renderer) heldCaret(h *heldLeaf, a, b selPos) Range {
	rg := heldRange(a, b)
	if l := h.leaf.vp; l != nil && l.view.buf != nil {
		rg.EndOfRow = l.view.index().EndsRow(linebuf.Pos(rg.Head), b.at.Y, &r.vpScratch)
	}
	return rg
}

// wordIn returns the columns of the word at cell at of leaf l, inclusive,
// and whether there is one there.
func (r *renderer) wordIn(l *selLeaf, at image.Point) (x0, x1 int, ok bool) {
	return r.words.At(l.rect.Dx(), at.X, func(x int) *uv.Cell { return r.leafCellOf(l, x, at.Y) })
}

// textCells calls fn with each cell on screen inside clip that the text of
// leaf l from from up to to takes.
func (r *renderer) textCells(l *selLeaf, from, to Pos, clip image.Rectangle, fn func(p image.Point)) {
	if !from.Before(to) || clip.Empty() {
		return
	}
	if l.vp != nil {
		r.vpCells(l.vp, from, to, clip, fn)
		return
	}
	at := l.rect.Min
	l.tm.Cells(from, to, l.size(), func(y, x0, x1 int) {
		y += at.Y
		if y < clip.Min.Y || y >= clip.Max.Y {
			return
		}
		for x := max(at.X+x0, clip.Min.X); x < at.X+x1 && x < clip.Max.X; x++ {
			fn(image.Pt(x, y))
		}
	})
}

// leafTextOf returns the text of leaf l from from up to to.
func leafTextOf(l *selLeaf, from, to Pos) string {
	if l.vp != nil {
		if l.vp.view.buf == nil {
			return ""
		}
		return l.vp.view.buf.TextIn(from, to)
	}
	return l.tm.Text(from, to)
}

// heldAt returns the selection kept outside the Root, in the last frame,
// of the node with key k, or nil.
func (r *Root) heldAt(k nodeKey) *heldLeaf {
	for i := range r.r.helds {
		if h := &r.r.helds[i]; h.leaf.key == k {
			return h
		}
	}
	return nil
}

// setHeld sets the range of h from the mouse, noting it as seen, and keeps
// its command for the Root to return.
func (r *Root) setHeld(h *heldLeaf, rg Range) {
	if cmd := h.set(rg); cmd != nil {
		r.heldCmds = append(r.heldCmds, cmd)
	}
	h.last = h.get()
}

// takeHeldCmds returns the commands of the ranges set from the mouse since
// it was last called.
func (r *Root) takeHeldCmds() tea.Cmd {
	if len(r.heldCmds) == 0 {
		return nil
	}
	cmd := tea.Batch(r.heldCmds...)
	r.heldCmds = r.heldCmds[:0]
	return cmd
}

// collapseHeld collapses the ranges kept outside the Root to their carets,
// but for that of the node with key except, if keep.
func (r *Root) collapseHeld(except nodeKey, keep bool) {
	for i := range r.r.helds {
		h := &r.r.helds[i]
		if keep && h.leaf.key == except {
			continue
		}
		if rg := h.get(); !rg.Empty() {
			r.setHeld(h, rg.Collapse())
		}
	}
}

// checkHeld makes the selection a range kept outside the Root that changed
// to hold text since the Root last saw it, such as one made with the
// keyboard, clearing any other.
func (r *Root) checkHeld() {
	for i := range r.r.helds {
		h := &r.r.helds[i]
		rg := h.get()
		if rg == h.last {
			continue
		}
		h.last = rg
		if rg.Empty() || r.sel.ok && r.sel.held && r.sel.key == h.leaf.key {
			continue
		}
		key := h.leaf.key
		r.collapseHeld(key, true)
		r.selSeq++
		r.sel = selection{ok: true, key: key, seq: r.selSeq, held: true, active: true}
		if f := r.r.frame; f != nil {
			if k := f.find(key); k >= 0 {
				r.sel.id = f.nearestID(k)
			}
		}
		if r.flash.ok && !r.flash.held {
			r.flash = selection{}
		}
	}
}

// heldEnd returns the end of a selection in h that the pointer at p, on
// screen, is at, as unit takes text there.
func (r *Root) heldEnd(h *heldLeaf, p image.Point, unit textsel.Unit) selPos {
	return r.r.textEnd(&h.leaf, p.Sub(h.leaf.rect.Min), unit, true)
}

// startHeld starts selecting in h, kept outside the Root, of entry i of
// frame f, which the left button went down on at p.
func (r *Root) startHeld(f *Frame, i int, h *heldLeaf, p image.Point, unit textsel.Unit, mod tea.KeyMod) {
	key := h.leaf.key
	r.collapseHeld(key, true)
	end := r.heldEnd(h, p, unit)
	anchor := end
	extend := mod.Contains(tea.ModShift) && unit == textsel.UnitCell
	if extend {
		// The caret moves, and the Anchor stays.
		at := h.get().Anchor
		anchor = selPos{leaf: key, from: at, to: at, ok: true}
	}
	r.selSeq++
	r.sel = selection{
		ok: true, key: key, id: f.nearestID(i), seq: r.selSeq, press: p, pointer: p,
		unit: unit, held: true, anchor: anchor, head: end, active: unit != textsel.UnitCell || extend, dragging: true,
	}
	r.setHeld(h, r.r.heldCaret(h, anchor, end))
}

// dragHeld moves the Head of the range kept outside the Root that is being
// dragged, if any, to under the pointer, in the last frame.
func (r *Root) dragHeld() {
	s := &r.sel
	if !s.ok || !s.held || !s.dragging {
		return
	}
	h := r.heldAt(s.key)
	if h == nil {
		return
	}
	s.head = r.heldEnd(h, s.pointer, s.unit)
	if rg := r.r.heldCaret(h, s.anchor, s.head); rg != h.get() {
		r.setHeld(h, rg)
	}
}

// heldSelection returns the text of the selection kept outside the Root
// that is the selection, and whether it holds any.
func (r *Root) heldSelection() (string, bool) {
	h := r.heldAt(r.sel.key)
	if h == nil {
		return "", false
	}
	rg := h.get()
	if rg.Empty() {
		return "", false
	}
	m, key := &r.r.selMemo, textKey{seq: r.sel.seq, rg: rg}
	if text, ok := m.Get(key); ok {
		return text, true
	}
	t := selText{revs: m.Read()}
	t.from(&h.leaf)
	text := leafTextOf(&h.leaf, rg.From(), rg.To())
	m.Done(key, text, t.revs, !t.live)
	return text, true
}

// heldIn returns the selection kept outside the Root, in the last frame, of
// entry i of frame f, or of a node inside it when the entry is not
// selectable itself, or nil.
func (r *Root) heldIn(f *Frame, i int) *heldLeaf {
	if h := r.heldAt(f.entries[i].key); h != nil || f.entries[i].selectable {
		return h
	}
	for k := range r.r.helds {
		h := &r.r.helds[k]
		for j := f.find(h.leaf.key); j >= 0; j = f.entries[j].parent {
			if j == i {
				return h
			}
		}
	}
	return nil
}

// selectAllHeld selects all of the text of h, kept outside the Root, of
// entry i of frame f, and reports whether it has any.
func (r *Root) selectAllHeld(f *Frame, i int, h *heldLeaf) bool {
	key := h.leaf.key
	r.collapseHeld(key, true)
	r.flash = selection{}
	r.selSeq++
	r.sel = selection{ok: true, key: key, id: f.nearestID(i), seq: r.selSeq, held: true, active: true}
	r.setHeld(h, Range{Head: Pos{Line: math.MaxInt, Col: math.MaxInt}})
	if h.get().Empty() {
		r.sel = selection{}
		return false
	}
	return true
}

// Selection returns the text selected with the mouse and whether any is
// selected, as laid out in the last frame. Text is read from the nodes
// inside the node selected in: all of a [Text] or component, including what
// is scrolled out of sight, with the lines that a styled text wrapped to
// fit joined again, and from a [TextSource], what it says it shows. Text
// drawn around them, such as a [Box]'s border, is not selected. Every item
// of a [List] that the selection covers is read, including those the last
// frame did not build, which are built now to read, as the list would build
// them, or read with [ListNode.ItemText]. The text is kept until the next
// frame, so reading it again costs nothing, unless it was read from a
// [TextMap] or [TextSource], which is asked again.
//
// The selection may be the range of an [Editable], or of a viewport given
// [ViewportNode.Caret], made with the mouse or by the program, which is
// read from it: a range made since the last frame is seen at once.
func (r *Root) Selection() (string, bool) {
	r.checkHeld()
	s := &r.sel
	if s.ok && s.held {
		return r.heldSelection()
	}
	if !s.ok || !s.active || r.r.rngSeq != s.seq || !r.r.rng.show {
		return "", false
	}
	return r.r.selectedText()
}

// ClearSelection clears the text selected with the mouse, if any, such as
// when escape is pressed. The range of an [Editable], or of a viewport's
// caret, that is the selection is collapsed to its Head.
func (r *Root) ClearSelection() {
	r.checkHeld()
	if r.sel.ok && r.sel.held {
		if h := r.heldAt(r.sel.key); h != nil {
			if rg := h.get(); !rg.Empty() {
				r.setHeld(h, rg.Collapse())
			}
		}
	}
	r.sel = selection{}
}

// SelectAll selects all the text of the node with ID id, or of its nearest
// ancestor made selectable, including what is scrolled out of sight and
// every item of a [List] in it, such as for ctrl+a, and reports whether
// there is such a node in the last frame with any text. One without text
// clears the selection. It can be copied at once, with
// [Root.CopySelection]. No [SelectionMsg] is sent for it. It costs no more
// for a list of many items: their text is read only when it is wanted.
//
// The ID of an [Editable]'s node, or of a node it is in that is not made
// selectable itself, selects all of the Editable's text, as its range, and
// so does that of a viewport given [ViewportNode.Caret], of its buffer.
func (r *Root) SelectAll[K comparable](id K) bool {
	f := r.r.frame
	i := f.index(id)
	if i >= 0 {
		r.checkHeld()
		if h := r.heldIn(f, i); h != nil {
			return r.selectAllHeld(f, f.find(h.leaf.key), h)
		}
	}
	for i >= 0 && !f.entries[i].selectable {
		i = f.entries[i].parent
	}
	if i < 0 {
		return false
	}
	e := &f.entries[i]
	r.collapseHeld(nodeKey{}, false)
	r.flash = selection{}
	r.selSeq++
	r.sel = selection{ok: true, key: e.key, id: f.nearestID(i), seq: r.selSeq, active: true}
	// It is found in the last frame now, rather than in the next, so that
	// it can be copied before that is drawn.
	if !r.r.selectAll(&r.sel, e.vis) {
		r.ClearSelection()
		return false
	}
	return true
}

// CopySelection copies the text selected with the mouse to the clipboard,
// with the function given with [WithClipboard] and with OSC 52, clears the
// selection, and sweeps a band across the text copied (see
// [WithCopyFlash]). It returns nil when nothing is selected, so ctrl+c can
// copy, or quit with nothing to copy:
//
//	case "ctrl+c":
//		if cmd := m.ui.CopySelection(); cmd != nil {
//			return m, cmd
//		}
//		return m, tea.Quit
//
// A [CopyMsg] is sent once the text is copied.
//
// The range of an [Editable], or of a viewport's caret, is copied and left
// as it is, and the band sweeps across it in place of the selection's color
// until the band has crossed it, or the range changes.
func (r *Root) CopySelection() tea.Cmd {
	text, ok := r.Selection()
	if !ok || text == "" {
		return nil
	}
	var flash tea.Cmd
	if r.copyFlash && anim.Motion() {
		r.flash, r.flashAt = r.sel, r.now()
		r.flash.dragging = false
		if r.sel.held {
			if h := r.heldAt(r.sel.key); h != nil {
				r.flashRange = h.get()
			}
		}
		flash = r.after(r.interval, func(time.Time) tea.Msg { return flashMsg{root: r} })
	}
	id, write := r.sel.id, r.clipboard
	if !r.sel.held {
		r.ClearSelection()
	}
	copyText := func() tea.Msg {
		// The system clipboard goes first: the terminal writes the same
		// clipboard for OSC 52 on its own schedule, and a write that checks
		// what landed would read it mid-way.
		var err error
		if write != nil {
			err = write(text)
		}
		return tea.Sequence(tea.SetClipboard(text), func() tea.Msg { return CopyMsg{ID: id, Text: text, Err: err} })()
	}
	return tea.Batch(copyText, flash)
}

// flashFrame keeps the copy flash drawing, for a tick of msg, until it is
// over, or motion is turned off.
func (r *Root) flashFrame(msg tea.Msg) tea.Cmd {
	if f, ok := msg.(flashMsg); !ok || f.root != r || !r.flash.ok {
		return nil
	}
	if r.now().Sub(r.flashAt) >= flashFor || !anim.Motion() {
		r.flash = selection{}
		return nil
	}
	return r.after(r.interval, func(time.Time) tea.Msg { return flashMsg{root: r} })
}

// startSelect starts selecting text in the node of entry i, which the left
// button went down on at p, and reports whether it did. A double click
// selects a word, and a triple click a line.
func (r *Root) startSelect(f *Frame, i int, p image.Point, mod tea.KeyMod) bool {
	e := &f.entries[i]
	if !p.In(e.vis) {
		return false
	}
	unit := r.selectUnit(e.key, p)
	if h := r.heldAt(e.key); h != nil {
		r.sel = selection{}
		r.startHeld(f, i, h, p, unit, mod)
		return true
	}
	// Another selection starting ends that of an Editable.
	r.collapseHeld(nodeKey{}, false)
	block := mod.Contains(tea.ModAlt)
	if block {
		unit = textsel.UnitCell
	}
	r.selSeq++
	r.sel = selection{
		ok: true, key: e.key, id: f.nearestID(i), seq: r.selSeq, press: p, pointer: p,
		unit: unit, block: block, active: unit != textsel.UnitCell, dragging: true,
	}
	return true
}

// continuesClick reports whether a press at p on the selectable node with
// key k is the next click of a double or triple click.
func (r *Root) continuesClick(k nodeKey, p image.Point) bool {
	m := &r.mouse
	return m.selKey == k && m.clicks.Next(p, r.now(), clickSlop, doubleClick)
}

// selectUnit counts the presses at p on the node with key k quick enough
// after each other to be a double or triple click, returning what they
// select.
func (r *Root) selectUnit(k nodeKey, p image.Point) textsel.Unit {
	next := r.continuesClick(k, p)
	r.mouse.selKey = k
	return r.mouse.clicks.Press(next, p, r.now())
}

// moveSelect moves the end of the selection being dragged to p, in frame f,
// returning the command that scrolls the node while p is past its edge.
func (r *Root) moveSelect(f *Frame, p image.Point) tea.Cmd {
	s := &r.sel
	if !s.dragging {
		return nil
	}
	if f.find(s.key) < 0 {
		r.ClearSelection()
		return nil
	}
	s.pointer = p
	if p != s.press && !s.moved {
		s.active, s.moved = true, true
		// A press after a drag starts counting clicks again.
		r.mouse.clicks.Reset()
	}
	r.dragHeld()
	return r.autoScroll(f)
}

// endSelect ends the drag selecting text, if any, returning a command
// sending a [SelectionMsg] if it selected any. After a double click that
// did not drag, the message waits until a third click could no longer
// widen the selection to the line.
func (r *Root) endSelect() tea.Cmd {
	s := &r.sel
	if !s.dragging {
		return nil
	}
	// The release may be somewhere the last frame did not follow the pointer
	// to, such as when motion is throttled: the end is found there now.
	r.dragHeld()
	if rr := r.r; !s.held && rr.rngSeq == s.seq {
		rr.sel, rr.selLost = *s, false
		if _, ok := rr.settle(); ok {
			s.anchor, s.head, s.headFirst = rr.sel.anchor, rr.sel.head, rr.sel.headFirst
		}
		if rr.selLost {
			r.ClearSelection()
			return nil
		}
	}
	s.dragging = false
	text, ok := r.Selection()
	if !s.active || !ok {
		r.ClearSelection()
		return nil
	}
	if s.unit != textsel.UnitWord || s.moved {
		msg := SelectionMsg{ID: s.id, Text: text}
		return func() tea.Msg { return msg }
	}
	seq := s.seq
	wait := max(doubleClick-r.now().Sub(r.mouse.clicks.Time()), 0)
	return r.after(wait, func(time.Time) tea.Msg { return selDoneMsg{root: r, seq: seq} })
}

// selectionDone sends the [SelectionMsg] of a double click, for a tick of
// msg, if no further click widened or ended it.
func (r *Root) selectionDone(msg tea.Msg) tea.Cmd {
	d, ok := msg.(selDoneMsg)
	if !ok || d.root != r || d.seq != r.sel.seq || r.sel.dragging {
		return nil
	}
	text, ok := r.Selection()
	if !ok {
		return nil
	}
	out := SelectionMsg{ID: r.sel.id, Text: text}
	return func() tea.Msg { return out }
}

// checkSelect drops the selection when its node left frame f or is
// animating out in it.
func (r *Root) checkSelect(f *Frame) {
	if i := f.find(r.sel.key); r.sel.ok && (i < 0 || f.entries[i].leaving) {
		r.sel = selection{}
	}
}

// autoScroll returns the command for the next tick scrolling the node text
// is selected in, if the pointer is past its edge and none is due.
func (r *Root) autoScroll(f *Frame) tea.Cmd {
	if r.sel.scrolling {
		return nil
	}
	if _, _, h := r.scrollStep(f); h < 0 {
		return nil
	}
	r.sel.scrolling = true
	return r.after(scrollEvery, func(time.Time) tea.Msg { return selScrollMsg{root: r} })
}

// scrollStep returns which way the text being selected in should scroll,
// in frame f, for the pointer past the edge of what is shown of it to be
// over it, and the entry whose [on.Scroll] handler scrolls it, as the wheel
// would just inside that edge, or -1 when it cannot scroll that way.
func (r *Root) scrollStep(f *Frame) (dx, dy, h int) {
	s := &r.sel
	if !s.dragging || f == nil || !s.held && r.r.rngSeq != s.seq {
		return 0, 0, -1
	}
	// What is shown of the text, and all of it, as laid out. How much of a
	// TextMap is out of sight is not known, so it is taken to go on, as is
	// a list at the edge of what is shown past the items it built.
	var shown, all image.Rectangle
	if s.held {
		if l := r.heldAt(s.key); l != nil {
			shown, all = l.leaf.clip, textsel.Endless
			if l.leaf.vp != nil {
				all = l.leaf.rect
			}
		}
	} else {
		shown, all = r.r.outline.Extent()
	}
	if shown.Empty() {
		return 0, 0, -1
	}
	p := s.pointer
	dx = textsel.EdgeStep(p.X, shown.Min.X, shown.Max.X, all.Min.X, all.Max.X)
	dy = textsel.EdgeStep(p.Y, shown.Min.Y, shown.Max.Y, all.Min.Y, all.Max.Y)
	if dx == 0 && dy == 0 {
		return 0, 0, -1
	}
	in := image.Pt(min(max(p.X, shown.Min.X), shown.Max.X-1), min(max(p.Y, shown.Min.Y), shown.Max.Y-1))
	r.mouse.scrollChain, _ = f.under(in, r.mouse.scrollChain[:0])
	if h = f.innermost(r.mouse.scrollChain, kindsOf(on.KindScroll)); h < 0 {
		return 0, 0, -1
	}
	return dx, dy, h
}

// scrollSelection scrolls the text under the end of the selection a step,
// for a tick of msg, with the [on.Scroll] handler that scrolls it, as the
// wheel would, while the pointer is held past its edge.
func (r *Root) scrollSelection(msg tea.Msg) tea.Cmd {
	if t, ok := msg.(selScrollMsg); !ok || t.root != r {
		return nil
	}
	r.sel.scrolling = false
	f := r.r.frame
	dx, dy, h := r.scrollStep(f)
	if h < 0 {
		return nil
	}
	outer := r.mouse.routing
	r.mouse.routing = f
	defer func() { r.mouse.routing = outer }()
	t, p := f.target(h), r.sel.pointer
	var cmds []tea.Cmd
	wheel := func(d int, back, fwd tea.MouseButton) {
		if d != 0 {
			b := fwd
			if d < 0 {
				b = back
			}
			msg := tea.MouseWheelMsg{X: p.X, Y: p.Y, Button: b}
			cmds = run(cmds, t.handlers, kindsOf(on.KindScroll), t.event(msg, p, false))
		}
	}
	wheel(dy, tea.MouseWheelUp, tea.MouseWheelDown)
	wheel(dx, tea.MouseWheelLeft, tea.MouseWheelRight)
	return tea.Batch(append(cmds, r.autoScroll(f))...)
}
