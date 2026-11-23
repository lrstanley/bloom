// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command dropdowns is a settings dialog of select fields, each of which
// keeps its list of options next to it: a select lays out its field and its
// list together, in an overlay of its own, with the list in a layer that
// floats. The program never gathers the lists into its top-level overlay,
// yet a list is drawn whole past the edge of the dialog it is in, and over
// it. The select on the bar at the bottom of the screen opens its list
// above it, as there is no room below, and each list reads from its Context
// which side of its field it opened on, to leave the edge facing the field
// open.
//
// Keys: s opens the settings, tab and shift+tab move focus, enter and space
// open the focused select or pick the option under the cursor, up and down
// move the cursor, esc closes the list, then the dialog, q quits. A click
// outside a list closes it, and one outside the dialog closes the dialog.
package main

import (
	"fmt"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the buttons, which show their IDs as their labels. Each select
// makes its own ID.
const (
	buttonSettings bloom.ID = "settings"
	buttonCancel   bloom.ID = "cancel"
	buttonSave     bloom.ID = "save"
)

// scopeSettings is the ID of the settings dialog, a focus scope. NewID keeps
// it apart from the settings button's, although they share a name.
var scopeSettings = bloom.NewID("settings")

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	theme    *selectField
	font     *selectField
	language *selectField
	profile  *selectField

	focus  bloom.Focus
	status string

	settings anim.Presence
	saved    [3]int // what the dialog's selects were when it opened, for cancel
}

func newModel() *model {
	m := &model{
		st:       newStyles(tint.TintCatppuccinMacchiato),
		settings: anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		status:   "press s for the settings, or tab to the profile",
	}
	// Clicks move the focus, before the handler of what was clicked runs.
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithFocus(&m.focus))
	m.theme = m.newSelect("Theme", 0, []string{
		"Macchiato", "Mocha", "Frappé", "Latte",
	})
	m.font = m.newSelect("Font size", 2, []string{
		"12", "13", "14", "16", "18",
	})
	m.language = m.newSelect("Language", 0, []string{
		"English", "Deutsch", "Español", "Français", "Italiano", "日本語", "Português", "Svenska",
	})
	m.profile = m.newSelect("Profile", 0, []string{
		"default", "work", "presentation", "focus",
	})
	m.focus.Set(buttonSettings)
	return m
}

// newSelect makes a select showing options[value]. Opening its list closes
// the others, and picking an option says so in the status line.
func (m *model) newSelect(label string, value int, options []string) *selectField {
	return &selectField{
		st:         m.st,
		focus:      &m.focus,
		closeLists: m.closeLists,
		picked: func(s *selectField) {
			m.status = fmt.Sprintf("%s: %s", s.label, s.options[s.value])
		},
		id:      bloom.NewID("select"),
		label:   label,
		options: options,
		value:   value,
	}
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
		if s := msg.String(); s == "tab" || s == "shift+tab" {
			m.closeLists()
		}
		// Tab moves the focus, and enter and space press what is focused.
		cmd, ok := m.focus.Update(msg, m.ui.Frame())
		if !ok {
			cmd = m.key(msg)
		}
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, anim.Update(msg, &m.settings))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	// The open list, if any, takes the keys that move its cursor and close
	// it.
	if open := m.openList(); open != nil && open.key(msg) {
		return nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "s":
		if m.focus.Scope().IsZero() {
			return m.openSettings()
		}
	case "esc":
		return m.closeSettings(false)
	}
	return nil
}

// selects returns every select, the dialog's first.
func (m *model) selects() []*selectField {
	return []*selectField{m.theme, m.font, m.language, m.profile}
}

// openList returns the select whose list is open, or nil.
func (m *model) openList() *selectField {
	for _, s := range m.selects() {
		if s.open {
			return s
		}
	}
	return nil
}

func (m *model) closeLists() {
	for _, s := range m.selects() {
		s.open = false
	}
}

func (m *model) openSettings() tea.Cmd {
	m.closeLists()
	m.saved = [3]int{m.theme.value, m.font.value, m.language.value}
	m.focus.Push(scopeSettings)
	m.focus.Set(m.theme.id)
	return m.settings.Show()
}

// closeSettings closes the settings dialog, keeping what was picked in it
// when save is set.
func (m *model) closeSettings(save bool) tea.Cmd {
	if m.focus.Scope() != scopeSettings {
		return nil
	}
	m.closeLists()
	if save {
		m.status = fmt.Sprintf("saved: %s, %spt, %s",
			m.theme.options[m.theme.value], m.font.options[m.font.value], m.language.options[m.language.value])
	} else {
		m.theme.value, m.font.value, m.language.value = m.saved[0], m.saved[1], m.saved[2]
		m.status = "settings left as they were"
	}
	m.focus.Pop()
	return m.settings.Hide()
}

// layout places the about panel, the status line and the bar, with the
// settings dialog over them.
func (m *model) layout() bloom.Child {
	about := bloom.Box(m.st.panel, bloom.Column(
		bloom.Text("Dropdowns").Style(m.st.heading),
		bloom.Fixed(1, nil),
		bloom.Text("Each select keeps its list in an overlay of its own, next to its field. "+
			"The list floats, so it's drawn whole past the edges of the dialog it's in.").Style(m.st.text),
		bloom.Fixed(1, nil),
		bloom.Text("The profile's list opens above the bar, as there's no room below it.").Style(m.st.text),
	))

	bar := bloom.Row(
		m.button(buttonSettings, m.openSettings),
		bloom.Space(),
		bloom.Text(m.profile.label).Style(m.st.label),
		bloom.Fixed(selectWidth, m.profile),
	).Gap(1)

	base := bloom.Column(
		bloom.Fill(bloom.Centered(bloom.Row(bloom.Fixed(56, about)))),
		bloom.Text(" "+m.status).Style(m.st.status).Truncate(),
		bar,
	)

	body := bloom.Column(
		bloom.Text("Settings").Style(m.st.heading),
		m.row(m.theme),
		m.row(m.font),
		m.row(m.language),
		bloom.Row(
			bloom.Space(),
			m.button(buttonCancel, func() tea.Cmd { return m.closeSettings(false) }),
			m.button(buttonSave, func() tea.Cmd { return m.closeSettings(true) }),
		).Gap(2),
	).Gap(1)
	settings := bloom.Layer(bloom.Box(m.st.dialog, body)).
		ID(scopeSettings).
		Modal().
		On(on.ClickOutside(func() tea.Cmd { return m.closeSettings(false) })).
		Backdrop(m.st.backdrop).
		Shadow(1, 1, fx.Dim{Amount: 0.7}).
		Presence(m.settings, fx.Fade{})
	return bloom.Overlay(base, settings)
}

// row is a select in the dialog, with its label.
func (m *model) row(s *selectField) bloom.Child {
	return bloom.Row(
		bloom.Fixed(12, bloom.Text(s.label).Style(m.st.label)),
		bloom.Fixed(selectWidth, s),
	)
}

// button is a button labeled with its ID, which runs press when pressed.
func (m *model) button(id bloom.ID, press func() tea.Cmd) bloom.Child {
	st := m.st.button
	if m.focus.Is(id) {
		st = m.st.buttonOn
	}
	return bloom.Text(string(id)).Style(st).ID(id).Focusable().On(on.Press(press))
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
