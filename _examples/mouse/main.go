// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command mouse is a file browser driven by mouse handlers on the nodes
// they are about, with no IDs to tell events apart. Files light up under
// the pointer with a hover style, which keeps no state in the model; a
// click selects one and a double click opens it; the wheel scrolls the
// file; and the one-cell divider between the panes can be dragged, keeping
// the drag after the pointer leaves it. The file's text is selectable:
// dragging over it selects text inside the reader alone, including lines
// scrolled out of sight, and ctrl+c copies it. Wheel and motion events are
// throttled with bloom.ThrottleMouse.
//
// Keys: ctrl+c copies the selection, or quits when there is none; esc
// clears the selection; q quits. Everything else is the mouse.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

var files = []string{"main.go", "model.go", "view.go", "update.go", "styles.go", "keys.go"}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	reader   reader
	split    int // the sidebar's width, which dragging the divider sets
	selected int
	status   string
}

func newModel() *model {
	m := &model{
		st:     newStyles(tint.TintCatppuccinMocha),
		split:  22,
		status: "hover, click, or double click a file; drag the divider or over the text",
	}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithClipboard(clip.Write))
	m.reader = reader{st: m.st, buf: bloom.NewBuffer("double click a file to open it")}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case bloom.SelectionMsg:
		m.status = fmt.Sprintf("selected %d lines; ctrl+c copies", strings.Count(msg.Text, "\n")+1)
	case bloom.CopyMsg:
		m.status = fmt.Sprintf("copied %d characters", len(msg.Text))
		if msg.Err != nil {
			m.status = "copied with OSC 52 only: " + msg.Err.Error()
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if copied := m.ui.CopySelection(); copied != nil {
				return m, tea.Batch(cmd, copied)
			}
			return m, tea.Quit
		case "esc":
			m.ui.ClearSelection()
		case "q":
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m *model) View() tea.View {
	// The file list, the divider, and the reader side by side, over the
	// status line.
	v := m.ui.View(m.w, m.h, bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fixed(m.split, bloom.Box(m.st.panel, m.fileList())),
			bloom.Fixed(1, m.divider()),
			bloom.Fill(bloom.Box(m.st.panel, &m.reader)),
		)),
		bloom.Text(" "+m.status).Style(m.st.status).Truncate(),
	))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

// fileList is a row for each file, which lights up under the pointer.
func (m *model) fileList() bloom.Child {
	rows := make([]bloom.Child, 0, len(files)+1)
	for i, f := range files {
		st, hover := m.st.item, m.st.hovered
		if i == m.selected {
			st, hover = m.st.selected, m.st.selected
		}
		rows = append(rows, bloom.Text(f).Style(st).HoverStyle(hover).On(
			on.Click(func() tea.Cmd {
				m.selected, m.status = i, "selected "+f
				return nil
			}),
			on.DoubleClick(func() tea.Cmd {
				m.reader.open(f)
				m.status = "opened " + f
				return nil
			}),
		))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}

// divider is the line between the panes, which sets the sidebar's width
// while dragged.
func (m *model) divider() bloom.Child {
	line := strings.TrimSuffix(strings.Repeat("┃\n", max(m.h, 1)), "\n")
	return bloom.Text(line).Style(m.st.divider).HoverStyle(m.st.grabbed).On(on.Drag(func(e on.Event) tea.Cmd {
		m.split = min(max(e.Screen.X, 12), max(m.w-20, 12))
		m.status = fmt.Sprintf("dragging: the sidebar is %d cells", m.split)
		if e.Done {
			m.status = fmt.Sprintf("the sidebar is %d cells", m.split)
		}
		return nil
	}))
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
