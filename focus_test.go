// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"testing"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
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
