// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command launch is a deploy console that puts the motion effects to work.
// Cards shimmer as placeholders until their data arrives, and their numbers
// then roll over like an odometer as they change. Deploying needs the
// service's name typed in as confirmation, and a wrong one shakes the input.
// A tour spotlights each card in turn, decoding its explanation into place,
// and a successful deploy bursts confetti from the button and glitches a
// LIVE badge into the header.
//
// Keys: tab and shift+tab move focus, typing edits the input, enter deploys,
// ctrl+t steps the tour, ctrl+p pauses the numbers, ctrl+r starts over, esc
// ends the tour, ctrl+c quits.
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
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs nodes are given: cardID.Sub(i) is the card for statistic i.
const (
	cardID       bloom.ID = "card"
	inputConfirm bloom.ID = "confirm"
	buttonDeploy bloom.ID = "deploy"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	stats stats
	form  form
	tour  tour

	focus bloom.Focus
}

func newModel() *model {
	// Effects that play once run from 0 to 1 at a steady pace, and rest at 1,
	// where they have finished.
	effect := func(d time.Duration) anim.Value {
		return anim.NewValue(1, anim.Duration(d), anim.Ease(anim.Linear))
	}
	m := &model{st: newStyles(tint.TintAyuMirage)}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithFocus(&m.focus))
	m.stats = stats{
		st:      m.st,
		shimmer: anim.NewLoop(shimmerPeriod),
		every:   time.Second,
		roll:    effect(700 * time.Millisecond),
	}
	for i, c := range cards {
		m.stats.vals[i], m.stats.prev[i] = c.start, c.start
	}
	m.form = form{
		st:    m.st,
		focus: &m.focus,
		shake: effect(450 * time.Millisecond),
		burst: effect(1800 * time.Millisecond),
		badge: effect(700 * time.Millisecond),
	}
	m.tour = tour{
		st:       m.st,
		presence: anim.NewPresence(anim.Duration(250 * time.Millisecond)),
		text:     effect(800 * time.Millisecond),
	}
	m.focus.Set(inputConfirm)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd {
	return m.stats.load()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case loadedMsg, statsMsg:
		cmds = append(cmds, m.stats.Update(msg))
	case tea.KeyPressMsg:
		cmd, ok := m.focus.Update(msg, m.ui.Frame())
		if !ok {
			cmd = m.key(msg)
		}
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, anim.Update(msg,
		&m.stats.shimmer, &m.stats.roll,
		&m.form.shake, &m.form.burst, &m.form.badge,
		&m.tour.presence, &m.tour.text,
	))
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
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		return m.tour.end()
	case "ctrl+t":
		return m.tour.next()
	case "ctrl+p":
		return m.stats.pause()
	}
	return m.form.key(msg)
}

// layout places the header, the cards, the form and the keys, with the
// tour over them.
func (m *model) layout() bloom.Child {
	header := bloom.Row(
		bloom.Text("◆ deploy").Style(m.st.heading),
		bloom.Text("· production").Style(m.st.label),
		bloom.Space(),
		m.form.badgeNode(),
	).Gap(1)

	paused := ""
	if m.stats.paused {
		paused = " (paused)"
	}
	hints := fmt.Sprintf(
		"ctrl+t tour · ctrl+p pause · ctrl+r reset · updates: %d%s · tour: %d",
		m.stats.updates, paused, m.tour.step,
	)

	base := bloom.Child(bloom.Column(
		header,
		bloom.Fixed(1, nil),
		&m.stats,
		bloom.Fixed(1, nil),
		&m.form,
		bloom.Space(),
		bloom.Text(hints).Style(m.st.status),
	))

	// Confetti bursts from the middle of the button, over everything.
	if m.form.deployed && m.form.burst.Animating() {
		if r, ok := m.ui.Frame().Rect(buttonDeploy); ok {
			center := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
			burst := fx.Burst{At: center, Count: 90, Seed: m.form.bursts}
			base = bloom.Transition(m.form.burst.Float(), base, burst)
		}
	}
	return bloom.Overlay(base, m.tour.Layer(m.ui.Frame()))
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithFPS(60)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
