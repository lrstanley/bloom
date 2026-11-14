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
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintCatppuccinMacchiato

var (
	fg      = theme.Fg
	muted   = theme.BrightBlack
	accent  = theme.Purple
	dark    = theme.Bg
	field   = lipgloss.Lighten(theme.Bg, 0.06)
	fieldOn = lipgloss.Lighten(theme.Bg, 0.16)
	listBg  = lipgloss.Lighten(theme.Bg, 0.1)

	heading  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	text     = lipgloss.NewStyle().Foreground(fg)
	label    = lipgloss.NewStyle().Foreground(muted)
	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(1, 2)
	dialog   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2)
	input    = lipgloss.NewStyle().Foreground(fg).Background(field).Padding(0, 1)
	inputOn  = input.Background(fieldOn)
	button   = lipgloss.NewStyle().Foreground(fg).Background(muted).Padding(0, 2)
	buttonOn = button.Foreground(dark).Background(accent).Bold(true)
	status   = lipgloss.NewStyle().Foreground(muted)
	backdrop = fx.Dim{Amount: 0.5}

	// A list's border joins its field's sides, and the edge facing the
	// field is left open.
	list         = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(accent).Background(listBg).BorderBackground(listBg)
	option       = lipgloss.NewStyle().Foreground(fg).Background(listBg).Padding(0, 1)
	optionPicked = option.Foreground(accent).Bold(true)
	optionCursor = option.Foreground(dark).Background(accent)
)

// selectWidth is the width of a select's field and of its list.
const selectWidth = 22

// control is the ID of a node that can take focus: a select, or a button,
// which shows its ID as its label.
type control string

const (
	selectTheme    control = "theme"
	selectFont     control = "font"
	selectLanguage control = "language"
	selectProfile  control = "profile"

	buttonSettings control = "settings"
	buttonCancel   control = "cancel"
	buttonSave     control = "save"
)

// scopeSettings is the ID of the settings dialog, a focus scope.
const scopeSettings = "settings"

// selectField is a field that opens a list of options to pick one from.
type selectField struct {
	id      control
	label   string
	options []string
	value   int
	open    bool
	cursor  int // the option under the cursor while open
}

type model struct {
	ui     *bloom.Root
	w, h   int
	focus  bloom.Focus[control]
	status string

	theme, font, language, profile *selectField

	settings anim.Presence
	saved    [3]int // what the dialog's selects were when it opened, for cancel
}

func newModel() *model {
	m := &model{
		theme: &selectField{id: selectTheme, label: "Theme", options: []string{
			"Macchiato", "Mocha", "Frappé", "Latte",
		}},
		font: &selectField{id: selectFont, label: "Font size", options: []string{
			"12", "13", "14", "16", "18",
		}, value: 2},
		language: &selectField{id: selectLanguage, label: "Language", options: []string{
			"English", "Deutsch", "Español", "Français", "Italiano", "日本語", "Português", "Svenska",
		}},
		profile: &selectField{id: selectProfile, label: "Profile", options: []string{
			"default", "work", "presentation", "focus",
		}},
		settings: anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		status:   "press s for the settings, or tab to the profile",
	}
	// Clicks move the focus, before the handler of what was clicked runs.
	m.ui = bloom.New(bloom.WithColors(theme.Fg, theme.Bg), bloom.WithFocus(&m.focus))
	m.focus.Set(buttonSettings)
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

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	open := m.openList()
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "s":
		if m.focus.Scope() == nil {
			return m.openSettings()
		}
	case "up", "k":
		if open != nil {
			open.cursor = (open.cursor + len(open.options) - 1) % len(open.options)
		}
	case "down", "j":
		if open != nil {
			open.cursor = (open.cursor + 1) % len(open.options)
		}
	case "esc":
		if open != nil {
			open.open = false
			return nil
		}
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

// toggle opens the list of s, or picks the option under its cursor and
// closes it.
func (m *model) toggle(s *selectField) {
	if s.open {
		m.pick(s, s.cursor)
		return
	}
	m.closeLists()
	s.open, s.cursor = true, s.value
}

func (m *model) pick(s *selectField, i int) {
	s.value, s.open = i, false
	m.status = fmt.Sprintf("%s: %s", s.label, s.options[i])
}

// press presses the button with ID b.
func (m *model) press(b control) tea.Cmd {
	switch b {
	case buttonSettings:
		return m.openSettings()
	case buttonCancel:
		return m.closeSettings(false)
	case buttonSave:
		return m.closeSettings(true)
	case selectTheme, selectFont, selectLanguage, selectProfile:
		// Selects are toggled, not pressed.
	}
	return nil
}

func (m *model) openSettings() tea.Cmd {
	m.closeLists()
	m.saved = [3]int{m.theme.value, m.font.value, m.language.value}
	m.focus.Push(scopeSettings)
	m.focus.Set(selectTheme)
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
	about := bloom.Box(panel, bloom.Column(
		bloom.Text("Dropdowns").Style(heading),
		bloom.Fixed(1, nil),
		bloom.Text("Each select keeps its list in an overlay of its own, next to its field. "+
			"The list floats, so it's drawn whole past the edges of the dialog it's in.").Style(text),
		bloom.Fixed(1, nil),
		bloom.Text("The profile's list opens above the bar, as there's no room below it.").Style(text),
	))

	bar := bloom.Row(
		m.button(buttonSettings),
		bloom.Space(),
		bloom.Text(m.profile.label).Style(label),
		bloom.Fixed(selectWidth, m.selectNode(m.profile)),
	).Gap(1)

	base := bloom.Column(
		bloom.Fill(bloom.Centered(bloom.Row(bloom.Fixed(56, about)))),
		bloom.Text(" "+m.status).Style(status).Truncate(),
		bar,
	)

	body := bloom.Column(
		bloom.Text("Settings").Style(heading),
		bloom.Fixed(1, nil),
		m.row(m.theme),
		bloom.Fixed(1, nil),
		m.row(m.font),
		bloom.Fixed(1, nil),
		m.row(m.language),
		bloom.Fixed(1, nil),
		m.buttons(buttonCancel, buttonSave),
	)
	settings := bloom.Layer(bloom.Box(dialog, body)).
		ID(scopeSettings).
		Modal().
		On(on.ClickOutside(func() tea.Cmd { return m.closeSettings(false) })).
		Backdrop(backdrop).
		Shadow(1, 1, fx.Dim{Amount: 0.7}).
		Presence(m.settings, fx.Fade{})
	return bloom.Overlay(base, settings)
}

// row is a select in the dialog, with its label.
func (m *model) row(s *selectField) bloom.Child {
	return bloom.Row(
		bloom.Fixed(12, bloom.Text(s.label).Style(label)),
		bloom.Fixed(selectWidth, m.selectNode(s)),
	)
}

// selectNode is the field of s, and its list while it is open. Both are in
// an overlay of their own, with the list in a layer that floats: the list is
// placed against the whole screen, and drawn over what the select is in,
// such as the dialog, rather than cut off at its edges.
//
// The click outside handler is on what holds both, as a click on the list is
// inside what the list is declared in, wherever the list is drawn. So a click
// on the field to close the list doesn't open it again.
func (m *model) selectNode(s *selectField) bloom.Child {
	st := input
	if m.focus.Is(s.id) {
		st = inputOn
	}
	arrow := "▾"
	if s.open {
		arrow = "▴"
	}
	// The value, then the arrow at the end of the field, inside its padding.
	value := s.options[s.value]
	value += strings.Repeat(" ", max(selectWidth-3-ansi.StringWidth(value), 0))
	in := bloom.Text(value + arrow).Style(st).
		ID(s.id).Focusable().
		On(on.Press(func() tea.Cmd {
			m.toggle(s)
			return nil
		}))

	var layers []bloom.LayerNode
	if s.open {
		layers = append(layers, bloom.Layer(optionList{m: m, s: s}).
			AnchorTo(s.id, bloom.Bottom).
			Float().
			Shadow(1, 1, fx.Dim{Amount: 0.6}))
	}
	return bloom.Row(bloom.Overlay(in, layers...)).On(on.ClickOutside(func() tea.Cmd {
		s.open = false
		return nil
	}))
}

// optionList is the open list of a select. It is a [bloom.Layouter], to read
// from its Context which side of the field it opened on.
type optionList struct {
	m *model
	s *selectField
}

func (l optionList) View() string { return "" }

func (l optionList) Layout(ctx bloom.Context) bloom.Child {
	rows := make([]bloom.Child, len(l.s.options))
	for i, o := range l.s.options {
		st := option
		switch i {
		case l.s.cursor:
			st = optionCursor
		case l.s.value:
			st = optionPicked
		}
		rows[i] = bloom.Text(o).Style(st).HoverStyle(optionCursor).On(on.Click(func() tea.Cmd {
			l.m.pick(l.s, i)
			return nil
		}))
	}
	// The edge facing the field is open: the list hangs from the field
	// below it, and stands on it above it.
	box := list.BorderTop(false)
	if side, _ := ctx.Side(); side == bloom.Top {
		box = list.BorderBottom(false)
	}
	return bloom.Row(bloom.Fixed(selectWidth, bloom.Box(box, bloom.Column(rows...))))
}

func (m *model) button(id control) bloom.Child {
	st := button
	if m.focus.Is(id) {
		st = buttonOn
	}
	return bloom.Text(string(id)).Style(st).ID(id).Focusable().
		On(on.Press(func() tea.Cmd { return m.press(id) }))
}

// buttons is a row of buttons, at its end.
func (m *model) buttons(ids ...control) bloom.Child {
	row := []bloom.Child{bloom.Space()}
	for _, id := range ids {
		row = append(row, m.button(id))
	}
	return bloom.Row(row...).Gap(2)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
