// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fields are the labels and placeholders of the form's inputs.
var fields = map[bloom.ID]struct { //nolint:exhaustive // Only inputs have labels.
	label       string
	placeholder string
}{
	fieldName:     {"Name", "Ada Lovelace"},
	fieldEmail:    {"Email", "ada@example.com"},
	fieldLanguage: {"Language", "start typing…"},
}

// isField reports whether c is an input, rather than a button.
func isField(c bloom.ID) bool {
	_, ok := fields[c]
	return ok
}

var languages = []string{
	"Go", "Rust", "Python", "TypeScript", "Zig",
	"Haskell", "OCaml", "Elixir", "Gleam", "Kotlin",
}

// form is the profile form: its inputs and what was typed into them, with
// the language input's suggestions, and its buttons.
type form struct {
	st     styles
	focus  *bloom.Focus // the model's, which moves between the inputs and buttons
	values map[bloom.ID]string
	save   func() tea.Cmd // runs when the save button is pressed
}

func (f *form) Layout(bloom.Context) bloom.Child {
	return bloom.Box(f.st.panel, bloom.Column(
		bloom.Text("Profile").Style(f.st.heading),
		f.field(fieldName),
		f.field(fieldEmail),
		f.field(fieldLanguage),
		bloom.Row(
			bloom.Space(),
			button(&f.st, f.focus, buttonReset, f.reset),
			button(&f.st, f.focus, buttonSave, f.save),
		).Gap(2),
	).Gap(1))
}

// key edits the focused input, and enter in the language input picks the
// first suggestion.
func (f *form) key(msg tea.KeyPressMsg) tea.Cmd {
	c := f.focus.Current()
	switch msg.String() {
	case "enter":
		if s := f.suggestions(); c == fieldLanguage && len(s) > 0 {
			f.values[fieldLanguage] = s[0]
		}
	case "backspace":
		if isField(c) {
			v := []rune(f.values[c])
			f.values[c] = string(v[:max(len(v)-1, 0)])
		}
	default:
		if isField(c) && msg.Text != "" {
			f.values[c] += msg.Text
		}
	}
	return nil
}

// reset empties the form.
func (f *form) reset() tea.Cmd {
	clear(f.values)
	f.focus.Set(fieldName)
	return nil
}

func (f *form) suggestions() []string {
	q := strings.ToLower(f.values[fieldLanguage])
	var out []string
	for _, l := range languages {
		if q != "" && strings.Contains(strings.ToLower(l), q) && !strings.EqualFold(l, q) {
			out = append(out, l)
		}
	}
	return out
}

func (f *form) field(id bloom.ID) bloom.Child {
	text, st := f.values[id], f.st.input
	if text == "" {
		text, st = fields[id].placeholder, f.st.hint
	}
	in := bloom.Text(text).Style(st).ID(id).Focusable()
	if f.focus.Is(id) {
		if f.values[id] == "" {
			st = f.st.inputOnHint
		} else {
			st = f.st.inputOn
		}
		in = in.Style(st).Cursor(tea.NewCursor(1+ansi.StringWidth(f.values[id]), 0))
	}
	return bloom.Row(
		bloom.Fixed(10, bloom.Text(fields[id].label).Style(f.st.label)),
		bloom.Fill(in),
	)
}

// dropdown is the layer of the language input's suggestions, and reports
// whether it is shown: while the input has focus, and has suggestions.
func (f *form) dropdown() (bloom.LayerNode, bool) {
	s := f.suggestions()
	if !f.focus.Is(fieldLanguage) || len(s) == 0 {
		return bloom.LayerNode{}, false
	}
	opts := make([]bloom.Child, len(s))
	for i, l := range s {
		opts[i] = bloom.Text(l).Style(f.st.option).On(on.Click(func() tea.Cmd {
			f.values[fieldLanguage] = l
			return nil
		}))
	}
	return bloom.Layer(bloom.Box(f.st.dropdown, bloom.Column(opts...))).
		AnchorTo(fieldLanguage, bloom.Bottom), true
}
