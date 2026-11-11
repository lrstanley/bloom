// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command forms is a profile form with stacked confirmation dialogs, in the
// style of vex. Inputs are focusable and show the terminal cursor, the
// language field suggests matches in a dropdown anchored below it, and
// saving asks for confirmation in a modal dialog, then again in a second one
// stacked on top when a profile was saved before. Each dialog is a focus
// scope: tab only moves between its buttons, and closing it gives focus
// back to where it was. The top dialog recolors everything beneath it, as
// vex does, and the second one springs into place.
//
// Keys: tab and shift+tab move focus, typing edits the focused input, enter
// presses the focused button or picks the first suggestion, esc closes the
// top dialog, ctrl+c quits. A click outside a dialog closes it.
package main

import (
	"fmt"
	"math"
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
var theme = tint.TintRosePine

var (
	fg      = theme.Fg
	muted   = theme.BrightBlack
	accent  = theme.Purple
	warn    = theme.Red
	good    = theme.Cyan
	dark    = theme.Bg
	field   = lipgloss.Lighten(theme.Bg, 0.05)
	fieldOn = lipgloss.Lighten(theme.Bg, 0.16)

	rounded   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	panel     = rounded.BorderForeground(muted).Padding(1, 2)
	heading   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	label     = lipgloss.NewStyle().Foreground(muted)
	input     = lipgloss.NewStyle().Foreground(fg).Background(field).Padding(0, 1)
	inputOn   = input.Background(fieldOn)
	hint      = input.Foreground(muted)
	button    = lipgloss.NewStyle().Foreground(fg).Background(muted).Padding(0, 2)
	buttonOn  = button.Foreground(dark).Background(accent).Bold(true)
	dropdown  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(accent)
	option    = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	dialog    = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(accent).Padding(1, 3)
	danger    = dialog.BorderForeground(warn)
	toastBox  = rounded.BorderForeground(good).Padding(0, 1)
	statusBar = lipgloss.NewStyle().Foreground(muted)
	backdrop  = fx.Recolor{Fg: lipgloss.Lighten(theme.Bg, 0.15), Bg: lipgloss.Darken(theme.Bg, 0.3)}
)

// control is the ID of a node that can take focus: an input, or a button,
// which shows its ID as its label.
type control string

// The form's inputs and buttons.
const (
	fieldName     control = "name"
	fieldEmail    control = "email"
	fieldLanguage control = "language"

	buttonReset     control = "reset"
	buttonSave      control = "save"
	buttonCancel    control = "cancel"
	buttonConfirm   control = "confirm"
	buttonBack      control = "back"
	buttonOverwrite control = "overwrite"
)

// fields are the labels and placeholders of the form's inputs.
var fields = map[control]struct{ label, placeholder string }{ //nolint:exhaustive // Only inputs have labels.
	fieldName:     {"Name", "Ada Lovelace"},
	fieldEmail:    {"Email", "ada@example.com"},
	fieldLanguage: {"Language", "start typing…"},
}

// isField reports whether c is an input, rather than a button.
func isField(c control) bool {
	_, ok := fields[c]
	return ok
}

// scopeID is the ID of a focus scope.
type scopeID string

const (
	scopeForm      scopeID = "form"
	scopeConfirm   scopeID = "confirm"
	scopeOverwrite scopeID = "overwrite"
)

var languages = []string{
	"Go", "Rust", "Python", "TypeScript", "Zig",
	"Haskell", "OCaml", "Elixir", "Gleam", "Kotlin",
}

type model struct {
	ui     *bloom.Root
	w, h   int
	focus  bloom.Focus[control]
	values map[control]string
	saved  int

	confirm   anim.Presence
	overwrite anim.Presence // fades the overwrite dialog and its backdrop
	drop      anim.Presence // a spring, so it overshoots, for where it sits
	toast     anim.Presence
}

func newModel() *model {
	m := &model{
		values:    make(map[control]string),
		confirm:   anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		overwrite: anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		drop:      anim.NewPresence(anim.Spring(9, 0.35)),
		toast:     anim.NewPresence(anim.Duration(600*time.Millisecond), anim.Ease(anim.Linear)),
	}
	// Clicks move the focus, before the handler of what was clicked runs.
	m.ui = bloom.New(bloom.WithColors(theme.Fg, theme.Bg), bloom.WithFocus(&m.focus))
	m.focus.Set(fieldName)
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
		// Tab moves the focus, and enter and space press a focused button.
		cmd, ok := m.focus.Update(msg, m.ui.Frame())
		if !ok {
			cmd = m.key(msg)
		}
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, anim.Update(msg, &m.confirm, &m.overwrite, &m.drop, &m.toast))
	return m, tea.Batch(cmds...)
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	c, _ := m.focus.Current()
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		return m.closeTop()
	case "enter":
		if s := m.suggestions(); c == fieldLanguage && len(s) > 0 {
			m.values[fieldLanguage] = s[0]
		}
	case "backspace":
		if isField(c) {
			v := []rune(m.values[c])
			m.values[c] = string(v[:max(len(v)-1, 0)])
		}
	default:
		if isField(c) && msg.Text != "" {
			m.values[c] += msg.Text
		}
	}
	return nil
}

// press presses the button with ID b.
func (m *model) press(b control) tea.Cmd {
	switch b {
	case buttonSave:
		m.focus.Push(scopeConfirm)
		m.focus.Set(buttonConfirm)
		return m.confirm.Show()
	case buttonReset:
		clear(m.values)
		m.focus.Set(fieldName)
	case buttonConfirm:
		if m.saved == 0 {
			return m.save()
		}
		m.focus.Push(scopeOverwrite)
		m.focus.Set(buttonBack)
		return tea.Batch(m.overwrite.Show(), m.drop.Show())
	case buttonOverwrite:
		return m.save()
	case buttonCancel, buttonBack:
		return m.closeTop()
	case fieldName, fieldEmail, fieldLanguage:
		// Inputs are typed into, not pressed.
	}
	return nil
}

// closeTop closes the topmost dialog, giving focus back to where it was.
func (m *model) closeTop() tea.Cmd {
	switch m.focus.Scope() {
	case scopeOverwrite:
		m.focus.Pop()
		return tea.Batch(m.overwrite.Hide(), m.drop.Hide())
	case scopeConfirm:
		m.focus.Pop()
		return m.confirm.Hide()
	}
	return nil
}

func (m *model) save() tea.Cmd {
	m.saved++
	return tea.Batch(m.closeTop(), m.closeTop(), m.toast.ShowFor(2*time.Second))
}

func (m *model) suggestions() []string {
	q := strings.ToLower(m.values[fieldLanguage])
	var out []string
	for _, l := range languages {
		if q != "" && strings.Contains(strings.ToLower(l), q) && !strings.EqualFold(l, q) {
			out = append(out, l)
		}
	}
	return out
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) field(id control) bloom.Child {
	text, st := m.values[id], input
	if text == "" {
		text, st = fields[id].placeholder, hint
	}
	in := bloom.Text(text).Style(st).ID(id).Focusable()
	if m.focus.Is(id) {
		if m.values[id] == "" {
			st = inputOn.Foreground(muted)
		} else {
			st = inputOn
		}
		in = in.Style(st).Cursor(tea.NewCursor(1+ansi.StringWidth(m.values[id]), 0))
	}
	return bloom.Row(
		bloom.Fixed(10, bloom.Text(fields[id].label).Style(label)),
		bloom.Fill(in),
	)
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

func (m *model) layout() bloom.Child {
	form := bloom.Box(panel, bloom.Column(
		bloom.Text("Profile").Style(heading),
		bloom.Fixed(1, nil),
		m.field(fieldName),
		bloom.Fixed(1, nil),
		m.field(fieldEmail),
		bloom.Fixed(1, nil),
		m.field(fieldLanguage),
		bloom.Fixed(1, nil),
		m.buttons(buttonReset, buttonSave),
	))

	focused, _ := m.focus.Current()
	status := fmt.Sprintf(
		" focus: %s · in the form: %v · dialogs open: %d · saved %d times",
		focused, m.focus.Within(m.ui.Frame(), scopeForm), m.dialogs(), m.saved,
	)

	base := bloom.Column(
		bloom.Fill(bloom.Centered(bloom.Row(bloom.Fixed(60, form).ID(scopeForm).Scope()))),
		bloom.Fixed(1, bloom.Text(status).Style(statusBar)),
	)
	return bloom.Overlay(base, m.layers()...)
}

func (m *model) dialogs() int {
	n := 0
	if m.confirm.Shown() {
		n++
	}
	if m.overwrite.Shown() {
		n++
	}
	return n
}

func (m *model) layers() []bloom.LayerNode {
	var layers []bloom.LayerNode

	// The dropdown is a plain layer under the language input, not modal.
	if s := m.suggestions(); m.focus.Is(fieldLanguage) && len(s) > 0 {
		opts := make([]bloom.Child, len(s))
		for i, l := range s {
			opts[i] = bloom.Text(l).Style(option).On(on.Click(func() tea.Cmd {
				m.values[fieldLanguage] = l
				return nil
			}))
		}
		menu := bloom.Layer(bloom.Box(dropdown, bloom.Column(opts...))).
			AnchorTo(fieldLanguage, bloom.Bottom)
		layers = append(layers, menu)
	}

	profile := fmt.Sprintf("%s <%s>, %s",
		m.values[fieldName], m.values[fieldEmail], m.values[fieldLanguage])
	confirmBody := bloom.Column(
		bloom.Text("Save profile?").Style(heading),
		bloom.Text(profile),
		bloom.Fixed(1, nil),
		m.buttons(buttonCancel, buttonConfirm),
	)

	confirm := bloom.Layer(bloom.Box(dialog, confirmBody)).
		ID(scopeConfirm).
		Modal().
		On(on.ClickOutside(m.closeTop)).
		Backdrop(backdrop).
		Shadow(1, 1, fx.Dim{Amount: 0.7}).
		Presence(m.confirm, fx.Fade{}, fx.Slide{From: fx.Bottom})

	// The spring overshoots 1, which bounces the dialog past its place. It
	// only moves the dialog: its fade and backdrop follow an eased presence.
	drop := int(math.Round((1 - m.drop.Float()) * -10))
	overwriteBody := bloom.Column(
		bloom.Text("Overwrite the saved profile?").Style(heading.Foreground(warn)),
		bloom.Text(fmt.Sprintf("It was saved %d time(s) before.", m.saved)),
		bloom.Fixed(1, nil),
		m.buttons(buttonBack, buttonOverwrite),
	)
	overwrite := bloom.Layer(bloom.Box(danger, overwriteBody)).
		ID(scopeOverwrite).
		Modal().
		On(on.ClickOutside(m.closeTop)).
		Offset(0, drop).
		Backdrop(backdrop).
		Shadow(2, 1, fx.Dim{Amount: 0.7}).
		Presence(m.overwrite, fx.Fade{})

	toast := bloom.Layer(bloom.Box(toastBox, bloom.Text("✓ Profile saved"))).
		Anchor(bloom.End, bloom.End).
		Margin(1).
		Presence(m.toast, fx.Typewriter{}, fx.Fade{})

	return append(layers, confirm, overwrite, toast)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
