// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command showcase puts much of bloom on one screen, colored with
// bubbletint themes. Choosing a tint in the sidebar recolors everything: the
// panes wave back in with the new colors, and a toast decodes the tint's
// name. A pane dragged by its title floats under the pointer while the
// others fly out of its way, and a double click on the title zooms the pane
// to the full width. As the terminal narrows, panes collapse by
// priority, and a tour spotlights each part of the screen in turn.
//
// Keys: t and T cycle the tints, ? steps the tour, esc ends it, and q or
// ctrl+c quits. Everything else is the mouse.
package main

import (
	"fmt"
	"image"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// tints are the themes the sidebar offers.
var tints = []*tint.Tint{
	tint.TintTokyoNight,
	tint.TintRosePine,
	tint.TintCatppuccinMocha,
	tint.TintKanagawaWave,
	tint.TintDracula,
	tint.TintNord,
}

// The IDs of the panes, for layout animation, the tour, and telling which
// pane a dragged title is over.
const (
	paneTints    bloom.ID = "Tints"
	paneActivity bloom.ID = "Activity"
	panePalette  bloom.ID = "Palette"
	paneServices bloom.ID = "Services"
)

const hints = "drag a title to move a pane · double click it to zoom · t tint · ? tour · q quit"

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int
	tint int // the tint in use, an index of tints.

	order   []bloom.ID // the panes right of the sidebar, left to right.
	zoomed  bloom.ID   // the pane taking the full width, if any.
	zooming bool       // the panes glide, through a zoom, rather than fly.

	// The pane being dragged by its title, if any: where it was grabbed,
	// its size when picked up, and where the pointer is.
	dragging bloom.ID
	grab     image.Point
	size     image.Point
	at       image.Point

	wave anim.Value // the panes' entrance, played again for each tint.

	activity activity
	services serviceList
	toast    toast
	tour     tour
}

func newModel() *model {
	// Effects that play once run from 0 to 1 at a steady pace, and rest at
	// 1, where they have finished.
	effect := func(d time.Duration) anim.Value {
		return anim.NewValue(1, anim.Duration(d), anim.Ease(anim.Linear))
	}
	m := &model{
		st:    newStyles(tints[0]),
		order: []bloom.ID{paneActivity, panePalette, paneServices},
		wave:  effect(500 * time.Millisecond),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	m.activity = activity{st: m.st, bars: anim.NewLoop(6 * time.Second)}
	m.services = serviceList{st: m.st}
	m.toast = toast{st: m.st, shown: anim.NewPresence(anim.Spring(9, 0.6)), text: effect(700 * time.Millisecond)}
	m.tour = tour{st: m.st, shown: anim.NewPresence(anim.Duration(250 * time.Millisecond)), text: effect(800 * time.Millisecond)}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the tint's colors are used instead.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.activity.bars.Loop(), m.wave.Play(0, 1))
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, m.updateComponents(msg), anim.Update(msg, &m.wave))
	return m, tea.Batch(cmds...)
}

// updateComponents gives msg to each component: its animations move along,
// and a themeMsg gives it the new styles.
func (m *model) updateComponents(msg tea.Msg) tea.Cmd {
	m.services.Update(msg)
	return tea.Batch(m.activity.Update(msg), m.toast.Update(msg), m.tour.Update(msg))
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the tint's background too, around and beneath the
	// layout, where terminals let programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	// The bars slow down while the terminal is not focused.
	v.ReportFocus = true
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "t":
		return m.switchTint(m.tint + 1)
	case "T", "shift+t":
		return m.switchTint(m.tint - 1)
	case "?":
		return m.tour.next()
	case "esc":
		return m.tour.shown.Hide()
	}
	return nil
}

// switchTint changes to tint i, wrapping around, waving the panes back in
// with its colors. It changes the theme at once, rather than with a command,
// so that presses of t that arrive together each move on a tint.
func (m *model) switchTint(i int) tea.Cmd {
	m.tint = (i + len(tints)) % len(tints)
	m.st = newStyles(tints[m.tint])
	t := m.st.theme

	// Each component keeps a copy of the styles of its own, and takes the new
	// ones from a themeMsg.
	themed := m.updateComponents(themeMsg{st: &m.st})

	// The view paints the screen with the tint's background, and the root is
	// told the tint's colors, for effects such as the tour's spotlight that
	// blend toward them.
	m.ui.SetColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg))
	return tea.Batch(
		themed,
		m.wave.Play(0, 1),
		m.toast.notify("✦ "+t.DisplayName),
	)
}

// themeMsg gives a component the styles of a new theme, which it copies.
// They are held by pointer so that the message stays small, as each
// component checks every message for it.
type themeMsg struct{ st *styles }

// layout places the panes in a row under the header, with the toast and the
// tour over them, and the dragged pane, if any, over those.
func (m *model) layout() bloom.Child {
	row := []bloom.Child{m.slot(paneTints)}
	for _, id := range m.order {
		row = append(row, m.slot(id))
	}

	// Each tint's colors wave in. A transition's cells replace what is
	// beneath them, background and all, so the body brings its own.
	body := bloom.Child(bloom.Box(m.st.base, bloom.Row(row...).Gap(1)))
	if m.wave.Animating() {
		body = bloom.Transition(m.wave.Float(), body, fx.Wave{From: fx.Left})
	}

	header := bloom.Row(
		bloom.Text("❀ bloom").Style(m.st.logo),
		bloom.Text("showcase").Style(m.st.muted),
		bloom.Space(),
		bloom.Text(m.st.theme.DisplayName).Style(m.st.badge),
	).Gap(1)

	screen := bloom.Box(m.st.screen, bloom.Column(
		header,
		bloom.Fill(body),
		bloom.Text(hints).Style(m.st.muted).Truncate(),
	))
	layers := []bloom.LayerNode{m.toast.Layer(), m.tour.Layer(m.ui.Frame())}
	if !m.dragging.IsZero() {
		layers = append([]bloom.LayerNode{m.dragLayer()}, layers...)
	}
	return bloom.Overlay(screen, layers...)
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFPS(60), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
