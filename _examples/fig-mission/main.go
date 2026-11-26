// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command fig-mission is a dashboard under a FIGlet header that steps down to
// smaller fonts as the window narrows, and to plain text when none fits,
// with a shine sweeping across its strokes now and then: a [fig.PaintFunc]
// over a gradient. Deploying shows a FIGlet
// toast over the panels, sliding in from the top: a transparent layer, so the
// panels show through between its strokes, or an opaque one, to compare.
//
// Keys: d deploys, t toggles the toast's transparency, q quits.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	version int
	deploys []string

	header header
	toast  toast
}

func newModel() *model {
	m := &model{
		st:      newStyles(tint.TintCatppuccinMocha),
		version: 41,
		deploys: []string{
			"v1.4.35 rolled out", "v1.4.36 rolled out", "v1.4.37 rolled out", "v1.4.38 rolled out",
			"v1.4.39 rolled back", "v1.4.40 rolled out", "v1.4.41 rolled out",
		},
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	m.header = newHeader()
	m.header.setStyles(m.st)
	m.toast = newToast()
	m.toast.setStyles(m.st)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.header.shimmer.Loop() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "d":
			m.version++
			m.deploys = append(m.deploys, fmt.Sprintf("v1.4.%d rolled out", m.version))
			cmds = append(cmds, m.toast.show())
		case "t":
			m.toast.opaque = !m.toast.opaque
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.header.shimmer, &m.toast.presence))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

// layout places the header and a summary over the panels, with the keys at
// the bottom, and the toast over it all.
func (m *model) layout() bloom.Child {
	up := servicesUp()
	summary := bloom.Text(fmt.Sprintf("%s %d of %d services up  ·  us-east-1  ·  v1.4.%d live",
		m.st.warn.Render("●"), up, len(services), m.version)).Style(m.st.dim)

	body := bloom.Row(
		bloom.Fill(m.st.card("services", fmt.Sprintf("%d/%d up", up, len(services)), serviceList{st: &m.st})).Min(30),
		bloom.Fill(m.st.card("requests/s", fmt.Sprintf("%d now", load[len(load)-1]), chart{st: &m.st, vals: load})).Min(26).Collapsible(2),
		bloom.Fill(m.st.card("deploys", fmt.Sprintf("%d today", len(m.deploys)), deployLog{st: &m.st, deploys: m.deploys})).Min(28).Collapsible(1),
	)

	footer := bloom.Row(
		bloom.Fill(m.st.keys("d", "deploy", "t", "transparency", "q", "quit")),
		bloom.Text(fmt.Sprintf("header: %s · toast: %s", headerFont(m.w), m.toast.kind())).Style(m.st.dim),
	)

	base := bloom.Column(
		bloom.Centered(&m.header),
		bloom.Centered(summary),
		bloom.Fill(body),
		footer,
	)
	return bloom.Overlay(base, m.toast.Layer())
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
