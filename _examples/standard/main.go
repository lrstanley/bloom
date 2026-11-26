// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command standard is the usual shape of a Bubble Tea program: a sidebar, a
// reader, and a status bar. The sidebar builds its own layout with Layout,
// the reader is a plain component with a View string, and the status bar
// sizes itself. The sidebar is kept by value, as components often are, so
// a click on a page sends a message for Update to handle, rather than
// changing the sidebar in the handler, which would only change a copy.
//
// The example is laid out as the others are: the styles are made from a
// tint in styles.go, each component is in a file of its own, and this file
// has the model, which keeps the components and places them in View.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// pages are the pages the sidebar lists and the reader shows.
var pages = []string{"Home", "Settings", "About"}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	sidebar sidebar
	reader  reader
	status  statusbar
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintEverforestDarkMed)}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	m.sidebar = sidebar{st: m.st}
	m.status = statusbar{msg: "click a page, or q to quit"}
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
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case openMsg:
		m.sidebar.selected, m.reader.page = int(msg), int(msg)
		m.status.msg = "opened " + pages[msg]
	}
	return m, cmd
}

func (m *model) View() tea.View {
	// The layout is simple enough to build here. A screen with more to it
	// builds its layout in a method of its own, as most of the examples do.
	v := m.ui.View(m.w, m.h, bloom.Column(
		bloom.Fill(bloom.Row(
			// The sidebar is placed by pointer, so that the tree doesn't copy it,
			// and its styles, every frame.
			bloom.Fixed(20, &m.sidebar).Collapsible(1),
			bloom.Fill(bloom.Box(m.st.page, bloom.View(&m.reader))).Min(30),
		)),
		m.status,
	))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
