// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"bytes"
	"image"
	"reflect"
	"slices"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

// opKind is what an op does to the frame.
type opKind uint8

const (
	opBlit    opKind = iota // copy src into rect, blanking the rest of it
	opClear                 // blank rect, background and all
	opDraw                  // draw a uv.Drawable into rect
	opEffects               // apply effects to rect of the frame
	opPush                  // draw what follows into its own cells, for effects
	opPop                   // apply effects to those cells and composite them
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
	effects  []fx.Effect
	t        float64
	from     image.Rectangle // for opPop of a slot moving with effects: where it was

	// For opPop: the index of its opPush, the frame entries drawn since it,
	// and how far its effects moved what was drawn, known once executed.
	push         int
	entries      [2]int
	offset       image.Point
	cursorHidden bool // an effect hid the cell of the cursor drawn inside
}

func (o *op) equal(p *op) bool {
	if o.kind != p.kind || o.kind == opDraw || o.src != p.src || o.rect != p.rect ||
		o.clip != p.clip || o.viewport != p.viewport || o.sub != p.sub || o.t != p.t || o.from != p.from || len(o.effects) != len(p.effects) {
		return false
	}
	for i := range o.effects {
		if !sameEffect(o.effects[i], p.effects[i]) {
			return false
		}
	}
	return true
}

// sameStyle reports whether a and b render the same. Styles cannot be
// compared with ==, so they are compared deeply, but first by their bytes,
// which is far cheaper and matches the usual case of styles copied from the
// same one each frame. A style with a transform function never matches, as
// the function may give something else each time, and is rendered every time.
func sameStyle(a, b *lipgloss.Style) bool {
	if a.GetTransform() != nil || b.GetTransform() != nil {
		return false
	}
	size := unsafe.Sizeof(*a)
	if bytes.Equal(unsafe.Slice((*byte)(unsafe.Pointer(a)), size), unsafe.Slice((*byte)(unsafe.Pointer(b)), size)) {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// sameEffect reports whether a and b are equal, treating effects that cannot
// be compared, like an [fx.Func], as different.
func sameEffect(a, b fx.Effect) bool {
	if a == nil || b == nil {
		return a == b
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	return va.Type() == vb.Type() && fixed(va) && a == b
}

// fixed reports whether v is comparable and cannot change without changing
// v itself. An effect behind a pointer can change without the pointer
// changing, including one wrapped by another effect, such as [fx.Delay].
func fixed(v reflect.Value) bool {
	if !v.Comparable() {
		return false
	}
	k := v.Kind()
	if k == reflect.Pointer || k == reflect.UnsafePointer || k == reflect.Chan {
		return false
	}
	if k == reflect.Interface {
		return v.IsNil() || fixed(v.Elem())
	}
	if k == reflect.Struct {
		for _, f := range v.Fields() {
			if !fixed(f) {
				return false
			}
		}
	}
	if k == reflect.Array {
		for i := range v.Len() {
			if !fixed(v.Index(i)) {
				return false
			}
		}
	}
	return true
}

// renderer lays out, paints, and composites trees. It keeps what it can
// between frames: the engine's arena, parsed strings, and the last frame, so
// an unchanged frame is not composited again.
type renderer struct {
	engine

	styled   memos[styledMemo] // by box index, from the last frame
	columns  memos[columnMemo] // by twice the box index, for scrollbars
	drag     scrollbarDrag     // the scrollbar being dragged, if any
	style    lipgloss.Style    // scratch for comparing styles without allocating
	fxs      []fx.Effect       // effects of this frame's ops, copied from the tree
	prevFxs  []fx.Effect
	env      canvas.Env
	mixer    canvas.Mixer
	parse    bool // false when only the layout is wanted, as for Solve
	canvas   *canvas.Canvas
	scratch  []*canvas.Canvas
	targets  []target
	ops      []op
	prevOps  []op
	moves    []deferredMove // slots moving with effects, painted last
	entries  []entry
	handlers [][]on.Handler // by entry
	hovered  []mouseTarget  // the hoverable nodes under the pointer, kept by Root
	cursor   *tea.Cursor
	cursorAt int // index of the op drawn after the cursor was placed
	out      string
	drawn    image.Point // size of the last composited frame
	drawnEnv canvas.Env
	frame    *Frame
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

	if r.drawn != image.Pt(w, h) || !r.drawnEnv.Equal(r.env) || !r.sameOps() {
		r.canvas.Resize(w, h)
		r.execute()
		r.out = r.canvas.Render()
		r.drawn, r.drawnEnv = image.Pt(w, h), r.env
	} else {
		// The same ops have the same effects, which moved and hid things the
		// same way.
		for i := range r.ops {
			r.ops[i].offset, r.ops[i].cursorHidden = r.prevOps[i].offset, r.prevOps[i].cursorHidden
		}
	}
	r.placeEntries()
	r.cache.EndFrame(w * h)

	clear(r.prevOps)
	clear(r.prevFxs)
	r.ops, r.prevOps = r.prevOps[:0], r.ops
	r.fxs, r.prevFxs = r.prevFxs[:0], r.fxs
	return r.out
}

// paintTree lays out c and builds the frame's ops and entries, returning the
// root box.
func (r *renderer) paintTree(w, h int, c Child) int {
	clear(r.entries)
	clear(r.handlers)
	r.ops, r.entries, r.handlers, r.fxs, r.cursor = r.ops[:0], r.entries[:0], r.handlers[:0], r.fxs[:0], nil
	root := r.layout(w, h, c)
	if root >= 0 {
		r.paint(root, r.area, -1)
		r.paintMoves(0)
	}
	r.styled.retire()
	r.columns.retire()
	return root
}

// placeEntries moves the entries and cursor drawn inside each effect pass by
// the offset its effects moved its cells, clips them to where the cells were
// composited, and then hides the cursor if something drawn later covers it.
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
		if r.cursor != nil && r.cursorAt > o.push && r.cursorAt <= i {
			at := image.Pt(r.cursor.X, r.cursor.Y).Add(o.offset)
			if at.In(o.clip) && !o.cursorHidden {
				r.cursor.X, r.cursor.Y = at.X, at.Y
			} else {
				r.cursor = nil
			}
		}
	}
	r.hideCoveredCursor()
}

// size returns the frame size for a tree laid out in w×h.
func (r *renderer) size(root, w, h int) (width, height int) {
	if root >= 0 {
		rect := r.area
		return rect.Dx(), rect.Dy()
	}
	return max(w, 0), max(h, 0)
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
// Lip Gloss is slow, as is truncating a long line, and a box tends to look
// the same from one frame to the next.
type styledMemo struct {
	style    lipgloss.Style
	s        string
	truncate bool
	w, h     int
	out      string
}

// textAt returns text node n as drawn at w×h for box i.
func (r *renderer) textAt(i int, n TextNode, w, h int) string {
	if !n.styled && !n.truncate {
		return n.s
	}
	m := r.styled.at(i)
	// Comparing through pointers to storage that outlives the call keeps
	// the styles from being copied to the heap.
	r.style = n.style
	if v := &m.v; m.ok && v.w == w && v.h == h && v.truncate == n.truncate && v.s == n.s && sameStyle(&v.style, &r.style) {
		return v.out
	}
	out := textOf(n, w, h, r.method)
	m.ok, m.v = true, styledMemo{style: n.style, s: n.s, truncate: n.truncate, w: w, h: h, out: out}
	return out
}

func (r *renderer) blit(s string, rect, clip image.Rectangle) {
	var src *canvas.Canvas
	if r.parse {
		src = r.cache.Get(s)
	}
	r.ops = append(r.ops, op{kind: opBlit, src: src, rect: rect, clip: clip})
}

// paint adds the ops and entries for box i, clipped to clip, whose nearest
// ancestor with an entry is entry parent.
func (r *renderer) paint(i int, clip image.Rectangle, parent int) {
	if i < 0 {
		return
	}
	b := &r.boxes[i]
	vis := b.rect.Intersect(clip)
	// A slot moving with effects is drawn on its way while where it was is
	// in sight, though where it goes may not be.
	travels := b.moving && b.from.Overlaps(clip)
	if b.hidden || (vis.Empty() && !travels) {
		return
	}
	if hasEntry(b) && !vis.Empty() {
		parent = r.addEntry(b, vis, clip, parent)
	}
	if b.moving {
		r.moves = append(r.moves, deferredMove{box: i, clip: clip, parent: parent})
		return
	}

	switch n := b.child.(type) {
	case TextNode:
		if n.hover != nil && r.isHovered(b) {
			n.style, n.styled = *n.hover, true
		}
		var text string
		if r.parse {
			text = r.textAt(i, n, b.rect.Dx(), b.rect.Dy())
		}
		r.blit(text, b.rect, vis)
		if n.cursor != nil {
			r.placeCursor(n.cursor, b.rect.Min, vis)
		}
	case SlotNode, FlexNode, GridNode, WrapNode, ScrollNode, ListNode, centerNode:
		for _, k := range r.kidsOf(i) {
			r.paint(k, vis, parent)
		}
	case BoxNode:
		if b.cramped {
			return
		}
		style := n.style
		if n.hover != nil && r.isHovered(b) {
			style = *n.hover
		}
		var frame string
		if r.parse {
			frame = r.textAt(i, TextNode{style: style, styled: true}, b.rect.Dx(), b.rect.Dy())
		}
		if border := n.bordered(b.rect); len(n.borderFx) > 0 && !border.Empty() {
			// Drawn on its own, so the effects see only the frame, with the
			// border as the outer cells of what they are given.
			push := len(r.ops)
			r.ops = append(r.ops, op{kind: opPush, rect: b.rect})
			r.blit(frame, b.rect, b.rect)
			r.ops = append(r.ops, op{
				kind: opPop, rect: b.rect, clip: vis, viewport: clip, sub: border,
				t: n.borderT, effects: r.snapshot(n.borderFx),
				push: push, entries: [2]int{len(r.entries), len(r.entries)},
			})
		} else {
			r.blit(frame, b.rect, vis)
		}
		// What is inside, then the children on its edges, over its border.
		for _, k := range r.kidsOf(i) {
			r.paint(k, vis, parent)
		}
	case transitionNode:
		if len(n.effects) > 0 {
			r.paintEffects(r.firstKid(i), b.rect, clip, n.t, n.effects, parent, len(r.entries))
			return
		}
		r.paint(r.firstKid(i), vis, parent)
	case OverlayNode:
		kids := r.kidsOf(i)
		mark := len(r.moves)
		r.paint(kids[0], vis, parent)
		r.paintMoves(mark)
		for _, k := range kids[1:] {
			r.paintLayer(k, vis, parent)
		}
	case drawableNode:
		r.ops = append(r.ops, op{kind: opDraw, draw: n.d, rect: b.rect, clip: vis})
	case ScrollbarNode:
		r.paintScrollbar(i, b, n, clip, parent)
	case LayerNode:
		// Layers are painted by their overlay.
	case Layouter:
		r.paint(b.expanded, vis, parent)
	default:
		r.blit(r.viewOf(i), b.rect, vis)
	}
}

// paintEffects paints box i into its own cells covering rect, which start
// out blank, to be composited after effects are applied for t, clipped to
// clip, which is also the viewport effects move the cells in. The cells are
// drawn whole, as effects may move cells that start out clipped into view.
// The entries from index from on move with the cells.
func (r *renderer) paintEffects(i int, rect, clip image.Rectangle, t float64, effects []fx.Effect, parent, from int) {
	push := len(r.ops)
	r.ops = append(r.ops, op{kind: opPush, rect: rect})
	mark := len(r.moves)
	r.paint(i, rect, parent)
	r.paintMoves(mark)
	r.popEffects(rect, clip, image.Rectangle{}, t, effects, push, from)
}

// popEffects ends the cells started at op push, which effects are applied to
// for t, for a node at rect, moving from moved if it is moving with effects,
// to be composited inside clip. Entries from index from on move with them.
func (r *renderer) popEffects(rect, clip, moved image.Rectangle, t float64, effects []fx.Effect, push, from int) {
	r.ops = append(r.ops, op{
		kind: opPop, rect: rect, clip: clip, viewport: clip, t: t, from: moved,
		effects: r.snapshot(effects), push: push, entries: [2]int{from, len(r.entries)},
	})
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
	n, _ := b.child.(LayerNode)
	if len(n.backdrop) > 0 {
		r.ops = append(r.ops, op{kind: opEffects, rect: clip, viewport: clip, t: n.t, effects: r.snapshot(n.backdrop)})
	}
	if len(n.shadow) > 0 {
		shadow := b.rect.Add(image.Pt(n.shadowX, n.shadowY)).Intersect(clip)
		r.ops = append(r.ops, op{kind: opEffects, rect: shadow, viewport: clip, t: n.t, effects: r.snapshot(n.shadow)})
	}
	from := len(r.entries)
	if hasEntry(b) {
		// A layer with effects may move; placeEntries clips it once moved.
		vis := b.rect.Intersect(clip)
		if len(n.effects) > 0 {
			vis = b.rect
		}
		parent = r.addEntry(b, vis, clip, parent)
	}

	// A layer is opaque: whatever its child does not draw, such as a Space,
	// is blank rather than showing the base through. Cells drawn for effects
	// start out blank; otherwise the layer's rect is blanked first.
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
		effects = append(slices.Clip(effects), cutout{})
	}
	if len(effects) > 0 {
		r.paintEffects(r.firstKid(i), b.rect, area, n.t, effects, parent, from)
	} else {
		r.ops = append(r.ops, op{kind: opClear, rect: b.rect.Intersect(area)})
		mark := len(r.moves)
		r.paint(r.firstKid(i), b.rect.Intersect(area), parent)
		r.paintMoves(mark)
	}
	if b.moving {
		r.popEffects(b.rect, clip, b.from, b.moveT, n.moves, push, from)
	}
}

// cutout hides the blank cells of a transparent layer, after its own effects,
// so that what is beneath shows through them.
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
		r.popEffects(b.rect, mv.clip, b.from, b.moveT, n.moves, push, from)
	}
	r.moves = r.moves[:mark]
}

// beginTravel starts drawing box b, moving with effects, into cells of its
// own covering where it was and where it is, inside clip, returning the
// index of the op that starts them.
func (r *renderer) beginTravel(b *box, clip image.Rectangle) int {
	r.ops = append(r.ops, op{kind: opPush, rect: b.rect.Union(b.from).Intersect(clip).Union(b.rect)})
	return len(r.ops) - 1
}

// isHovered reports whether the pointer was over box b when the mouse was
// last routed.
func (r *renderer) isHovered(b *box) bool {
	return len(r.hovered) > 0 && containsKey(r.hovered, b.key())
}

// placeCursor shows c, relative to origin, if that cell is inside clip.
func (r *renderer) placeCursor(c *tea.Cursor, origin image.Point, clip image.Rectangle) {
	at := origin.Add(image.Pt(c.X, c.Y))
	if !at.In(clip) {
		return
	}
	cur := *c
	cur.X, cur.Y = at.X, at.Y
	r.cursor, r.cursorAt = &cur, len(r.ops)
}

// hideCoveredCursor hides the cursor when something drawn after it, such as
// a dialog, covers its cell.
func (r *renderer) hideCoveredCursor() {
	if r.cursor == nil {
		return
	}
	for _, o := range r.ops[r.cursorAt:] {
		if o.kind != opPop && o.kind != opEffects && image.Pt(r.cursor.X, r.cursor.Y).In(o.rect.Intersect(o.clip)) {
			r.cursor = nil
			return
		}
	}
}

// target is where ops draw: the frame, or the cells of an opPush.
type target struct {
	c      *canvas.Canvas
	origin image.Point // frame position of the target's top-left cell
}

// execute draws the frame's ops onto the canvas.
func (r *renderer) execute() {
	r.targets = append(r.targets[:0], target{c: r.canvas})
	for i := range r.ops {
		o := &r.ops[i]
		t := r.targets[len(r.targets)-1]
		switch o.kind {
		case opBlit:
			t.c.Blit(o.src, o.rect.Sub(t.origin), o.clip.Sub(t.origin))
		case opClear:
			t.c.ClearArea(o.rect.Sub(t.origin))
		case opDraw:
			scr := t.c.Screen(o.rect.Min.Sub(t.origin), o.rect.Size(), o.clip.Sub(t.origin))
			o.draw.Draw(scr, scr.Bounds())
		case opEffects:
			s := t.c.Region(o.rect.Sub(t.origin), r.env, &r.mixer)
			s.Viewport = o.viewport.Sub(t.origin)
			for _, e := range o.effects {
				e.Apply(s, o.t)
			}
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
			for _, e := range o.effects {
				e.Apply(applied, o.t)
			}
			parent.c.Composite(s, o.clip.Sub(parent.origin))
			o.offset = s.Offset
			if r.cursor != nil && r.cursorAt > o.push && r.cursorAt <= i {
				at := image.Pt(r.cursor.X, r.cursor.Y).Sub(o.rect.Min)
				o.cursorHidden = !s.Visible(at.X, at.Y)
			}
		}
	}
}
