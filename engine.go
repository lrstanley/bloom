// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"hash/maphash"
	"image"
	"slices"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/flex"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/internal/linebuf"
	"bloomfx.dev/bloom/internal/memo"
	"bloomfx.dev/bloom/internal/motion"
	"bloomfx.dev/bloom/internal/same"
	"bloomfx.dev/bloom/on"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// box is a node of the tree being laid out, kept in the engine's arena and
// referred to by index, so a frame's boxes cost no allocations once the
// arena has grown to fit them. What a box does is decided by the type of its
// child: one of this package's nodes, or a component laid out by its Layout.
type box struct {
	child    Child
	meta     meta
	modal    bool
	float    bool // a layer placed and drawn against the screen; see LayerNode.Float
	inert    bool // inside something a presence is animating out; see renderer.addEntry
	parent   int
	kidsOff  int
	kidsLen  int
	expanded int    // root of a component's subtree, or -1
	laid     bool   // a component, laid out by calling its Layout
	path     uint64 // where the box is in the nearest box with an ID, or in the tree, kept from frame to frame
	slot     int    // where the box is in its parent's children

	view   string // a component's View, called once per frame
	viewOK bool

	// memo is the last measurement, since a child is often measured with
	// the same space more than once in a frame.
	memoAvail   Size
	memoSize    Size
	memoOK      bool
	expandedFor Size
	expandedIn  Direction // the side its layer went on, as in Context.Side
	// space is the space a component's subtree, expanded, was laid out
	// in, as its Context reads it, and the sizes it read, including those
	// read later, such as by the item builder of a list it returned. It is
	// nil until the component is laid out.
	space *laidSpace

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

	// textW is the natural width of a text node's string, once measured, so
	// that drawing it does not measure a long text again; 0 until then.
	textW int

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
	kids       []int                         // children of each box, in spans
	stack      []int                         // children of the boxes being built
	frameMemos memo.Table[frameMemo]         // by box index, from the last layout
	textMemos  memo.Map[textWidth, textMemo] // by box and the width it was rendered at
	scratch    lipgloss.Style                // for comparing styles without allocating
	tracks     []flex.Track
	ints       []int
	apart      bool      // it builds the items of lists apart from a frame; see itemApart
	listUp     []listRec // items of the lists being expanded; see listWindow
	listDown   []listRec
	vps        []vpLayout        // the viewports laid out this frame
	vpRows     []linebuf.RowSpan // the rows they show, in spans
	motion     *motion.Tracker   // nil for pooled renderers, which keep no state between frames
	screen     func() *Frame     // the Root's frame on screen, for Context.Frame; nil outside a Root
	lastFrame  func() *Frame     // the Root's frame drawn last, which the screen can be behind; nil outside a Root
	// seeds counts the boxes this layout looked up in the last frame, to
	// start animating from where they were then, and seedAt is where the
	// last frame drew each ID, once more than scanSeeds are looked up. The
	// next layout drops it, as a frame many boxes start animating in at once
	// is rare.
	seeds  int
	seedAt map[ID]image.Rectangle
	// moved is where the nearest animated box being arranged is on screen,
	// which the boxes in it animate relative to, so they move with it.
	moved image.Point
	area  image.Rectangle
	// side is the side of its node that the layer being arranged went on,
	// for the Context of the components in it, or zero.
	side Direction
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
	e.seeds, e.seedAt = 0, nil
	defer e.frameMemos.Retire()
	defer e.textMemos.Retire()
	root := e.tree(c)
	if root < 0 {
		if e.motion != nil {
			e.motion.Begin(w, h)
			e.motion.End()
		}
		return -1
	}
	if w <= 0 || h <= 0 {
		s := e.measure(root, space(w, h))
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

// tree drops the last tree and builds the one for c, returning the index of
// its root box, or -1 for a nil c.
func (e *engine) tree(c Child) int {
	// Clearing drops last frame's components and strings, which the arena's
	// storage would otherwise keep alive past a smaller frame.
	clear(e.boxes)
	clear(e.memo)
	e.boxes, e.kids = e.boxes[:0], e.kids[:0]
	e.vps, e.vpRows = e.vps[:0], e.vpRows[:0]
	if c == nil || absent(c) {
		return -1
	}
	return e.build(c, -1, 0)
}

// space is a w×h space, unbounded on an axis of zero or less.
func space(w, h int) Size {
	if w <= 0 {
		w = unbounded
	}
	if h <= 0 {
		h = unbounded
	}
	return Size{W: w, H: h}
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
	builtin, own := nodeMeta(c, &b.meta)
	switch c.(type) {
	case ViewNode, absentNode:
		// Drawn, not laid out.
	default:
		b.laid = own || !builtin
	}
	if !b.meta.id.IsZero() {
		// The nodes in one with an ID are known by where they are in it,
		// rather than in the whole tree, so that they stay the same nodes as
		// it moves, such as a layer brought to the front: a click on one goes
		// down on one frame and comes up on the next.
		b.path = idPath(b.meta.id)
	}
	switch n := c.(type) {
	case SlotNode:
		kids = []Child{n.child}
	case FlexNode:
		kids = n.children
	case GridNode:
		kids = n.children
	case WrapNode:
		kids = n.children
	case BoxNode:
		kids = []Child{n.child}
	case TransitionNode:
		b.inert, kids = n.leaving, []Child{n.child}
	case CenterNode:
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
		b.modal, b.float, b.inert, kids = n.modal, n.float, n.leaving, []Child{n.child}
	case ViewportNode:
		if n.caret != nil {
			// Its caret is set from the mouse, as the range of an Editable is.
			b.meta.selectable = true
		}
		if n.onScroll != nil {
			b.meta.handlers = with(n.handlers, []on.Handler{e.viewportScroll(n, b.key())})
		}
	case ViewNode:
		switch n.v.(type) {
		case TextSource, TextMap:
			b.meta.selectable = true
		}
	case TextSource:
		if !builtin {
			b.meta.selectable = true
		}
	}
	if parent >= 0 && e.boxes[parent].meta.noTip {
		b.meta.noTip = true
	}
	if parent >= 0 && e.boxes[parent].inert {
		b.inert = true
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

// idSeed seeds the paths that IDs start, which are compared only within one
// process.
var idSeed = maphash.MakeSeed()

// idPath returns the path that the nodes in one with ID id are known by
// from.
func idPath(id ID) uint64 {
	return childPath(maphash.String(idSeed, string(id)), 0)
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

// expand returns the root of the subtree that component c, box i, builds
// for avail. It is built again only for a size c read from its Context that
// changed, or in a layer on another side, so a component that reads no size
// is laid out once however often it is measured, and the components in its
// subtree keep their own layouts.
func (e *engine) expand(i int, c Child, avail Size) int {
	if s := e.boxes[i].space; s != nil && e.boxes[i].expandedIn == e.side {
		read := s.reads.Load()
		if (read&readW == 0 || int(s.w.Load()) == avail.W) && (read&readH == 0 || int(s.h.Load()) == avail.H) {
			// A size not read yet is the one it is used in now, for what
			// it built to read later.
			s.w.Store(int64(avail.W))
			s.h.Store(int64(avail.H))
			return e.boxes[i].expanded
		}
	}
	s := new(laidSpace)
	s.w.Store(int64(avail.W))
	s.h.Store(int64(avail.H))
	child := c.Layout(Context{space: s, wc: e.method == ansi.WcWidth, screen: e.screen, side: e.side})
	root := -1
	if child != nil && !absent(child) {
		root = e.build(child, i, -1)
	}
	b := &e.boxes[i]
	b.expanded, b.expandedIn, b.space = root, e.side, s
	return root
}

// viewOf returns the View of box i, a [ViewNode], calling it once per
// frame.
func (e *engine) viewOf(i int) string {
	if b := &e.boxes[i]; !b.viewOK {
		if n, _ := b.child.(ViewNode); n.v != nil {
			b.view = n.v.View()
		}
		b.viewOK = true
	}
	return e.boxes[i].view
}

// textOf returns what a text node shows at w×h, or at its natural size for
// a w or h of zero, measuring with method. Lines wider than w wrap, unless
// the node is truncated or does not wrap. A width or height the style sets itself is kept.
// For a truncated node cut to fit, full is the line it was cut from, as its
// style's transform made it; it is empty otherwise.
func textOf(n TextNode, w, h int, method ansi.Method) (out, full string) {
	if n.truncate {
		var inner int
		n, inner = truncLine(n, w)
		if method.StringWidth(n.s) > inner {
			full, n.s = n.s, truncateLine(n.s, inner, method)
		}
	}
	if !n.styled {
		if w > 0 && !n.truncate && !n.nowrap {
			// Unstyled text is drawn as it is, with no style to fill its
			// box, so it is only wrapped, not padded to w×h.
			return method.Wrap(n.s, w, ""), full
		}
		return n.s, full // cut off at the edge of its box if too wide
	}
	if n.nowrap {
		n = clipLines(n, w)
	}
	st := n.style
	if w > 0 && st.GetWidth() == 0 {
		st = st.Width(max(w-st.GetHorizontalMargins(), 0))
	}
	if h > 0 && st.GetHeight() == 0 {
		st = st.Height(max(h-st.GetVerticalMargins(), 0))
	}
	return st.Render(n.s), full
}

// truncLine returns truncated node n as its line is cut, w cells wide or at
// its natural width for a w of zero: for a styled node, with its text as
// its style renders it first, as [styleText] makes it, so all of it is kept
// to one line, and the room the line has.
func truncLine(n TextNode, w int) (_ TextNode, room int) {
	if n.styled {
		t := styleText(n)
		if t.s != n.s {
			t.s = lineBreaks.Replace(t.s) // what the style added
		}
		n = t
	}
	if w <= 0 {
		w = unbounded
	}
	return n, lineWidth(n.style, w)
}

// cutFrom returns the line truncated node n is cut from when w cells wide,
// or "" when it fits, as [textOf] decides without drawing it.
func cutFrom(n TextNode, w int, method ansi.Method) string {
	n, room := truncLine(n, w)
	if method.StringWidth(n.s) > room {
		return n.s
	}
	return ""
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
	case SlotNode, TransitionNode, CenterNode, OverlayNode, LayerNode:
		// A slot outside a row or column, a transition, a centered node, an
		// overlay, and a layer are as big as their first child: the slot's
		// child, or the base.
		return e.measure(e.firstKid(i), avail)
	case ListNode:
		e.expandList(i, n, avail)
		return e.boxes[i].listSize
	case ViewportNode:
		return e.measureViewport(n, avail)
	case DrawableNode:
		if m, ok := n.d.(Measurer); ok {
			return m.Measure(avail)
		}
		return Size{}
	case TooltipSinkNode:
		return e.measureSink(i, n, avail)
	case ViewNode:
		if m, ok := n.v.(Measurer); ok {
			return m.Measure(avail)
		}
		w, h := e.cache.Size(e.viewOf(i))
		return Size{W: w, H: h}
	case Measurer:
		return n.Measure(avail)
	case MinSizeNode:
		s := e.measure(e.expand(i, n, avail), avail)
		if !n.tooSmall(avail) {
			// It fits, so it asks for at least its minimum.
			s.W, s.H = max(s.W, n.min.W), max(s.H, n.min.H)
		}
		return s
	default:
		if e.boxes[i].laid {
			return e.measure(e.expand(i, n, avail), avail)
		}
		return Size{}
	}
}

// measureText returns the natural size of a text node within avail. Text
// too wide for avail wraps, and is measured wrapped, unless it is
// truncated or does not wrap. A styled node is measured from its style's frame rather than
// rendered, unless it wraps or its style changes it in ways only rendering
// tells.
func (e *engine) measureText(i int, n TextNode, avail Size) Size {
	w, h, tab := e.cache.SizeTab(n.s)
	e.boxes[i].textW = w
	if !n.styled {
		if w > avail.W && avail.W > 0 && !n.truncate && !n.nowrap {
			// Too wide: measured wrapped to the available width.
			return e.rendered(i, n, avail.W).size
		}
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
	// Tabs drawn as spaces, a string the style holds, a transform, and an
	// inline style, which joins the lines of text not truncated to one
	// already, change the text in ways only rendering tells.
	changed := tab && tabWidth(st) >= 0 || st.GetTransform() != nil || st.Value() != "" || st.GetInline() && !n.truncate
	if (n.truncate || n.nowrap) && !changed {
		// Truncated or unwrapped lines are cut to fit rather than wrapped,
		// so its size follows from the style alone, without rendering a
		// long text.
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
	if changed || st.GetWidth() > 0 || st.GetMaxWidth() > 0 || st.GetMaxHeight() > 0 {
		// The style changes the text, or wraps it to a width of its own.
		natural := e.rendered(i, n, 0).size
		w, h = natural.W, natural.H
	}
	if w > avail.W {
		// Too wide: rendered at the available width, where it wraps.
		return e.rendered(i, n, avail.W).size
	}
	return Size{W: w, H: h}
}

// textWidth is a text node's box, and a width it was rendered at, or 0 for
// its natural size.
type textWidth struct{ box, w int }

// textMemo is the size of a text node as rendered at a width in the last
// frame, and for an unstyled node, the text as wrapped to it.
type textMemo struct {
	style            lipgloss.Style
	s                string
	styled, truncate bool
	nowrap           bool
	size             Size
	wrapped          string
}

// rendered returns the memo of text node n, box i, rendered w cells wide,
// or at its natural size for 0. Rendering with Lip Gloss is slow, and a
// long text wrapped to fit, styled or not, takes long enough to wrap that
// doing it every frame would be most of the frame, so it is kept from frame
// to frame while the box shows the same text in the same style, at each
// width that the last frame measured or drew it at: a row can measure a
// child at more than one width in a frame.
func (e *engine) rendered(i int, n TextNode, w int) *textMemo {
	m := e.textMemos.At(textWidth{box: i, w: w})
	e.scratch = n.style
	if v := &m.V; m.OK && v.styled == n.styled && v.truncate == n.truncate && v.nowrap == n.nowrap && v.s == n.s && same.Style(&v.style, &e.scratch) {
		return v
	}
	out, _ := textOf(n, w, 0, e.method)
	sw, sh := e.cache.Size(out)
	m.OK, m.V = true, textMemo{style: n.style, s: n.s, styled: n.styled, truncate: n.truncate, nowrap: n.nowrap, size: Size{W: sw, H: sh}}
	if !n.styled {
		// Kept for drawing it. A styled node is drawn at its height too, so
		// what it was measured as is not what is drawn.
		m.V.wrapped = out
	}
	return &m.V
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
	if n, ok := e.boxes[i].child.(SlotNode); ok && e.motion != nil && !n.id.IsZero() && n.animation != nil {
		r = e.animate(i, n.id, n.animation, n.moves, r)
		defer func(at image.Point) { e.moved = at }(e.moved)
		e.moved = r.Min
	}
	e.boxes[i].rect = r
	switch n := e.boxes[i].child.(type) {
	case SlotNode, TransitionNode:
		e.arrange(e.firstKid(i), r)
	case FlexNode:
		e.arrangeFlex(i, n, r)
	case GridNode:
		e.arrangeGrid(i, n, r)
	case WrapNode:
		e.flow(i, n, r.Dx(), r.Min, true)
	case CenterNode:
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
	default:
		if e.boxes[i].laid {
			e.arrange(e.expand(i, n, Size{W: r.Dx(), H: r.Dy()}), r)
		}
	}
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

// animate returns where box i, with ID id, is drawn on its way to target:
// gliding there, or, moving with effects, at target, with its move noted on
// the box for painting. A box still gliding finishes its glide first. One
// that was laid out last frame without an animation, such as a window that
// followed the pointer while it was dragged, starts from where it was then.
func (e *engine) animate(i int, id ID, cfg *layoutAnimation, moves []fx.Effect, target image.Rectangle) image.Rectangle {
	if !e.motion.Has(string(id)) {
		if at, ok := e.lastRect(id); ok {
			e.motion.Start(string(id), cfg.opts, at, e.moved)
		}
	}
	if len(moves) > 0 && !e.motion.Gliding(string(id)) {
		b := &e.boxes[i]
		b.from, b.moveT, b.moving = e.motion.Move(string(id), cfg.opts, target, e.moved)
		return target
	}
	return e.motion.Rect(string(id), cfg.opts, target, e.moved)
}

// scanSeeds is how many boxes a layout looks up in the last frame by
// scanning it, before it indexes the frame for the rest.
const scanSeeds = 8

// lastRect returns where the last frame drew the box with ID id. The first
// few boxes a layout looks up scan the frame, and the rest share an index of
// it, so that many boxes starting to animate at once, such as a long list
// turning Animate on, each take a lookup rather than a scan.
func (e *engine) lastRect(id ID) (image.Rectangle, bool) {
	var f *Frame
	if e.lastFrame != nil {
		f = e.lastFrame()
	}
	if f == nil {
		return image.Rectangle{}, false
	}
	e.seeds++
	if e.seeds <= scanSeeds {
		return f.Rect(id)
	}
	if e.seedAt == nil {
		e.seedAt = make(map[ID]image.Rectangle)
		// Backward, so that the first of a duplicated ID is the one, as for
		// Frame.Rect.
		for i := len(f.entries) - 1; i >= 0; i-- {
			if en := &f.entries[i]; !en.id.IsZero() {
				e.seedAt[en.id] = en.rect
			}
		}
	}
	at, ok := e.seedAt[id]
	return at, ok
}
