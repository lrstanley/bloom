// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"

	"bloomfx.dev/bloom/on"
)

// ListNode shows as many items of a list as fit in the space it is given,
// laying out and drawing only those, however many items there are. Build one
// with [List].
type ListNode struct {
	meta

	count  int
	item   func(i int) Child
	offset int
	keep   int // -1 for none
	gap    int
}

// List returns a list of count items, top to bottom, where item returns the
// node for item i. It is called only for items that may be shown, so a list
// of a million rows costs no more to draw than the rows on screen. Each item
// is as wide as the list and as tall as it wants. The list shows items from
// the first set with [ListNode.Offset], cutting off the last one that does
// not fit, and never leaves space below its last item while there are items
// before the first that would fill it.
//
// [Frame.List] tells which items a frame showed, and [ScrollbarOf] the list's
// ID makes a scrollbar for it, in items. In space of unbounded height, such
// as inside a [Scroll], every item from the first is laid out.
func List(count int, item func(i int) Child) ListNode {
	return ListNode{count: max(count, 0), item: item, keep: -1}
}

// Offset sets the first item shown.
func (n ListNode) Offset(i int) ListNode { n.offset = i; return n }

// Keep scrolls the list as little as it can from its offset so that item i
// is shown whole, such as the item under a cursor. An item taller than the
// list is shown from its top. While it is given, the list cannot be scrolled
// away from item i: to let the wheel do that, move i along with the offset,
// or stop giving it.
func (n ListNode) Keep(i int) ListNode { n.keep = i; return n }

// Gap puts n cells between items.
func (n ListNode) Gap(cells int) ListNode { n.gap = max(cells, 0); return n }

// ID sets the ID that [Frame.Rect], [Frame.List], and [ScrollbarOf] refer to
// the list by.
func (n ListNode) ID[K comparable](id K) ListNode { n.id = id; return n }

// On adds mouse handlers to the list, made by package [on]. They take events
// on the gaps between items too, such as the wheel.
func (n ListNode) On(handlers ...on.Handler) ListNode {
	n.handlers = with(n.handlers, handlers)
	return n
}

// Selectable lets the left button select text in the list by dragging over
// it, as a terminal selects text, but kept inside the list. Holding alt as
// the drag starts selects a rectangle of cells instead. The innermost
// selectable node under the pointer takes the drag, unless a node inside it
// has an [on.Drag] handler. See [Root.Selection] for the text selected.
func (n ListNode) Selectable() ListNode { n.selectable = true; return n }

// View implements [Child], drawing the node at its natural size.
func (n ListNode) View() string { return Render(0, 0, n) }

// ListView is the part of a [List] that a frame showed, as [Frame.List]
// reports it.
type ListView struct {
	// First is the first item shown, which is where the list was scrolled to
	// once its offset was clamped and [ListNode.Keep] applied. Keep it as the
	// offset to scroll from.
	First int
	// Visible is how many items from First were shown whole, or 1 when the
	// first is too tall to be, so a page is Visible items long. A last item
	// that was cut off is not counted.
	Visible int
	// Count is how many items the list has.
	Count int
}

// listRec is an item of a list being expanded: its box, or -1 for a nil
// item, and its height.
type listRec struct{ box, h int }

// listWindow is the items of list n, box i, built while it is expanded w
// cells wide. Items are built outward from the anchor without skipping any,
// so each is kept by its distance from it: those from the anchor down the
// list in the engine's listUp arena from index up, and those before it,
// nearest first, in its listDown arena from index down. Lists inside items
// measured meanwhile use the arenas past these, and give back what they
// used.
type listWindow struct {
	i                   int
	n                   ListNode
	w, anchor, up, down int
}

// listItem returns item j of the list of win, building and measuring it the
// first time it is asked for.
func (e *engine) listItem(win *listWindow, j int) listRec {
	arena, at := &e.listUp, win.up+j-win.anchor
	if j < win.anchor {
		arena, at = &e.listDown, win.down+win.anchor-1-j
	}
	if at < len(*arena) {
		return (*arena)[at]
	}
	r := listRec{box: -1}
	if c := win.n.item(j); c != nil && !absent(c) {
		// The item's index is its slot, so it is followed from frame to frame
		// as the list scrolls, rather than where it is on screen.
		r.box = e.build(c, win.i, j)
		r.h = e.measure(r.box, Size{W: win.w, H: unbounded}).H
	}
	*arena = append(*arena, r)
	return r
}

// listSpan returns the rows item j of the list of win spans: its height and
// the gap after it, or nothing for a nil item. A run of items fits in h rows
// while their spans add up to no more than h and a gap, for the gap after the
// last.
func (e *engine) listSpan(win *listWindow, j int) int {
	if r := e.listItem(win, j); r.box >= 0 {
		return r.h + win.n.gap
	}
	return 0
}

// listFirst returns the first item the list of win shows in h rows: its
// offset, clamped, moved as little as needed to show the kept item whole, and
// moved back to fill any space below its last item.
func (e *engine) listFirst(win *listWindow, h int) int {
	n, room := win.n, h+win.n.gap
	first := min(max(n.offset, 0), n.count-1)
	if k := win.anchor; k < first {
		first = k
	} else if k > first {
		// The lowest first from which item k ends in sight, no lower than
		// where the list was.
		j, used := k, e.listSpan(win, k)
		for j > first {
			s := e.listSpan(win, j-1)
			if used+s > room {
				break
			}
			used += s
			j--
		}
		first = j
	}

	used := 0
	for j := first; j < n.count && used < room; j++ {
		used += e.listSpan(win, j)
	}
	for first > 0 && used < room {
		s := e.listSpan(win, first-1)
		if used+s > room {
			break
		}
		used += s
		first--
	}
	return first
}

// expandList works out which items of list n, box i, show in avail, and
// builds them as the box's children, building again only when avail
// changed.
func (e *engine) expandList(i int, n ListNode, avail Size) {
	b := &e.boxes[i]
	if b.listOK && b.expandedFor == avail {
		return
	}
	b.listOK, b.expandedFor, b.kidsLen = true, avail, 0
	b.list, b.listSize = ListView{Count: n.count}, Size{}
	if n.count == 0 || avail.H <= 0 {
		return
	}

	win := listWindow{
		i: i, n: n, w: avail.W, anchor: min(max(n.offset, 0), n.count-1),
		up: len(e.listUp), down: len(e.listDown),
	}
	if n.keep >= 0 && n.keep < n.count {
		win.anchor = n.keep
	}
	first := e.listFirst(&win, avail.H)

	// The items in sight become the box's children, in a span of their own.
	view, natural, start := ListView{First: first, Count: n.count}, Size{}, len(e.stack)
	for j, y := first, 0; j < n.count && y < avail.H; j++ {
		r := e.listItem(&win, j)
		if r.box < 0 {
			continue
		}
		if y+r.h <= avail.H {
			view.Visible++
		}
		e.stack = append(e.stack, r.box)
		natural.W = max(natural.W, e.measure(r.box, Size{W: avail.W, H: unbounded}).W)
		natural.H = min(y+r.h, avail.H)
		y += r.h + n.gap
	}
	if view.Visible == 0 && len(e.stack) > start {
		view.Visible = 1
	}
	e.listUp, e.listDown = e.listUp[:win.up], e.listDown[:win.down]

	b = &e.boxes[i] // building items grew the arena
	b.kidsOff, b.kidsLen = len(e.kids), len(e.stack)-start
	b.list, b.listSize = view, natural
	e.kids = append(e.kids, e.stack[start:]...)
	e.stack = e.stack[:start]
}

// arrangeList places the items list n, box i, shows in r, top to bottom.
func (e *engine) arrangeList(i int, n ListNode, r image.Rectangle) {
	e.expandList(i, n, Size{W: r.Dx(), H: r.Dy()})
	y := r.Min.Y
	for _, k := range e.kidsOf(i) {
		h := e.measure(k, Size{W: r.Dx(), H: unbounded}).H
		e.arrange(k, image.Rect(r.Min.X, y, r.Max.X, y+h))
		y += h + n.gap
	}
}
