// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command inputs puts two Bubbles text inputs side by side, each in a
// focusable box, and shows the real terminal cursor in the focused one.
// Both inputs are focused once, as far as Bubbles knows, so both report a
// cursor: bloom's focus, given to the root with WithFocus, picks the one
// shown, as the terminal has only one. Keys go only to the input in the
// focused box.
//
// Keys: tab and shift+tab move focus, a click focuses a box, typing edits
// the focused input, and esc or ctrl+c quits.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the boxes that take focus.
const (
	paneLeft  bloom.ID = "left"
	paneRight bloom.ID = "right"
)

var panes = []bloom.ID{paneLeft, paneRight}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	focus  bloom.Focus
	inputs map[bloom.ID]*textinput.Model
}

// newInput returns the input in the box with ID id.
func newInput(id bloom.ID) *textinput.Model {
	in := textinput.New()
	in.Prompt = string(id) + "> "
	in.Placeholder = "type here"
	// Report where the cursor is, rather than drawing one.
	in.SetVirtualCursor(false)
	// Focused once, so it takes keys, and reports its cursor; bloom shows
	// the cursor only while the input's box has bloom's focus.
	in.Focus()
	return &in
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintEverforestDarkMed)}
	// Clicks move the focus.
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)), bloom.WithFocus(&m.focus))
	m.inputs = map[bloom.ID]*textinput.Model{
		paneLeft:  newInput(paneLeft),
		paneRight: newInput(paneRight),
	}
	m.focus.Set(paneLeft)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
		// Tab and shift+tab move the focus.
		if cmd, ok := m.focus.Update(msg, m.ui.Frame()); ok {
			return m, tea.Batch(cmd, m.ui.Update(msg))
		}
	}

	// Everything else goes to the input in the focused box only.
	var cmd tea.Cmd
	if in, ok := m.inputs[m.focus.Current()]; ok {
		*in, cmd = in.Update(msg)
	}
	return m, tea.Batch(cmd, m.ui.Update(msg))
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

// layout places a box for each input, and the line of help under them.
func (m *model) layout() bloom.Child {
	boxes := make([]bloom.Child, 0, len(panes))
	for _, id := range panes {
		st := m.st.panel
		if m.focus.Is(id) {
			st = m.st.focused
		}
		in := m.inputs[id]
		// The text with the cursor needs no ID: the box around it is what
		// takes the focus.
		field := bloom.Text(in.View()).Cursor(in.Cursor())
		boxes = append(boxes, bloom.Fill(bloom.Box(st, field)).ID(id).Focusable())
	}
	return bloom.Column(
		bloom.Auto(bloom.Row(boxes...)),
		bloom.Auto(bloom.Text(fmt.Sprintf("focus: %s · tab or click to move it · esc quits", m.focus.Current())).Style(m.st.hint)),
	)
}

// resize gives each input the width of its box, inside the border and
// padding, less its prompt and a cell for the cursor after the text.
func (m *model) resize() {
	frame := bloom.Solve(m.w, m.h, m.layout())
	for _, id := range panes {
		if r, ok := frame.Rect(id); ok {
			in := m.inputs[id]
			in.SetWidth(max(0, r.Dx()-m.st.panel.GetHorizontalFrameSize()-lipgloss.Width(in.Prompt)-1))
		}
	}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
