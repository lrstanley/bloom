// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// entry is a node of a frame that input refers to: one with an ID, mouse
// handlers, a hover style, or a tooltip, or a modal layer.
type entry struct {
	id                      any
	key                     nodeKey
	rect                    image.Rectangle // where the node was laid out
	vis                     image.Rectangle // the part of rect on screen
	block                   image.Rectangle // for a modal layer, the area it takes events from
	parent                  int             // nearest ancestor entry, or -1
	focusable, scope, modal bool
	hoverable               bool // the node has a hover style
	selectable              bool
	cut                     bool // a truncated text node cut to fit
	leaving                 bool // drawn by a presence animating out, so it takes no events
	list                    ListView
	isList                  bool
	vp                      ViewportView
	isVP                    bool
}

// nodeKey is what a node is followed by from frame to frame: its ID, or
// without one, where it is in the tree.
type nodeKey struct {
	id   any
	path uint64
}

func (b *box) key() nodeKey {
	if b.meta.id != nil {
		return nodeKey{id: b.meta.id}
	}
	return nodeKey{path: b.path}
}

func (r *renderer) addEntry(i int, vis, clip image.Rectangle, parent int) int {
	b := &r.boxes[i]
	e := entry{
		id: b.meta.id, key: b.key(), rect: b.rect, vis: vis, parent: parent,
		focusable: b.meta.focusable, scope: b.meta.scope, modal: b.modal,
		hoverable: b.meta.hover != nil, selectable: b.meta.selectable,
	}
	if _, ok := b.child.(ListNode); ok {
		e.list, e.isList = b.list, true
	}
	if l := r.viewportOf(i); l != nil {
		e.vp, e.isVP = l.view, true
	}
	if b.modal {
		e.block = clip
	}
	r.entries = append(r.entries, e)
	r.handlers = append(r.handlers, b.meta.handlers)
	if r.tipsOn && b.meta.tip != nil {
		r.tips = append(r.tips, tipAt{entry: len(r.entries) - 1, tip: b.meta.tip, pinned: b.meta.tipOpen})
	}
	return len(r.entries) - 1
}

// hasEntry reports whether box b needs an entry in the frame. A node with a
// tooltip has one only while tooltips are on. Text cut to fit, whose full
// text is its tooltip, is given one as it is painted, once it is known to
// be cut; see [renderer.noteCut].
func (r *renderer) hasEntry(b *box) bool {
	return b.meta.id != nil || b.modal || len(b.meta.handlers) > 0 || b.meta.hover != nil || b.meta.selectable ||
		(r.tipsOn && b.meta.tip != nil)
}

// isScrollbar reports whether box b is a [ScrollbarNode], whose entry covers
// only the cells it draws, and is added as it is painted.
func isScrollbar(b *box) bool {
	_, ok := b.child.(ScrollbarNode)
	return ok
}

// Frame is the geometry of a laid-out frame: where each node with an ID
// went, which nodes can be focused, and what the mouse can hit, with the
// mouse handlers nodes were given. It does not change once made, so it is
// safe to keep.
type Frame struct {
	entries  []entry
	handlers [][]on.Handler // by entry
	tips     []tipAt        // of the entries with tooltips, in order
	cursor   *tea.Cursor
	needs    mouseNeeds
}

// mouseNeeds is what the nodes of a frame need the terminal to report of
// the mouse.
type mouseNeeds uint8

const (
	needsClicks mouseNeeds = 1 << iota // clicks, the wheel, and drags
	needsMotion                        // the pointer moving with no button down, such as to open tooltips
	needsFocus                         // a click to focus a node, with WithFocus
)

// needsOf returns what the nodes of a frame need of the mouse. A tooltip
// pinned open needs nothing: it shows already.
func needsOf(entries []entry, handlers [][]on.Handler, tips []tipAt) mouseNeeds {
	var n mouseNeeds
	if slices.ContainsFunc(tips, func(t tipAt) bool { return !t.pinned && !t.pinOnly }) {
		n |= needsMotion
	}
	for i := range entries {
		e := &entries[i]
		switch {
		case e.hoverable || has(handlers[i], hovers):
			n |= needsMotion
		case len(handlers[i]) > 0 || e.selectable:
			n |= needsClicks
		}
		if e.focusable {
			n |= needsFocus
		}
	}
	return n
}

// mouseMode returns the least the terminal must report of the mouse for
// the frame, which clicks focus nodes in when clickFocus.
func (f *Frame) mouseMode(clickFocus bool) tea.MouseMode {
	switch {
	case f.needs&needsMotion != 0:
		return tea.MouseModeAllMotion
	case f.needs&needsClicks != 0, clickFocus && f.needs&needsFocus != 0:
		return tea.MouseModeCellMotion
	}
	return tea.MouseModeNone
}

// Rect returns where the node with ID id was laid out, which may be partly
// off screen, and whether it is in the frame. Retained layout animations
// report the visual rectangle drawn this frame, except for a node moving
// with effects, such as with [SlotNode.MoveWith], which is laid out where it
// goes from the start.
func (f *Frame) Rect[K comparable](id K) (image.Rectangle, bool) {
	if i := f.index(id); i >= 0 {
		return f.entries[i].rect, true
	}
	return image.Rectangle{}, false
}

// Truncated reports whether the [TextNode.Truncate] node with ID id was cut
// to fit in the frame, so it shows "…" in place of the rest of its text. It
// is false for a node that fits, that is not in the frame, or that is not
// truncated. A frame from [Solve] reports it too, as the node would be drawn.
func (f *Frame) Truncated[K comparable](id K) bool {
	i := f.index(id)
	return i >= 0 && f.entries[i].cut
}

// List returns which items the [List] with ID id showed, and whether it is
// in the frame. Use it in Update to page by [ListView.Visible] items, and to
// scroll from [ListView.First], where the list actually was.
func (f *Frame) List[K comparable](id K) (ListView, bool) {
	if i := f.index(id); i >= 0 && f.entries[i].isList {
		return f.entries[i].list, true
	}
	return ListView{}, false
}

// Viewport returns where the [Viewport] with ID id was scrolled to, and
// whether it is in the frame. Use it in Update to scroll from where it
// actually was, with the methods of [ViewportView].
func (f *Frame) Viewport[K comparable](id K) (ViewportView, bool) {
	if i := f.index(id); i >= 0 && f.entries[i].isVP {
		return f.entries[i].vp, true
	}
	return ViewportView{}, false
}

// Hit returns the IDs of the nodes at x, y on screen that mouse events
// there reach, outermost first, so the last is the topmost node hit. It is
// nil when nothing with an ID is there, or when a modal layer takes the
// point although it is outside the layer. It is for handling a
// [tea.MouseMsg] in Update rather than with handlers.
func (f *Frame) Hit(x, y int) []any {
	if f == nil {
		return nil
	}
	chain, _ := f.under(image.Pt(x, y), nil)
	var path []any
	for _, i := range slices.Backward(chain) {
		if id := f.entries[i].id; id != nil {
			path = append(path, id)
		}
	}
	return path
}

// hit returns the entry the point p is on, the topmost one drawn, or -1.
// When a modal layer takes the point although it is outside the layer,
// outside is set and the entry is the layer's.
//
// Entries animating out take no point. A modal layer animating out still
// takes p from what is beneath it, as if p were outside it, unless another
// modal layer beneath it takes p, which then gets it, so that a click right
// after closing the top dialog of a stack reaches the next.
func (f *Frame) hit(p image.Point) (entry int, outside bool) {
	block := -1
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := &f.entries[i]
		if e.leaving {
			if block < 0 && e.modal && p.In(e.block) && f.modalUnder(i, p) < 0 {
				block = i
			}
			continue
		}
		if block >= 0 {
			return block, true
		}
		if p.In(e.vis) {
			return i, false
		}
		if e.modal && p.In(e.block) {
			return i, true
		}
	}
	if block >= 0 {
		return block, true
	}
	return -1, false
}

// modalUnder returns the topmost modal layer drawn before entry i that
// takes p and isn't animating out, or -1.
func (f *Frame) modalUnder(i int, p image.Point) int {
	for j := i - 1; j >= 0; j-- {
		if e := &f.entries[j]; e.modal && !e.leaving && p.In(e.block) {
			return j
		}
	}
	return -1
}

// under appends to chain the entries that events at p reach, innermost
// first: the entry hit and those it is inside, stopping at the topmost
// modal layer that takes p, whose index is floor, or 0 when there is none.
// Nothing before floor takes events at p, besides the entries inside the
// modal layer itself.
func (f *Frame) under(p image.Point, chain []int) (_ []int, floor int) {
	// A modal layer animating out is the floor only when no other modal
	// layer beneath it takes p; see [Frame.hit].
	for i := len(f.entries) - 1; i >= 0; i-- {
		if e := &f.entries[i]; e.modal && p.In(e.block) {
			if !e.leaving {
				floor = i
				break
			}
			if j := f.modalUnder(i, p); j >= 0 {
				floor = j
			} else {
				floor = i
			}
			break
		}
	}
	i, outside := f.hit(p)
	if outside {
		return chain, floor
	}
	for ; i >= floor; i = f.entries[i].parent {
		chain = append(chain, i)
	}
	return chain, floor
}

// find returns the entry of the node with key k, or -1.
func (f *Frame) find(k nodeKey) int { return findKey(f.entries, k) }

// findKey returns the entry of the node with key k, or -1.
func findKey(entries []entry, k nodeKey) int {
	for i := range entries {
		if entries[i].key == k {
			return i
		}
	}
	return -1
}

// inScope reports whether entry i is inside the scope with ID scope.
func (f *Frame) inScope(i int, scope any) bool {
	for ; i >= 0; i = f.entries[i].parent {
		if e := &f.entries[i]; e.scope && e.id == scope {
			return true
		}
	}
	return false
}

// nearestID returns the ID of entry i, or of its nearest ancestor with one.
func (f *Frame) nearestID(i int) any {
	for ; i >= 0; i = f.entries[i].parent {
		if id := f.entries[i].id; id != nil {
			return id
		}
	}
	return nil
}

// index returns the entry with ID id, or -1.
func (f *Frame) index(id any) int {
	if f != nil && id != nil {
		for i := range f.entries {
			if f.entries[i].id == id {
				return i
			}
		}
	}
	return -1
}
