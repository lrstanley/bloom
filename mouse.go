// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bloom/on"
)

// doubleClick is the longest time between the clicks of a double click.
const doubleClick = 400 * time.Millisecond

// kinds is a set of [on.Kind]s.
type kinds uint16

const (
	leftClicks kinds = 1<<on.KindClick | 1<<on.KindPress
	hovers     kinds = 1 << on.KindHover
)

func kindsOf(k on.Kind) kinds { return 1 << k }

// clicks returns the kinds of handler a click with button runs.
func clicks(button tea.MouseButton) kinds {
	switch button { //nolint:exhaustive // Other buttons have no handlers.
	case tea.MouseLeft:
		return leftClicks
	case tea.MouseRight:
		return kindsOf(on.KindRightClick)
	case tea.MouseMiddle:
		return kindsOf(on.KindMiddleClick)
	}
	return 0
}

func has(handlers []on.Handler, k kinds) bool {
	for _, h := range handlers {
		if k&kindsOf(h.Kind()) != 0 {
			return true
		}
	}
	return false
}

// run appends the commands of the handlers of kinds k to cmds.
func run(cmds []tea.Cmd, handlers []on.Handler, k kinds, e on.Event) []tea.Cmd {
	for _, h := range handlers {
		if k&kindsOf(h.Kind()) != 0 {
			if cmd := h.Handle(e); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

// mouseTarget is a node that events go to across frames, with where it was
// and the handlers it had when last seen, should it leave the frame.
type mouseTarget struct {
	key      nodeKey
	rect     image.Rectangle
	handlers []on.Handler
}

func (f *Frame) target(i int) mouseTarget {
	return mouseTarget{key: f.entries[i].key, rect: f.entries[i].rect, handlers: f.handlers[i]}
}

// event returns the event for t of a mouse event at p.
func (t *mouseTarget) event(msg tea.MouseMsg, p image.Point, inside bool) on.Event {
	return on.Event{
		Msg: msg, Screen: p, X: p.X - t.rect.Min.X, Y: p.Y - t.rect.Min.Y,
		W: t.rect.Dx(), H: t.rect.Dy(), Inside: inside,
	}
}

// mouse is what a Root keeps between mouse events.
type mouse struct {
	// routed is the event OnMouse routed last, which Update then skips.
	routed  tea.MouseMsg
	at      image.Point // where the pointer was last
	atOK    bool
	checked *Frame // the frame hover and drag were last brought up to date with
	routing *Frame // the frame route is running handlers for, for Context.Frame
	chain   []int  // scratch

	next        []mouseTarget // scratch for the renderer's hovered
	press       press
	drag        *mouseTarget
	dragStart   image.Point
	lastClick   nodeKey // the node clicked last, if it has a double click handler
	lastClickAt time.Time

	// What the presses in a row on a selectable node selected, the last at
	// selAt on selKey at selTime, if selOK, for a double click to select a
	// word and a triple click a line.
	selUnit selUnit
	selOK   bool
	selKey  nodeKey
	selAt   image.Point
	selTime time.Time

	held        heldClick // a click inside a selectable node, held back
	heldSeq     int       // counts clicks held back, for the tick of one to know it
	scrollChain []int     // scratch for scrolling the text being selected
}

// heldClick is a click of the left button inside a selectable node held
// back until it can no longer become a double click, which selects a word
// instead (see [WithClickDelay]): the handlers of the node with key it
// runs, and the event it runs them with.
type heldClick struct {
	ok       bool
	key      nodeKey
	handlers []on.Handler
	event    on.Event
}

// press is the button down, and the nodes it went down on that take its
// click and a double click, which run when it comes up on them again.
type press struct {
	button        tea.MouseButton
	click, double nodeKey
	ok, doubleOK  bool
	hold          bool // the click is held back, as it may become a double click
}

// focusTarget is a [Focus] of any type that clicks can move, for
// [WithFocus].
type focusTarget interface {
	click(f *Frame, chain []int)
}

// mouseEvent is the OnMouse handler of the view [Root.View] made for frame
// f, which is on screen when Bubble Tea calls it. Bubble Tea runs it on the
// event loop, so handlers run where Update does, but the command it returns
// runs there too: the handlers' commands are batched, which Bubble Tea runs
// in the background like those Update returns.
func (r *Root) mouseEvent(f *Frame, msg tea.MouseMsg) tea.Cmd {
	r.mouse.routed = msg
	cmd := r.route(f, msg)
	if cmd == nil {
		return nil
	}
	return func() tea.Msg { return tea.BatchMsg{cmd} }
}

// input handles the messages [Root.Update] gets about the mouse, and keeps
// hover and drags up to date with the frame on screen.
func (r *Root) input(msg tea.Msg) tea.Cmd {
	s := &r.mouse
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if s.routed != nil && s.routed == msg {
			s.routed = nil
			return nil
		}
		s.routed = nil
		return r.route(r.r.frame, msg)
	case tea.BlurMsg:
		// The pointer may be anywhere now, so nothing is hovered until it
		// moves again.
		s.atOK = false
		return tea.Batch(r.hover(nil, r.r.frame, nil, nil)...)
	}
	f := r.r.frame
	if f == nil || f == s.checked {
		return nil
	}
	s.checked = f
	r.checkSelect(f)
	cmds := r.checkDrag(nil, f)
	if s.atOK {
		s.chain, _ = f.under(s.at, s.chain[:0])
		cmds = r.hover(cmds, f, nil, s.chain)
	}
	return tea.Batch(cmds...)
}

// route runs the handlers for msg in frame f.
func (r *Root) route(f *Frame, msg tea.MouseMsg) tea.Cmd {
	if f == nil {
		return nil
	}
	s := &r.mouse
	// An older view's OnMouse routes its own frame, which may not be the
	// last one drawn: handlers reading Context.Frame get the one they are in.
	outer := s.routing
	s.routing = f
	defer func() { s.routing = outer }()
	m := msg.Mouse()
	p := image.Pt(m.X, m.Y)
	s.at, s.atOK, s.checked = p, true, f
	chain, floor := f.under(p, s.chain[:0])
	s.chain = chain

	cmds := r.checkDrag(nil, f)
	switch msg.(type) {
	case tea.MouseClickMsg:
		if m.Button == tea.MouseLeft {
			// A drag still going lost its release, such as outside the window.
			cmds = r.endDrag(cmds, f, msg, p, chain)
		}
		if r.focus != nil {
			r.focus.click(f, chain)
		}
		cmds = f.clickOutside(cmds, msg, p, chain, floor)
		s.press = press{button: m.Button}
		if i := f.innermost(chain, clicks(m.Button)); i >= 0 {
			s.press.click, s.press.ok = f.entries[i].key, true
		}
		if m.Button == tea.MouseLeft {
			cmds = r.pressLeft(cmds, f, msg, p, chain)
		}
	case tea.MouseReleaseMsg:
		if m.Button == tea.MouseLeft || m.Button == tea.MouseNone {
			cmds = r.endDrag(cmds, f, msg, p, chain)
			if r.sel.dragging {
				r.moveSelect(f, p)
				if r.sel.moved {
					// Selecting text is not a click on what it was selected in.
					s.press.ok, s.press.doubleOK = false, false
				}
				cmds = append(cmds, r.endSelect())
			}
		}
		if pr := s.press; m.Button == pr.button || m.Button == tea.MouseNone {
			s.press = press{}
			cmds = r.release(cmds, f, msg, p, pr, chain)
		}
	case tea.MouseWheelMsg:
		if i := f.innermost(chain, kindsOf(on.KindScroll)); i >= 0 {
			t := f.target(i)
			cmds = run(cmds, t.handlers, kindsOf(on.KindScroll), t.event(msg, p, true))
		}
	case tea.MouseMotionMsg:
		if m.Button == tea.MouseNone {
			// No button is down, so a press still pending lost its release.
			s.press = press{}
			cmds = r.endDrag(cmds, f, msg, p, chain)
			cmds = append(cmds, r.endSelect())
		} else {
			cmds = r.dragEvent(cmds, f, msg, p, chain, false)
			cmds = append(cmds, r.moveSelect(f, p))
		}
	}
	if i := f.innermost(chain, kindsOf(on.KindMouse)); i >= 0 {
		t := f.target(i)
		cmds = run(cmds, t.handlers, kindsOf(on.KindMouse), t.event(msg, p, true))
	}
	return tea.Batch(r.hover(cmds, f, msg, chain)...)
}

// pressLeft handles the left button going down at p, where the entries of
// frame f under the pointer are chain: the drag or selection it starts.
func (r *Root) pressLeft(cmds []tea.Cmd, f *Frame, msg tea.MouseMsg, p image.Point, chain []int) []tea.Cmd {
	s := &r.mouse
	// A click anywhere ends the selection, and the flash of the last copy,
	// and may start another.
	r.ClearSelection()
	r.flash = selection{}
	if i := f.innermost(chain, kindsOf(on.KindDoubleClick)); i >= 0 {
		s.press.double, s.press.doubleOK = f.entries[i].key, true
	}
	// The innermost node that drags or selects takes the drag, and a drag
	// handler beats selecting in the same node.
	i := f.dragTarget(chain)
	if i >= 0 && has(f.handlers[i], kindsOf(on.KindDrag)) {
		cmds = r.settleHeld(cmds, nodeKey{}, p)
		t := f.target(i)
		s.drag, s.dragStart = &t, p
		return r.dragEvent(cmds, f, msg, p, chain, false)
	}
	var k nodeKey
	if i >= 0 {
		k = f.entries[i].key
	}
	cmds = r.settleHeld(cmds, k, p)
	if i >= 0 && r.startSelect(f, i, p, msg.Mouse().Mod) && r.clickDelay {
		// A click here may be the first of a double click, which selects a
		// word rather than clicks.
		if r.sel.unit == unitCell {
			s.press.hold = true
		} else {
			s.press.ok = false
		}
	}
	return cmds
}

// release runs the click handlers of the node pr went down on, if the
// button came up on it too.
func (r *Root) release(cmds []tea.Cmd, f *Frame, msg tea.MouseMsg, p image.Point, pr press, chain []int) []tea.Cmd {
	s := &r.mouse
	if i := f.inChain(chain, pr.click); pr.ok && i >= 0 {
		t := f.target(i)
		if pr.hold {
			cmds = append(cmds, r.holdClick(t, t.event(msg, p, true)))
		} else {
			cmds = run(cmds, t.handlers, clicks(pr.button), t.event(msg, p, true))
		}
	}
	i := f.inChain(chain, pr.double)
	if !pr.doubleOK || i < 0 {
		// A click on anything else comes between two clicks on the node.
		s.lastClick = nodeKey{}
		return cmds
	}
	now := r.now()
	if s.lastClick == pr.double && now.Sub(s.lastClickAt) <= doubleClick {
		// A third click starts the next double click.
		s.lastClick = nodeKey{}
		t := f.target(i)
		return run(cmds, t.handlers, kindsOf(on.KindDoubleClick), t.event(msg, p, true))
	}
	s.lastClick, s.lastClickAt = pr.double, now
	return cmds
}

// holdClick holds back the click of the node of t, with event e, until it
// can no longer become a double click, returning the command for when it
// runs.
func (r *Root) holdClick(t mouseTarget, e on.Event) tea.Cmd {
	m := &r.mouse
	m.heldSeq++
	m.held = heldClick{ok: true, key: t.key, handlers: t.handlers, event: e}
	seq := m.heldSeq
	wait := max(doubleClick-r.now().Sub(m.selTime), 0)
	return r.after(wait, func(time.Time) tea.Msg { return clickMsg{root: r, seq: seq} })
}

// settleHeld deals with the click held back, if any, for a press at p on
// the selectable node with key k: it is dropped when the press makes it a
// double click, and otherwise runs now, before the press.
func (r *Root) settleHeld(cmds []tea.Cmd, k nodeKey, p image.Point) []tea.Cmd {
	if !r.mouse.held.ok {
		return cmds
	}
	if r.continuesClick(k, p) {
		r.mouse.held = heldClick{}
		return cmds
	}
	return r.runHeld(cmds)
}

// runHeldClick runs the click held back, for a tick of msg, if no press
// came since.
func (r *Root) runHeldClick(msg tea.Msg) tea.Cmd {
	c, ok := msg.(clickMsg)
	if !ok || c.root != r || !r.mouse.held.ok || c.seq != r.mouse.heldSeq {
		return nil
	}
	return tea.Batch(r.runHeld(nil)...)
}

// runHeld runs the click held back, with the handlers its node has in the
// frame on screen, should the view have made new ones since.
func (r *Root) runHeld(cmds []tea.Cmd) []tea.Cmd {
	h := r.mouse.held
	r.mouse.held = heldClick{}
	f := r.r.frame
	if f != nil {
		if i := f.find(h.key); i >= 0 {
			h.handlers = f.handlers[i]
		}
	}
	outer := r.mouse.routing
	r.mouse.routing = f
	defer func() { r.mouse.routing = outer }()
	return run(cmds, h.handlers, leftClicks, h.event)
}

// innermost returns the first entry of chain with a handler of kinds k, or
// -1.
func (f *Frame) innermost(chain []int, k kinds) int {
	for _, i := range chain {
		if has(f.handlers[i], k) {
			return i
		}
	}
	return -1
}

// dragTarget returns the first entry of chain with a drag handler or that
// is selectable, or -1.
func (f *Frame) dragTarget(chain []int) int {
	for _, i := range chain {
		if f.entries[i].selectable || has(f.handlers[i], kindsOf(on.KindDrag)) {
			return i
		}
	}
	return -1
}

// inChain returns the entry of chain with key k, or -1.
func (f *Frame) inChain(chain []int, k nodeKey) int {
	for _, i := range chain {
		if f.entries[i].key == k {
			return i
		}
	}
	return -1
}

// clickOutside runs the [on.ClickOutside] handlers of the nodes that take
// events at p, other than those in chain, which the click is on.
func (f *Frame) clickOutside(cmds []tea.Cmd, msg tea.MouseMsg, p image.Point, chain []int, floor int) []tea.Cmd {
	k := kindsOf(on.KindClickOutside)
	for i := floor; i < len(f.entries); i++ {
		if has(f.handlers[i], k) && !slices.Contains(chain, i) {
			t := f.target(i)
			cmds = run(cmds, t.handlers, k, t.event(msg, p, false))
		}
	}
	return cmds
}

// dragEvent runs the drag handlers of the node being dragged, if any, for
// an event at p, where the entries under the pointer are chain.
func (r *Root) dragEvent(cmds []tea.Cmd, f *Frame, msg tea.MouseMsg, p image.Point, chain []int, done bool) []tea.Cmd {
	s := &r.mouse
	if s.drag == nil {
		return cmds
	}
	e := s.drag.event(msg, p, f.inChain(chain, s.drag.key) >= 0)
	e.Start, e.Done = s.dragStart, done
	return run(cmds, s.drag.handlers, kindsOf(on.KindDrag), e)
}

// endDrag ends the drag going, if any.
func (r *Root) endDrag(cmds []tea.Cmd, f *Frame, msg tea.MouseMsg, p image.Point, chain []int) []tea.Cmd {
	cmds = r.dragEvent(cmds, f, msg, p, chain, true)
	r.mouse.drag = nil
	return cmds
}

// checkDrag follows the node being dragged into frame f, ending the drag
// if the node is gone or a modal layer covers it.
func (r *Root) checkDrag(cmds []tea.Cmd, f *Frame) []tea.Cmd {
	s := &r.mouse
	if s.drag == nil {
		return cmds
	}
	i := f.find(s.drag.key)
	if i >= 0 {
		*s.drag = f.target(i)
	}
	if i < 0 || f.covered(i) {
		return r.endDrag(cmds, f, nil, s.at, nil)
	}
	return cmds
}

// covered reports whether a modal layer drawn after entry i takes events
// from some of it.
func (f *Frame) covered(i int) bool {
	for j := i + 1; j < len(f.entries); j++ {
		if e := &f.entries[j]; e.modal && e.block.Overlaps(f.entries[i].vis) {
			return true
		}
	}
	return false
}

// hover runs the [on.Hover] handlers of the nodes the pointer left and
// moved onto, now that the entries of frame f under it are chain, and notes
// them for hover styles.
func (r *Root) hover(cmds []tea.Cmd, f *Frame, msg tea.MouseMsg, chain []int) []tea.Cmd {
	s, hovered := &r.mouse, r.r.hovered
	next := s.next[:0]
	if s.atOK && f != nil {
		for _, i := range chain {
			if f.entries[i].hoverable || has(f.handlers[i], hovers) {
				next = append(next, f.target(i))
			}
		}
	}
	for _, t := range hovered {
		if containsKey(next, t.key) {
			continue
		}
		// A node still in the frame has new handlers; one gone has its last.
		if f != nil {
			if i := f.find(t.key); i >= 0 {
				t = f.target(i)
			}
		}
		cmds = run(cmds, t.handlers, hovers, t.event(msg, s.at, false))
	}
	// The pointer leaves the innermost node first, and enters it last.
	for i := len(next) - 1; i >= 0; i-- {
		if t := &next[i]; !containsKey(hovered, t.key) {
			cmds = run(cmds, t.handlers, hovers, t.event(msg, s.at, true))
		}
	}
	clear(hovered)
	r.r.hovered, s.next = next, hovered[:0]
	return cmds
}

func containsKey(ts []mouseTarget, k nodeKey) bool {
	return slices.ContainsFunc(ts, func(t mouseTarget) bool { return t.key == k })
}

// ClickAt runs the handlers for a click of the left button at x, y, as a
// press and a release with nothing drawn between them, and returns their
// commands. It is for tests, which can also pass any [tea.MouseMsg] to
// [Root.Update].
func (r *Root) ClickAt(x, y int) tea.Cmd {
	return tea.Batch(
		r.route(r.r.frame, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}),
		r.route(r.r.frame, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}),
	)
}

// Click runs the handlers for a click of the left button in the middle of
// the part of the node with ID id that is on screen, as [Root.ClickAt]
// does, and reports whether the node is on screen.
func (r *Root) Click[K comparable](id K) (tea.Cmd, bool) {
	f := r.r.frame
	i := f.index(id)
	if i < 0 || f.entries[i].vis.Empty() {
		return nil, false
	}
	v := f.entries[i].vis
	return r.ClickAt((v.Min.X+v.Max.X-1)/2, (v.Min.Y+v.Max.Y-1)/2), true
}

// ThrottleMouse returns a [tea.WithFilter] filter that drops wheel and
// motion events that come within every of the last of the same kind let
// through, which spares a frame for each one when the terminal sends them
// faster than they can be drawn. A dropped event is not made up for: the
// wheel scrolls less, and when the pointer stops right after one, hover
// waits until it moves again.
func ThrottleMouse(every time.Duration) func(tea.Model, tea.Msg) tea.Msg {
	var wheel, motion time.Time
	return func(_ tea.Model, msg tea.Msg) tea.Msg {
		var last *time.Time
		switch msg.(type) {
		case tea.MouseWheelMsg:
			last = &wheel
		case tea.MouseMotionMsg:
			last = &motion
		default:
			return msg
		}
		now := time.Now()
		if now.Sub(*last) < every {
			return nil
		}
		*last = now
		return msg
	}
}
