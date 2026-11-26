// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command responsive is a reader that adapts to the terminal's size. The
// details panel collapses first and the article list second as the terminal
// narrows; the toolbar is a component that rebuilds itself for the space it
// gets; the tags wrap onto as many lines as they need; the reader is a
// Viewport, which pages by its height on screen; the status bar is a
// Measurer, which says what bloom.Solve finds in the frame about to be
// drawn; and below the size the reader needs, bloom.MinSize shows a note
// saying the window is too small in place of the layout.
//
// Keys: up/down pick an article, pgup/pgdown or j/k scroll, q quits. The
// mouse wheel scrolls the reader and a click picks an article.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the panes, for finding them in a frame.
const (
	paneNav     bloom.ID = "nav"
	paneReader  bloom.ID = "reader"
	paneDetails bloom.ID = "details"
	paneText    bloom.ID = "text" // the reader's viewport, to scroll it
)

// minWidth and minHeight are the smallest window the reader can be used in:
// the reader's minimum width with its border, and the toolbar, status bar,
// and a few rows of text.
const minWidth, minHeight = 40, 10

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	selected int
	text     *bloom.Buffer // the selected article's body
	at       bloom.ViewportPos
}

func newModel() *model {
	m := &model{
		st:   newStyles(tint.TintGruvboxDark),
		text: bloom.NewBuffer(articles[0].body),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
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
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		// The reader scrolls from where it was on screen, by its height there
		// for a page. While it isn't on screen, as when the window is too
		// small for it, keys that would change the layout wait.
		v, ok := m.ui.Frame().Viewport(paneText)
		if !ok {
			break
		}
		switch msg.String() {
		case "up":
			m.pick(m.selected - 1)
		case "down":
			m.pick(m.selected + 1)
		case "j":
			m.at = v.Down(1)
		case "k":
			m.at = v.Up(1)
		case "pgdown", "space":
			m.at = v.PageDown()
		case "pgup":
			m.at = v.PageUp()
		}
	}
	return m, cmd
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

// layout places the screen, or below the size the reader needs, a note
// saying the window is too small in its place.
func (m *model) layout() bloom.Child {
	// The status bar says which panes fit and where the reader is. They are
	// found with bloom.Solve, which lays out the frame about to be drawn
	// without drawing it. The status bar is solved empty: as a Measurer, it
	// is the same size whatever it says.
	status := statusbar{st: &m.st, w: m.w, h: m.h}
	status.frame = bloom.Solve(m.w, m.h, m.screen(status))
	return bloom.MinSize(minWidth, minHeight, m.screen(status))
}

// screen places the toolbar over the panes, and the status bar under them.
func (m *model) screen(status statusbar) bloom.Child {
	// The details collapse first as the terminal narrows, then the list.
	nav := bloom.Fixed(24, bloom.Box(m.st.panel, m.articleList())).
		ID(paneNav).
		Collapsible(2)

	// The text wraps to the reader's width. The wheel scrolls it from
	// anywhere over the reader, its border included, three rows a notch.
	text := bloom.Viewport(m.text).ID(paneText).At(m.at).Wrap(bloom.WrapWords).Style(m.st.prose)
	reader := bloom.Fill(bloom.Box(m.st.pane, text)).
		ID(paneReader).
		Min(40).
		On(on.Scroll(func(_, dy int) tea.Cmd {
			if v, ok := m.ui.Frame().Viewport(paneText); ok {
				m.at = v.Down(dy * 3)
			}
			return nil
		}))

	info := bloom.Percent(28, details{st: &m.st, art: articles[m.selected]}).
		ID(paneDetails).
		Min(28).
		Collapsible(1)

	return bloom.Column(
		toolbar{st: &m.st, title: "reader"},
		bloom.Fill(bloom.Row(nav, reader, info).Gap(1)),
		bloom.View(status),
	)
}

// articleList is the list of articles, which can be clicked.
func (m *model) articleList() bloom.Child {
	items := make([]bloom.Child, 0, len(articles)+1)
	for i, a := range articles {
		st := m.st.item
		if i == m.selected {
			st = m.st.selected
		}
		items = append(items, bloom.Text(a.title).Style(st).On(on.Click(func() tea.Cmd { m.pick(i); return nil })))
	}
	items = append(items, bloom.Space())
	return bloom.Column(items...)
}

// pick opens article i, from its top.
func (m *model) pick(i int) {
	m.selected = min(max(i, 0), len(articles)-1)
	m.text.SetContent(articles[m.selected].body)
	m.at = bloom.ViewportPos{}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
