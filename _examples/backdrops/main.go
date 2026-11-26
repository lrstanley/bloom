// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command backdrops shows every backdrop effect. Behind the screen, an
// ambient effect draws a moving texture of glyphs into its empty cells:
// rings that ripple out from clicks and from the pointer as it moves,
// patches of glyphs that drift and gather around the pointer, or falling
// rain, all on one anim.Ticker. Over it, a dialog's backdrop changes the
// screen beneath it, fading in and out with the dialog: dimmed,
// desaturated, recolored, spotlit, redrawn in braille, or with an ambient
// effect of its own.
//
// Keys: tab and the arrows switch the screen's backdrop, b the dialog's, d
// opens and closes the dialog, space pauses, q quits. Click anywhere to drop
// a ripple.
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

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	clock   anim.Ticker
	drops   fx.Drops
	pointer image.Point // where the pointer is, and where it last left a drop
	trail   image.Point
	hovered bool // whether the pointer has moved since starting
	screen  int  // index into screens
	backed  int  // index into dialogs
	dialog  dialog
}

func newModel() *model {
	m := &model{
		st:    newStyles(tint.TintTokyoNight),
		clock: anim.NewTicker(),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	m.dialog = dialog{st: m.st, presence: anim.NewPresence(anim.Duration(350 * time.Millisecond))}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.clock.Start() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.MouseClickMsg:
		m.drops.Add(fx.Drop{At: image.Pt(msg.Mouse().X, msg.Mouse().Y), Time: m.clock.Elapsed()})
	case tea.MouseMotionMsg:
		m.pointer, m.hovered = image.Pt(msg.Mouse().X, msg.Mouse().Y), true
		// The pointer leaves small drops behind it as it moves.
		if d := m.pointer.Sub(m.trail); d.X*d.X+4*d.Y*d.Y >= 25 {
			m.drops.Add(fx.Drop{At: m.pointer, Time: m.clock.Elapsed(), Size: 0.3})
			m.trail = m.pointer
		}
	case tea.KeyPressMsg:
		if k := msg.String(); k == "q" || k == "ctrl+c" {
			return m, tea.Quit
		}
		cmds = append(cmds, m.key(msg))
	}
	return m, tea.Batch(append(cmds, anim.Update(msg, &m.clock, &m.dialog.presence))...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	// The clock slows while the terminal is not focused.
	v.ReportFocus = true
	// The pointer is followed everywhere, not just over hoverable nodes.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// key switches the backdrops, opens and closes the dialog, and pauses.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "tab", "right", "l", "down", "j":
		m.screen = (m.screen + 1) % len(screens)
	case "shift+tab", "left", "h", "up", "k":
		m.screen = (m.screen + len(screens) - 1) % len(screens)
	case "b":
		// The first press opens the dialog; the rest switch its backdrop.
		next := m.backed
		if m.dialog.presence.Shown() {
			next = (next + 1) % len(dialogs)
		}
		return m.showDialog(next)
	case "d":
		return m.dialog.presence.Toggle()
	case "esc":
		return m.dialog.presence.Hide()
	case "space":
		return m.clock.Toggle()
	}
	return nil
}

// pickScreen switches the screen's backdrop to i.
func (m *model) pickScreen(i int) tea.Cmd {
	m.screen = i
	return nil
}

// showDialog opens the dialog with backdrop i.
func (m *model) showDialog(i int) tea.Cmd {
	m.backed = i
	return m.dialog.presence.Show()
}

// lens is the radius the drift gathers in around the pointer, once it has
// moved.
func (m *model) lens() int {
	if !m.hovered {
		return 0
	}
	return 16
}

// layout places the panel over the screen's backdrop, with the keys at the
// bottom, and the dialog over it all.
func (m *model) layout() bloom.Child {
	screen, backed := screens[m.screen], dialogs[m.backed]

	state := "running"
	if !m.clock.Animating() {
		state = "paused"
	}
	hints := fmt.Sprintf(" %s · tab: screen backdrop · b: dialog backdrop · d: dialog · space: pause (%s) · q: quit",
		screen.name, state)
	page := bloom.Column(
		bloom.Fill(bloom.Centered(panel{
			st: &m.st, screen: m.screen, backed: m.backed,
			pickScreen: m.pickScreen, pickDialog: m.showDialog,
		})),
		bloom.Text(hints).Style(m.st.status).Truncate(),
	)

	// The screen's backdrop draws into the empty cells of the whole screen,
	// behind the panel, whose own background keeps it out.
	base := bloom.Transition(1, page, screen.effects(m)...)

	return bloom.Overlay(base, m.dialog.Layer(backed, backed.effects(m)...))
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFPS(60), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
