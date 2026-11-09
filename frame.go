// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bloom/on"
)

// entry is a node of a frame that input refers to: one with an ID, mouse
// handlers, or a hover style, or a modal layer.
type entry struct {
	id                      any
	key                     nodeKey
	rect                    image.Rectangle // where the node was laid out
	vis                     image.Rectangle // the part of rect on screen
	block                   image.Rectangle // for a modal layer, the area it takes events from
	parent                  int             // nearest ancestor entry, or -1
	focusable, scope, modal bool
	hoverable               bool // the node has a hover style
	list                    ListView
	isList                  bool
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

func (r *renderer) addEntry(b *box, vis, clip image.Rectangle, parent int) int {
	e := entry{
		id: b.meta.id, key: b.key(), rect: b.rect, vis: vis, parent: parent,
		focusable: b.meta.focusable, scope: b.meta.scope, modal: b.modal,
		hoverable: b.meta.hover != nil,
	}
	if _, ok := b.child.(ListNode); ok {
		e.list, e.isList = b.list, true
	}
	if b.modal {
		e.block = clip
	}
	r.entries = append(r.entries, e)
	r.handlers = append(r.handlers, b.meta.handlers)
	return len(r.entries) - 1
}

// hasEntry reports whether box b needs an entry in the frame.
func hasEntry(b *box) bool {
	return b.meta.id != nil || b.modal || len(b.meta.handlers) > 0 || b.meta.hover != nil
}

// Frame is the geometry of a laid-out frame: where each node with an ID
// went, which nodes can be focused, and what the mouse can hit, with the
// mouse handlers nodes were given. It does not change once made, so it is
// safe to keep.
type Frame struct {
	entries  []entry
	handlers [][]on.Handler // by entry
	cursor   *tea.Cursor
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

// List returns which items the [List] with ID id showed, and whether it is
// in the frame. Use it in Update to page by [ListView.Visible] items, and to
// scroll from [ListView.First], where the list actually was.
func (f *Frame) List[K comparable](id K) (ListView, bool) {
	if i := f.index(id); i >= 0 && f.entries[i].isList {
		return f.entries[i].list, true
	}
	return ListView{}, false
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
func (f *Frame) hit(p image.Point) (entry int, outside bool) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		e := &f.entries[i]
		if p.In(e.vis) {
			return i, false
		}
		if e.modal && p.In(e.block) {
			return i, true
		}
	}
	return -1, false
}

// under appends to chain the entries that events at p reach, innermost
// first: the entry hit and those it is inside, stopping at the topmost
// modal layer that takes p, whose index is floor, or 0 when there is none.
// Nothing before floor takes events at p, besides the entries inside the
// modal layer itself.
func (f *Frame) under(p image.Point, chain []int) (_ []int, floor int) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		if e := &f.entries[i]; e.modal && p.In(e.block) {
			floor = i
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
func (f *Frame) find(k nodeKey) int {
	for i := range f.entries {
		if f.entries[i].key == k {
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
