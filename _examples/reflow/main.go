// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command reflow shows automatic layout animation: a spring sidebar, a
// sortable list, a percentage split, a details panel, and an anchored layer
// following the selected item. The model holds only application state;
// ID'd slots and layers use Animate to follow their layout targets.
//
// Keys: b toggles the sidebar, s reverses the list, r cycles the split ratio,
// d toggles details, up/down select an item, and q or ctrl+c quits.
package main

import (
	"fmt"
	"os"
	"slices"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintNord

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	good   = theme.Green

	panel         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	heading       = lipgloss.NewStyle().Bold(true).Foreground(accent)
	itemStyle     = lipgloss.NewStyle().Padding(0, 1).Foreground(fg)
	selectedStyle = itemStyle.Background(lipgloss.Lighten(theme.Bg, 0.12))
	markerStyle   = lipgloss.NewStyle().Foreground(good).Bold(true)
)

const (
	sidebarText = "Workspace\n\nProjects\nActivity\nSettings"
	previewText = "Preview\n\nChildren reflow at the current visual size.\n\nTry b, s, r and d."
	help        = "b sidebar / s sort / r ratio / d details / up,down select / q quit"
)

var names = []string{"Atlas", "Birch", "Cedar", "Delta", "Ember"}

type model struct {
	ui                         *bloom.Root
	w, h                       int
	sidebar, details, reversed bool
	ratio, selected            int
}

func newModel() *model {
	return &model{ui: bloom.New(bloom.WithColors(theme.Fg, theme.Bg)), sidebar: true, details: true, ratio: 30}
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
		case "b":
			m.sidebar = !m.sidebar
		case "s":
			m.reversed = !m.reversed
		case "r":
			m.ratio += 20
			if m.ratio > 70 {
				m.ratio = 30
			}
		case "d":
			m.details = !m.details
		case "up":
			m.selected = (m.selected + len(names) - 1) % len(names)
		case "down":
			m.selected = (m.selected + 1) % len(names)
		}
	}
	return m, cmd
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	return v
}

func (m *model) layout() bloom.Child {
	sidebarWidth, detailsHeight := 0, 0
	if m.sidebar {
		sidebarWidth = 18
	}
	if m.details {
		detailsHeight = 5
	}

	// Every pane has an ID and animates, so it glides to its new place and
	// size, with its content reflowing as it goes.
	sidebar := bloom.Fixed(sidebarWidth, bloom.Box(panel, bloom.Text(sidebarText))).
		ID("sidebar").
		Animate(anim.Spring(16, 0.85))
	list := bloom.Percent(m.ratio, bloom.Box(panel, m.list())).
		ID("list").
		Animate()
	preview := bloom.Fill(bloom.Box(panel, bloom.Text(previewText))).
		ID("preview").
		Animate()
	selection := bloom.Text("Details\nSelected: " + names[m.selected])
	details := bloom.Fixed(detailsHeight, bloom.Box(panel, selection)).
		ID("details").
		Animate()
	content := bloom.Fill(bloom.Column(bloom.Fill(bloom.Row(list, preview)), details)).
		ID("content").
		Animate()

	status := fmt.Sprintf(
		"sidebar: %t | reverse: %t | ratio: %d | details: %t | selected: %s",
		m.sidebar, m.reversed, m.ratio, m.details, names[m.selected],
	)

	base := bloom.Column(
		bloom.Text("Reflow / automatic layout animations").Style(heading),
		bloom.Fill(bloom.Row(sidebar, content)),
		bloom.Text(status).Truncate(),
		bloom.Text(help).Truncate(),
	)

	// The marker follows the selected item wherever it moves.
	marker := bloom.Layer(bloom.Text("< selected").Style(markerStyle)).
		Size(10, 1).
		AnchorTo(names[m.selected], bloom.Right).
		ID("selection").
		Animate(anim.Spring(20, 1))

	return bloom.Overlay(base, marker)
}

// list is the items in their current order. Each has an ID and animates, so
// it slides to its new place when the order changes.
func (m *model) list() bloom.Child {
	order := []int{0, 1, 2, 3, 4}
	if m.reversed {
		slices.Reverse(order)
	}

	items := make([]bloom.Child, 0, len(order)+1)
	for _, i := range order {
		style := itemStyle
		if i == m.selected {
			style = selectedStyle
		}
		item := bloom.Text(names[i]).Style(style)
		items = append(items, bloom.Fixed(2, item).ID(names[i]).Animate())
	}
	items = append(items, bloom.Space())
	return bloom.Column(items...)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
