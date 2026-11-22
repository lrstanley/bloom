// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command tooltips is a deployments view whose tooltips come from bloom.
// The toolbar's icon buttons say what they do: the pointer resting on one
// shows its tooltip after a moment, and the next one along shows at once.
// Most place themselves where they hide the least, and the settings button
// asks for its right side. Images too long for their column are cut with
// "…", and unfold in place while the pointer is over them: moving on along
// the row, onto the status the unfold covers, closes it. A status that
// needs explaining has a tooltip of its own, which finds empty space to go
// in, and a failed one is drawn in red. The secret column is cut too, but is
// marked NoTooltip, so it never shows in full.
//
// Tooltips can go to the status line instead, which holds a TooltipSink.
// The selected row's image is pinned open, so it shows in full as the
// selection moves with the keyboard, and tab moves the focus along the
// toolbar, which shows each button's tooltip.
//
// Keys: tab and shift+tab move the focus along the toolbar, enter and space
// press the focused button, up and down (or k and j) move the selection, t
// sends tooltips to the status line and back, esc clears the selection and
// the focus, q quits.
// A click on a row selects it.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	table table

	focus bloom.Focus[action]
	tips  bloom.Tooltips
	sink  bool // tooltips go to the status line
	note  string
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintCatppuccinMocha), note: "prod-eu-1 · 6 services"}
	m.ui = bloom.New(
		bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg),
		bloom.WithFocus(&m.focus),
		bloom.WithTooltips(m.tips),
	)
	m.table = table{st: m.st, selected: -1}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmd, ok := m.focus.Update(msg, m.ui.Frame())
		if !ok {
			cmd = m.key(msg)
		}
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	// The toolbar and the table, with the status line at the bottom.
	v := m.ui.View(m.w, m.h, bloom.Column(
		bloom.Box(m.st.page, bloom.Column(
			toolbar{st: &m.st, focus: &m.focus, note: m.note, press: m.press},
			bloom.Fixed(1, nil),
			&m.table,
		)),
		bloom.Space(),
		statusline{st: &m.st, sink: m.sink},
	))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it, and reports focus, which tooltips close on losing.
	v.BackgroundColor = m.st.theme.Bg
	v.ReportFocus = true
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "esc":
		m.table.selected = -1
		m.focus.Clear()
	case "t":
		m.sink = !m.sink
		mode := bloom.TipDefault
		if m.sink {
			mode = bloom.TipSink
		}
		m.tips.Explicit, m.tips.Truncated = mode, mode
		m.ui.SetTooltips(m.tips)
	default:
		m.table.key(msg)
	}
	return nil
}

// press runs the toolbar button with ID id.
func (m *model) press(id action) tea.Cmd {
	name := "every service"
	if m.table.selected >= 0 {
		name = deployments[m.table.selected].service
	}
	switch id {
	case actionRefresh:
		m.note = "refreshed just now"
	case actionDeploy:
		m.note = "deploying " + name
	case actionRollback:
		m.note = "rolling back " + name
	case actionStop:
		m.note = "stopping " + name
	case actionSettings:
		m.note = "no settings yet"
	}
	return nil
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
