// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/words"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/on"
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
// by default.
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
	// minContrast is the least contrast, as WCAG measures it, that a
	// selected cell's own foreground is kept at against the selection's
	// background.
	minContrast = 3
	// selectionTint is how far toward the terminal's default foreground the
	// default selection background is from its default background.
	selectionTint = 0.3
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

// selUnit is what a selection takes whole: a double click selects words,
// and a triple click lines.
type selUnit uint8

const (
	unitCell selUnit = iota
	unitWord
	unitLine
)

// selPos is where a selection starts or ends: a cell of a text leaf, found
// from the pointer once the frame is laid out, and followed from frame to
// frame by the leaf's key, so that it stays on the text as the leaf moves,
// such as inside a [Scroll].
type selPos struct {
	leaf nodeKey
	at   image.Point
	ok   bool
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
	unit    selUnit
	block   bool // a rectangle of cells, rather than a run of text
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
}

// selRange is a selection as laid out in a frame: from leaf a at pa to
// leaf b at pb, in the order the leaves were drawn, or with block, the
// corners of a rectangle. It is comparable, so a frame whose selection did
// not change is not composited again.
type selRange struct {
	a, b   int
	pa, pb image.Point
	block  bool
	show   bool
}

// selStyle is a [WithSelectionStyle] style, as it is applied to cells.
type selStyle struct {
	set       bool
	fg, bg    color.Color
	attrs     uint8
	underline bool
	reverse   bool
}

func newSelStyle(st lipgloss.Style, set bool) selStyle {
	s := selStyle{set: set}
	if !set {
		return s
	}
	c := cellStyle(st)
	s.fg, s.bg, s.attrs = c.Fg, c.Bg, c.Attrs&^uv.AttrReverse
	s.underline, s.reverse = c.Underline != 0, c.Attrs&uv.AttrReverse != 0
	return s
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
	return r.sel.ok && b.meta.selectable && b.key() == r.sel.key
}

// collectLeaves notes the text leaves of box i, the selected node or a box
// inside it at path, clipped to clip: every [Text], component, and
// [TextSource] in it, including those out of sight, such as scrolled away,
// so that a selection takes them too. What a [BoxNode] draws on its edges
// is decoration, and the layout of a TextSource draws cells but no text of
// its own, which inner marks.
func (r *renderer) collectLeaves(i int, clip image.Rectangle, path uint64, inner bool) {
	if i < 0 || r.boxes[i].hidden {
		return
	}
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	kid := func(k int, inner bool) {
		if k >= 0 {
			r.collectLeaves(k, vis, childPath(path, r.boxes[k].slot), inner)
		}
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
		if s, ok := b.child.(TextSource); ok && !inner {
			l.source = s
		}
		r.leaves = append(r.leaves, l)
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
	case SlotNode, FlexNode, GridNode, WrapNode, ScrollNode, ListNode, OverlayNode, LayerNode, centerNode, transitionNode:
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
			r.leaves = append(r.leaves, selLeaf{key: key, box: i, rect: rect, clip: shown, vp: l})
		}
	case drawableNode, ScrollbarNode:
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
	text := r.textAt(i, n, b.rect.Dx(), b.rect.Dy())
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
	for _, in := range []func(l *selLeaf) bool{
		func(l *selLeaf) bool { return p.In(l.clip) },
		func(l *selLeaf) bool { return p.In(l.rect) },
	} {
		for i := len(r.leaves) - 1; i >= 0; i-- {
			if l := &r.leaves[i]; !l.inner && in(l) {
				return r.posIn(i, p.Sub(l.rect.Min))
			}
		}
	}
	best, first := -1, -1
	for i := range r.leaves {
		l := &r.leaves[i]
		if l.inner || l.rect.Empty() {
			continue
		}
		if first < 0 {
			first = i
		}
		rows := p.Y >= l.rect.Min.Y && p.Y < l.rect.Max.Y
		if p.Y < l.rect.Max.Y && (!rows || p.X < l.rect.Min.X) {
			continue // after p
		}
		if best < 0 || inColumn(l, p) || !inColumn(&r.leaves[best], p) {
			best = i
		}
	}
	switch {
	case best >= 0:
		return r.posIn(best, p.Sub(r.leaves[best].rect.Min))
	case first >= 0:
		return r.posIn(first, image.Pt(0, -1))
	}
	return selPos{}
}

// posIn returns the cell of leaf i nearest at: its start above it, its end
// below it, and the nearest of the row otherwise.
func (r *renderer) posIn(i int, at image.Point) selPos {
	l := &r.leaves[i]
	w, h := l.rect.Dx(), l.rect.Dy()
	switch {
	case at.Y < 0:
		at = image.Point{}
	case at.Y >= h:
		at = image.Pt(w-1, h-1)
	default:
		at.X = min(max(at.X, 0), w-1)
	}
	return selPos{leaf: l.key, at: at, ok: true}
}

func inColumn(l *selLeaf, p image.Point) bool {
	return p.X >= l.rect.Min.X && p.X < l.rect.Max.X
}

// clampAt returns at inside leaf i.
func (r *renderer) clampAt(i int, at image.Point) image.Point {
	l := &r.leaves[i]
	return image.Pt(min(max(at.X, 0), l.rect.Dx()-1), min(max(at.Y, 0), l.rect.Dy()-1))
}

// resolveSel finds the selection in the leaves noted for this frame, from
// where the pointer pressed and is now, and widens it to the words or
// lines it takes whole.
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
	a, b := r.findLeaf(s.anchor.leaf), r.findLeaf(s.head.leaf)
	if a < 0 || b < 0 {
		return selRange{}, false
	}
	g := selRange{a: a, b: b, pa: r.clampAt(a, s.anchor.at), pb: r.clampAt(b, s.head.at), block: s.block, show: s.active}
	if g.block {
		return g, true
	}
	if b < a || a == b && (g.pb.Y < g.pa.Y || g.pb.Y == g.pa.Y && g.pb.X < g.pa.X) {
		g.a, g.b, g.pa, g.pb = g.b, g.a, g.pb, g.pa
	}
	switch s.unit {
	case unitLine:
		g.pa.X, g.pb.X = 0, r.leaves[g.b].rect.Dx()-1
	case unitWord:
		x0, _, okA := r.wordAt(g.a, g.pa)
		if okA {
			g.pa.X = x0
		}
		if _, x1, ok := r.wordAt(g.b, g.pb); ok {
			g.pb.X = x1
		}
		if g.a == g.b && g.pa == g.pb && !okA {
			// A double click on a blank selects nothing.
			g.show = false
		}
	case unitCell:
	}
	return g, true
}

// leafCell returns the cell at x, y of leaf i, or nil where it has none.
func (r *renderer) leafCell(i, x, y int) *uv.Cell {
	if l := &r.leaves[i]; l.vp != nil {
		return r.cache.Get(r.vpRowCells(l.vp, y)).CellAt(x, 0)
	}
	if src := r.cells(&r.leaves[i]); src != nil {
		return src.CellAt(x, y)
	}
	return r.cellAt(r.leaves[i].rect.Min.Add(image.Pt(x, y)))
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
	w := r.leaves[i].rect.Dx()
	if w <= 0 || p.X >= w {
		return 0, 0, false
	}
	row, offs := r.wordRow[:0], r.wordOffs[:0]
	for x := range w {
		c := r.leafCell(i, x, p.Y)
		offs = append(offs, len(row))
		switch {
		case c == nil || c.Content == "" && c.Width != 0:
			row = append(row, ' ')
		case c.Width == 0:
			// The second half of a wide character: part of the first.
			offs[x] = offs[max(x-1, 0)]
		default:
			row = append(row, c.Content...)
		}
	}
	r.wordRow, r.wordOffs = row, offs

	segs := r.wordSegs[:0]
	at, hit := offs[p.X], -1
	for it := words.FromBytes(row); it.Next(); {
		if at >= it.Start() && at < it.End() {
			hit = len(segs)
		}
		segs = append(segs, wordSeg{start: it.Start(), end: it.End(), word: r.wordish(it.Value())})
	}
	r.wordSegs = segs
	if hit < 0 || !segs[hit].word {
		return 0, 0, false
	}
	lo, hi := hit, hit
	for lo > 0 && segs[lo-1].word {
		lo--
	}
	for hi < len(segs)-1 && segs[hi+1].word {
		hi++
	}
	x0 = -1
	for x, off := range offs {
		if off >= segs[lo].start && off < segs[hi].end {
			if x0 < 0 {
				x0 = x
			}
			x1 = x
		}
	}
	return x0, x1, x0 >= 0
}

// wordSeg is a segment of a row that Unicode text segmentation found, and
// whether it is part of a word.
type wordSeg struct {
	start, end int
	word       bool
}

// wordish reports whether seg, a segment Unicode text segmentation found, is
// part of a word: one with a letter, digit, or symbol such as an emoji, or
// one of only the word characters, which join the words around them.
func (r *renderer) wordish(seg []byte) bool {
	s := string(seg)
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsMark(c) || unicode.Is(unicode.So, c) {
			return true
		}
	}
	for _, c := range s {
		if !strings.ContainsRune(r.wordChars, c) {
			return false
		}
	}
	return true
}

// paintSelection finds the selection in the leaves of box i, the selected
// node, clipped to clip, and adds the op that draws it over them.
func (r *renderer) paintSelection(i int, clip image.Rectangle) {
	if !r.parse {
		return
	}
	r.leaves = r.leaves[:0]
	r.collectLeaves(i, clip, 0, false)
	g, ok := r.settle()
	if !ok {
		return
	}
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	if b.hidden || vis.Empty() || !g.show {
		return
	}
	r.ops = append(r.ops, op{kind: opSelect, rect: b.rect, clip: vis, sel: g, t: r.flashT})
}

// settle finds the selection in the leaves noted as the range that is drawn
// and copied for it, until the next frame.
func (r *renderer) settle() (selRange, bool) {
	g, ok := r.resolveSel()
	r.rng, r.rngSeq = g, -1
	if ok {
		r.rngSeq = r.sel.seq
	}
	return g, ok
}

// selectAll selects as s all the text of its node, selectable and shown at
// vis in the last frame, and reports whether it has any.
func (r *renderer) selectAll(s *selection, vis image.Rectangle) bool {
	r.rngSeq = -1
	box := -1
	for k := range r.boxes {
		if b := &r.boxes[k]; b.meta.selectable && !b.hidden && b.key() == s.key {
			box = k
			break
		}
	}
	if box < 0 {
		return false
	}
	r.leaves = r.leaves[:0]
	r.collectLeaves(box, vis, 0, false)
	first, last := -1, -1
	for k := range r.leaves {
		if r.leaves[k].inner {
			continue
		}
		if first < 0 {
			first = k
		}
		last = k
	}
	if first < 0 {
		return false
	}
	end := r.leaves[last].rect.Size().Sub(image.Pt(1, 1))
	s.anchor = selPos{leaf: r.leaves[first].key, ok: true}
	s.head = selPos{leaf: r.leaves[last].key, at: end, ok: true}
	r.sel = *s
	_, ok := r.settle()
	return ok
}

// executeSelect draws the selection over the cells of t it covers.
func (r *renderer) executeSelect(o *op, t target) {
	if o.t < 0 {
		r.eachSelected(o, func(p image.Point) {
			if c := t.c.CellAt(p.X-t.origin.X, p.Y-t.origin.Y); c != nil {
				r.highlight(c)
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
			r.flashCell(c, p, o.t, area)
		}
	})
}

// eachSelected calls fn with each cell on screen that the selection of op
// o covers.
func (r *renderer) eachSelected(o *op, fn func(p image.Point)) {
	// Only what is in sight is visited, however much is selected.
	g := &o.sel
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
		in := l.clip.Intersect(o.clip).Sub(l.rect.Min)
		if l.inner || in.Empty() {
			continue
		}
		from, to := r.leafSpan(g, i)
		for y := max(from.Y, in.Min.Y); y <= min(to.Y, in.Max.Y-1); y++ {
			x0, x1 := rowSpan(from, to, y, l.rect.Dx())
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

// rowSpan returns the columns of row y that a run from from to to takes,
// in an area w cells wide.
func rowSpan(from, to image.Point, y, w int) (x0, x1 int) {
	x0, x1 = 0, w-1
	if y == from.Y {
		x0 = from.X
	}
	if y == to.Y {
		x1 = to.X
	}
	return x0, x1
}

// blockRect returns the rectangle of a block selection, as laid out.
func (r *renderer) blockRect(g *selRange) image.Rectangle {
	p := r.leaves[g.a].rect.Min.Add(g.pa)
	q := r.leaves[g.b].rect.Min.Add(g.pb)
	return image.Rect(min(p.X, q.X), min(p.Y, q.Y), max(p.X, q.X)+1, max(p.Y, q.Y)+1)
}

// highlight draws cell c as selected.
func (r *renderer) highlight(c *uv.Cell) {
	s := &r.selStyle
	c.Style.Attrs &^= uv.AttrReverse
	if s.reverse && s.fg == nil && s.bg == nil {
		c.Style.Attrs |= uv.AttrReverse | s.attrs
		return
	}
	switch {
	case s.bg != nil:
		c.Style.Bg = s.bg
	case !s.set:
		c.Style.Bg = r.mixer.Mix(r.env.Bg, r.env.Fg, selectionTint)
	}
	c.Style.Attrs |= s.attrs
	if s.underline {
		c.Style.Underline = uv.UnderlineSingle
	}
	if s.fg != nil {
		c.Style.Fg = s.fg
		return
	}
	r.readable(c)
}

// readable gives cell c the terminal's default foreground or background,
// whichever reads better, when its own foreground is hard to read on its
// background.
func (r *renderer) readable(c *uv.Cell) {
	fg, bg := c.Style.Fg, c.Style.Bg
	if fg == nil {
		fg = r.env.Fg
	}
	if bg == nil {
		bg = r.env.Bg
	}
	if contrast(fg, bg) < minContrast {
		c.Style.Fg = r.env.Fg
		if contrast(r.env.Bg, bg) > contrast(r.env.Fg, bg) {
			c.Style.Fg = r.env.Bg
		}
	}
}

// flashCell draws cell c, at p, as the band [Root.CopySelection] sweeps
// across area draws it at t: as selected where the band is, fading out to
// either side. The band runs diagonally, from the top-left of area to its
// bottom-right.
func (r *renderer) flashCell(c *uv.Cell, p image.Point, t float64, area image.Rectangle) {
	// A cell is about twice as tall as it is wide, so a row counts as two
	// columns for the band to lean at 45 degrees.
	diagonal := func(p image.Point) float64 { return float64(p.X + 2*p.Y) }
	start, end := diagonal(area.Min), diagonal(area.Max.Sub(image.Pt(1, 1)))
	width := max((end-start)/4, 4)
	center := start - width + t*(end-start+2*width)
	k := 1 - math.Abs(diagonal(p)-center)/width
	if k <= 0 {
		return
	}
	plain := c.Style
	r.highlight(c)
	if k >= 0.5 {
		return
	}
	// The band's edges blend the background back to the cell's own.
	from, to := plain.Bg, c.Style.Bg
	if from == nil {
		from = r.env.Bg
	}
	if to == nil {
		to = r.env.Bg
	}
	c.Style = plain
	c.Style.Bg = r.mixer.Mix(from, to, k*2)
	r.readable(c)
}

// contrast returns the contrast ratio of a and b, as WCAG measures it, from
// 1 for the same luminance to 21 for black and white.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// luminance returns the relative luminance of c, as WCAG defines it.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// selectedText returns the text of the selection found in the last frame.
func (r *renderer) selectedText() string {
	g := &r.rng
	if g.block {
		return r.blockText(g)
	}
	var b strings.Builder
	prevRow := 0
	for i := g.a; i <= g.b; i++ {
		l := &r.leaves[i]
		if l.inner {
			continue
		}
		from, to := r.leafSpan(g, i)
		if i > g.a {
			// A leaf beside the last on the same row is a cell of a table,
			// such as a value beside its label: a space between them.
			if l.rect.Min.Y+from.Y == prevRow {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
		}
		b.WriteString(r.leafText(l, from, to))
		prevRow = l.rect.Min.Y + to.Y
	}
	return strings.TrimRight(b.String(), "\n")
}

// leafText returns the text of leaf l from from to to, in reading order.
func (r *renderer) leafText(l *selLeaf, from, to image.Point) string {
	if l.vp != nil {
		return vpText(l.vp, from, to, false)
	}
	if l.source != nil {
		return l.source.SelectedText(Span{Start: from, End: to, Size: Size{W: l.rect.Dx(), H: l.rect.Dy()}})
	}
	src := r.cells(l)
	if src == nil {
		return ""
	}
	if l.truncated && from.Y == to.Y {
		if s, ok := cutText(src, lineBreaks.Replace(ansi.Strip(l.text)), from.Y, from.X, to.X); ok {
			return s
		}
	}
	var joins []join
	if l.text != "" && !l.truncated && to.Y > from.Y {
		joins = wrapJoins(src, l.text)
	}
	var b strings.Builder
	for y := from.Y; y <= to.Y; y++ {
		x0, x1 := rowSpan(from, to, y, l.rect.Dx())
		b.WriteString(strings.TrimRight(rowText(src.CellAt, x0, x1, y), " "))
		if y < to.Y {
			j := joinBreak
			if y < len(joins) {
				j = joins[y]
			}
			switch j {
			case joinBreak:
				b.WriteByte('\n')
			case joinSpace:
				b.WriteByte(' ')
			case joinNone:
			}
		}
	}
	return b.String()
}

// blockText returns the text of block selection g, a line to a row.
func (r *renderer) blockText(g *selRange) string {
	area := r.blockRect(g)
	if l := &r.leaves[g.a]; g.a == g.b && l.vp != nil {
		return vpText(l.vp, area.Min.Sub(l.rect.Min), area.Max.Sub(l.rect.Min).Sub(image.Pt(1, 1)), true)
	}
	if l := &r.leaves[g.a]; g.a == g.b && l.source != nil {
		return l.source.SelectedText(Span{
			Start: area.Min.Sub(l.rect.Min), End: area.Max.Sub(l.rect.Min).Sub(image.Pt(1, 1)), Block: true,
			Size: Size{W: l.rect.Dx(), H: l.rect.Dy()},
		})
	}
	cell := func(x, y int) *uv.Cell { return r.cellAt(image.Pt(x, y)) }
	var b strings.Builder
	for y := area.Min.Y; y < area.Max.Y; y++ {
		if y > area.Min.Y {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(rowText(cell, area.Min.X, area.Max.X-1, y), " "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// rowText returns the text of the cells from x0 to x1 of row y, inclusive,
// with a blank where there is no cell, taking whole a wide character that
// x0 is the second half of.
func rowText(cell func(x, y int) *uv.Cell, x0, x1, y int) string {
	for x0 > 0 {
		if c := cell(x0, y); c == nil || c.Width != 0 {
			break
		}
		x0--
	}
	var b strings.Builder
	for x := x0; x <= x1; x++ {
		switch c := cell(x, y); {
		case c == nil || c.Content == "" && c.Width != 0:
			b.WriteByte(' ')
		case c.Width == 0:
			// The second half of a wide character.
		default:
			b.WriteString(c.Content)
		}
	}
	return b.String()
}

// cutText returns the text of full, a line drawn cut to fit with "…" in row
// y of src, that the cells from x0 to x1 of the row select, and whether the
// row was cut. A selection that takes the "…" takes the rest of the line,
// and one that ends just before it, the rest of the word it cut.
func cutText(src *canvas.Canvas, full string, y, x0, x1 int) (string, bool) {
	if y < 0 || y >= src.Height() {
		return "", false
	}
	cut := -1
	for x := src.Width() - 1; x >= 0; x-- {
		c := src.CellAt(x, y)
		if c.Content == "…" {
			cut = x
			break
		}
		if c.Content != "" && c.Content != " " {
			break
		}
	}
	if cut < 0 {
		return "", false
	}
	// What is shown before the cut is where it starts in full, after any
	// padding of its style.
	start := 0
	for start < cut {
		if c := src.CellAt(start, y); c.Content != "" && c.Content != " " {
			break
		}
		start++
	}
	shown := rowText(src.CellAt, start, cut-1, y)
	at := strings.Index(full, shown)
	if at < 0 {
		return "", false
	}
	offset := func(x int) int {
		if x <= start {
			return at
		}
		return at + len(rowText(src.CellAt, start, min(x, cut)-1, y))
	}
	from, to := offset(x0), len(full)
	if x1 < cut {
		to = offset(x1 + 1)
		if x1 == cut-1 {
			for to < len(full) && full[to] != ' ' {
				to++
			}
		}
	}
	return strings.TrimRight(full[from:max(to, from)], " "), true
}

// join is what comes between two rows of a text node in the text copied.
type join uint8

const (
	joinBreak join = iota // a line break of its own
	joinSpace             // wrapped where it broke at a space
	joinNone              // wrapped in the middle of a word
)

// wrapJoins returns what ends each row of src, the cells of text s as
// drawn, in the text copied: the rows s wrapped to fit are joined, as the
// lines they are, by finding each row's text in s in turn. Where a row is
// not found in s, such as a border its style drew, it ends with a break.
func wrapJoins(src *canvas.Canvas, s string) []join {
	plain := strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	joins := make([]join, src.Height())
	at, prev := 0, -1
	for y := range joins {
		row := strings.TrimSpace(rowText(src.CellAt, 0, src.Width()-1, y))
		if row == "" {
			prev = -1
			continue
		}
		gap := at
		for gap < len(plain) && (plain[gap] == ' ' || plain[gap] == '\n') {
			gap++
		}
		if !strings.HasPrefix(plain[gap:], row) {
			prev = -1
			continue
		}
		if prev >= 0 && prev == y-1 {
			switch skipped := plain[at:gap]; {
			case strings.Contains(skipped, "\n"):
			case skipped != "":
				joins[prev] = joinSpace
			default:
				joins[prev] = joinNone
			}
		}
		at, prev = gap+len(row), y
	}
	return joins
}

// Selection returns the text selected with the mouse and whether any is
// selected, as laid out in the last frame. Text is read from the nodes
// inside the node selected in: all of a [Text] or component, including what
// is scrolled out of sight, with the lines that a styled text wrapped to
// fit joined again, and from a [TextSource], what it says it shows. Text
// drawn around them, such as a [Box]'s border, is not selected.
func (r *Root) Selection() (string, bool) {
	s := &r.sel
	if !s.ok || !s.active || r.r.rngSeq != s.seq || !r.r.rng.show {
		return "", false
	}
	return r.r.selectedText(), true
}

// ClearSelection clears the text selected with the mouse, if any, such as
// when escape is pressed.
func (r *Root) ClearSelection() { r.sel = selection{} }

// SelectAll selects all the text of the node with ID id, or of its nearest
// ancestor made selectable, including what is scrolled out of sight, such
// as for ctrl+a, and reports whether there is such a node in the last frame
// with any text. One without text clears the selection. It can be copied at
// once, with [Root.CopySelection]. No [SelectionMsg] is sent for it.
func (r *Root) SelectAll[K comparable](id K) bool {
	f := r.r.frame
	i := f.index(id)
	for i >= 0 && !f.entries[i].selectable {
		i = f.entries[i].parent
	}
	if i < 0 {
		return false
	}
	e := &f.entries[i]
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
func (r *Root) CopySelection() tea.Cmd {
	text, ok := r.Selection()
	if !ok || text == "" {
		return nil
	}
	var flash tea.Cmd
	if r.copyFlash {
		r.flash, r.flashAt = r.sel, r.now()
		r.flash.dragging = false
		flash = r.after(r.interval, func(time.Time) tea.Msg { return flashMsg{root: r} })
	}
	id, write := r.sel.id, r.clipboard
	r.ClearSelection()
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
// over.
func (r *Root) flashFrame(msg tea.Msg) tea.Cmd {
	if f, ok := msg.(flashMsg); !ok || f.root != r || !r.flash.ok {
		return nil
	}
	if r.now().Sub(r.flashAt) >= flashFor {
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
	block := mod.Contains(tea.ModAlt)
	unit := r.selectUnit(e.key, p)
	if block {
		unit = unitCell
	}
	r.selSeq++
	r.sel = selection{
		ok: true, key: e.key, id: f.nearestID(i), seq: r.selSeq, press: p, pointer: p,
		unit: unit, block: block, active: unit != unitCell, dragging: true,
	}
	return true
}

// continuesClick reports whether a press at p on the selectable node with
// key k is the next click of a double or triple click.
func (r *Root) continuesClick(k nodeKey, p image.Point) bool {
	m := &r.mouse
	return m.selOK && m.selKey == k && m.selAt.Y == p.Y && abs(m.selAt.X-p.X) <= clickSlop &&
		m.selUnit < unitLine && r.now().Sub(m.selTime) <= doubleClick
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// selectUnit counts the presses at p on the node with key k quick enough
// after each other to be a double or triple click, returning what they
// select.
func (r *Root) selectUnit(k nodeKey, p image.Point) selUnit {
	m := &r.mouse
	unit := unitCell
	if r.continuesClick(k, p) {
		unit = m.selUnit + 1
	}
	m.selUnit, m.selOK, m.selKey, m.selAt, m.selTime = unit, true, k, p, r.now()
	return unit
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
		r.mouse.selOK = false
	}
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
	if rr := r.r; rr.rngSeq == s.seq {
		rr.sel = *s
		if _, ok := rr.settle(); ok {
			s.anchor, s.head = rr.sel.anchor, rr.sel.head
		}
	}
	s.dragging = false
	text, ok := r.Selection()
	if !s.active || !ok {
		r.ClearSelection()
		return nil
	}
	if s.unit != unitWord || s.moved {
		msg := SelectionMsg{ID: s.id, Text: text}
		return func() tea.Msg { return msg }
	}
	seq := s.seq
	wait := max(doubleClick-r.now().Sub(r.mouse.selTime), 0)
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

// checkSelect drops the selection when its node left frame f.
func (r *Root) checkSelect(f *Frame) {
	if r.sel.ok && f.find(r.sel.key) < 0 {
		r.ClearSelection()
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
	if !s.dragging || f == nil || r.r.rngSeq != s.seq {
		return 0, 0, -1
	}
	// What is shown of the text, and all of it, as laid out.
	var shown, all image.Rectangle
	for i := range r.r.leaves {
		if l := &r.r.leaves[i]; !l.inner {
			shown, all = shown.Union(l.clip), all.Union(l.rect)
		}
	}
	if shown.Empty() {
		return 0, 0, -1
	}
	p := s.pointer
	dx = edgeStep(p.X, shown.Min.X, shown.Max.X, all.Min.X, all.Max.X)
	dy = edgeStep(p.Y, shown.Min.Y, shown.Max.Y, all.Min.Y, all.Max.Y)
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

// edgeStep returns -1 when p is before lo, where what is shown of content
// from start to end starts, and some of it is out of sight there, 1 when p
// is at or past hi, where what is shown ends, and some of it is out of
// sight there, or 0.
func edgeStep(p, lo, hi, start, end int) int {
	switch {
	case p < lo && start < lo:
		return -1
	case p >= hi && end > hi:
		return 1
	}
	return 0
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
