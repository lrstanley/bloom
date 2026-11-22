// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command logs tails a server log of a quarter of a million lines, which
// grows as it is shown, in a bloom.Viewport. The viewport lays out and
// draws only the lines in sight, so appending to the log and scrolling
// through it cost the same at any length.
//
//   - It follows the end of the log while it is there, as tail -f does,
//     and stays put when scrolled away from it.
//   - Lines wrap at words, or are cut off and scroll sideways, and either
//     way the top line stays at the top when the wrapping changes.
//   - A search marks every match of a regular expression in the lines in
//     sight, and moves between them, through the lines out of sight too.
//   - The scrollbars on the border follow it, and drag it, and the wheel
//     scrolls it, with the same function.
//   - Text is selected with the mouse, and the rows of a wrapped line are
//     copied as the one line it is, without the line numbers beside it.
//
// Keys: up/down or j/k scroll a line, pgup/pgdown a page, home/end or g/G
// go to the start or the end; left/right or h/l scroll sideways; w wraps;
// # shows line numbers; / searches, n and N move between matches, and esc
// clears the search; p pauses the log; ctrl+c copies the selection, or quits
// when there is none; q quits.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

const (
	paneLog = "log"
	// preloaded is how many lines the log starts with.
	preloaded = 250_000
	// every is how often a line is added to the log.
	every = 150 * time.Millisecond
)

type tickMsg struct{}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	log    logview
	paused bool
	status string
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintTokyoNight), status: "following the log"}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithClipboard(clip.Write))
	m.log = newLogview()
	m.log.setStyles(m.st)
	m.log.preload(preloaded)
	return m
}

func tick() tea.Cmd { return tea.Tick(every, func(time.Time) tea.Msg { return tickMsg{} }) }

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return tick() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		if !m.paused {
			m.log.add()
		}
		return m, tea.Batch(cmd, tick())
	case bloom.SelectionMsg:
		m.status = fmt.Sprintf("selected %q; ctrl+c copies", preview(msg.Text))
	case bloom.CopyMsg:
		m.status = fmt.Sprintf("copied %q", preview(msg.Text))
	case tea.KeyPressMsg:
		if m.log.searching {
			v, _ := m.ui.Frame().Viewport(paneLog)
			if s := m.log.typeQuery(msg, v); s != "" {
				m.status = s
			}
			return m, cmd
		}
		return m, tea.Batch(cmd, m.key(msg))
	}
	return m, cmd
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

// key handles a key pressed while not searching.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	// Scrolling goes from where the viewport was in the frame on screen,
	// which knows how its lines wrapped.
	v, _ := m.ui.Frame().Viewport(paneLog)
	following := m.log.pos.AtEnd()
	switch msg.String() {
	case "up", "k":
		m.log.pos = v.Up(1)
	case "down", "j":
		m.log.pos = v.Down(1)
	case "pgup":
		m.log.pos = v.PageUp()
	case "pgdown":
		m.log.pos = v.PageDown()
	case "home", "g":
		m.log.pos = v.Top()
	case "end", "G":
		m.log.pos = bloom.ViewportEnd
	case "left", "h":
		m.log.pos = v.Left(8)
	case "right", "l":
		m.log.pos = v.Right(8)
	case "w":
		m.log.wrap = !m.log.wrap
		m.status = map[bool]string{true: "wrapping at words", false: "cut off; left and right scroll"}[m.log.wrap]
	case "#":
		m.log.numbers = !m.log.numbers
	case "p":
		m.paused = !m.paused
		m.status = map[bool]string{true: "paused", false: "resumed"}[m.paused]
	case "/":
		m.log.searching, m.log.query = true, ""
	case "n", "N":
		if s := m.log.find(v, msg.String() == "n"); s != "" {
			m.status = s
		}
	case "esc":
		m.log.re, m.log.hasFound = nil, false
		m.ui.ClearSelection()
		m.status = "search cleared"
	case "ctrl+c":
		if copied := m.ui.CopySelection(); copied != nil {
			return copied
		}
		return tea.Quit
	case "q":
		return tea.Quit
	}
	// Back at the end, the log is followed again. A key that leaves it
	// there, such as w, keeps the status it set.
	if !following && m.log.pos.AtEnd() {
		m.status = "following the log"
	}
	return nil
}

// preview is s cut to a line short enough for the status line.
func preview(s string) string {
	s = strings.ReplaceAll(s, "\n", "⏎")
	if r := []rune(s); len(r) > 40 {
		return string(r[:39]) + "…"
	}
	return s
}

// layout places the log, and the status bar under it.
func (m *model) layout() bloom.Child {
	v, _ := m.ui.Frame().Viewport(paneLog)
	return bloom.Column(
		bloom.Fill(&m.log),
		statusbar{
			st:        &m.st,
			status:    m.status,
			searching: m.log.searching,
			query:     m.log.query,
			view:      v,
			lines:     m.log.buf.Len(),
			paused:    m.paused,
		},
	)
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
