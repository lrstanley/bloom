// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"slices"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// Focus is which node has the keyboard focus, by its [ID]. Keep it in the
// model: the layout only says which nodes can be focused, with Focusable, and
// where focus scopes are.
//
//	const (
//		fieldName  bloom.ID = "name"
//		fieldEmail bloom.ID = "email"
//		buttonSave bloom.ID = "save"
//	)
//
//	type model struct {
//		focus bloom.Focus
//	}
//
// [Focus.Push] confines focus to a scope, such as a modal [Layer], until
// [Focus.Pop] puts it back where it was:
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
type Focus struct {
	current ID
	scopes  []focusScope
}

type focusScope struct {
	id, prev ID
}

// Current returns the ID of the focused node, or "" when no node is focused.
func (f *Focus) Current() ID { return f.current }

// Is reports whether the node with ID id is focused.
func (f *Focus) Is(id ID) bool { return !id.IsZero() && f.current == id }

// Set focuses the node with ID id. Setting "" clears the focus.
func (f *Focus) Set(id ID) { f.current = id }

// Clear removes the focus.
func (f *Focus) Clear() { f.current = "" }

// Push confines focus to the scope with ID scope, clearing it until a node
// there is focused, and remembers the focus to go back to on [Focus.Pop].
func (f *Focus) Push(scope ID) {
	// Clipping makes append copy, so a copy of the model that still holds
	// the old scopes is not changed by this one.
	f.scopes = append(slices.Clip(f.scopes), focusScope{id: scope, prev: f.current})
	f.Clear()
}

// Pop leaves the innermost scope, focusing what was focused when it was
// pushed.
func (f *Focus) Pop() {
	if n := len(f.scopes); n > 0 {
		f.current = f.scopes[n-1].prev
		f.scopes = f.scopes[:n-1]
	}
}

// Scope returns the ID of the innermost scope pushed, or "".
func (f *Focus) Scope() ID {
	if n := len(f.scopes); n > 0 {
		return f.scopes[n-1].id
	}
	return ""
}

// Within reports whether the focused node is inside the scope with ID scope
// in frame.
func (f *Focus) Within(frame *Frame, scope ID) bool {
	if f.current.IsZero() {
		return false
	}
	i := frame.index(f.current)
	return i >= 0 && frame.inScope(i, scope)
}

// Next focuses the next node that can be focused, in the order of frame,
// inside the innermost scope pushed, wrapping around. It reports whether
// there was one.
func (f *Focus) Next(frame *Frame) bool { return f.step(frame, 1) }

// Prev focuses the previous node that can be focused, as [Focus.Next] does.
func (f *Focus) Prev(frame *Frame) bool { return f.step(frame, -1) }

func (f *Focus) step(frame *Frame, dir int) bool {
	if frame == nil {
		return false
	}
	var order []ID
	at := -1
	for i := range frame.entries {
		if f.focusable(frame, i) {
			id := frame.entries[i].id
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
func (f *Focus) click(frame *Frame, chain []int) {
	for _, i := range chain {
		if f.focusable(frame, i) {
			f.Set(frame.entries[i].id)
			return
		}
	}
}

// focusable reports whether entry i of frame can take focus now: it can be
// focused, has an ID, is on screen, and is inside the innermost scope pushed.
func (f *Focus) focusable(frame *Frame, i int) bool {
	e := &frame.entries[i]
	if e.id.IsZero() || !e.focusable || e.vis.Empty() {
		return false
	}
	scope := f.Scope()
	return scope.IsZero() || frame.inScope(i, scope)
}

// Update handles tab and shift+tab, which move the focus, and enter and
// space, which run the [on.Press] handlers of the focused node in frame. It
// returns their commands, and reports whether it handled msg: tab keys are
// handled even when frame has no focusable nodes, and enter and space only
// when the focused node has a Press handler that takes the key, rather than
// passing it on with [on.Pass], such as a button that is disabled leaving
// enter to the form. Clicks move the focus with [WithFocus].
func (f *Focus) Update(msg tea.Msg, frame *Frame) (tea.Cmd, bool) {
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
		i := frame.index(f.current)
		if i < 0 || !has(frame.handlers[i], kindsOf(on.KindPress)) {
			return nil, false
		}
		if !f.focusable(frame, i) {
			// It can't take focus now, such as from outside the scope pushed.
			return nil, false
		}
		t := frame.target(i)
		cmds, took := runTook(nil, t.handlers, kindsOf(on.KindPress), on.Event{W: t.rect.Dx(), H: t.rect.Dy()})
		return tea.Batch(cmds...), took
	}
	return nil, false
}
