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
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the form's inputs and buttons, the nodes that can take focus. A
// button shows its ID as its label.
const (
	fieldName     bloom.ID = "name"
	fieldEmail    bloom.ID = "email"
	fieldLanguage bloom.ID = "language"

	buttonReset     bloom.ID = "reset"
	buttonSave      bloom.ID = "save"
	buttonCancel    bloom.ID = "cancel"
	buttonConfirm   bloom.ID = "confirm"
	buttonBack      bloom.ID = "back"
	buttonOverwrite bloom.ID = "overwrite"
)

// The IDs of the focus scopes, which are never shown. NewID keeps them apart
// from the buttons', although a dialog and its button share a name.
var (
	scopeForm      = bloom.NewID("form")
	scopeConfirm   = bloom.NewID("confirm")
	scopeOverwrite = bloom.NewID("overwrite")
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	form  form
	toast toast

	focus bloom.Focus
	saved int

	confirm   anim.Presence
	overwrite anim.Presence // fades the overwrite dialog and its backdrop
	drop      anim.Presence // a spring, so it overshoots, for where it sits
}

func newModel() *model {
	m := &model{
		st:        newStyles(tint.TintRosePine),
		confirm:   anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		overwrite: anim.NewPresence(anim.Duration(200 * time.Millisecond)),
		drop:      anim.NewPresence(anim.Spring(9, 0.35)),
	}
	// Clicks move the focus, before the handler of what was clicked runs.
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)), bloom.WithFocus(&m.focus))
	m.form = form{st: m.st, focus: &m.focus, values: make(map[bloom.ID]string), save: m.askToSave}
	m.toast = toast{st: m.st, presence: anim.NewPresence(anim.Duration(600*time.Millisecond), anim.Ease(anim.Linear))}
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
	cmds = append(cmds, anim.Update(msg, &m.confirm, &m.overwrite, &m.drop, &m.toast.presence))
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

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		return m.closeTop()
	}
	return m.form.key(msg)
}

// askToSave opens the dialog that confirms saving.
func (m *model) askToSave() tea.Cmd {
	m.focus.Push(scopeConfirm)
	m.focus.Set(buttonConfirm)
	return m.confirm.Show()
}

// confirmSave saves the profile, or, when it was saved before, opens a
// second dialog on top that asks to overwrite it.
func (m *model) confirmSave() tea.Cmd {
	if m.saved == 0 {
		return m.save()
	}
	m.focus.Push(scopeOverwrite)
	m.focus.Set(buttonBack)
	return tea.Batch(m.overwrite.Show(), m.drop.Show())
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
	return tea.Batch(m.closeTop(), m.closeTop(), m.toast.show())
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

// layout places the form, with the status line below it, and the layers
// over them.
func (m *model) layout() bloom.Child {
	status := fmt.Sprintf(
		" focus: %s · in the form: %v · dialogs open: %d · saved %d times",
		m.focus.Current(), m.focus.Within(m.ui.Frame(), scopeForm), m.dialogs(), m.saved,
	)

	base := bloom.Column(
		bloom.Fill(bloom.Centered(bloom.Row(bloom.Fixed(60, &m.form).ID(scopeForm).Scope()))),
		bloom.Fixed(1, bloom.Text(status).Style(m.st.statusBar)),
	)
	return bloom.Overlay(base, m.layers()...)
}

func (m *model) layers() []bloom.LayerNode {
	var layers []bloom.LayerNode

	// The dropdown is a plain layer under the language input, not modal.
	if menu, ok := m.form.dropdown(); ok {
		layers = append(layers, menu)
	}

	profile := fmt.Sprintf("%s <%s>, %s",
		m.form.values[fieldName], m.form.values[fieldEmail], m.form.values[fieldLanguage])
	confirmBody := bloom.Column(
		bloom.Text("Save profile?").Style(m.st.heading),
		bloom.Text(profile),
		bloom.Fixed(1, nil),
		bloom.Row(
			bloom.Space(),
			button(&m.st, &m.focus, buttonCancel, m.closeTop),
			button(&m.st, &m.focus, buttonConfirm, m.confirmSave),
		).Gap(2),
	)

	confirm := bloom.Layer(bloom.Box(m.st.dialog, confirmBody)).
		ID(scopeConfirm).
		Modal().
		On(on.ClickOutside(m.closeTop)).
		Backdrop(m.st.backdrop).
		Shadow(1, 1, fx.Dim{Amount: 0.7}).
		Presence(m.confirm, fx.Fade{}, fx.Slide{From: fx.Bottom})

	// The spring overshoots 1, which bounces the dialog past its place. It
	// only moves the dialog: its fade and backdrop follow an eased presence.
	drop := int(math.Round((1 - m.drop.Float()) * -10))
	overwriteBody := bloom.Column(
		bloom.Text("Overwrite the saved profile?").Style(m.st.headingWarn),
		bloom.Text(fmt.Sprintf("It was saved %d time(s) before.", m.saved)),
		bloom.Fixed(1, nil),
		bloom.Row(
			bloom.Space(),
			button(&m.st, &m.focus, buttonBack, m.closeTop),
			button(&m.st, &m.focus, buttonOverwrite, m.save),
		).Gap(2),
	)
	overwrite := bloom.Layer(bloom.Box(m.st.danger, overwriteBody)).
		ID(scopeOverwrite).
		Modal().
		On(on.ClickOutside(m.closeTop)).
		Offset(0, drop).
		Backdrop(m.st.backdrop).
		Shadow(2, 1, fx.Dim{Amount: 0.7}).
		Presence(m.overwrite, fx.Fade{})

	return append(layers, confirm, overwrite, m.toast.Layer())
}

// button is a button labeled with its ID, which runs press when pressed. It
// is highlighted while it has focus.
func button(st *styles, focus *bloom.Focus, id bloom.ID, press func() tea.Cmd) bloom.Child {
	s := st.button
	if focus.Is(id) {
		s = st.buttonOn
	}
	return bloom.Text(string(id)).Style(s).ID(id).Focusable().On(on.Press(press))
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
