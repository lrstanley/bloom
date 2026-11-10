// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"cmp"
	"image"
	"slices"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/flex"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/internal/motion"
	"github.com/lrstanley/bloom/on"
)

// box is a node of the tree being laid out, kept in the engine's arena and
// referred to by index, so a frame's tree costs no allocations once the
// arena has grown to fit it. What a box does is decided by the type of its
// child: one of this package's nodes, a [Layouter], or any other component.
type box struct {
	child    Child
	meta     meta
	modal    bool
	parent   int
	kidsOff  int
	kidsLen  int
	expanded int    // root of a Layouter's subtree, or -1
	path     uint64 // where the box is in the tree, kept from frame to frame
	slot     int    // where the box is in its parent's children

	view   string // a component's View, called once per frame
	viewOK bool

	// memo is the last measurement, since a child is often measured with
	// the same space more than once in a frame.
	memoAvail   Size
	memoSize    Size
	memoOK      bool
	expandedFor Size

	// list is what a [ListNode] shows, and listSize its natural size, for the
	// space it was last expanded in, once listOK.
	list     ListView
	listSize Size
	listOK   bool

	// vp is the layout of a [ViewportNode], an index into the engine's vps
	// plus one, or zero before it is laid out.
	vp int

	// frame is the frame of the box's style, for a box or a styled text.
	frame   frame
	frameOK bool

	rect   image.Rectangle // set by arrange; empty for boxes not laid out
	hidden bool            // collapsed by its container
	// A slot or layer moving with effects is drawn at rect, with them
	// applied for moveT, from where it was, from.
	moving bool
	moveT  float64
	from   image.Rectangle
	// cramped marks a [BoxNode] given less space than its frame, which
	// would show a broken piece of it, such as while a panel animates closed.
	cramped bool
}

// engine measures and arranges a tree. It is reused between frames.
type engine struct {
	method     ansi.Method
	cache      *canvas.Cache
	memo       map[memoKey]Size // every measurement this frame
	boxes      []box
	kids       []int            // children of each box, in spans
	stack      []int            // children of the boxes being built
	frameMemos memos[frameMemo] // by box index, from the last layout
	textMemos  memos[textMemo]  // by twice the box index, and once more for a width
	scratch    lipgloss.Style   // for comparing styles without allocating
	tracks     []flex.Track
	ints       []int
	listUp     []listRec // items of the lists being expanded; see listWindow
	listDown   []listRec
	vps        []vpLayout      // the viewports laid out this frame
	vpRows     []vpRow         // the rows they show, in spans
	motion     *motion.Tracker // nil for pooled renderers, which keep no state between frames
	screen     func() *Frame   // the Root's frame on screen, for Context.Frame; nil outside a Root
	// moved is where the nearest animated box being arranged is on screen,
	// which the boxes in it animate relative to, so they move with it.
	moved image.Point
	area  image.Rectangle
}

// memoKey is a box measured within some space.
type memoKey struct {
	box   int
	avail Size
}

// layout builds the tree for c and lays it out in a w×h area, returning the
// index of its root box, or -1 for a nil c. A w or h of zero or less is the
// tree's natural size on that axis.
func (e *engine) layout(w, h int, c Child) int {
	// Clearing drops last frame's components and strings, which the arena's
	// storage would otherwise keep alive past a smaller frame.
	clear(e.boxes)
	clear(e.memo)
	e.boxes, e.kids = e.boxes[:0], e.kids[:0]
	e.vps, e.vpRows = e.vps[:0], e.vpRows[:0]
	defer e.frameMemos.retire()
	defer e.textMemos.retire()
	if c == nil || absent(c) {
		if e.motion != nil {
			e.motion.Begin(w, h)
			e.motion.End()
		}
		return -1
	}
	root := e.build(c, -1, 0)
	if w <= 0 || h <= 0 {
		avail := Size{W: w, H: h}
		if w <= 0 {
			avail.W = unbounded
		}
		if h <= 0 {
			avail.H = unbounded
		}
		s := e.measure(root, avail)
		w, h = s.W, s.H
	}
	e.area = image.Rect(0, 0, w, h)
	if e.motion != nil {
		e.motion.Begin(w, h)
	}
	e.arrange(root, e.area)
	if e.motion != nil {
		e.motion.End()
	}
	return root
}

// build adds a box for c, the child at index slot of box parent, and its
// children, returning its index.
func (e *engine) build(c Child, parent, slot int) int {
	i := len(e.boxes)
	var path uint64
	if parent >= 0 {
		path = e.boxes[parent].path
	}
	e.boxes = append(e.boxes, box{parent: parent, child: c, expanded: -1, path: childPath(path, slot), slot: slot})

	var kids []Child
	b := &e.boxes[i]
	switch n := c.(type) {
	case TextNode:
		b.meta = n.meta
	case SlotNode:
		b.meta, kids = n.meta, []Child{n.child}
	case FlexNode:
		b.meta, kids = n.meta, n.children
	case GridNode:
		b.meta, kids = n.meta, n.children
	case WrapNode:
		b.meta, kids = n.meta, n.children
	case BoxNode:
		b.meta, kids = n.meta, []Child{n.child}
	case transitionNode:
		kids = []Child{n.child}
	case centerNode:
		kids = []Child{n.child}
	case ScrollNode:
		kids = []Child{n.child}
	case OverlayNode:
		kids = make([]Child, 0, 1+len(n.layers))
		base := n.base
		if absent(base) {
			base = Text("")
		}
		kids = append(kids, base)
		for _, l := range n.layers {
			kids = append(kids, l)
		}
	case LayerNode:
		b.meta, b.modal, kids = n.meta, n.modal, []Child{n.child}
	case ListNode:
		// Its items are built once it knows how much space it has.
		b.meta = n.meta
	case ViewportNode:
		b.meta = n.meta
		if n.onScroll != nil {
			b.meta.handlers = with(n.handlers, []on.Handler{e.viewportScroll(n, b.key())})
		}
	default:
		if _, ok := c.(TextSource); ok {
			b.meta.selectable = true
		}
	}

	_, grid := c.(GridNode)
	start := len(e.stack)
	for j, k := range kids {
		if k == nil || absent(k) {
			continue
		}
		if _, slot := k.(SlotNode); slot && grid {
			// A slot sizes its content within a grid cell as it would in a row.
			k = Row(k)
		}
		e.stack = append(e.stack, e.build(k, i, j))
	}
	if n, ok := c.(BoxNode); ok {
		// Edge children follow what is inside, built here rather than
		// gathered into kids, which would cost an allocation per frame.
		for j, edge := range n.edges {
			e.stack = append(e.stack, e.build(edge.child, i, 1+j))
		}
	}
	e.boxes[i].kidsOff, e.boxes[i].kidsLen = len(e.kids), len(e.stack)-start
	e.kids = append(e.kids, e.stack[start:]...)
	e.stack = e.stack[:start]
	return i
}

// childPath returns the path of the child at index slot of a box at path
// parent. Slots count children that are absent, so that a child does not
// move in the tree when one before it comes and goes.
func childPath(parent uint64, slot int) uint64 {
	// The splitmix64 finalizer, which spreads paths that differ by little.
	x := parent ^ uint64(slot+1)*0x9e3779b97f4a7c15 //nolint:gosec // Wrapping is intended.
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

func (e *engine) kidsOf(i int) []int {
	b := &e.boxes[i]
	return e.kids[b.kidsOff : b.kidsOff+b.kidsLen]
}

func (e *engine) firstKid(i int) int {
	if kids := e.kidsOf(i); len(kids) > 0 {
		return kids[0]
	}
	return -1
}

// expand returns the root of the subtree l builds for avail, building it
// again only when avail changed.
func (e *engine) expand(i int, l Layouter, avail Size) int {
	if b := &e.boxes[i]; b.expanded >= 0 && b.expandedFor == avail {
		return b.expanded
	}
	root := -1
	if child := l.Layout(Context{Width: avail.W, Height: avail.H, wc: e.method == ansi.WcWidth, screen: e.screen}); child != nil {
		root = e.build(child, i, -1)
	}
	e.boxes[i].expanded, e.boxes[i].expandedFor = root, avail
	return root
}

// viewOf returns a component's View, calling it once per frame.
func (e *engine) viewOf(i int) string {
	if b := &e.boxes[i]; !b.viewOK {
		b.view, b.viewOK = b.child.View(), true
	}
	return e.boxes[i].view
}

// textOf returns what a text node shows at w×h, or at its natural size for
// a w or h of zero, measuring with method. A width or height the style sets
// itself is kept.
func textOf(n TextNode, w, h int, method ansi.Method) string {
	if n.truncate {
		if tf := n.style.GetTransform(); tf != nil {
			// The transform goes first, so what it makes is kept to one line.
			n.s, n.style = lineBreaks.Replace(tf(n.s)), n.style.UnsetTransform()
		}
		room := w
		if room <= 0 {
			room = unbounded
		}
		if inner := lineWidth(n.style, room); method.StringWidth(n.s) > inner {
			n.s = truncateLine(n.s, inner, method)
		}
	}
	if !n.styled {
		return n.s
	}
	st := n.style
	if w > 0 && st.GetWidth() == 0 {
		st = st.Width(max(w-st.GetHorizontalMargins(), 0))
	}
	if h > 0 && st.GetHeight() == 0 {
		st = st.Height(max(h-st.GetVerticalMargins(), 0))
	}
	return st.Render(n.s)
}

// measure returns the natural size of box i within avail. Every
// measurement is kept for the frame: a row or column measures its children
// in more than one space, and without it, nesting them would measure the
// leaves a number of times exponential in the depth.
func (e *engine) measure(i int, avail Size) Size {
	if i < 0 {
		return Size{}
	}
	if b := &e.boxes[i]; b.memoOK && b.memoAvail == avail {
		return b.memoSize
	}
	key := memoKey{box: i, avail: avail}
	s, ok := e.memo[key]
	if !ok {
		s = e.measureChild(i, avail)
		s = Size{W: min(max(s.W, 0), avail.W), H: min(max(s.H, 0), avail.H)}
		if e.memo == nil {
			e.memo = make(map[memoKey]Size)
		}
		e.memo[key] = s
	}
	b := &e.boxes[i]
	b.memoAvail, b.memoSize, b.memoOK = avail, s, true
	return s
}

func (e *engine) measureChild(i int, avail Size) Size {
	switch n := e.boxes[i].child.(type) {
	case TextNode:
		return e.measureText(i, n, avail)
	case FlexNode:
		return e.measureFlex(i, n, avail)
	case GridNode:
		return e.measureGrid(i, n, avail)
	case WrapNode:
		return e.flow(i, n, avail.W, image.Point{}, false)
	case BoxNode:
		f := e.frameOf(i, n.style)
		s := e.measure(e.boxContent(i, n), Size{W: max(avail.W-f.l-f.r, 0), H: max(avail.H-f.t-f.b, 0)})
		return Size{W: s.W + f.l + f.r, H: s.H + f.t + f.b}
	case ScrollbarNode:
		if n.horizontal {
			return Size{H: 1}
		}
		return Size{W: 1}
	case ScrollNode:
		return e.measure(e.firstKid(i), n.content(avail.W))
	case SlotNode, transitionNode, centerNode, OverlayNode, LayerNode:
		// A slot outside a row or column, a transition, a centered node, an
		// overlay, and a layer are as big as their first child: the slot's
		// child, or the base.
		return e.measure(e.firstKid(i), avail)
	case ListNode:
		e.expandList(i, n, avail)
		return e.boxes[i].listSize
	case ViewportNode:
		return e.measureViewport(n, avail)
	case drawableNode:
		if m, ok := n.d.(Measurer); ok {
			return m.Measure(avail)
		}
		return Size{}
	case Measurer:
		return n.Measure(avail)
	case Layouter:
		return e.measure(e.expand(i, n, avail), avail)
	default:
		w, h := e.cache.Size(e.viewOf(i))
		return Size{W: w, H: h}
	}
}

// measureText returns the natural size of a text node within avail. A
// styled node is measured from its style's frame rather than rendered,
// unless it is too wide and wraps, or its style changes it in ways only
// rendering tells.
func (e *engine) measureText(i int, n TextNode, avail Size) Size {
	w, h := e.cache.Size(n.s)
	if !n.styled {
		return Size{W: w, H: h}
	}
	// Lip Gloss's Width and Height include padding and borders, not margins.
	st, f := n.style, e.frameOf(i, n.style)
	if sw := st.GetWidth(); sw > 0 {
		w = sw + f.mx
	} else {
		w += f.l + f.r
	}
	h = max(h+f.t+f.b-f.my, st.GetHeight()) + f.my
	if n.truncate && st.GetTransform() == nil {
		// A truncated line is cut to fit rather than wrapped, so its size
		// follows from the style alone, without rendering a long line.
		if mw := st.GetMaxWidth(); mw > 0 {
			w = min(w, mw)
		}
		if mh := st.GetMaxHeight(); mh > 0 {
			h = min(h, mh)
		}
		if st.GetWidth() == 0 {
			w = min(w, avail.W)
		}
		return Size{W: w, H: h}
	}
	if st.GetWidth() > 0 || st.GetTransform() != nil || st.GetMaxWidth() > 0 || st.GetMaxHeight() > 0 {
		// The style wraps or changes the text in ways only rendering tells.
		natural := e.renderedSize(i, n, 0)
		w, h = natural.W, natural.H
	}
	if w > avail.W {
		// Too wide: rendered at the available width, where it wraps.
		return e.renderedSize(i, n, avail.W)
	}
	return Size{W: w, H: h}
}

// textMemo is the size of a text node, box i, as rendered w cells wide, or
// at its natural size for 0, in the last frame.
type textMemo struct {
	style    lipgloss.Style
	s        string
	truncate bool
	w        int
	size     Size
}

// renderedSize returns the size of text node n, box i, rendered w cells
// wide, or at its natural size for 0. Rendering with Lip Gloss is slow, and
// a long text wrapped to fit takes long enough to render that doing it every
// frame would be most of the frame, so the size is kept from frame to frame
// while the box shows the same text in the same style at the same width.
func (e *engine) renderedSize(i int, n TextNode, w int) Size {
	slot := 2 * i
	if w > 0 {
		slot++
	}
	m := e.textMemos.at(slot)
	e.scratch = n.style
	if v := &m.v; m.ok && v.w == w && v.truncate == n.truncate && v.s == n.s && sameStyle(&v.style, &e.scratch) {
		return v.size
	}
	sw, sh := e.cache.Size(textOf(n, w, 0, e.method))
	m.ok, m.v = true, textMemo{style: n.style, s: n.s, truncate: n.truncate, w: w, size: Size{W: sw, H: sh}}
	return m.v.size
}

// axes returns a size's main and cross lengths for a row, or a column when
// vertical is set, and size builds one back.
func axes(s Size, vertical bool) (main, cross int) {
	if vertical {
		return s.H, s.W
	}
	return s.W, s.H
}

func size(main, cross int, vertical bool) Size {
	if vertical {
		return Size{W: cross, H: main}
	}
	return Size{W: main, H: cross}
}

func (s Size) point() image.Point { return image.Pt(s.W, s.H) }

// slotOf returns how box i is placed in a row or column: its slot, or [Auto]
// for a child given directly.
func (e *engine) slotOf(i int) SlotNode {
	sl, _ := e.boxes[i].child.(SlotNode)
	return sl
}

// measureFlex returns the natural size of a row or column: the natural sizes
// of its children on the main axis, and on the cross axis, the largest of
// its children at the sizes they get when it is that long.
//
// Growing children share the space left by the others by weight rather
// than by their natural sizes, so they are given the least space in which
// [flex.Split] gives each of them its natural size (see [flex.Fit]).
func (e *engine) measureFlex(i int, n FlexNode, avail Size) Size {
	mainAvail, crossAvail := axes(avail, n.vertical)
	kids := e.kidsOf(i)
	main, visible := 0, 0
	// The growing children's tracks and natural sizes, borrowed from the
	// arenas. Children measured in between give back what they borrow, so
	// each stays contiguous.
	tracksAt, wantsAt := len(e.tracks), len(e.ints)
	for _, k := range kids {
		sl := e.slotOf(k)
		m := sl.track(0, mainAvail).Basis
		if sl.sizing == sizeFill || sl.natural(mainAvail) {
			m, _ = axes(e.measure(k, avail), n.vertical)
			if sl.sizing == sizePercent && sl.n > 0 {
				// The container must be big enough for the percentage of it
				// to fit the child.
				m = (m*100 + sl.n - 1) / sl.n
			}
		}
		m = max(m, sl.min)
		if sl.max > 0 {
			m = min(m, sl.max)
		}
		t := sl.track(m, mainAvail)
		if t.Grow > 0 {
			e.tracks = append(e.tracks, t)
			e.ints = append(e.ints, m)
		} else {
			main += m
		}
		if !t.Empty() {
			visible++
		}
	}
	end := len(e.ints)
	main += flex.Fit(e.ints[end:end], e.tracks[tracksAt:], e.ints[wantsAt:end])
	e.tracks, e.ints = e.tracks[:tracksAt], e.ints[:wantsAt]
	main = min(main+n.gap*max(visible-1, 0), mainAvail)

	sizes, _, mark := e.planFlex(i, n, main, crossAvail)
	cross := 0
	for j, k := range kids {
		if sizes[j] != flex.Collapsed {
			_, c := axes(e.measure(k, size(sizes[j], crossAvail, n.vertical)), n.vertical)
			cross = max(cross, c)
		}
	}
	e.ints = e.ints[:mark]
	return size(main, cross, n.vertical)
}

// planFlex shares mainAvail between the children of row or column i, and
// returns the size and offset of each. Both slices are borrowed from the
// ints arena: give them back with e.ints = e.ints[:mark] once done, after
// the children, which borrow their own, are measured or arranged.
func (e *engine) planFlex(i int, n FlexNode, mainAvail, crossAvail int) (sizes, offsets []int, mark int) {
	kids := e.kidsOf(i)
	tracksAt := len(e.tracks)
	for _, k := range kids {
		sl := e.slotOf(k)
		natural := 0
		if sl.natural(mainAvail) {
			natural, _ = axes(e.measure(k, size(mainAvail, crossAvail, n.vertical)), n.vertical)
		}
		e.tracks = append(e.tracks, sl.track(natural, mainAvail))
	}

	mark = len(e.ints)
	e.ints = slices.Grow(e.ints, 2*len(kids))[:mark+2*len(kids)]
	sizes = flex.Split(e.ints[mark:mark], mainAvail, n.gap, e.tracks[tracksAt:])
	offsets = flex.Place(e.ints[mark+len(kids):mark+len(kids)], sizes, mainAvail, n.gap, n.justify)
	e.tracks = e.tracks[:tracksAt]
	return sizes, offsets, mark
}

// arrange places box i in r, and its children in turn.
func (e *engine) arrange(i int, r image.Rectangle) {
	if i < 0 {
		return
	}
	if n, ok := e.boxes[i].child.(SlotNode); ok && e.motion != nil && n.id != nil && n.animation != nil {
		r = e.animate(i, n.id, n.animation, n.moves, r)
		defer func(at image.Point) { e.moved = at }(e.moved)
		e.moved = r.Min
	}
	e.boxes[i].rect = r
	switch n := e.boxes[i].child.(type) {
	case SlotNode, transitionNode:
		e.arrange(e.firstKid(i), r)
	case FlexNode:
		e.arrangeFlex(i, n, r)
	case GridNode:
		e.arrangeGrid(i, n, r)
	case WrapNode:
		e.flow(i, n, r.Dx(), r.Min, true)
	case centerNode:
		kid := e.firstKid(i)
		s := e.measure(kid, Size{W: r.Dx(), H: r.Dy()})
		at := r.Min.Add(image.Pt(Center.offset(s.W, r.Dx()), Center.offset(s.H, r.Dy())))
		e.arrange(kid, image.Rectangle{Min: at, Max: at.Add(s.point())})
	case BoxNode:
		// Built directly, as image.Rect would swap the edges of a frame too
		// big for r into a rectangle over the frame, rather than an empty one.
		f := e.frameOf(i, n.style)
		e.boxes[i].cramped = r.Dx() < f.l+f.r || r.Dy() < f.t+f.b
		minX, minY := min(r.Min.X+f.l, r.Max.X), min(r.Min.Y+f.t, r.Max.Y)
		e.arrange(e.boxContent(i, n), image.Rectangle{
			Min: image.Pt(minX, minY),
			Max: image.Pt(max(r.Max.X-f.r, minX), max(r.Max.Y-f.b, minY)),
		})
		e.arrangeEdges(i, n, r)
	case ScrollNode:
		// The child is laid out whole, moved by the offsets, and clipped to r
		// when painted.
		kid := e.firstKid(i)
		s := e.measure(kid, n.content(r.Dx()))
		at := image.Pt(r.Min.X-n.x, r.Min.Y-n.y)
		w := r.Dx()
		if n.horizontal {
			w = max(s.W, w)
		}
		e.arrange(kid, image.Rectangle{Min: at, Max: at.Add(image.Pt(w, max(s.H, r.Dy())))})
	case OverlayNode:
		kids := e.kidsOf(i)
		e.arrange(kids[0], r)
		for _, k := range kids[1:] {
			e.arrangeLayer(k, r)
		}
	case ListNode:
		e.arrangeList(i, n, r)
	case ViewportNode:
		e.layoutViewport(i, n, r)
	case Layouter:
		e.arrange(e.expand(i, n, Size{W: r.Dx(), H: r.Dy()}), r)
	}
}

// frameOf returns the frame of style, the style of box i. It is worked out
// once a frame, and kept from the last frame while the box at i has the same
// style, as boxes usually do.
func (e *engine) frameOf(i int, style lipgloss.Style) frame {
	b := &e.boxes[i]
	if b.frameOK {
		return b.frame
	}
	m := e.frameMemos.at(i)
	if e.scratch = style; !m.ok || !sameStyle(&m.v.style, &e.scratch) {
		m.ok, m.v = true, frameMemo{style: style, frame: frameOf(style)}
	}
	b.frame, b.frameOK = m.v.frame, true
	return m.v.frame
}

// frameMemo is the frame of the style a box had in the last frame.
type frameMemo struct {
	style lipgloss.Style
	frame frame
}

// memos keeps a value for each box, by its index, from one frame to the
// next, for boxes that tend to stay the same. Values a frame did not use are
// dropped once it is done, as they would keep what they refer to alive.
type memos[T any] struct {
	slots []memo[T]
	gen   uint64 // the frame being drawn
}

// memo is a value kept for a box, once ok.
type memo[T any] struct {
	ok   bool
	used uint64 // the frame it was last used in
	v    T
}

// at returns the memo at index i, marked used by this frame.
func (m *memos[T]) at(i int) *memo[T] {
	if i >= len(m.slots) {
		m.slots = append(m.slots, make([]memo[T], i+1-len(m.slots))...)
	}
	s := &m.slots[i]
	s.used = m.gen
	return s
}

// retire drops the memos this frame did not use, and ends it.
func (m *memos[T]) retire() {
	slots, gen := m.slots, m.gen
	for i := range slots {
		if slots[i].used != gen && slots[i].ok {
			slots[i] = memo[T]{}
		}
	}
	m.gen++
}

// boxContent returns the box of what is inside box node n at index i, or -1
// when it has none. The children on its edges come after it.
func (e *engine) boxContent(i int, n BoxNode) int {
	if kids := e.kidsOf(i); len(kids) > len(n.edges) {
		return kids[0]
	}
	return -1
}

// boxEdges returns the boxes of the children on box node n's edges, in the
// order of n.edges.
func (e *engine) boxEdges(i int, n BoxNode) []int {
	kids := e.kidsOf(i)
	return kids[len(kids)-len(n.edges):]
}

// arrangeEdges places the children on the edges of box node n at index i,
// laid out in r. See [BoxNode.Edge].
func (e *engine) arrangeEdges(i int, n BoxNode, r image.Rectangle) {
	if len(n.edges) == 0 {
		return
	}
	kids, ring, f := e.boxEdges(i, n), n.bordered(r), e.frameOf(i, n.style)
	for _, side := range [...]Direction{Top, Bottom, Left, Right} {
		if l, ok := f.edgeLine(side, ring); ok && !e.boxes[i].cramped {
			e.arrangeEdge(n, kids, side, l)
			continue
		}
		for j, edge := range n.edges {
			if edge.side == side {
				e.boxes[kids[j]].hidden = true
			}
		}
	}
}

// edgeLine is the row or column of a side of a box's border.
type edgeLine struct {
	vertical   bool
	at         int // the row, or column, of the side
	lo, hi     int // the span of it that children share
	mid        int // the middle of the side
	start, end int // the whole side, corners and all, for [BoxNode.EdgeAt]
}

// edgeLine returns the line of side of the border of a box with frame f,
// the outer cells of ring, reporting false if that side has no border.
func (f frame) edgeLine(side Direction, ring image.Rectangle) (edgeLine, bool) {
	// Children on the top and bottom are kept a cell from the corners.
	horizontal := edgeLine{
		lo: ring.Min.X + f.bl + 1, hi: ring.Max.X - f.br - 1, mid: (ring.Min.X + ring.Max.X) / 2,
		start: ring.Min.X, end: ring.Max.X,
	}
	vertical := edgeLine{
		vertical: true, lo: ring.Min.Y + f.bt, hi: ring.Max.Y - f.bb, mid: (ring.Min.Y + ring.Max.Y) / 2,
		start: ring.Min.Y, end: ring.Max.Y,
	}
	switch side { //nolint:exhaustive // A corner, or the zero value, is not a side.
	case Top:
		horizontal.at = ring.Min.Y
		return horizontal, f.bt > 0
	case Bottom:
		horizontal.at = ring.Max.Y - 1
		return horizontal, f.bb > 0
	case Left:
		vertical.at = ring.Min.X
		return vertical, f.bl > 0
	case Right:
		vertical.at = ring.Max.X - 1
		return vertical, f.br > 0
	}
	return edgeLine{}, false
}

// arrangeEdge places the children of box node n on side, whose line is l,
// given their boxes, kids.
func (e *engine) arrangeEdge(n BoxNode, kids []int, side Direction, l edgeLine) {
	// The ends first, in order, from the corners inward. Children at a cell
	// of their own take no room from the rest.
	for j, edge := range n.edges {
		if edge.side != side {
			continue
		}
		if edge.exact {
			at, length := edge.at, l.end-l.start
			if at < 0 {
				at += length
			}
			if at < 0 || at >= length {
				e.boxes[kids[j]].hidden = true
				continue
			}
			e.placeEdge(kids[j], l, l.start+at, e.edgeSize(kids[j], l, length-at))
			continue
		}
		if edge.align == Center {
			continue
		}
		s := l.hi - l.lo
		if edge.align != Stretch {
			s = e.edgeSize(kids[j], l, s)
		}
		at := l.lo
		if edge.align == End {
			at = l.hi - s
		}
		if !e.placeEdge(kids[j], l, at, s) {
			continue
		}
		if edge.align == End {
			l.hi -= s + 1
		} else {
			l.lo += s + 1
		}
	}

	// Then the middle, sized in order from what is left, and centered on the
	// side as far as the ends allow. Measuring again in the same space is
	// free.
	total := -1
	for j, edge := range n.edges {
		if edge.side == side && !edge.exact && edge.align == Center {
			if s := e.edgeSize(kids[j], l, l.hi-l.lo-total-1); s > 0 {
				total += s + 1
			}
		}
	}
	at, used := min(max(l.mid-total/2, l.lo), l.hi-total), -1
	for j, edge := range n.edges {
		if edge.side == side && !edge.exact && edge.align == Center {
			s := e.edgeSize(kids[j], l, l.hi-l.lo-used-1)
			if e.placeEdge(kids[j], l, at, s) {
				at, used = at+s+1, used+s+1
			}
		}
	}
}

// edgeSize returns how much of room along line l box k takes.
func (e *engine) edgeSize(k int, l edgeLine, room int) int {
	main, _ := axes(e.measure(k, size(max(room, 0), 1, l.vertical)), l.vertical)
	return main
}

// placeEdge arranges box k at at along line l, size cells long, or hides it
// for a size of zero, reporting whether it was placed.
func (e *engine) placeEdge(k int, l edgeLine, at, size int) bool {
	switch {
	case size <= 0:
		e.boxes[k].hidden = true
		return false
	case l.vertical:
		e.arrange(k, image.Rect(l.at, at, l.at+1, at+size))
	default:
		e.arrange(k, image.Rect(at, l.at, at+size, l.at+1))
	}
	return true
}

func (e *engine) arrangeFlex(i int, n FlexNode, r image.Rectangle) {
	mainAvail, crossAvail := axes(Size{W: r.Dx(), H: r.Dy()}, n.vertical)
	sizes, offsets, mark := e.planFlex(i, n, mainAvail, crossAvail)
	for j, k := range e.kidsOf(i) {
		if sizes[j] == flex.Collapsed {
			e.boxes[k].hidden = true
			continue
		}
		sl := e.slotOf(k)
		align := n.align
		if sl.aligned {
			align = sl.align
		}
		cross, crossAt := crossAvail, 0
		if align != Stretch {
			_, cross = axes(e.measure(k, size(sizes[j], crossAvail, n.vertical)), n.vertical)
			crossAt = align.offset(cross, crossAvail)
		}
		at := r.Min.Add(size(offsets[j], crossAt, n.vertical).point())
		e.arrange(k, image.Rectangle{Min: at, Max: at.Add(size(sizes[j], cross, n.vertical).point())})
	}
	e.ints = e.ints[:mark]
}

// arrangeLayer places a layer of an overlay laid out in r.
func (e *engine) arrangeLayer(i int, r image.Rectangle) {
	n, _ := e.boxes[i].child.(LayerNode)
	area := r.Inset(n.margin)
	kid := e.firstKid(i)

	w, h := max(n.w, 0), max(n.h, 0)
	if w == 0 || h == 0 {
		s := e.measure(kid, Size{W: area.Dx(), H: area.Dy()})
		w, h = cmp.Or(w, s.W), cmp.Or(h, s.H)
	}
	w, h = min(w, area.Dx()), min(h, area.Dy())

	at := area.Min.Add(image.Pt(n.x.offset(w, area.Dx()), n.y.offset(h, area.Dy())))
	if n.anchor != nil {
		target, ok := e.rectOf(n.anchor)
		if !ok {
			e.boxes[i].hidden = true
			return
		}
		// The layer goes beyond the side or corner, against the way from it
		// across the node, and lines up with the node's start otherwise.
		at = target.Min
		dx, dy := cmp.Or(n.side, Bottom).Vector()
		switch {
		case dx < 0:
			at.X = target.Max.X
		case dx > 0:
			at.X = target.Min.X - w
		}
		switch {
		case dy < 0:
			at.Y = target.Max.Y
		case dy > 0:
			at.Y = target.Min.Y - h
		}
		at.X = min(max(at.X, area.Min.X), area.Max.X-w)
		at.Y = min(max(at.Y, area.Min.Y), area.Max.Y-h)
	}
	at = at.Add(image.Pt(n.offsetX, n.offsetY))

	lr := image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}
	if e.motion != nil && n.id != nil && n.animation != nil {
		lr = e.animate(i, n.id, n.animation, n.moves, lr)
		defer func(at image.Point) { e.moved = at }(e.moved)
		e.moved = lr.Min
	}
	e.boxes[i].rect = lr
	e.arrange(kid, lr)
}

// animate returns where box i, with ID id, is drawn on its way to target:
// gliding there, or, moving with effects, at target, with its move noted on
// the box for painting. A box still gliding finishes its glide first.
func (e *engine) animate(i int, id any, cfg *layoutAnimation, moves []fx.Effect, target image.Rectangle) image.Rectangle {
	if len(moves) > 0 && !e.motion.Gliding(id) {
		b := &e.boxes[i]
		b.from, b.moveT, b.moving = e.motion.Move(id, cfg.opts, target, e.moved)
		return target
	}
	return e.motion.Rect(id, cfg.opts, target, e.moved)
}

// rectOf returns the rectangle of the box with ID id, if some of it is on
// screen: laid out, and not clipped away by the boxes it is in.
func (e *engine) rectOf(id any) (image.Rectangle, bool) {
	if i := e.shown(id); i >= 0 {
		return e.boxes[i].rect, true
	}
	return image.Rectangle{}, false
}

// shown returns the index of the box with ID id, if some of it is on screen,
// or -1.
func (e *engine) shown(id any) int {
	for i := range e.boxes {
		b := &e.boxes[i]
		if b.meta.id != id || b.hidden {
			continue
		}
		vis := b.rect
		for p := b.parent; p >= 0 && !vis.Empty(); p = e.boxes[p].parent {
			vis = vis.Intersect(e.boxes[p].rect)
		}
		if !vis.Empty() {
			return i
		}
	}
	return -1
}
