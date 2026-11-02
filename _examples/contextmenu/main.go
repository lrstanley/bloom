// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command contextmenu is a mock file list with right click menus: one for
// a file, and one for the empty space below the files. A menu is a layer
// that opens where the right button came up, flipping to the left of or
// above the pointer when it would run off the screen, and closes on a click
// outside it, on escape, or once one of its items is clicked. The click
// that closes a menu still reaches what it is on, so a right click on
// another file moves the menu there.
//
// Keys: esc closes the menu, q quits. Everything else is the mouse.
package main

import (
	"cmp"
	"fmt"
	"image"
	"os"
	"path"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNightStorm

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	danger = theme.Red
	dark   = theme.Bg
	raised = lipgloss.Lighten(theme.Bg, 0.08)
	// A menu has no border, so its background sets it apart from the list.
	menuBg = lipgloss.Lighten(theme.Bg, 0.14)

	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	item     = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	hovered  = item.Background(raised)
	selected = item.Foreground(dark).Background(accent).Bold(true)
	status   = lipgloss.NewStyle().Foreground(muted)

	menuItem  = item.Background(menuBg).Width(menuWidth)
	menuHover = menuItem.Foreground(dark).Background(accent)
	menuRule  = menuItem.Foreground(muted)
)

// menuWidth is the width of a menu and its items.
const menuWidth = 16

type file struct{ name, size string }

// action is an item of a menu, or a rule between items when run is nil.
type action struct {
	label  string
	danger bool
	run    func()
}

var rule = action{}

type model struct {
	ui       *bloom.Root
	w, h     int
	files    []*file // pointers, to find the selected file again after sorting
	selected int
	created  int // the number of files made with "New file", to name the next
	status   string

	menu   []action    // the open menu's items, or none while it is closed
	menuAt image.Point // where the right button came up for the menu
}

func newModel() *model {
	return &model{
		ui: bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		files: []*file{
			{"main.go", "2.1 kB"},
			{"model.go", "4.8 kB"},
			{"view.go", "3.3 kB"},
			{"update.go", "1.7 kB"},
			{"styles.go", "920 B"},
			{"go.mod", "312 B"},
		},
		status: "right click a file, or the empty space below the files",
	}
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
		case "esc":
			m.closeMenu()
		}
	}
	return m, cmd
}

func (m *model) openMenu(at image.Point, items ...action) tea.Cmd {
	m.menu, m.menuAt = items, at
	return nil
}

func (m *model) closeMenu() tea.Cmd {
	m.menu = nil
	return nil
}

// fileMenu is the menu for the file at index i.
func (m *model) fileMenu(i int) []action {
	f := m.files[i]
	return []action{
		{label: "Open", run: func() { m.status = "opened " + f.name }},
		{label: "Duplicate", run: func() {
			ext := path.Ext(f.name)
			name := strings.TrimSuffix(f.name, ext) + " copy" + ext
			m.files = slices.Insert(m.files, i+1, &file{name, f.size})
			m.selected, m.status = i+1, "duplicated "+f.name
		}},
		{label: "Copy path", run: func() { m.status = "copied ~/project/" + f.name }},
		rule,
		{label: "Delete", danger: true, run: func() {
			m.files = slices.Delete(m.files, i, i+1)
			m.selected = min(m.selected, len(m.files)-1)
			m.status = "deleted " + f.name
		}},
	}
}

// listMenu is the menu for the empty space below the files.
func (m *model) listMenu() []action {
	return []action{
		{label: "New file", run: func() {
			m.created++
			m.files = append(m.files, &file{fmt.Sprintf("untitled-%d.txt", m.created), "0 B"})
			m.selected, m.status = len(m.files)-1, "created "+m.files[len(m.files)-1].name
		}},
		{label: "Sort by name", run: func() {
			var sel *file
			if m.selected >= 0 {
				sel = m.files[m.selected]
			}
			slices.SortStableFunc(m.files, func(a, b *file) int { return cmp.Compare(a.name, b.name) })
			m.selected, m.status = slices.Index(m.files, sel), "sorted by name"
		}},
	}
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
	rows := make([]bloom.Child, 0, len(m.files)+1)
	for i, f := range m.files {
		st, hover := item, hovered
		if i == m.selected {
			st, hover = selected, selected
		}
		label := fmt.Sprintf("%-20s %8s", f.name, f.size)
		rows = append(rows, bloom.Text(label).Style(st).HoverStyle(hover).On(
			on.Click(func() tea.Cmd {
				m.selected, m.status = i, "selected "+f.name
				return nil
			}),
			// A right click selects the file too, as file managers do.
			on.RightClickAt(func(e on.Event) tea.Cmd {
				m.selected, m.status = i, "selected "+f.name
				return m.openMenu(e.Screen, m.fileMenu(i)...)
			}),
		))
	}
	rows = append(rows, bloom.Space())

	base := bloom.Column(
		bloom.Fill(bloom.Box(panel, bloom.Column(rows...)).
			On(on.RightClickAt(func(e on.Event) tea.Cmd { return m.openMenu(e.Screen, m.listMenu()...) }))),
		bloom.Text(" "+m.status).Style(status).Truncate(),
	)
	// The overlay stays while the menu is closed: without IDs, the files are
	// followed between frames by where they are in the tree, so a click that
	// closes the menu on press still finds its file on release.
	var layers []bloom.LayerNode
	if len(m.menu) > 0 {
		layers = append(layers, m.menuLayer())
	}
	return bloom.Overlay(base, layers...)
}

// menuLayer is the open menu, with its top-left corner where the right
// button came up, unless it would run off the screen, such as after the
// terminal shrinks.
func (m *model) menuLayer() bloom.LayerNode {
	items := make([]bloom.Child, len(m.menu))
	for i, a := range m.menu {
		if a.run == nil {
			items[i] = bloom.Text(strings.Repeat("─", menuWidth-2)).Style(menuRule)
			continue
		}
		st, hover := menuItem, menuHover
		if a.danger {
			st, hover = st.Foreground(danger), hover.Background(danger)
		}
		items[i] = bloom.Text(a.label).Style(st).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			a.run()
			return m.closeMenu()
		}))
	}

	w, h := menuWidth, len(m.menu)
	at := m.menuAt
	if at.X+w > m.w {
		at.X -= w - 1
	}
	if at.Y+h > m.h {
		at.Y -= h - 1
	}
	return bloom.Layer(bloom.Column(items...)).
		Anchor(bloom.Start, bloom.Start).
		Offset(max(min(at.X, m.w-w), 0), max(min(at.Y, m.h-h), 0)).
		Shadow(1, 1, fx.Dim{Amount: 0.6}).
		On(on.ClickOutside(m.closeMenu))
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
