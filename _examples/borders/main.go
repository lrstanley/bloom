// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command borders shows what can be drawn on a box's border: a title,
// clickable filter tabs, and a position on its edges, clickable icons down
// its side, scrollbars that can be dragged, which are only there while there
// is more to scroll to, text running down a side, and looping border
// effects, each from one phase that runs for as long as the example does.
//
// Keys: up and down, or the mouse wheel, scroll the list; left and right
// scroll the log; tab changes the filter; space pauses the effects; q quits.
// The scrollbars light up under the pointer, and can be dragged, or clicked
// to jump.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The viewports of the list and the log, which their scrollbars follow.
const (
	paneList = "list"
	paneLog  = "log"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	list secrets
	log  logBox

	phase  anim.Sequence
	paused bool
	status string
}

func newModel() *model {
	var paths, log []string
	for _, mount := range []string{"kv", "pki", "db"} {
		for _, app := range []string{"billing", "auth", "search", "mail", "queue", "web"} {
			for _, env := range []string{"prod", "staging"} {
				paths = append(paths, fmt.Sprintf("%s/%s/%s", mount, app, env))
			}
		}
	}
	for i, p := range paths[:5] {
		log = append(log, fmt.Sprintf("12:0%d:41 read %s by token hvs.CAESIJ%04d (policy=default,%s-reader) in %dms, cache=miss, lease renewed",
			i, p, 7919*i%10000, strings.Split(p, "/")[1], 3+i*7))
	}
	m := &model{
		st:     newStyles(tint.TintTokyoNightStorm),
		phase:  anim.NewLoop(4 * time.Second),
		status: "drag a scrollbar, or click an icon on the list's left side",
	}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg))
	m.list = secrets{st: m.st, phase: &m.phase, paths: paths, listed: &bloom.Buffer{}}
	m.log = logBox{st: m.st, lines: bloom.NewBuffer(strings.Join(log, "\n"))}
	m.list.setFilter(0)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.phase.Loop() }

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
			if m.paused {
				m.phase.Stop()
			} else {
				cmds = append(cmds, m.phase.Loop())
			}
		}
		m.list.key(msg, m.ui.Frame())
		m.log.key(msg, m.ui.Frame())
	case statusMsg:
		m.status = string(msg)
	}
	cmds = append(cmds, anim.Update(msg, &m.phase))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	// The loop slows down while the terminal is not focused.
	v.ReportFocus = true
	return v
}

// layout places the list and the log, the effects beside them, and the
// status line under them all.
func (m *model) layout() bloom.Child {
	t := m.phase.Float()
	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(bloom.Column(bloom.Fill(&m.list), bloom.Fixed(7, &m.log))),
			bloom.Fixed(34, bloom.Column(
				bloom.Fill(m.st.effectBox("alert", "fx.Pulse", "draws attention to itself", t, fx.Pulse{Color: m.st.bad})),
				bloom.Fill(m.st.effectBox("syncing", "fx.Comet", "busy, once around per loop", t, fx.Comet{Color: m.st.warn, Length: 12})),
				// Marching ants move one run per loop, so they are sped up.
				bloom.Fill(m.st.effectBox("selected", "fx.March", "marching ants", 8*t, fx.March{})),
				bloom.Fill(m.st.effectBox("ripple", "fx.Chase", "three cycles of a gradient", t, fx.Chase{To: m.st.good, Cycles: 3})),
			)),
		).Gap(1)),
		bloom.Text(m.status).Style(m.st.label).Truncate(),
	)
}

// effectBox is a box with one looping border effect, at phase t, described
// inside it and named on its border.
func (st *styles) effectBox(name, code, about string, t float64, e fx.Effect) bloom.Child {
	return bloom.Box(st.panel, bloom.Text(about).Style(st.item)).
		Edge(bloom.Top, bloom.Start, bloom.Text(name).Style(st.title)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(code).Style(st.label)).
		BorderFx(t, e)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
