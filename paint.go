// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/internal/linebuf"
	"bloomfx.dev/bloom/internal/memo"
	"bloomfx.dev/bloom/internal/same"
	"bloomfx.dev/bloom/internal/textsel"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// opKind is what an op does to the frame.
type opKind uint8

const (
	opBlit    opKind = iota // copy src into rect, blanking the rest of it
	opStretch               // copy src into rect, stretched to fill it at mid
	opClear                 // blank rect's glyphs, keeping backgrounds
	opDraw                  // draw a uv.Drawable into rect
	opEffects               // apply effects to rect of the frame
	opPush                  // draw what follows into its own cells, for effects
	opPop                   // apply effects to those cells and composite them
	opSelect                // draw the selection over the text leaves of the node it is in
)

// op is one step of drawing a frame. A frame is a list of them, built by
// walking the laid-out tree, so that a frame can be compared with the one
// before it without drawing it.
type op struct {
	kind     opKind
	src      *canvas.Canvas
	draw     uv.Drawable
	rect     image.Rectangle
	clip     image.Rectangle
	viewport image.Rectangle
	sub      image.Rectangle // for opPop: the part of rect effects apply to; empty for all of it
	mid      image.Point     // for opStretch: the column and row of src repeated
	effects  []fx.Effect
	t        float64
	from     image.Rectangle // for opPop of a slot moving with effects: where it was
	sel      selRange        // for opSelect, with t the copy flash's, or -1
	held     int             // for opSelect of a range kept outside the Root: its index in helds, plus one
	rev      uint64          // for opSelect: the revisions of the buffers its positions are in
	buf      *Buffer         // and for a range kept outside the Root, the buffer

	// For opPop: the index of its opPush, the frame entries drawn since it,
	// and how far its effects moved what was drawn, known once executed.
	push    int
	entries [2]int
	offset  image.Point
}

func (o *op) equal(p *op) bool {
	if o.kind != p.kind || o.kind == opDraw || o.src != p.src || o.rect != p.rect ||
		o.clip != p.clip || o.viewport != p.viewport || o.sub != p.sub || o.mid != p.mid || o.t != p.t || o.from != p.from || o.sel != p.sel || o.held != p.held || o.rev != p.rev || o.buf != p.buf || len(o.effects) != len(p.effects) {
		return false
	}
	for i := range o.effects {
		if !same.Effect(o.effects[i], p.effects[i]) {
			return false
		}
	}
	return true
}

// renderer lays out, paints, and composites trees. It keeps what it can
// between frames: the engine's arena, parsed strings, and the last frame, so
// an unchanged frame is not composited again.
type renderer struct {
	engine

	styled   memo.Table[styledMemo]     // by box index, from the last frame
	stretch  memo.Map[int, stretchMemo] // by box index, from the last frame; few boxes have one
	columns  memo.Table[columnMemo]     // by twice the box index, for scrollbars
	drag     scrollbarDrag              // the scrollbar being dragged, if any
	vpMarks  []linebuf.Overlay          // scratch for the marks of a viewport's line, in cells
	vpSpans  []linebuf.Overlay          // and as they were given, in bytes
	vpPens   linebuf.SGRs               // the styles of viewports' text, kept from frame to frame
	vpCell   linebuf.RowMemo            // the viewport row whose cells selection last asked for
	style    lipgloss.Style             // scratch for comparing styles without allocating
	fxs      []fx.Effect                // effects of this frame's ops, copied from the tree
	prevFxs  []fx.Effect
	env      canvas.Env
	mixer    canvas.Mixer
	parse    bool // false when only the layout is wanted, as for Solve
	canvas   *canvas.Canvas
	scratch  []*canvas.Canvas
	targets  []target
	ops      []op
	prevOps  []op
	moves    []deferredMove  // slots moving with effects, painted last
	floats   []deferredFloat // layers that float, waiting for what they are in to be painted
	open     int             // effect passes begun and not yet ended
	entries  []entry
	handlers [][]on.Handler // by entry
	hovered  []mouseTarget  // the hoverable nodes under the pointer, kept by Root

	dragged  *mouseTarget   // the node being dragged, if any, kept by Root
	cursor   *tea.Cursor    // the cursor shown, picked from placed
	placed   []placedCursor // the cursors painted this frame that may be shown
	prev     []placedCursor // and those of the last frame
	focus    *Focus         // the Root's focus, given by WithFocus
	out      string
	drawn    image.Point // size of the last composited frame
	drawnEnv canvas.Env
	frame    *Frame

	// sel is the Root's selection, found in the text leaves of the node it
	// is in as rng, and drawn over them with selPaint, or as the copy flash
	// at flashT when that is not negative.
	sel      selection
	leaves   []selLeaf
	outline  textsel.Outline // the order of the leaves, and the lists among them
	rng      selRange
	ends     [2]selEnd             // the ends of rng, in order
	rngSeq   int                   // the seq of the selection rng is for, or -1
	selMemo  textsel.Memo[textKey] // the text of the selection read last
	selLost  bool                  // the selection's positions could not be followed through the edits to its text
	selPaint textsel.Painter
	flashT   float64

	// helds are the selections kept outside the Root in this frame, such as
	// by an Editable, and prevHelds those of the last, which the copy flash
	// is drawn over, for heldFlash at heldFlashT, when that is not negative.
	helds      []heldLeaf
	prevHelds  []heldLeaf
	heldFlash  nodeKey
	heldFlashT float64
	vpScratch  []int // for the wrapped rows of a viewport the mouse is selecting in

	words  textsel.Words // finds words, for a double click
	rooted bool          // a Root's renderer, whose word characters the buffers it shows take

	// aside builds the items of lists that a frame did not build, to read
	// the text selected in them, apart from the frame; see
	// [renderer.buildAside]. It is made when wanted, and dropped once the
	// text is read.
	aside *renderer

	// With tipsOn, the tooltips of the nodes of the frame are kept in tips,
	// by entry, for Root, whose top draws what it wants over the frame into
	// topOps, once the frame's entries are where they were drawn. With
	// autoTips too, text cut to fit has its full text as its tooltip.
	// topOnCanvas is set while what the top drew is on the canvas, over the
	// frame's own cells, and topRedrawn once the top had the canvas drawn
	// again without it, to read the frame's own cells.
	tipsOn      bool
	autoTips    bool
	tips        []tipAt
	top         func(w, h int)
	topOps      []op
	prevTop     []op
	topOnCanvas bool
	topRedrawn  bool

	// The tooltip sinks of the frame with cells on screen, whose ops are
	// given the text sinkText returns once the tree is painted, before the
	// frame is compared with the last one; see fillSinks.
	sinks    []sinkAt
	sinkText func() string
}

func newRenderer(method ansi.Method) *renderer {
	return &renderer{
		method: method, cache: canvas.NewCache(method),
		env:    canvas.DefaultEnv,
		parse:  true,
		canvas: canvas.New(0, 0, method),
	}
}

// render lays out and draws c in a w×h frame, returning it as a string. For
// a w or h of zero or less, the frame is c's natural size.
func (r *renderer) render(w, h int, c Child) string {
	root := r.paintTree(w, h, c)
	w, h = r.size(root, w, h)
	if len(r.sinks) > 0 {
		r.fillSinks()
	}

	// The same ops have the same effects, which moved and hid things the
	// same way, but only the cursors they were run with: one placed
	// elsewhere inside them runs them again.
	samePlaced := slices.EqualFunc(r.placed, r.prev, func(a, b placedCursor) bool { return a.c.Position == b.c.Position && a.op == b.op })
	kept := r.drawn == image.Pt(w, h) && r.drawnEnv.Equal(r.env) && r.sameOps() &&
		(samePlaced || !r.inPass(r.placed) && !r.inPass(r.prev))
	if kept {
		for i := range r.ops {
			r.ops[i].offset = r.prevOps[i].offset
		}
		if samePlaced {
			for i := range r.placed {
				r.placed[i].hidden = r.prev[i].hidden
			}
		}
	} else {
		r.canvas.Resize(w, h)
		r.execute(false)
	}
	r.placeEntries()
	if r.top != nil || len(r.topOps) > 0 {
		kept = r.paintTop(w, h, kept)
	}
	if !kept {
		// Keeping the string when the bytes are the same lets comparing
		// it with what a view drew skip comparing the bytes.
		if out := r.canvas.Render(); out != r.out {
			r.out = out
		}
		r.drawn, r.drawnEnv = image.Pt(w, h), r.env
	}
	r.cache.EndFrame(w * h)

	clear(r.prevOps)
	clear(r.prevFxs)
	r.ops, r.prevOps = r.prevOps[:0], r.ops
	r.fxs, r.prevFxs = r.prevFxs[:0], r.fxs
	r.placed, r.prev = r.prev[:0], r.placed
	return r.out
}

// paintTop has the renderer's top draw over the frame of w×h, whose own ops
// were the last frame's if kept, and draws it, reporting whether the frame
// is the last one drawn. What the top draws is like a layer drawn last: it
// takes part in telling whether the frame changed, so what it drew last
// goes away when it no longer draws it, and the cursor beneath gives way to
// any other in sight.
// It adds no entries, so the mouse goes through it.
func (r *renderer) paintTop(w, h int, kept bool) bool {
	clear(r.prevTop)
	r.topOps, r.prevTop = r.prevTop[:0], r.topOps
	r.topRedrawn = false
	if r.top != nil {
		r.top(w, h)
	}
	if r.cursor != nil && covers(r.topOps, image.Pt(r.cursor.X, r.cursor.Y)) {
		// Another cursor may be in sight.
		r.pickCursor(r.topOps)
	}
	if r.topRedrawn {
		// The canvas was drawn again for the top to read, without what it
		// drew last, so what it draws now is drawn whatever it is.
		kept = false
	}
	if kept && slices.EqualFunc(r.topOps, r.prevTop, func(a, b op) bool { return a.equal(&b) }) {
		return true
	}
	if kept && r.topOnCanvas {
		// The canvas holds what the top drew last.
		r.redraw()
	}
	for i := range r.topOps {
		if o := &r.topOps[i]; o.kind == opBlit {
			r.canvas.Blit(o.src, o.rect, o.clip)
		}
	}
	r.topOnCanvas = len(r.topOps) > 0
	return false
}

// contentCanvas returns the canvas with only the frame's own cells on it,
// for the top to read what it would cover: what the top drew over them last
// is taken off by drawing them again.
func (r *renderer) contentCanvas() *canvas.Canvas {
	if r.topOnCanvas {
		r.redraw()
		r.topRedrawn = true
	}
	return r.canvas
}

// redraw draws the frame's ops onto the canvas again, without what the top
// drew over them.
func (r *renderer) redraw() {
	r.canvas.Clear()
	r.execute(true)
}

// paintTree lays out c and builds the frame's ops and entries, returning the
// root box.
func (r *renderer) paintTree(w, h int, c Child) int {
	clear(r.entries)
	clear(r.handlers)
	clear(r.tips)
	r.ops, r.entries, r.handlers, r.fxs, r.cursor = r.ops[:0], r.entries[:0], r.handlers[:0], r.fxs[:0], nil
	r.placed = r.placed[:0]
	r.tips, r.sinks = r.tips[:0], r.sinks[:0]
	r.floats, r.open = r.floats[:0], 0
	clear(r.leaves)
	clear(r.ends[:])
	r.leaves, r.rngSeq = r.leaves[:0], -1
	r.outline.Reset()
	r.selMemo.Reset()
	clear(r.prevHelds)
	r.helds, r.prevHelds = r.prevHelds[:0], r.helds
	root := r.layout(w, h, c)
	if root >= 0 {
		r.paint(root, r.area, -1)
		r.paintMoves(0)
		r.paintFloats(0)
	}
	r.styled.Retire()
	r.stretch.Retire()
	r.columns.Retire()
	return root
}

// placeEntries moves the entries drawn inside each effect pass by the offset
// its effects moved its cells, clips them to where the cells were
// composited, and then picks the cursor shown (see [renderer.pickCursor]).
func (r *renderer) placeEntries() {
	for i := range r.ops {
		o := &r.ops[i]
		if o.kind != opPop {
			continue
		}
		for j := o.entries[0]; j < o.entries[1]; j++ {
			e := &r.entries[j]
			e.rect = e.rect.Add(o.offset)
			e.vis = e.vis.Add(o.offset).Intersect(o.clip)
		}
		for j := range r.sinks {
			if s := &r.sinks[j]; s.op > o.push && s.op < i {
				s.vis = s.vis.Add(o.offset).Intersect(o.clip)
			}
		}
	}
	r.pickCursor(nil)
}

// size returns the frame size for a tree laid out in w×h.
func (r *renderer) size(root, w, h int) (width, height int) {
	if root >= 0 {
		rect := r.area
		return rect.Dx(), rect.Dy()
	}
	return max(w, 0), max(h, 0)
}

// hideUnder notes which cursors drawn inside effect pass i the effects hid,
// leaving them off surface s.
func (r *renderer) hideUnder(i int, s *canvas.Surface) {
	o := &r.ops[i]
	for j := range r.placed {
		if p := &r.placed[j]; o.push < p.op && p.op <= i {
			at := image.Pt(p.c.X, p.c.Y).Sub(o.rect.Min)
			p.hidden = p.hidden || !s.Visible(at.X, at.Y)
		}
	}
}

// inPass reports whether one of placed is drawn inside an effect pass of
// the frame, which can hide it.
func (r *renderer) inPass(placed []placedCursor) bool {
	for i := range r.ops {
		if o := &r.ops[i]; o.kind == opPop {
			for _, p := range placed {
				if o.push < p.op && p.op <= i {
					return true
				}
			}
		}
	}
	return false
}

func (r *renderer) sameOps() bool {
	if len(r.ops) != len(r.prevOps) {
		return false
	}
	for i := range r.ops {
		if !r.ops[i].equal(&r.prevOps[i]) {
			return false
		}
	}
	return true
}

// styledMemo is a text node as drawn in a box, kept since rendering with
// Lip Gloss is slow, as are truncating a long line and wrapping a long text,
// and a box tends to look the same from one frame to the next.
type styledMemo struct {
	style            lipgloss.Style
	s                string
	styled, truncate bool
	nowrap           bool
	w, h             int
	out              string
	full             string // the line a truncated node was cut from, if it was
}

// textAt returns text node n as drawn at w×h for box i, and for a
// truncated node that was cut to fit, the line it was cut from, as
// [textOf] does.
func (r *renderer) textAt(i int, n TextNode, w, h int) (out, full string) {
	if !n.styled && !n.truncate {
		if !r.wraps(i, n, w) {
			return n.s, "" // it fits, and is drawn as it is
		}
		return r.rendered(i, n, w).wrapped, "" // as measuring it wrapped it
	}
	m := r.styled.At(i)
	// Comparing through pointers to storage that outlives the call keeps
	// the styles from being copied to the heap.
	r.style = n.style
	if v := &m.V; m.OK && v.w == w && v.h == h && v.styled == n.styled && v.truncate == n.truncate && v.nowrap == n.nowrap && v.s == n.s && same.Style(&v.style, &r.style) {
		return v.out, v.full
	}
	out, full = textOf(n, w, h, r.method)
	m.OK, m.V = true, styledMemo{style: n.style, s: n.s, styled: n.styled, truncate: n.truncate, nowrap: n.nowrap, w: w, h: h, out: out, full: full}
	return out, full
}

func (r *renderer) blit(s string, rect, clip image.Rectangle) {
	var src *canvas.Canvas
	if r.parse {
		src = r.cache.Get(s)
	}
	r.ops = append(r.ops, op{kind: opBlit, src: src, rect: rect, clip: clip})
}

// blitText is [renderer.blit] for text that the program may not have made
// all of, which is sanitized unless it is trusted.
func (r *renderer) blitText(s string, trusted bool, rect, clip image.Rectangle) {
	if trusted {
		r.blit(s, rect, clip)
		return
	}
	var src *canvas.Canvas
	if r.parse {
		src = r.cache.Sanitized(s)
	}
	r.ops = append(r.ops, op{kind: opBlit, src: src, rect: rect, clip: clip})
}

// paint adds the ops and entries for box i, clipped to clip, whose nearest
// ancestor with an entry is entry parent, and the selection over it if the
// Root is selecting in it, once all of it is painted, so that what is drawn
// on top later, such as a dialog, covers the selection.
func (r *renderer) paint(i int, clip image.Rectangle, parent int) {
	if i < 0 {
		return
	}
	b := &r.boxes[i]
	selected := r.selected(b)
	r.paintBox(i, clip, parent)
	// A slot moving with effects is drawn later, by paintMoves.
	if selected && !b.moving {
		r.paintSelection(i, clip)
	}
}

// paintBox adds the ops and entries for box i, as [renderer.paint] does,
// without the selection.
func (r *renderer) paintBox(i int, clip image.Rectangle, parent int) {
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	// A slot moving with effects is drawn on its way while where it was is
	// in sight, though where it goes may not be.
	travels := b.moving && b.from.Overlaps(clip)
	if b.hidden || (vis.Empty() && !travels) {
		return
	}
	from, own := len(r.entries), -1
	if r.hasEntry(b) && !vis.Empty() && !isScrollbar(b) {
		parent = r.addEntry(i, vis, clip, parent)
		own = parent
	}
	if b.moving {
		r.moves = append(r.moves, deferredMove{box: i, clip: clip, parent: parent})
		return
	}

	switch n := b.child.(type) {
	case TextNode:
		r.paintText(i, &n, vis, clip, parent, own)
	case SlotNode, FlexNode, GridNode, WrapNode, ScrollNode, ListNode, CenterNode:
		for _, k := range r.kidsOf(i) {
			r.paint(k, vis, parent)
		}
	case BoxNode:
		if b.cramped {
			return
		}
		style, hovered := n.style, false
		if n.hover != nil && r.isHovered(b) {
			style, hovered = *n.hover, true
		}
		if border := n.bordered(b.rect); len(n.borderFx) > 0 && !border.Empty() {
			// Drawn on its own, so the effects see only the frame, with the
			// border as the outer cells of what they are given.
			push := r.push(b.rect)
			r.paintFrame(i, style, hovered, b.rect)
			r.pop(op{
				kind: opPop, rect: b.rect, clip: vis, viewport: clip, sub: border,
				t: n.borderT, effects: r.snapshot(n.borderFx),
				push: push, entries: [2]int{len(r.entries), len(r.entries)},
			})
		} else {
			r.paintFrame(i, style, hovered, vis)
		}
		// What is inside, then the children on its edges, over its border.
		for _, k := range r.kidsOf(i) {
			r.paint(k, vis, parent)
		}
	case TransitionNode:
		if len(n.effects) > 0 {
			// Its own entry, if it has one, moves with its cells.
			r.paintEffects(r.firstKid(i), b.rect, clip, n.t, n.effects, parent, from)
			return
		}
		r.paint(r.firstKid(i), vis, parent)
	case OverlayNode:
		kids := r.kidsOf(i)
		mark, floats := len(r.moves), len(r.floats)
		r.paint(kids[0], vis, parent)
		r.paintMoves(mark)
		r.paintFloats(floats)
		for _, k := range kids[1:] {
			if r.boxes[k].float {
				r.floats = append(r.floats, deferredFloat{box: k, parent: parent})
			} else {
				r.paintLayer(k, vis, parent)
			}
		}
	case DrawableNode:
		r.ops = append(r.ops, op{kind: opDraw, draw: n.d, rect: b.rect, clip: vis})
	case TooltipSinkNode:
		r.paintSink(i, vis)
	case ViewportNode:
		r.paintViewport(i, n, vis)
		if n.caret != nil {
			r.paintCaret(i, n, vis, parent)
		}
	case ScrollbarNode:
		r.paintScrollbar(i, b, n, clip, parent)
	case LayerNode:
		// Layers are painted by their overlay.
	case ViewNode:
		r.paintView(i, n, vis)
	default:
		if b.laid {
			r.paint(b.expanded, vis, parent)
		}
	}
}

// paintText adds the ops for text node n, box i, visible at vis inside
// clip, whose own entry is own, or -1 for none, with parent the nearest
// entry it is inside otherwise. A truncated node cut to fit is noted in its
// entry, which a node with an automatic tooltip is given if it has none.
func (r *renderer) paintText(i int, n *TextNode, vis, clip image.Rectangle, parent, own int) {
	b := &r.boxes[i]
	if n.hover != nil && r.isHovered(b) {
		n.style, n.styled = *n.hover, true
	}
	var text, full string
	switch {
	case r.parse:
		text, full = r.textAt(i, *n, b.rect.Dx(), b.rect.Dy())
	case n.truncate && (own >= 0 || r.autoTips):
		// Not drawn, as for Solve, but where it was cut is still known.
		full = cutFrom(*n, b.rect.Dx(), r.method)
	}
	if full != "" {
		r.noteCut(i, full, vis, clip, parent, own)
	}
	r.blitText(text, n.trusted, b.rect, vis)
	if n.cursor != nil {
		r.placeCursor(n.cursor, b.rect.Min, vis, r.rankCursor(i, parent))
	}
}

// noteCut notes that text node box i was cut from the line full, in its
// entry own, and gives it the line as its tooltip while the renderer has
// autoTips, unless it has one of its own or is inside a node that shows
// none, and is not pinned open, which shows it all the same. Without an
// entry, it is given one for its tooltip, visible at vis inside clip,
// inside entry parent.
func (r *renderer) noteCut(i int, full string, vis, clip image.Rectangle, parent, own int) {
	b := &r.boxes[i]
	auto := r.autoTips && (!b.meta.noTip || b.meta.tipOpen) && b.meta.tip == nil && !b.inert
	if own < 0 {
		if !auto || vis.Empty() {
			return
		}
		own = r.addEntry(i, vis, clip, parent)
	}
	r.entries[own].cut = true
	if auto {
		r.tips = append(r.tips, tipAt{entry: own, full: full, box: i, pinned: b.meta.tipOpen, pinOnly: b.meta.noTip})
	}
}

// paintEffects paints box i into its own cells covering rect, which start
// out blank, to be composited after effects are applied for t, clipped to
// clip, which is also the viewport effects move the cells in. The cells are
// drawn whole, as effects may move cells that start out clipped into view.
// The entries from index from on move with the cells.
func (r *renderer) paintEffects(i int, rect, clip image.Rectangle, t float64, effects []fx.Effect, parent, from int) {
	push := r.push(rect)
	mark := len(r.moves)
	r.paint(i, rect, parent)
	r.paintMoves(mark)
	r.popEffects(rect, clip, image.Rectangle{}, t, effects, push, from)
}

// popEffects ends the cells started at op push, which effects are applied to
// for t, for a node at rect, moving from moved if it is moving with effects,
// to be composited inside clip. Entries from index from on move with them.
func (r *renderer) popEffects(rect, clip, moved image.Rectangle, t float64, effects []fx.Effect, push, from int) {
	r.pop(op{
		kind: opPop, rect: rect, clip: clip, viewport: clip, t: t, from: moved,
		effects: r.snapshot(effects), push: push, entries: [2]int{from, len(r.entries)},
	})
}

// push begins an effect pass: what is drawn until its pop goes into cells
// of its own covering rect. It returns the index of its op.
func (r *renderer) push(rect image.Rectangle) int {
	r.ops = append(r.ops, op{kind: opPush, rect: rect})
	r.open++
	return len(r.ops) - 1
}

// pop ends the innermost effect pass with o.
func (r *renderer) pop(o op) {
	r.ops = append(r.ops, o)
	r.open--
}

// deferredFloat is a layer that floats, box, waiting for what it is in to
// be painted, with its nearest ancestor with an entry.
type deferredFloat struct {
	box, parent int
}

// paintFloats paints the layers that float found since index mark, once
// what they are in is painted: the layer, or the base of the overlay, that
// their overlay is in, or the whole tree. Each is painted against the whole
// screen, over the rest of what it is in, and under what is painted after
// it, such as later layers. While an effect pass is open, they wait for
// what the pass is in, as they would be cut off at the edges of its cells
// and moved by its effects.
func (r *renderer) paintFloats(mark int) {
	if r.open > 0 {
		return
	}
	// A layer is painted as what its own floats are in, which leaves them
	// painted, and r.floats as it found it.
	for k := mark; k < len(r.floats); k++ {
		f := r.floats[k]
		r.paintLayer(f.box, r.area, f.parent)
	}
	r.floats = r.floats[:mark]
}

// outside returns the parts of s outside r: above, below, left of, and right
// of it, some of them empty.
func outside(s, r image.Rectangle) [4]image.Rectangle {
	in := s.Intersect(r)
	if in.Empty() {
		return [4]image.Rectangle{s}
	}
	return [4]image.Rectangle{
		{Min: s.Min, Max: image.Pt(s.Max.X, in.Min.Y)},
		{Min: image.Pt(s.Min.X, in.Max.Y), Max: s.Max},
		{Min: image.Pt(s.Min.X, in.Min.Y), Max: image.Pt(in.Min.X, in.Max.Y)},
		{Min: image.Pt(in.Max.X, in.Min.Y), Max: image.Pt(s.Max.X, in.Max.Y)},
	}
}

// snapshot copies effects into storage the renderer owns, so that the
// previous frame's ops do not change when a caller reuses a slice of them.
func (r *renderer) snapshot(effects []fx.Effect) []fx.Effect {
	start := len(r.fxs)
	r.fxs = append(r.fxs, effects...)
	return r.fxs[start:len(r.fxs):len(r.fxs)]
}

func (r *renderer) paintLayer(i int, clip image.Rectangle, parent int) {
	b := &r.boxes[i]
	if b.hidden || b.rect.Empty() {
		return
	}
	defer r.paintFloats(len(r.floats))
	n, _ := b.child.(LayerNode)
	if len(n.backdrop) > 0 {
		r.ops = append(r.ops, op{kind: opEffects, rect: clip, viewport: clip, t: n.t, effects: r.snapshot(n.backdrop)})
	}
	if len(n.shadow) > 0 {
		// An opaque layer hides its own shadow: drawn under it, the shadow
		// would show through its cells without a background of their own.
		shadow, effects := b.rect.Add(image.Pt(n.shadowX, n.shadowY)).Intersect(clip), r.snapshot(n.shadow)
		parts := [4]image.Rectangle{shadow}
		if !n.clear {
			parts = outside(shadow, b.rect)
		}
		for _, part := range parts {
			if !part.Empty() {
				r.ops = append(r.ops, op{kind: opEffects, rect: part, viewport: clip, t: n.t, effects: effects})
			}
		}
	}
	from := len(r.entries)
	if r.hasEntry(b) {
		// A layer with effects may move; placeEntries clips it once moved.
		vis := b.rect.Intersect(clip)
		if len(n.effects) > 0 {
			vis = b.rect
		}
		parent = r.addEntry(i, vis, clip, parent)
	}

	// A layer is opaque: whatever its child does not draw, such as a Space,
	// is blank rather than showing the base through. Cells drawn for effects
	// start out blank; otherwise the layer's rect is blanked first. Either
	// way, cells without a background of their own keep the one beneath,
	// as everywhere else, so a layer looks the same moving and still.
	// A layer moving with effects is drawn into cells of its own, among the
	// layers, unlike a slot, which waits for the rest of its container.
	// Its cells are drawn whole, as its effects draw it on its way from
	// where it was, which may be in sight though where it goes is not.
	push, area := -1, clip
	if b.moving {
		push, area = r.beginTravel(b, clip), clip.Union(b.rect)
	}
	effects := n.effects
	if n.clear {
		// Blank cells are hidden before the layer's own effects, which then
		// leave them be rather than filling them in, as a fade does, and again
		// after, for cells an effect drew blank.
		if len(effects) > 0 {
			effects = slices.Concat([]fx.Effect{cutout{}}, effects, []fx.Effect{cutout{}})
		} else {
			effects = []fx.Effect{cutout{}}
		}
	}
	if len(effects) > 0 {
		r.paintEffects(r.firstKid(i), b.rect, area, n.t, effects, parent, from)
	} else {
		r.ops = append(r.ops, op{kind: opClear, rect: b.rect.Intersect(area)})
		mark := len(r.moves)
		r.paint(r.firstKid(i), b.rect.Intersect(area), parent)
		r.paintMoves(mark)
	}
	if r.selected(b) {
		r.paintSelection(i, area)
	}
	if b.moving {
		r.popEffects(b.rect, clip, b.from, b.moveT, n.moves, push, from)
	}
}

// applyEffects applies effects to s for t. Under reduced motion, those that
// move cells are left out, and a node, when fade is set, fades instead,
// after the rest, unless it fades already.
func applyEffects(s *canvas.Surface, effects []fx.Effect, t float64, fade bool) {
	reduced, faded, moved := anim.Motion() == anim.Reduced, false, false
	for _, e := range effects {
		if reduced && fx.Moves(e) {
			moved = true
			continue
		}
		_, isFade := e.(fx.Fade)
		faded = faded || isFade
		e.Apply(s, t)
	}
	if fade && moved && !faded {
		fx.Fade{}.Apply(s, t)
	}
}

// cutout hides the blank cells of a transparent layer, before and after its
// own effects, so that what is beneath shows through them.
type cutout struct{}

// Apply implements [fx.Effect].
func (cutout) Apply(s *canvas.Surface, _ float64) {
	r := s.Reach()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := s.Cell(x, y); c != nil && blankCell(c) {
				s.Hide(x, y)
			}
		}
	}
}

// blankCell reports whether c shows nothing: a space with no background,
// underline, or reverse video.
func blankCell(c *uv.Cell) bool {
	return c.Width == 1 && (c.Content == " " || c.Content == "") && c.Style.Bg == nil &&
		c.Style.Underline == 0 && c.Style.Attrs&uv.AttrReverse == 0
}

// deferredMove is a slot moving with effects, whose painting waits until
// the rest of what it is drawn with has been painted, so that its cells
// pass over its neighbors: box i, inside clip, its container's visible area,
// and its nearest ancestor with an entry.
type deferredMove struct {
	box, parent int
	clip        image.Rectangle
}

// paintMoves paints the slots moving with effects deferred since mark, each
// into its own cells covering where it was and where it is, to have its
// effects draw it on its way from one to the other.
func (r *renderer) paintMoves(mark int) {
	for k := mark; k < len(r.moves); k++ {
		mv := r.moves[k]
		b := &r.boxes[mv.box]
		n, _ := b.child.(SlotNode)
		push, from := r.beginTravel(b, mv.clip), len(r.entries)
		inner := len(r.moves)
		r.paint(r.firstKid(mv.box), b.rect, mv.parent)
		r.paintMoves(inner)
		if r.selected(b) {
			// Drawn into the slot's own cells, so it moves with them.
			r.paintSelection(mv.box, b.rect)
		}
		r.popEffects(b.rect, mv.clip, b.from, b.moveT, n.moves, push, from)
	}
	r.moves = r.moves[:mark]
}

// beginTravel starts drawing box b, moving with effects, into cells of its
// own covering where it was and where it is, inside clip, returning the
// index of the op that starts them.
func (r *renderer) beginTravel(b *box, clip image.Rectangle) int {
	return r.push(b.rect.Union(b.from).Intersect(clip).Union(b.rect))
}

// isHovered reports whether the pointer was over box b when the mouse was
// last routed.
func (r *renderer) isHovered(b *box) bool {
	return len(r.hovered) > 0 && !b.inert && containsKey(r.hovered, b.key())
}

// isDragged reports whether box b is being dragged with the mouse.
func (r *renderer) isDragged(b *box) bool {
	return r.dragged != nil && r.dragged.key == b.key()
}

// cursorRank is what a cursor was placed inside, which picks the one shown
// when the Root has a focus: a cursor inside the focused node over one
// that the focus has nothing to do with, and never one inside a node that
// could take the focus but does not have it.
type cursorRank int8

const (
	cursorUnfocused cursorRank = iota - 1 // inside a focusable node not focused, or leaving
	cursorFree                            // inside no focusable node the focus moves to
	cursorFocused                         // inside the focused node
)

// placedCursor is a cursor painted, which [renderer.pickCursor] shows if
// nothing hides it and the focus prefers it.
type placedCursor struct {
	c      tea.Cursor // in frame cells, before effects move it
	op     int        // index of the op drawn after it was placed
	rank   cursorRank
	hidden bool // an effect pass it is drawn inside hid its cell
}

// placeCursor places c, relative to origin, if that cell is inside clip,
// and the focus allows it, as rank tells (see [renderer.rankCursor]).
// Without a focus, only the cursor drawn last can be shown.
func (r *renderer) placeCursor(c *tea.Cursor, origin image.Point, clip image.Rectangle, rank cursorRank) {
	at := origin.Add(image.Pt(c.X, c.Y))
	if !at.In(clip) || rank == cursorUnfocused {
		return
	}
	if r.focus == nil {
		r.placed = r.placed[:0]
	}
	cur := *c
	cur.X, cur.Y = at.X, at.Y
	r.placed = append(r.placed, placedCursor{c: cur, op: len(r.ops), rank: rank})
}

// pickCursor shows the cursor placed that is on screen once effects moved
// it, and not hidden by them or covered by what is drawn after it, such as
// a dialog, or by over: of those, the one inside the focused node, or the
// one drawn last.
func (r *renderer) pickCursor(over []op) {
	r.cursor = nil
	best := cursorUnfocused // no cursor placed has this rank
	for j := len(r.placed) - 1; j >= 0 && best < cursorFocused; j-- {
		p := &r.placed[j]
		if p.hidden || p.rank <= best {
			continue
		}
		if at, ok := r.onScreen(p); ok && !covers(over, at) {
			cur := p.c
			cur.X, cur.Y = at.X, at.Y
			r.cursor, best = &cur, p.rank
		}
	}
}

// onScreen returns where cursor p is once the effect passes it is drawn
// inside moved it, and reports whether it is still inside them and not
// covered by what is drawn after it.
func (r *renderer) onScreen(p *placedCursor) (image.Point, bool) {
	at := image.Pt(p.c.X, p.c.Y)
	for i := range r.ops {
		if o := &r.ops[i]; o.kind == opPop && o.push < p.op && p.op <= i {
			if at = at.Add(o.offset); !at.In(o.clip) {
				return at, false
			}
		}
	}
	return at, !covers(r.ops[p.op:], at)
}

// rankCursor ranks a cursor drawn by box i, inside entry parent, by the
// nearest focusable node it is in that the Root's focus moves to: the
// cursor is that node's, shown only while it is focused. One in something
// animating out, which takes no input, is never shown. Without a focus,
// every cursor is free, and the one drawn last is shown.
func (r *renderer) rankCursor(i, parent int) cursorRank {
	if r.focus == nil {
		return cursorFree
	}
	if r.boxes[i].inert {
		return cursorUnfocused
	}
	for j := parent; j >= 0; j = r.entries[j].parent {
		e := &r.entries[j]
		if !e.focusable {
			continue
		}
		if !e.id.IsZero() {
			if r.focus.Is(e.id) {
				return cursorFocused
			}
			return cursorUnfocused
		}
	}
	return cursorFree
}

// covers reports whether one of ops draws over cell at.
func covers(ops []op, at image.Point) bool {
	for _, o := range ops {
		if o.kind != opPop && o.kind != opEffects && o.kind != opSelect && at.In(o.rect.Intersect(o.clip)) {
			return true
		}
	}
	return false
}

// target is where ops draw: the frame, or the cells of an opPush.
type target struct {
	c      *canvas.Canvas
	origin image.Point // frame position of the target's top-left cell
}

// execute draws the frame's ops onto the canvas. Drawing them again, once
// [renderer.placeEntries] has moved the cursor to where it is on screen,
// leaves whether an effect hid it as it was found.
func (r *renderer) execute(again bool) {
	r.topOnCanvas = false
	r.targets = append(r.targets[:0], target{c: r.canvas})
	for i := range r.ops {
		o := &r.ops[i]
		t := r.targets[len(r.targets)-1]
		switch o.kind {
		case opBlit:
			t.c.Blit(o.src, o.rect.Sub(t.origin), o.clip.Sub(t.origin))
		case opStretch:
			t.c.BlitStretched(o.src, o.mid, o.rect.Sub(t.origin), o.clip.Sub(t.origin))
		case opClear:
			t.c.Blank(o.rect.Sub(t.origin))
		case opSelect:
			r.executeSelect(o, t)
		case opDraw:
			scr := t.c.Screen(o.rect.Min.Sub(t.origin), o.rect.Size(), o.clip.Sub(t.origin))
			o.draw.Draw(scr, scr.Bounds())
		case opEffects:
			s := t.c.Region(o.rect.Sub(t.origin), r.env, &r.mixer)
			s.Viewport = o.viewport.Sub(t.origin)
			applyEffects(s, o.effects, o.t, false)
		case opPush:
			depth := len(r.targets) - 1
			if depth >= len(r.scratch) {
				r.scratch = append(r.scratch, canvas.New(0, 0, r.method))
			}
			sc := r.scratch[depth]
			sc.Resize(o.rect.Dx(), o.rect.Dy())
			r.targets = append(r.targets, target{c: sc, origin: o.rect.Min})
		case opPop:
			r.targets = r.targets[:len(r.targets)-1]
			parent := r.targets[len(r.targets)-1]
			// Its cells cover what was pushed, which for a slot moving with
			// effects is more than its rect: where it was, too.
			node := o.rect.Sub(r.ops[o.push].rect.Min)
			s := t.c.TravelLayer(node, o.rect.Sub(parent.origin), o.viewport.Sub(parent.origin), parent.c, r.env, &r.mixer)
			if !o.from.Empty() {
				s.From = o.from.Sub(parent.origin)
			}
			applied := s
			if !o.sub.Empty() {
				applied = s.Sub(o.sub.Sub(o.rect.Min))
			}
			applyEffects(applied, o.effects, o.t, true)
			parent.c.Composite(s, o.clip.Sub(parent.origin))
			o.offset = s.Offset
			if !again {
				r.hideUnder(i, s)
			}
		}
	}
}
