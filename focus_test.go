// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"testing"

	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func focusTree(dialog bool) Child {
	base := Column(
		Text("a").ID("a").Focusable(),
		Text("b").ID("b").Focusable(),
		Text("not focusable").ID("c"),
	)
	if !dialog {
		return base
	}
	return Overlay(base, Layer(Row(Text("ok").ID("ok").Focusable(), Text("cancel").ID("cancel").Focusable()).Gap(1)).ID("dialog").Modal())
}

func TestFocus(t *testing.T) {
	t.Parallel()

	var f Focus[string]
	r := New(WithFocus(&f))
	r.Render(20, 5, focusTree(false))
	for _, want := range []string{"a", "b", "a"} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next focused %v, want %v", focused(&f), want)
		}
	}
	f.Prev(r.Frame())
	if !f.Is("b") {
		t.Fatalf("Prev focused %v", focused(&f))
	}

	r.Render(20, 5, focusTree(true))
	f.Push("dialog")
	if _, ok := f.Current(); ok || f.Scope() != "dialog" {
		t.Fatal("Push did not clear the focus")
	}
	for _, want := range []string{"ok", "cancel", "ok"} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next inside the dialog focused %v, want %v", focused(&f), want)
		}
	}
	if !f.Within(r.Frame(), "dialog") {
		t.Fatal("focus is not within the dialog")
	}

	r.Render(20, 5, focusTree(true))
	r.ClickAt(0, 0)
	if !f.Is("ok") {
		t.Fatalf("a click outside the dialog focused %v", focused(&f))
	}

	f.Pop()
	if !f.Is("b") || f.Within(r.Frame(), "dialog") {
		t.Fatalf("Pop focused %v, want b", focused(&f))
	}

	r.Render(20, 5, focusTree(false))
	r.ClickAt(0, 0)
	if !f.Is("a") {
		t.Fatalf("clicking a focused %v", focused(&f))
	}
}

// TestFocusType checks that focus only moves between nodes with IDs of its
// type, and that untyped constants convert to it.
func TestFocusType(t *testing.T) {
	t.Parallel()

	type field int
	type button string
	tree := Row(
		Text("name").ID(field(0)).Focusable(),
		Text("x").ID("other").Focusable(),
		Text("ok").ID(button("ok")).Focusable(),
		Text("email").ID(field(1)).Focusable(),
	)
	r := New()
	r.Render(30, 1, tree)

	var f Focus[field]
	if id, ok := f.Current(); ok || f.Is(0) {
		t.Fatalf("a zero Focus has %v focused", id)
	}
	for _, want := range []field{0, 1, 0} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next focused %v, want %v", focused(&f), want)
		}
	}

	var buttons Focus[button]
	buttons.Set("ok")
	if id, ok := buttons.Current(); !ok || id != "ok" {
		t.Fatalf("Set an untyped constant: %v, %v", id, ok)
	}

	var all Focus[any]
	for _, want := range []any{field(0), "other", button("ok"), field(1)} {
		all.Next(r.Frame())
		if !all.Is(want) {
			t.Fatalf("Focus[any] focused %v, want %v", focused(&all), want)
		}
	}
}

// focused returns the ID of the node f has focused, for test messages.
func focused[K comparable](f *Focus[K]) K {
	id, _ := f.Current()
	return id
}

func TestFocusUpdate(t *testing.T) {
	t.Parallel()
	var f Focus[string]
	pressed := 0
	r := New(WithFocus(&f))
	r.Render(10, 2, Row(
		Text("a").ID("a").Focusable(),
		Auto(Text("b").ID("b").Focusable().On(on.Press(func() tea.Cmd { pressed++; return nil }))).ID("scope").Scope(),
	))
	for _, tt := range []struct {
		msg     tea.Msg
		handled bool
		want    string
	}{
		{tea.KeyPressMsg{Code: tea.KeyTab}, true, "a"},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, false, "a"}, // a has no Press handler
		{tea.KeyPressMsg{Code: tea.KeyTab}, true, "b"},
		{tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, true, "a"},
		{tea.KeyReleaseMsg{Code: tea.KeyTab}, false, "a"},
		{tea.KeyPressMsg{Code: 'x'}, false, "a"},
		{tea.MouseClickMsg{X: 1, Y: 0, Button: tea.MouseLeft}, false, "b"},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, true, "b"},
		{tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, true, "b"},
	} {
		r.Update(tt.msg)
		if _, got := f.Update(tt.msg, r.Frame()); got != tt.handled || !f.Is(tt.want) {
			t.Fatalf("%T %v: handled %v, focus %v; want %v, %s", tt.msg, tt.msg, got, focused(&f), tt.handled, tt.want)
		}
	}
	if pressed != 2 {
		t.Fatalf("enter and space pressed b %d times, want 2", pressed)
	}
	f.Push("scope")
	if _, ok := f.Update(tea.KeyPressMsg{Code: tea.KeyTab}, r.Frame()); !ok || !f.Within(r.Frame(), "scope") {
		t.Fatal("tab escaped scope")
	}
	r.ClickAt(0, 0)
	if !f.Is("b") {
		t.Fatal("click escaped scope")
	}
	if _, ok := f.Update(tea.KeyPressMsg{Code: tea.KeyTab}, nil); !ok {
		t.Fatal("tab not handled without frame")
	}
	if _, ok := f.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, nil); ok {
		t.Fatal("enter handled without frame")
	}
	// Focused, but outside the scope pushed, b can't be pressed.
	f.Set("b")
	f.Push("elsewhere")
	f.Set("b")
	if _, ok := f.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, r.Frame()); ok || pressed != 2 {
		t.Fatal("enter pressed a node outside the scope")
	}

	// Nor once it is no longer focusable, as a disabled button would be.
	f.Pop()
	r.Render(10, 2, Text("b").ID("b").On(on.Press(func() tea.Cmd { pressed++; return nil })))
	if _, ok := f.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, r.Frame()); ok || pressed != 2 {
		t.Fatal("enter pressed a node that can't be focused")
	}
}

// TestFocusEveryNode checks that focus moves to node types that could not
// be focused before every node type was given Focusable.
func TestFocusEveryNode(t *testing.T) {
	t.Parallel()

	buf := NewBuffer("v")
	tree := Column(
		Row(Text("r")).ID("row").Focusable(),
		Box(lipgloss.NewStyle(), Text("b")).ID("box").Focusable(),
		List(1, func(int) Child { return Text("l") }).ID("list").Focusable(),
		Fixed(1, Viewport(buf)).ID("slot"),
		Viewport(buf).ID("viewport").Focusable(),
		Centered(Text("c")).ID("center").Focusable(),
		Marquee("m", 0).ID("marquee").Focusable(),
	)
	var f Focus[string]
	r := New(WithFocus(&f))
	r.Render(10, 7, tree)
	for _, want := range []string{"row", "box", "list", "viewport", "center", "marquee", "row"} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next focused %v, want %v", focused(&f), want)
		}
	}
	r.ClickAt(0, 5)
	if !f.Is("center") {
		t.Fatalf("a click on the centered node focused %v", focused(&f))
	}
}

// TestFocusPicksCursor checks that with a focus, the cursor shown is the
// one inside the focused node, that one inside a node the focus could move
// to but has not is never shown, and that cursors the focus has nothing to
// do with are shown as they are without one.
func TestFocusPicksCursor(t *testing.T) {
	t.Parallel()

	panel := lipgloss.NewStyle()
	cur := tea.NewCursor(2, 0)
	input := func(id string) Child {
		// The cursor's own node has no ID: the box around it is what
		// takes the focus, as with a Bubbles text input.
		return Box(panel, Text("> "+id).Cursor(cur)).ID(id).Focusable()
	}
	viewport := func() Child {
		// Each test gets its own buffer and range, as they run in parallel.
		sel := Range{Anchor: Pos{Col: 1}, Head: Pos{Col: 1}}
		return Viewport(NewBuffer("text")).Caret(&sel).Cursor(cur).ID("vp").Focusable()
	}
	type other string
	tests := []struct {
		name  string
		tree  Child
		focus string // "" for none
		want  *image.Point
	}{
		{"the focused input", Column(input("a"), input("b")), "a", &image.Point{2, 0}},
		{"the focused input drawn last", Column(input("a"), input("b")), "b", &image.Point{2, 1}},
		{"nothing focused", Column(input("a"), input("b")), "", nil},
		{"focus on a node without a cursor", Column(Text("a").ID("a").Focusable(), input("b")), "a", nil},
		{"a cursor outside focusable nodes", Column(input("a"), Text("> c").Cursor(cur)), "", &image.Point{2, 1}},
		{"the focused cursor over a free one drawn later", Column(input("a"), Text("> c").Cursor(cur)), "a", &image.Point{2, 0}},
		{"a node of another ID type", Column(input("a"), Box(panel, Text("> c").Cursor(cur)).ID(other("c")).Focusable()), "", &image.Point{2, 1}},
		{"an unfocused input inside the focused panel", Column(input("a"), Column(input("b")).ID("panel").Focusable()), "panel", nil},
		{"text inside the focused panel", Column(input("a"), Column(Text("> b").Cursor(cur)).ID("panel").Focusable()), "panel", &image.Point{2, 1}},
		{"a focused viewport's caret", Column(input("a"), viewport()), "vp", &image.Point{1, 1}},
		{"an unfocused viewport's caret", Column(input("a"), viewport()), "a", &image.Point{2, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var f Focus[string]
			if tt.focus != "" {
				f.Set(tt.focus)
			}
			got := New(WithFocus(&f)).View(10, 2, tt.tree).Cursor
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("cursor shown at %v, want none", got.Position)
			case tt.want != nil && (got == nil || got.X != tt.want.X || got.Y != tt.want.Y):
				t.Fatalf("cursor = %+v, want %v", got, *tt.want)
			}
		})
	}
}

// TestFocusCursorFollowsClick checks that the cursor moves to the input a
// click focuses, and that without a focus the cursor drawn last is shown,
// whichever input the app treats as focused.
func TestFocusCursorFollowsClick(t *testing.T) {
	t.Parallel()

	cur := tea.NewCursor(2, 0)
	tree := Column(
		Text("> a").Cursor(cur).ID("a").Focusable(),
		Text("> b").Cursor(cur).ID("b").Focusable(),
	)
	var f Focus[string]
	f.Set("b")
	r := New(WithFocus(&f))
	if c := r.View(10, 2, tree).Cursor; c == nil || c.Y != 1 {
		t.Fatalf("cursor = %+v, want on b", c)
	}
	r.ClickAt(0, 0)
	if c := r.View(10, 2, tree).Cursor; !f.Is("a") || c == nil || c.Y != 0 {
		t.Fatalf("after clicking a, focus %v and cursor %+v, want on a", focused(&f), c)
	}

	if c := New().View(10, 2, tree).Cursor; c == nil || c.Y != 1 {
		t.Fatalf("without a focus, cursor = %+v, want the last drawn, on b", c)
	}
}

// TestFocusCursorUnderEffects checks that moving the focus between cursors
// inside an effect that has not changed hides or shows each as the effect
// does, though the frame's cells are kept.
func TestFocusCursorUnderEffects(t *testing.T) {
	t.Parallel()

	c := tea.NewCursor(0, 0)
	// The wipe shows the left half, where a is, and hides b.
	tree := Transition(0.5, Row(
		Fixed(5, Text("a").Cursor(c).ID("a").Focusable()),
		Fixed(5, Text("b").Cursor(c).ID("b").Focusable()),
	), fx.Wipe{From: fx.Left})
	var f Focus[string]
	r := New(WithFocus(&f))
	for _, id := range []string{"b", "a", "b", "a"} {
		f.Set(id)
		got := r.View(10, 1, tree).Cursor
		if id == "a" && (got == nil || got.X != 0) {
			t.Fatalf("focus on a: cursor %+v, want at 0,0", got)
		}
		if id == "b" && got != nil {
			t.Fatalf("focus on b, hidden by the wipe: cursor shown at %v", got.Position)
		}
	}
}

// TestFocusCoveredCursorGivesWay checks that the focused input's cursor,
// covered by a layer, gives way to the cursor the layer draws.
func TestFocusCoveredCursorGivesWay(t *testing.T) {
	t.Parallel()

	tree := Overlay(
		Text("> a").Cursor(tea.NewCursor(2, 0)).ID("a").Focusable(),
		Layer(Text("dialog > ").Cursor(tea.NewCursor(8, 0))).Anchor(Start, Start),
	)
	var f Focus[string]
	f.Set("a")
	if got := New(WithFocus(&f)).View(10, 1, tree).Cursor; got == nil || got.X != 8 {
		t.Fatalf("cursor %+v, want the layer's, at 8,0", got)
	}
}

// TestFocusLeavingCursor checks that the cursor of an input animating out,
// which takes no input, is not shown.
func TestFocusLeavingCursor(t *testing.T) {
	t.Parallel()

	leaving, _ := leavingPresence()
	tree := Column(
		Present(leaving, Text("> a").Cursor(tea.NewCursor(2, 0)).ID("a").Focusable()),
		Text("ok").ID("ok").Focusable(),
	)
	for _, id := range []string{"ok", "a"} {
		var f Focus[string]
		f.Set(id)
		if got := New(WithFocus(&f)).View(10, 2, tree).Cursor; got != nil {
			t.Fatalf("focus on %s: cursor of the input animating out shown at %v", id, got.Position)
		}
	}
}

// TestFocusCursorPickedOnScreen checks that the cursor is picked by where
// effects and layers leave each one: the focused one, though a layer covers
// where it was before an effect moved it, and a visible one over the
// focused one an effect hides.
func TestFocusCursorPickedOnScreen(t *testing.T) {
	t.Parallel()

	c := tea.NewCursor(0, 0)
	tests := []struct {
		name string
		tree Child
		want int // the cursor's column
	}{
		{
			// Halfway in from the left, a's cursor at 7 is drawn at 2,
			// clear of the layer over 6 to 9, which has a cursor at 7.
			"moved out from under a layer",
			Overlay(
				Transition(0.5, Text("       a").Cursor(tea.NewCursor(7, 0)).ID("a").Focusable(), fx.Slide{From: fx.Left}),
				Layer(Text("[  ]").Cursor(tea.NewCursor(1, 0))).Anchor(End, Start),
			),
			2,
		},
		{
			// The wipe shows the left half, hiding a, at 5.
			"hidden by an effect beside a free one",
			Column(
				Transition(0.5, Row(Fixed(5, Text("")), Fixed(5, Text("a").Cursor(c).ID("a").Focusable())), fx.Wipe{From: fx.Left}),
				Text("free").Cursor(tea.NewCursor(3, 0)),
			),
			3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var f Focus[string]
			f.Set("a")
			if got := New(WithFocus(&f)).View(10, 2, tt.tree).Cursor; got == nil || got.X != tt.want {
				t.Fatalf("cursor = %+v, want at column %d", got, tt.want)
			}
		})
	}

	// Without a focus, the cursor drawn last is the one, so none is shown
	// while it is covered.
	tree := Overlay(
		Column(Text("> a").Cursor(tea.NewCursor(2, 0)), Text("> b").Cursor(tea.NewCursor(2, 0))),
		Layer(Text("over b")).Anchor(Start, End),
	)
	if got := New().View(10, 2, tree).Cursor; got != nil {
		t.Fatalf("without a focus, a cursor drawn before a covered one is shown at %v", got.Position)
	}
}
