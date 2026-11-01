// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bloom/on"
)

// Focus is which node has the keyboard focus, by its ID, of type K. Keep it
// in the model: the layout only says which nodes can be focused, with
// Focusable, and where focus scopes are.
//
// Focus moves between focusable nodes whose IDs are Ks, and passes over
// those with IDs of other types, so give everything that takes focus the
// same ID type:
//
//	type control int
//
//	const (
//		fieldName control = iota
//		fieldEmail
//		buttonSave
//	)
//
//	type model struct {
//		focus bloom.Focus[control]
//	}
//
// For nodes of different kinds, K can be an interface that their ID types
// implement, and a Focus[any] moves between every focusable node.
//
// [Focus.Push] confines focus to a scope, such as a modal [Layer], until
// [Focus.Pop] puts it back where it was. A scope's ID can be of any type:
//
//	case openConfirmMsg:
//		m.focus.Push(dialogConfirm)
//		m.focus.Set(buttonOK)
//	case closeConfirmMsg:
//		m.focus.Pop()
//	case tea.KeyPressMsg:
//		if msg.String() == "tab" {
//			m.focus.Next(m.ui.Frame())
//		}
type Focus[K comparable] struct {
	current K
	focused bool
	scopes  []focusScope[K]
}

type focusScope[K comparable] struct {
	id      any
	prev    K
	focused bool
}

// Current returns the ID of the focused node, and whether a node is focused.
func (f *Focus[K]) Current() (K, bool) { return f.current, f.focused }

// Is reports whether the node with ID id is focused.
func (f *Focus[K]) Is(id K) bool { return f.focused && f.current == id }

// Set focuses the node with ID id.
func (f *Focus[K]) Set(id K) { f.current, f.focused = id, true }

// Clear removes the focus.
func (f *Focus[K]) Clear() {
	var zero K
	f.current, f.focused = zero, false
}

// Push confines focus to the scope with ID scope, clearing it until a node
// there is focused, and remembers the focus to go back to on [Focus.Pop].
func (f *Focus[K]) Push[S comparable](scope S) {
	// Clipping makes append copy, so a copy of the model that still holds
	// the old scopes is not changed by this one.
	f.scopes = append(slices.Clip(f.scopes), focusScope[K]{id: scope, prev: f.current, focused: f.focused})
	f.Clear()
}

// Pop leaves the innermost scope, focusing what was focused when it was
// pushed.
func (f *Focus[K]) Pop() {
	if n := len(f.scopes); n > 0 {
		s := f.scopes[n-1]
		f.current, f.focused = s.prev, s.focused
		f.scopes = f.scopes[:n-1]
	}
}

// Scope returns the ID of the innermost scope pushed, or nil.
func (f *Focus[K]) Scope() any {
	if n := len(f.scopes); n > 0 {
		return f.scopes[n-1].id
	}
	return nil
}

// Within reports whether the focused node is inside the scope with ID scope
// in frame.
func (f *Focus[K]) Within[S comparable](frame *Frame, scope S) bool {
	if !f.focused {
		return false
	}
	i := frame.index(f.current)
	return i >= 0 && frame.inScope(i, scope)
}

// Next focuses the next node that can be focused, in the order of frame,
// inside the innermost scope pushed, wrapping around. It reports whether
// there was one.
func (f *Focus[K]) Next(frame *Frame) bool { return f.step(frame, 1) }

// Prev focuses the previous node that can be focused, as [Focus.Next] does.
func (f *Focus[K]) Prev(frame *Frame) bool { return f.step(frame, -1) }

func (f *Focus[K]) step(frame *Frame, dir int) bool {
	if frame == nil {
		return false
	}
	var order []K
	at := -1
	for i := range frame.entries {
		if id, ok := f.focusable(frame, i); ok {
			if f.Is(id) {
				at = len(order)
			}
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		return false
	}
	switch {
	case at >= 0:
		at = (at + dir + len(order)) % len(order)
	case dir > 0:
		at = 0
	default:
		at = len(order) - 1
	}
	f.Set(order[at])
	return true
}

// click focuses the innermost node of chain that can be focused, for
// [WithFocus].
func (f *Focus[K]) click(frame *Frame, chain []int) {
	for _, i := range chain {
		if id, ok := f.focusable(frame, i); ok {
			f.Set(id)
			return
		}
	}
}

// focusable returns the ID of entry i of frame, if it can take focus now: it
// can be focused, has an ID of type K, is on screen, and is inside the
// innermost scope pushed.
func (f *Focus[K]) focusable(frame *Frame, i int) (K, bool) {
	e := &frame.entries[i]
	id, ok := e.id.(K)
	if !ok || !e.focusable || e.vis.Empty() {
		return id, false
	}
	scope := f.Scope()
	return id, scope == nil || frame.inScope(i, scope)
}

// Update handles tab and shift+tab, which move the focus, and enter and
// space, which run the [on.Press] handlers of the focused node in frame. It
// returns their commands, and reports whether it handled msg: tab keys are
// handled even when frame has no focusable nodes, and enter and space only
// when the focused node has a Press handler. Clicks move the focus with
// [WithFocus].
func (f *Focus[K]) Update(msg tea.Msg, frame *Frame) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "tab":
		f.Next(frame)
		return nil, true
	case "shift+tab":
		f.Prev(frame)
		return nil, true
	case "enter", "space":
		i := -1
		if f.focused {
			i = frame.index(f.current)
		}
		if i < 0 || !has(frame.handlers[i], kindsOf(on.KindPress)) {
			return nil, false
		}
		if _, can := f.focusable(frame, i); !can {
			// It can't take focus now, such as from outside the scope pushed.
			return nil, false
		}
		var cmds []tea.Cmd
		t := frame.target(i)
		return tea.Batch(run(cmds, t.handlers, kindsOf(on.KindPress), on.Event{W: t.rect.Dx(), H: t.rect.Dy()})...), true
	}
	return nil, false
}
