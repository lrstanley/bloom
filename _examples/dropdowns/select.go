// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// selectWidth is the width of a select's field and of its list.
const selectWidth = 22

// selectField is a field that opens a list of options to pick one from.
type selectField struct {
	st    styles
	focus *bloom.Focus[control] // the model's, to show when the field has it

	// closeLists closes every select's list, and runs before this one's
	// opens, so only one is open at a time. picked runs after an option is
	// picked.
	closeLists func()
	picked     func(s *selectField)

	id      control
	label   string
	options []string
	value   int
	open    bool
	cursor  int // the option under the cursor while open
}

// Layout is the field of s, and its list while it is open. Both are in an
// overlay of their own, with the list in a layer that floats: the list is
// placed against the whole screen, and drawn over what the select is in,
// such as the dialog, rather than cut off at its edges.
//
// The click outside handler is on what holds both, as a click on the list is
// inside what the list is declared in, wherever the list is drawn. So a click
// on the field to close the list doesn't open it again.
func (s *selectField) Layout(bloom.Context) bloom.Child {
	st := s.st.input
	if s.focus.Is(s.id) {
		st = s.st.inputOn
	}
	arrow := "▾"
	if s.open {
		arrow = "▴"
	}
	// The value, then the arrow at the end of the field, inside its padding.
	value := s.options[s.value]
	value += strings.Repeat(" ", max(selectWidth-3-ansi.StringWidth(value), 0))
	in := bloom.Text(value + arrow).Style(st).
		ID(s.id).Focusable().
		On(on.Press(func() tea.Cmd {
			s.toggle()
			return nil
		}))

	var layers []bloom.LayerNode
	if s.open {
		layers = append(layers, bloom.Layer(optionList{s: s}).
			AnchorTo(s.id, bloom.Bottom).
			Float().
			Shadow(1, 1, fx.Dim{Amount: 0.6}))
	}
	return bloom.Row(bloom.Overlay(in, layers...)).On(on.ClickOutside(func() tea.Cmd {
		s.open = false
		return nil
	}))
}

// key moves the cursor of the open list with up and down, and closes it with
// esc. It reports whether it used the key.
func (s *selectField) key(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "up", "k":
		s.cursor = (s.cursor + len(s.options) - 1) % len(s.options)
	case "down", "j":
		s.cursor = (s.cursor + 1) % len(s.options)
	case "esc":
		s.open = false
	default:
		return false
	}
	return true
}

// toggle opens the list of s, or picks the option under its cursor and
// closes it.
func (s *selectField) toggle() {
	if s.open {
		s.pick(s.cursor)
		return
	}
	s.closeLists()
	s.open, s.cursor = true, s.value
}

// pick picks option i, and closes the list.
func (s *selectField) pick(i int) {
	s.value, s.open = i, false
	s.picked(s)
}

// optionList is the open list of a select. It is a component, to read
// from its Context which side of the field it opened on.
type optionList struct {
	s *selectField
}

func (l optionList) Layout(ctx bloom.Context) bloom.Child {
	st := &l.s.st
	rows := make([]bloom.Child, len(l.s.options))
	for i, o := range l.s.options {
		row := st.option
		switch i {
		case l.s.cursor:
			row = st.optionCursor
		case l.s.value:
			row = st.optionPicked
		}
		rows[i] = bloom.Text(o).Style(row).HoverStyle(st.optionCursor).On(on.Click(func() tea.Cmd {
			l.s.pick(i)
			return nil
		}))
	}
	// The edge facing the field is open: the list hangs from the field
	// below it, and stands on it above it.
	box := st.listBelow
	if side, _ := ctx.Side(); side == bloom.Top {
		box = st.listAbove
	}
	return bloom.Row(bloom.Fixed(selectWidth, bloom.Box(box, bloom.Column(rows...))))
}
