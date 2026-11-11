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
	"cmp"
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintCatppuccinMocha

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	dark   = theme.Bg
	raised = lipgloss.Lighten(theme.Bg, 0.08)

	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	item     = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	hovered  = item.Background(raised)
	selected = item.Foreground(dark).Background(accent).Bold(true)
	divider  = lipgloss.NewStyle().Foreground(muted)
	grabbed  = divider.Foreground(accent)
	heading  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	code     = lipgloss.NewStyle().Foreground(fg)
	status   = lipgloss.NewStyle().Foreground(muted)
)

var files = []string{"main.go", "model.go", "view.go", "update.go", "styles.go", "keys.go"}

type model struct {
	ui       *bloom.Root
	w, h     int
	split    int // the sidebar's width, which dragging the divider sets
	selected int
	opened   string
	reader   *bloom.Buffer // the text of the opened file
	at       bloom.ViewportPos
	status   string
}

func newModel() *model {
	return &model{
		ui:     bloom.New(bloom.WithColors(theme.Fg, theme.Bg), bloom.WithClipboard(clip.Write)),
		split:  22,
		reader: bloom.NewBuffer("double click a file to open it"),
		status: "hover, click, or double click a file; drag the divider or over the text",
	}
}

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

// open opens file f in the reader, from its top.
func (m *model) open(f string) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("%2d  // line %d of %s", i+1, i+1, f)
	}
	m.reader.SetLines(lines...)
	m.opened, m.at, m.status = f, bloom.ViewportPos{}, "opened "+f
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	// All motion, not only while a button is held, for the hover styles.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func (m *model) layout() bloom.Child {
	reader := bloom.Column(
		bloom.Text(cmp.Or(m.opened, "reader")).Style(heading.Padding(0, 1)),
		// The text wraps when the reader is narrow, and the wheel scrolls it.
		bloom.Fill(bloom.Box(lipgloss.NewStyle().Padding(0, 1), bloom.Viewport(m.reader).At(m.at).
			Wrap(bloom.WrapWords).Style(code).Selectable().
			OnScroll(func(p bloom.ViewportPos) tea.Cmd { m.at = p; return nil }))),
	)
	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fixed(m.split, bloom.Box(panel, m.fileList())),
			bloom.Fixed(1, m.divider()),
			bloom.Fill(bloom.Box(panel, reader)),
		)),
		bloom.Text(" "+m.status).Style(status).Truncate(),
	)
}

// fileList is a row for each file, which lights up under the pointer.
func (m *model) fileList() bloom.Child {
	rows := make([]bloom.Child, 0, len(files)+1)
	for i, f := range files {
		st, hover := item, hovered
		if i == m.selected {
			st, hover = selected, selected
		}
		rows = append(rows, bloom.Text(f).Style(st).HoverStyle(hover).On(
			on.Click(func() tea.Cmd {
				m.selected, m.status = i, "selected "+f
				return nil
			}),
			on.DoubleClick(func() tea.Cmd {
				m.open(f)
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
	return bloom.Text(line).Style(divider).HoverStyle(grabbed).On(on.Drag(func(e on.Event) tea.Cmd {
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
