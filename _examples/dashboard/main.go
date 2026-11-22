// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command dashboard is a service dashboard redrawn at up to 120 frames a
// second: a sidebar that springs open and shut, live sparklines, a log
// stream, a scrolling table, a confirmation dialog that slides up over a
// dimmed backdrop, and a toast for each cell effect. The status bar shows
// how long the layout engine took to draw the last frame.
//
// Keys: up/down select, enter restarts the selected service, tab moves
// focus, ctrl+b toggles the sidebar, 1-6 show a toast with each effect, q
// quits. Everything can be clicked.
package main

import (
	"fmt"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

var services = []string{
	"api", "auth", "billing", "cache", "gateway",
	"ingest", "mailer", "search", "storage", "worker",
}

// paneID is the ID of a pane, for focus and layout animation.
type paneID string

const (
	paneSidebar paneID = "sidebar"
	paneMain    paneID = "main"
	paneTable   paneID = "table"
	paneLogs    paneID = "logs"
	paneDialog  paneID = "dialog"
)

type tickMsg time.Time

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	focus bloom.Focus[paneID]
	frame int
	drawn time.Duration // time the last frame took to lay out and draw

	sidebar sidebar
	logs    logs
	confirm confirm // the restart dialog
	toast   toast
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintAtomOneDark)}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithFocus(&m.focus))
	m.sidebar = sidebar{st: m.st, open: true}
	m.logs = logs{st: m.st}
	m.confirm = confirm{
		st:    m.st,
		shown: anim.NewPresence(anim.Duration(220*time.Millisecond), anim.FPS(120)),
		close: m.closeDialog,
	}
	m.toast = toast{
		st: m.st,
		shown: anim.NewPresence(
			anim.Duration(500*time.Millisecond),
			anim.Ease(anim.Linear),
			anim.FPS(120),
		),
	}
	m.focus.Set(paneTable)
	return m
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return tick() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.frame++
		if m.frame%6 == 0 {
			at := time.Time(msg).Format("15:04:05")
			svc := services[m.frame%len(services)]
			m.logs.add(fmt.Sprintf("%s  %-8s %3dms  200 OK", at, svc, 8+m.frame%91))
		}
		cmds = append(cmds, tick())
	case tea.KeyPressMsg:
		if m.confirm.shown.Shown() {
			cmds = append(cmds, m.key(msg.String()))
		} else if cmd, ok := m.focus.Update(msg, m.ui.Frame()); ok {
			cmds = append(cmds, cmd)
		} else {
			cmds = append(cmds, m.key(msg.String()))
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.confirm.shown, &m.toast.shown))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	start := time.Now()
	v := m.ui.View(m.w, m.h, m.layout())
	m.drawn = time.Since(start)
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

func (m *model) key(k string) tea.Cmd {
	if m.confirm.shown.Shown() {
		return m.confirm.key(k)
	}
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "up":
		m.sidebar.selected = (m.sidebar.selected + len(services) - 1) % len(services)
	case "down":
		m.sidebar.selected = (m.sidebar.selected + 1) % len(services)
	case "enter":
		m.focus.Push(paneDialog)
		return m.confirm.open()
	case "ctrl+b":
		m.sidebar.open = !m.sidebar.open
	case "1", "2", "3", "4", "5", "6":
		return m.toast.show(int(k[0] - '1'))
	}
	return nil
}

// closeDialog closes the restart dialog, restarting the selected service if
// restart is true.
func (m *model) closeDialog(restart bool) tea.Cmd {
	if restart {
		m.logs.add(fmt.Sprintf("restarting %s…", services[m.sidebar.selected]))
	}
	m.focus.Pop()
	return m.confirm.shown.Hide()
}

// layout places the components on the screen.
func (m *model) layout() bloom.Child {
	title := fmt.Sprintf("◆ fleet  ·  %d services  ·  %s", len(services), services[m.sidebar.selected])

	// The sidebar and the content beside it move together as the sidebar
	// springs open and shut.
	spring := []anim.Option{anim.Spring(14, 0.8), anim.FPS(120)}
	sidebarWidth := 0
	if m.sidebar.open {
		sidebarWidth = 22
	}
	sidebar := bloom.Fixed(sidebarWidth, bloom.Box(m.panel(paneSidebar), &m.sidebar)).
		ID(paneSidebar).
		Focusable().
		Collapsible(1).
		Animate(spring...)

	requests := bloom.Fill(bloom.Box(m.panel(paneTable), bloom.Scroll(table(&m.st, m.frame), m.sidebar.selected))).
		ID(paneTable).
		Focusable().
		Min(30)
	logs := bloom.Fixed(46, bloom.Box(m.panel(paneLogs), &m.logs)).
		ID(paneLogs).
		Focusable().
		Collapsible(2)
	content := bloom.Fill(bloom.Column(
		metrics{st: &m.st, frame: m.frame},
		bloom.Fill(bloom.Row(requests, logs).Gap(1)),
	)).ID(paneMain).Min(40).Animate(spring...)

	base := bloom.Column(
		bloom.Text(title).Style(m.st.header),
		bloom.Fill(bloom.Row(sidebar, content).Gap(1)),
		statusbar{st: &m.st, drawn: m.drawn},
	)
	return bloom.Overlay(base, m.toast.Layer(), m.confirm.Layer(services[m.sidebar.selected]))
}

// panel is the frame of pane id, which shows whether it has focus.
func (m *model) panel(id paneID) lipgloss.Style {
	if m.focus.Is(id) {
		return m.st.focused
	}
	return m.st.panel
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithFPS(120)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
