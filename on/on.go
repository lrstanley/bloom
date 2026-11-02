// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package on makes mouse handlers for bloom nodes, attached with their On
// method:
//
//	bloom.Text(name).On(
//		on.Click(func() tea.Cmd { m.selected = i; return nil }),
//		on.DoubleClick(func() tea.Cmd { return m.open(i) }),
//		on.RightClick(on.Msg(menuMsg{i})),
//	)
//
// Handlers run on the Bubble Tea event loop, as Update does, so they can
// change the model directly and return a command, which runs in the
// background. A handler only changes the real model if it reaches it, such
// as through a pointer receiver; one built in a component kept by value can
// send a message with [Msg] instead, for the component's Update to handle.
// Handlers must not block.
//
// For each kind of event, the innermost node under the pointer with a
// handler for it takes it, and the nodes it is inside do not, so a row's
// [Click] and its pane's [Scroll] work side by side. [Hover] is the
// exception: every node under the pointer is hovered. A modal layer takes
// every event from what is beneath it.
package on

import (
	"image"

	tea "charm.land/bubbletea/v2"
)

// Event is a mouse event at a node.
type Event struct {
	// Msg is the Bubble Tea event, in screen coordinates. It is nil when no
	// event caused the handler to run, such as when a hovered node is
	// covered by a dialog, or a key pressed a [Press] handler.
	Msg tea.MouseMsg
	// Screen is where the pointer is on screen.
	Screen image.Point
	// X and Y are where the pointer is, relative to the node's top-left
	// cell, which for a box is the outer corner of its border. During a
	// drag they can be outside the node.
	X, Y int
	// W and H are the node's size.
	W, H int
	// Inside reports whether the pointer is over the node, rather than over
	// something drawn on top of it, or outside it.
	Inside bool
	// Start is where a drag started, on screen.
	Start image.Point
	// Done is set on the last event of a drag: the button came up, or the
	// node left the frame or was covered by a modal layer.
	Done bool
}

// Kind is the kind of event a [Handler] takes.
type Kind uint8

// The kinds of handler, one for each function of this package.
const (
	KindClick Kind = iota + 1 // [Click] and [ClickAt]
	KindPress
	KindRightClick // [RightClick] and [RightClickAt]
	KindMiddleClick
	KindDoubleClick
	KindScroll
	KindHover
	KindDrag
	KindClickOutside
	KindMouse
)

// Handler is a mouse handler for a node, made by one of the functions of
// this package.
type Handler struct {
	kind Kind
	fn   func(Event) tea.Cmd
}

// Kind returns the kind of event the handler takes.
func (h Handler) Kind() Kind { return h.kind }

// Handle runs the handler for e, returning its command. bloom calls it;
// it is exported for routing events some other way, and for tests.
func (h Handler) Handle(e Event) tea.Cmd {
	if h.fn == nil {
		return nil
	}
	return h.fn(e)
}

func simple(kind Kind, fn func() tea.Cmd) Handler {
	return Handler{kind: kind, fn: func(Event) tea.Cmd { return fn() }}
}

// Click runs fn when the left button is pressed and released over the
// node.
func Click(fn func() tea.Cmd) Handler { return simple(KindClick, fn) }

// ClickAt runs fn when the left button is pressed and released over the
// node, with where it was released.
func ClickAt(fn func(e Event) tea.Cmd) Handler { return Handler{kind: KindClick, fn: fn} }

// RightClick runs fn when the right button is pressed and released over
// the node.
func RightClick(fn func() tea.Cmd) Handler { return simple(KindRightClick, fn) }

// RightClickAt runs fn when the right button is pressed and released over
// the node, with where it was released, such as to open a menu there.
func RightClickAt(fn func(e Event) tea.Cmd) Handler { return Handler{kind: KindRightClick, fn: fn} }

// MiddleClick runs fn when the middle button is pressed and released over
// the node.
func MiddleClick(fn func() tea.Cmd) Handler { return simple(KindMiddleClick, fn) }

// DoubleClick runs fn on the second of two left clicks on the node in
// quick succession. Both clicks still run the node's [Click] handlers.
func DoubleClick(fn func() tea.Cmd) Handler { return simple(KindDoubleClick, fn) }

// Press runs fn when the node is clicked with the left button, as [Click]
// does, or when enter or space is pressed while it has the focus, which
// [bloom.Focus.Update] handles.
//
// [bloom.Focus.Update]: https://pkg.go.dev/github.com/lrstanley/bloom#Focus.Update
func Press(fn func() tea.Cmd) Handler { return simple(KindPress, fn) }

// Scroll runs fn when the wheel turns over the node: dy is 1 for each step
// down and -1 for each step up, and dx is the same for left and right.
func Scroll(fn func(dx, dy int) tea.Cmd) Handler {
	return Handler{kind: KindScroll, fn: func(e Event) tea.Cmd {
		w, _ := e.Msg.(tea.MouseWheelMsg)
		switch w.Button { //nolint:exhaustive // Only the wheel scrolls.
		case tea.MouseWheelUp:
			return fn(0, -1)
		case tea.MouseWheelDown:
			return fn(0, 1)
		case tea.MouseWheelLeft:
			return fn(-1, 0)
		case tea.MouseWheelRight:
			return fn(1, 0)
		}
		return nil
	}}
}

// Hover runs fn with true when the pointer moves onto the node, and false
// when it leaves, or something drawn on top of the node covers it, which
// is noticed with the next message after the frame is drawn. The
// pointer only moves while no button is down when the view sets
// [tea.MouseModeAllMotion]. Terminals do not report the pointer leaving the
// window, so the hover ends when the window loses focus, which takes
// [tea.View.ReportFocus].
func Hover(fn func(over bool) tea.Cmd) Handler {
	return Handler{kind: KindHover, fn: func(e Event) tea.Cmd { return fn(e.Inside) }}
}

// Drag runs fn when the left button goes down over the node, each time the
// pointer moves while it is held, wherever the pointer is, and once more
// with Done set when it comes up. A drag is followed from frame to frame by
// the node's ID, or without one, by where the node is in the tree, so a
// node that moves within the tree while dragged needs an ID.
func Drag(fn func(e Event) tea.Cmd) Handler { return Handler{kind: KindDrag, fn: fn} }

// ClickOutside runs fn when a mouse button goes down anywhere but on the
// node, such as to close a menu. The click still reaches what it is on.
// On a modal layer, it runs for a click on the backdrop, which the layer
// still keeps from what is beneath it.
func ClickOutside(fn func() tea.Cmd) Handler { return simple(KindClickOutside, fn) }

// Mouse runs fn for every mouse event over the node: presses, releases,
// the wheel, and motion. The event is in e.Msg.
func Mouse(fn func(e Event) tea.Cmd) Handler { return Handler{kind: KindMouse, fn: fn} }

// Msg returns a handler function that sends msg to Update, for changing
// state there rather than in the handler:
//
//	bloom.Text(name).On(on.Click(on.Msg(selectMsg(i))))
func Msg(msg tea.Msg) func() tea.Cmd {
	return func() tea.Cmd { return func() tea.Msg { return msg } }
}
