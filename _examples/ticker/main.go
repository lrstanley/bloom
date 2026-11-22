// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command ticker is a market ticker driven by looping animation sequences: a
// headline bar scrolling across the window, a watchlist whose long names are
// cut off with "…" except the selected one, which scrolls, a banner that
// slides in, holds, and slides out again, and an indicator pulsing while the
// feed is live. Each is a looping [anim.Sequence] or an [anim.Presence], with
// no timers of its own.
//
// Keys: up/down select, b shows a banner, space pauses the motion, q quits.
// Items can be clicked.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// headlines scroll along the headline bar, one after the other.
var headlines = []string{
	"Central bank holds rates steady, citing stubborn services inflation",
	"Chipmaker beats estimates as demand for datacenter accelerators doubles",
	"Oil slips for a third session on weaker than expected import figures",
	"Regulators approve the long-awaited merger of two regional lenders",
}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	selected int
	paused   bool

	headline anim.Sequence // 0 to 1, once around the headline bar, over and over
	name     anim.Sequence // 0 to 1, once around the selected name
	pulse    anim.Sequence // 0 to 1 and back, for the live indicator
	banner   banner
}

func newModel() *model {
	linear := []anim.Option{anim.Ease(anim.Linear), anim.FPS(60)}
	m := &model{
		st:       newStyles(tint.TintNightOwl),
		headline: anim.NewLoop(24*time.Second, anim.FPS(60)),
		// The name waits a moment, truncated, before it starts to scroll.
		name: anim.NewSequence(0,
			anim.Jump(0),
			anim.Hold(900*time.Millisecond),
			anim.To(1, anim.Duration(7*time.Second)),
		).With(linear...),
		pulse: anim.NewSequence(0,
			anim.To(1, anim.Duration(900*time.Millisecond)),
			anim.To(0, anim.Duration(900*time.Millisecond)),
		).With(anim.Ease(anim.InOutQuad), anim.FPS(30)),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg))
	m.banner = newBanner()
	m.banner.setStyles(m.st)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd {
	return m.play()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, anim.Update(msg, &m.headline, &m.name, &m.pulse, &m.banner.presence))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	// The loops slow down while the terminal is not focused.
	v.ReportFocus = true
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "up":
		return m.selectItem((m.selected + len(stocks) - 1) % len(stocks))
	case "down":
		return m.selectItem((m.selected + 1) % len(stocks))
	case "b":
		return m.banner.show()
	case "space":
		m.paused = !m.paused
		if m.paused {
			m.headline.Stop()
			m.name.Stop()
			m.pulse.Stop()
			return nil
		}
		return m.play()
	}
	return nil
}

// play starts the motion that runs for as long as the feed is live.
func (m *model) play() tea.Cmd {
	return tea.Batch(m.headline.Loop(), m.name.Loop(), m.pulse.Loop())
}

// selectItem selects an item, whose name starts scrolling from the start.
func (m *model) selectItem(i int) tea.Cmd {
	m.selected = i
	if m.paused {
		return nil
	}
	return m.name.Loop()
}

// phase returns where a looping sequence is, or 0 while the motion is
// paused, which shows names cut off and the feed still.
func (m *model) phase(s *anim.Sequence) float64 {
	if m.paused {
		return 0
	}
	return s.Float()
}

// layout places the title bar and the headlines over the watchlist, with
// the keys at the bottom, and the banner over it all.
func (m *model) layout() bloom.Child {
	pulse := blend(m.st.muted, m.st.good, m.phase(&m.pulse))
	title := bloom.Row(
		bloom.Fill(bloom.Text("◆ markets").Style(m.st.header)),
		bloom.Text("•").Style(m.st.pulse.Foreground(pulse)),
		bloom.Text(" live").Style(m.st.live),
	)

	const sep = "  ◆  "
	feed := bloom.Marquee(strings.Join(headlines, sep), m.phase(&m.headline)).Gap(sep).Style(m.st.bar)
	news := bloom.Row(
		bloom.Text("NEWS").Style(m.st.badge),
		bloom.Fill(feed),
	)

	list := watchlist{st: &m.st, selected: m.selected, phase: m.phase(&m.name), pick: m.selectItem}
	base := bloom.Column(
		title,
		news,
		bloom.Fill(bloom.Box(m.st.panel, list)),
		bloom.Text("↑↓ select · b banner · space pause · q quit").Style(m.st.status).Truncate(),
	)

	return bloom.Overlay(base, m.banner.Layer(stocks[m.selected].name))
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithFPS(120)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
