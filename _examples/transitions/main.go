// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command transitions is a gallery of reveal transitions and of the
// combinators that sequence, stagger, ease and reverse effects. Every card
// runs its transition in and out on a loop driven by one animated value.
// Two dialogs open with transitions of their own: one with an iris that
// grows from the button that opened it, the other unfolding.
//
// Keys: d opens the iris dialog, u the unfolding one, esc closes it, space
// pauses with the gallery fully shown, q quits.
package main

import (
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the buttons that open dialogs.
const (
	buttonIris   bloom.ID = "iris"
	buttonUnfold bloom.ID = "unfold"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	t      anim.Sequence // the gallery's progress, in and out
	iris   anim.Presence
	unfold anim.Presence
	irisAt image.Point // frame coordinates of the button that opened the iris
	paused bool
}

func newModel() *model {
	open := []anim.Option{anim.Duration(500 * time.Millisecond), anim.Ease(anim.InOutCubic)}
	m := &model{
		st:     newStyles(tint.TintKanagawaWave),
		t:      progress(0, false),
		iris:   anim.NewPresence(open...),
		unfold: anim.NewPresence(open...),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	return m
}

// progress returns the gallery's progress from start: in and out over and
// over, holding at each end, or when paused, settling fully shown.
func progress(start float64, paused bool) anim.Sequence {
	steps := []anim.Step{
		anim.To(1), anim.Hold(900 * time.Millisecond),
		anim.To(0), anim.Hold(900 * time.Millisecond),
	}
	if paused {
		steps = []anim.Step{anim.To(1, anim.Duration(300*time.Millisecond))}
	}
	return anim.NewSequence(start, steps...).
		With(anim.Duration(1600*time.Millisecond), anim.Ease(anim.InOutCubic))
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.t.Loop() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space":
			m.paused = !m.paused
			m.t = progress(m.t.Float(), m.paused)
			if m.paused {
				cmds = append(cmds, m.t.Play())
			} else {
				cmds = append(cmds, m.t.Loop())
			}
		case "d":
			if !m.iris.Visible() && !m.unfold.Visible() {
				m.irisAt = center(m.ui.Frame(), buttonIris)
				cmds = append(cmds, m.iris.Show())
			}
		case "u":
			if !m.iris.Visible() && !m.unfold.Visible() {
				cmds = append(cmds, m.unfold.Show())
			}
		case "esc":
			cmds = append(cmds, m.iris.Hide(), m.unfold.Hide())
		}
	}

	cmds = append(cmds, anim.Update(msg, &m.t, &m.iris, &m.unfold))
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

// layout places the gallery over the bar of buttons, and the dialogs over
// them both.
func (m *model) layout() bloom.Child {
	t := m.t.Float()
	bar := bloom.Row(
		bloom.Text("d").Style(m.st.button),
		bloom.Text("Iris dialog").ID(buttonIris),
		bloom.Text("u").Style(m.st.button),
		bloom.Text("Unfold dialog").ID(buttonUnfold),
		bloom.Text(fmt.Sprintf("t = %.2f · space: pause · q: quit", t)).Style(m.st.status).Truncate(),
	).Gap(1)
	base := bloom.Column(
		bloom.Fill(gallery{st: &m.st, t: t}),
		bar,
	)

	iris := m.dialog(
		"Iris dialog",
		"Opened by an iris that grows\nfrom the button that opened it.",
	).Presence(m.iris, fx.Iris{From: fx.At(m.irisAt)})
	unfold := m.dialog(
		"Unfold dialog",
		"Opened by unfolding from\nits middle row outward.",
	).Presence(m.unfold, fx.Unfold{})

	return bloom.Overlay(base, iris, unfold)
}

// dialog returns a modal layer over a dimmed gallery.
func (m *model) dialog(title, text string) bloom.LayerNode {
	content := bloom.Column(
		bloom.Text(title).Style(m.st.name),
		bloom.Text(strings.TrimSpace(text)),
		bloom.Text("esc closes").Style(m.st.status),
	)
	return bloom.Layer(bloom.Box(m.st.dialog, content)).Modal().Backdrop(fx.Dim{Amount: 0.6})
}

// center returns the middle of the node with the given id in frame, or the
// zero point, which an iris takes to mean the middle of what it reveals.
func center(f *bloom.Frame, id bloom.ID) image.Point {
	r, ok := f.Rect(id)
	if !ok {
		return image.Point{}
	}
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithFPS(60)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
