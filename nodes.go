// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

//go:generate go run ./internal/gen

import (
	"image"
	"slices"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/flex"
	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// sizing is how a slot takes part in sharing a row's or column's main axis.
type sizing uint8

const (
	sizeAuto sizing = iota
	sizeFixed
	sizeFill
	sizePercent
)

// layoutAnimation is the animation of a slot or layer when layout changes,
// with the options of its values. Nodes keep a pointer to one, nil unless
// Animate was called, as Animate with no options has no options to tell.
type layoutAnimation struct{ opts []anim.Option }

// SlotNode places a child in a [Row] or [Column] with a sizing rule. Build
// one with [Fixed], [Fill], [Grow], [Percent], or [Auto]. A child given to
// a row or column directly is placed as with [Auto].
//
// Outside a row, column, or [Grid], such as in a [List] item or a [Box], a
// slot's sizing rule does nothing: it takes the space its parent gives it and
// passes it to its child. That makes Auto(child) a way to give any child an
// ID, focus, mouse handlers, or a tooltip without changing how it is laid
// out.
type SlotNode struct {
	meta

	child     Child
	sizing    sizing
	n         int // cells for Fixed, weight for Fill, percent for Percent
	min, max  int
	collapse  int
	align     Align
	aligned   bool
	animation *layoutAnimation
	moves     []fx.Effect
}

// Fixed places child at exactly n cells on the main axis. A nil child is
// empty space, such as Fixed(1, nil) for a blank line in a column; [Space]
// is empty space that grows to fill instead.
func Fixed(n int, child Child) SlotNode { return SlotNode{child: child, sizing: sizeFixed, n: n} }

// Fill places child in an equal share of the space left by its siblings.
func Fill(child Child) SlotNode { return Grow(1, child) }

// Grow places child in a share of the space left by its siblings, weighted
// against the other growing children's weights.
func Grow(weight int, child Child) SlotNode {
	return SlotNode{child: child, sizing: sizeFill, n: weight}
}

// Percent places child at p percent of its container's main axis. It can
// shrink when space runs short.
func Percent(p int, child Child) SlotNode {
	return SlotNode{child: child, sizing: sizePercent, n: p}
}

// Auto places child at its natural size. It can shrink when space runs
// short, down to its Min, which is zero unless set.
func Auto(child Child) SlotNode { return SlotNode{child: child} }

// Space is an empty slot that fills the space left by its siblings.
func Space() SlotNode { return Fill(nil) }

// Min sets the smallest size the slot shrinks to on the main axis.
func (n SlotNode) Min(cells int) SlotNode { n.min = cells; return n }

// Max sets the largest size the slot grows to on the main axis.
func (n SlotNode) Max(cells int) SlotNode { n.max = cells; return n }

// Collapsible lets the slot be dropped, with the gap beside it, when its
// container cannot fit the minimum sizes of all its children. Slots with
// the lowest priority are dropped first.
func (n SlotNode) Collapsible(priority int) SlotNode { n.collapse = max(priority, 1); return n }

// Align overrides the container's cross-axis alignment for this slot.
func (n SlotNode) Align(a Align) SlotNode { n.align, n.aligned = a, true; return n }

// Animate smoothly moves and resizes the slot when layout changes, reflowing
// its child at each intermediate size. It needs an ID and a retained [Root];
// without either it does nothing. Zero-size slots animate closed; collapsed
// slots and removed nodes disappear immediately. Options default to [anim.NewValue]
// and are captured when it starts animating. A slot drawn without Animate on
// the last frame glides from where it was then. Root controls the clock and FPS.
func (n SlotNode) Animate(opts ...anim.Option) SlotNode {
	n.animation = &layoutAnimation{opts: opts}
	return n
}

// MoveWith has [SlotNode.Animate] move the slot with effects rather than by
// gliding: when layout changes where it goes, it snaps there, and effects
// are applied to its cells as its animation runs from 0 to 1, with
// [canvas.Surface.From] where it was. Its cells may be drawn anywhere
// between the two, over the rest of its container, such as by [fx.Fly],
// which flies them over. A slot still gliding, such as one that was a
// dragged layer with the same ID, finishes its glide first. It does nothing
// without Animate.
func (n SlotNode) MoveWith(effects ...fx.Effect) SlotNode {
	n.moves = effects
	return n
}

// natural reports whether the slot starts at its child's natural size: an
// Auto slot, or a Percent slot in a container of unbounded size, as when a
// layout is drawn at its natural size, which has no size to take a percentage
// of.
func (n SlotNode) natural(avail int) bool {
	return n.sizing == sizeAuto || (n.sizing == sizePercent && avail >= unbounded)
}

// track returns the flex track for the slot, given its natural main-axis size
// and the container's main-axis size.
func (n SlotNode) track(natural, avail int) flex.Track {
	var t flex.Track
	switch {
	case n.natural(avail):
		t = flex.Auto(natural)
	case n.sizing == sizeFixed:
		t = flex.Fixed(n.n)
	case n.sizing == sizeFill:
		t = flex.Fill(n.n)
	default: // sizePercent
		t = flex.Auto(avail * n.n / 100)
	}
	t.Min = max(t.Min, n.min)
	if n.max > 0 {
		t.Max = n.max
	}
	t.Collapse = n.collapse
	return t
}

// FlexNode lays children out in a row or column. Build one with [Row] or
// [Column].
type FlexNode struct {
	meta

	children []Child
	vertical bool
	gap      int
	justify  flex.Justify
	align    Align
}

// Row lays children out left to right.
func Row(children ...Child) FlexNode { return FlexNode{children: children} }

// Column lays children out top to bottom.
func Column(children ...Child) FlexNode { return FlexNode{children: children, vertical: true} }

// Gap puts n cells between children.
func (n FlexNode) Gap(cells int) FlexNode { n.gap = cells; return n }

// Justify sets where children go on the main axis when none of them grow.
func (n FlexNode) Justify(j flex.Justify) FlexNode { n.justify = j; return n }

// Align sets where children go on the cross axis. The default is
// [Stretch].
func (n FlexNode) Align(a Align) FlexNode { n.align = a; return n }

// BoxNode draws a Lip Gloss style's frame, its margins, border, padding, and
// background, around a child. Build one with [Box].
type BoxNode struct {
	meta

	style    lipgloss.Style
	child    Child
	edges    []boxEdge
	borderT  float64
	borderFx []fx.Effect
}

// boxEdge is a child drawn over a side of a box's border, at an end or in
// the middle, or at a cell of its own along the side when exact.
type boxEdge struct {
	side  Direction
	align Align
	child Child
	at    int
	exact bool
}

// Box draws style's frame around child, which gets the space inside it. A box
// given less space than its frame takes is not drawn, rather than drawn
// broken.
func Box(style lipgloss.Style, child Child) BoxNode { return BoxNode{style: style, child: child} }

// Edge draws child over side of the box's border, such as a title or a
// counter on the top border, or a [Scrollbar] on the right one, without
// taking space from what is inside. side is [Top], [Bottom], [Left], or
// [Right]; a corner shows nothing. On the top and bottom, children are one
// row tall and kept a cell from the corners; on the left and right, they are
// one column wide, and a [Text] runs down the side, a character to a row.
// Children at [Start] and [End] are placed from that end inward, in the
// order given, and children at [Center] share the middle of what is left. A
// [Stretch] child takes all of what is left. Children are given no more room
// than is left, one cell apart, and one with no room is not shown. A side
// without a border shows none.
func (n BoxNode) Edge(side Direction, align Align, child Child) BoxNode {
	return n.edge(boxEdge{side: side, align: align, child: child})
}

// EdgeAt draws child over side of the box's border at cell at along it,
// counted from the top or left end of the side, where 0 is the corner, or
// back from the other end when negative, where -1 is the far corner. Use it
// to join lines drawn inside the box to its border, such as a ┬ where a
// column separator meets the top, or a ├ in place of a corner where boxes
// stack. The child is given the room from at to the end of the side, one
// row tall on the top and bottom, or one column wide on the left and right,
// where a [Text] runs down the side as with [BoxNode.Edge]. Children placed
// with EdgeAt take no room from those placed with Edge, and are drawn with
// them in the order given, so a later child is drawn over an earlier one. A
// cell outside the side, or a side without a border, shows nothing.
func (n BoxNode) EdgeAt(side Direction, at int, child Child) BoxNode {
	return n.edge(boxEdge{side: side, child: child, at: at, exact: true})
}

// edge adds e to the box's edges, with a Text on the left or right run down
// the side.
func (n BoxNode) edge(e boxEdge) BoxNode {
	if e.child == nil || absent(e.child) {
		return n
	}
	if t, ok := e.child.(TextNode); ok && (e.side == Left || e.side == Right) {
		t.s, t.truncate = stack(t.s), false
		e.child = t
	}
	n.edges = append(slices.Clip(n.edges), e)
	return n
}

// BorderFx applies effects to the box's frame for progress t, its border,
// padding, and background, but not what is inside it or on its edges. The
// effects of package [fx] made for borders, such as [fx.Chase], change only
// the border's cells. Effects that move cells do not move the frame.
func (n BoxNode) BorderFx(t float64, effects ...fx.Effect) BoxNode {
	n.borderT, n.borderFx = t, effects
	return n
}

// bordered returns the part of r, a rectangle the box was given, inside its
// margins, whose outer cells are its border.
func (n BoxNode) bordered(r image.Rectangle) image.Rectangle {
	s := n.style
	return image.Rect(r.Min.X+s.GetMarginLeft(), r.Min.Y+s.GetMarginTop(),
		r.Max.X-s.GetMarginRight(), r.Max.Y-s.GetMarginBottom()).Intersect(r)
}

// frame is the cells a Lip Gloss style's frame takes.
type frame struct {
	l, t, r, b     int // margins, border, and padding on each side
	mx, my         int // margins alone, across and down
	bl, bt, br, bb int // the border alone on each side
}

// frameOf returns the cells style's frame takes. Lip Gloss works out border
// sizes from the border's glyphs each time it is asked, so the layout keeps
// this for each box rather than asking again.
func frameOf(s lipgloss.Style) frame {
	f := frame{
		mx: s.GetHorizontalMargins(), my: s.GetVerticalMargins(),
		bl: s.GetBorderLeftSize(), bt: s.GetBorderTopSize(),
		br: s.GetBorderRightSize(), bb: s.GetBorderBottomSize(),
	}
	f.l = s.GetMarginLeft() + f.bl + s.GetPaddingLeft()
	f.t = s.GetMarginTop() + f.bt + s.GetPaddingTop()
	f.r = s.GetMarginRight() + f.br + s.GetPaddingRight()
	f.b = s.GetMarginBottom() + f.bb + s.GetPaddingBottom()
	return f
}

// Centered places child at its natural size in the middle of the space it is
// given.
func Centered(child Child) CenterNode { return CenterNode{child: child} }

// CenterNode places a child in its middle. Build one with [Centered].
type CenterNode struct {
	meta

	child Child
}

// ScrollNode shows part of a child bigger than the space it is given. Build
// one with [Scroll].
type ScrollNode struct {
	meta

	child      Child
	x, y       int
	horizontal bool
}

// Scroll gives child as much height as it wants and shows it from row
// offset, clipped to the space the scroll is given. The child is measured
// and drawn whole, so content far larger than the screen, such as a long
// log, is best windowed by the component itself.
func Scroll(child Child, offset int) ScrollNode {
	return ScrollNode{child: child, y: max(offset, 0)}
}

// X scrolls the child sideways too: it gets as much width as it wants, such
// as for lines that should not wrap, and is shown from column offset.
// Without it, the child gets the scroll's width.
func (n ScrollNode) X(offset int) ScrollNode {
	n.x, n.horizontal = max(offset, 0), true
	return n
}

// content returns the space the child of a scroll w cells wide is measured
// in: unbounded down, and across too when it scrolls sideways.
func (n ScrollNode) content(w int) Size {
	if n.horizontal {
		w = unbounded
	}
	return Size{W: w, H: unbounded}
}

// Transition draws child with effects applied to its cells for progress t,
// from 0, as it starts to enter, to 1, when it is fully shown. See
// [fx.Effect].
func Transition(t float64, child Child, effects ...fx.Effect) TransitionNode {
	return TransitionNode{child: child, t: t, effects: effects}
}

// TransitionNode draws a child with effects. Build one with [Transition] or
// [Present].
type TransitionNode struct {
	meta

	child   Child
	t       float64
	effects []fx.Effect
	leaving bool // drawn by a presence animating out, so it takes no input
}

// Present draws child with p's progress while visible. When hidden, child
// is absent, taking neither space nor a gap in its container. While p
// animates out, child is still drawn but takes no input: its mouse handlers
// do not run, and its nodes cannot be focused, selected, or show tooltips.
func Present(p anim.Presence, child Child, effects ...fx.Effect) TransitionNode {
	if !p.Visible() {
		return TransitionNode{child: absentNode{}}
	}
	n := Transition(p.Progress(), child, effects...)
	n.leaving = !p.Shown()
	return n
}

type absentNode struct{}

func (n absentNode) Layout(Context) Child { return n }

// absent distinguishes a hidden presence from an empty slot, which still
// takes space. Slots and single-child wrappers preserve absence.
func absent(c Child) bool {
	switch n := c.(type) {
	case absentNode:
		return true
	case SlotNode:
		return absent(n.child)
	case TransitionNode:
		return absent(n.child)
	case CenterNode:
		return absent(n.child)
	case ScrollNode:
		return absent(n.child)
	case LayerNode:
		return n.absent
	}
	return false
}

// DrawableNode draws a [uv.Drawable] onto the cells it is given. Build one
// with [Drawable].
type DrawableNode struct {
	meta

	d uv.Drawable
}

// Drawable returns a node that draws d directly onto the cells it is given,
// clipped to them. Its natural size is zero unless d implements [Measurer].
func Drawable(d uv.Drawable) DrawableNode { return DrawableNode{d: d} }

// OverlayNode puts layers over a base. Build one with [Overlay].
type OverlayNode struct {
	meta

	base   Child
	layers []LayerNode
}

// Overlay draws layers, in order, over base, which gets all the overlay's
// space. Layers do not take space from the base.
func Overlay(base Child, layers ...LayerNode) OverlayNode {
	if base == nil {
		base = Text("")
	}
	return OverlayNode{base: base, layers: layers}
}

// Direction is a side or a corner of a node or of the screen, such as where
// a layer is anchored to a node, or where an effect starts from.
type Direction = fx.Direction

// The sides and corners of a node or of the screen.
const (
	Bottom      = fx.Bottom
	Top         = fx.Top
	Left        = fx.Left
	Right       = fx.Right
	TopLeft     = fx.TopLeft
	TopRight    = fx.TopRight
	BottomLeft  = fx.BottomLeft
	BottomRight = fx.BottomRight
)

// LayerNode is a child of an [Overlay], such as a dialog, placed over the
// base. Build one with [Layer].
type LayerNode struct {
	meta

	child     Child
	x, y      Align
	offsetX   int
	offsetY   int
	margin    int
	w, h      int
	anchor    any
	side      Direction
	modal     bool
	float     bool
	absent    bool
	leaving   bool // drawn by a presence animating out, so it takes no input
	clear     bool
	backdrop  []fx.Effect
	shadowX   int
	shadowY   int
	shadow    []fx.Effect
	t         float64
	effects   []fx.Effect
	animation *layoutAnimation
	moves     []fx.Effect
}

// Layer returns a layer showing child at its natural size, in the middle of
// its overlay.
func Layer(child Child) LayerNode { return LayerNode{child: child, x: Center, y: Center, t: 1} }

// Anchor sets where the layer goes in its overlay on each axis.
func (n LayerNode) Anchor(x, y Align) LayerNode { n.x, n.y = x, y; return n }

// Offset moves the layer from where it is anchored.
func (n LayerNode) Offset(x, y int) LayerNode { n.offsetX, n.offsetY = x, y; return n }

// Margin keeps the layer at least cells away from the overlay's edges, or
// the screen's for a layer that floats, shrinking it if needed.
func (n LayerNode) Margin(cells int) LayerNode { n.margin = max(cells, 0); return n }

// Size sets the layer's size; zero for either is the child's natural size
// on that axis, measured with the other axis at the layer's size, so a layer
// of fixed width is as tall as its content wraps to at that width. The size
// is clamped to the overlay, or the screen for a layer that floats, less the
// margin.
func (n LayerNode) Size(w, h int) LayerNode { n.w, n.h = w, h; return n }

// Place puts the layer at r in its overlay, or on the screen for a layer
// that floats, for a layer whose place and size the model keeps, such as a
// window that is dragged around. It is Anchor(Start, Start), Offset to
// r.Min, and Size of r's, which a zero width or height of r leaves to the
// child as Size does, and it undoes AnchorTo.
func (n LayerNode) Place(r image.Rectangle) LayerNode {
	n.x, n.y, n.anchor = Start, Start, nil
	n.offsetX, n.offsetY = r.Min.X, r.Min.Y
	n.w, n.h = r.Dx(), r.Dy()
	return n
}

// AnchorTo places the layer against side of the node with ID id, such as a
// dropdown below its input, and aligned with its start, shifted to stay
// inside the overlay. A corner places the layer diagonally off that corner
// of the node, such as below it and to its right for BottomRight. The zero
// value is Bottom.
//
// A layer that does not fit on that side goes on the opposite one when it
// fits there, such as above its input near the bottom of the screen, and is
// shifted over the node otherwise. A corner flips each way on its own. A
// component in the layer reads the side it went on from [Context.Side], and
// [LayerNode.Offset] is mirrored with it, so a gap stays between the layer
// and the node.
//
// The node can be anywhere laid out before the layer: in the overlay's base,
// or, for a layer that floats, anywhere on screen. The layer is hidden while
// no such node is on screen.
func (n LayerNode) AnchorTo[K comparable](id K, side Direction) LayerNode {
	n.anchor, n.side = id, side
	return n
}

// Float places and draws the layer against the whole screen rather than the
// overlay it is in, so a component can keep its popover next to it, in its
// own overlay, without the popover being cut off at the edges of that
// overlay or of a dialog it is in:
//
//	bloom.Overlay(field, bloom.Layer(menu).AnchorTo(fieldID, bloom.Bottom).Float())
//
// It is drawn once everything else in the layer its overlay is in, such as a
// dialog, has been drawn, or in the overlay base it is in, or in the whole
// tree, so it covers the rest of that, and layers declared after it, such as
// a toast, cover it. Inside a node drawn with effects, such as a dialog
// fading in, it waits for that node, and is drawn without them. It is shown
// only while its own overlay is on screen.
//
// Mouse handlers, [on.ClickOutside], modal layers, and focus treat it as
// part of what it is declared in: a click on it is not outside the dialog
// it is in, and its focusable nodes are in that dialog's focus scope. A
// modal layer that floats takes mouse events from the whole screen.
func (n LayerNode) Float() LayerNode { n.float = true; return n }

// Modal makes the layer take every mouse event from what is beneath it.
// Clicks outside it run its [on.ClickOutside] handlers, such as to close a
// dialog when its backdrop is clicked. It is also a focus scope, for
// [Focus.Push].
func (n LayerNode) Modal() LayerNode { n.modal, n.scope = true, true; return n }

// Transparent lets what is beneath the layer show through the blank cells
// its child draws, rather than the layer covering its whole rectangle: cells
// holding a space with no background, underline, or reverse video. Use it
// for text drawn over other content, such as a banner from package fig, or
// ASCII art. A transparent layer is drawn into cells of its own each frame,
// as a layer with effects is.
func (n LayerNode) Transparent() LayerNode { n.clear = true; return n }

// Backdrop applies effects to everything drawn beneath the layer, with the
// layer's transition progress as their strength.
func (n LayerNode) Backdrop(effects ...fx.Effect) LayerNode { n.backdrop = effects; return n }

// Shadow applies effects to the frame beneath the layer, offset by x, y, the
// way a drop shadow darkens what is under it.
func (n LayerNode) Shadow(x, y int, effects ...fx.Effect) LayerNode {
	n.shadowX, n.shadowY, n.shadow = x, y, effects
	return n
}

// Transition draws the layer with effects applied to its cells for progress
// t, as with [Transition], and sets the strength of its backdrop and shadow.
func (n LayerNode) Transition(t float64, effects ...fx.Effect) LayerNode {
	n.t, n.effects = min(max(t, 0), 1), effects
	return n
}

// Presence sets the transition from p, omitting the entire layer, including
// its backdrop, shadow, and input entries, while p is not visible. While p
// animates out, the layer is still drawn but takes no input: its mouse
// handlers do not run, and its nodes cannot be focused, selected, or show
// tooltips. A modal layer still keeps events from what is beneath it until
// it is gone, unless another modal layer is beneath it, such as the next
// dialog of a stack, which then gets them.
func (n LayerNode) Presence(p anim.Presence, effects ...fx.Effect) LayerNode {
	n.absent = !p.Visible()
	n.leaving = p.Visible() && !p.Shown()
	return n.Transition(p.Progress(), effects...)
}

// Animate smoothly moves and resizes the layer when layout changes. It needs
// an ID and a retained [Root]; without either it does nothing. Options default
// to [anim.NewValue] and are captured when it starts animating. A layer drawn
// without Animate on the last frame, such as one following the pointer while
// dragged, glides from where it was then. Root controls the clock and FPS. A
// hidden anchor or removed layer disappears immediately.
func (n LayerNode) Animate(opts ...anim.Option) LayerNode {
	n.animation = &layoutAnimation{opts: opts}
	return n
}

// MoveWith has [LayerNode.Animate] move the layer with effects rather than
// by gliding, as [SlotNode.MoveWith] does a slot: it snaps to its new place,
// and effects draw it on its way there from where it was, over the layers
// before it. It does nothing without Animate.
func (n LayerNode) MoveWith(effects ...fx.Effect) LayerNode {
	n.moves = effects
	return n
}
