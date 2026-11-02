// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/flex"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintAtomOneDark

var (
	fg      = theme.Fg
	text    = theme.White
	accent  = theme.Blue
	muted   = theme.BrightBlack
	good    = theme.Green
	warn    = theme.Yellow
	bg      = theme.Bg
	panelBg = lipgloss.Lighten(theme.Bg, 0.05)

	// On the screen's background.
	onBg     = lipgloss.NewStyle().Background(bg)
	panel    = onBg.Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	focused  = panel.BorderForeground(accent)
	header   = onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1)
	item     = onBg.Foreground(text).Padding(0, 1)
	selected = item.Foreground(bg).Background(accent).Bold(true)
	label    = onBg.Foreground(muted)
	spark    = onBg.Foreground(good)
	logText  = onBg.Foreground(text).AlignVertical(lipgloss.Bottom)

	// On the raised background of the status bar, dialog, and toast.
	onPanel     = lipgloss.NewStyle().Background(panelBg)
	status      = onPanel.Foreground(text).Padding(0, 1)
	dialog      = onPanel.Border(lipgloss.ThickBorder()).BorderForeground(warn).Padding(1, 2)
	dialogTitle = onPanel.Bold(true)
	dialogText  = onPanel.Foreground(muted)
	button      = onPanel.Foreground(fg).Background(muted).Padding(0, 2)
	active      = button.Foreground(bg).Background(warn).Bold(true)
	toast       = onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(good).Padding(0, 1)
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

// toastEffects are what the toast shows for keys 1-6.
var toastEffects = []struct {
	name string
	fx   []fx.Effect
}{
	{"fade", []fx.Effect{fx.Fade{}}},
	{"slide", []fx.Effect{fx.Slide{From: fx.Right}}},
	{"wipe", []fx.Effect{fx.Wipe{From: fx.Left}}},
	{"dissolve", []fx.Effect{fx.Dissolve{Seed: 7}}},
	{"typewriter", []fx.Effect{fx.Typewriter{}}},
	{"scramble", []fx.Effect{fx.Scramble{Seed: 3}, fx.Fade{}}},
}

type tickMsg time.Time

type model struct {
	ui       *bloom.Root
	w, h     int
	focus    bloom.Focus[paneID]
	selected int
	restart  bool // the dialog's selected button
	frame    int
	logs     []string
	drawn    time.Duration // time the last frame took to lay out and draw

	sidebar bool          // whether the sidebar is open
	confirm anim.Presence // the restart dialog
	toast   anim.Presence
	effect  int // the toast's effect
}

func newModel() *model {
	m := &model{
		sidebar: true,
		confirm: anim.NewPresence(anim.Duration(220*time.Millisecond), anim.FPS(120)),
		toast: anim.NewPresence(
			anim.Duration(500*time.Millisecond),
			anim.Ease(anim.Linear),
			anim.FPS(120),
		),
	}
	m.ui = bloom.New(bloom.WithColors(fg, bg), bloom.WithFocus(&m.focus))
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
			m.log(fmt.Sprintf("%s  %-8s %3dms  200 OK", at, svc, 8+m.frame%91))
		}
		cmds = append(cmds, tick())
	case tea.KeyPressMsg:
		if m.confirm.Shown() {
			cmds = append(cmds, m.key(msg.String()))
		} else if cmd, ok := m.focus.Update(msg, m.ui.Frame()); ok {
			cmds = append(cmds, cmd)
		} else {
			cmds = append(cmds, m.key(msg.String()))
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.confirm, &m.toast))
	return m, tea.Batch(cmds...)
}

func (m *model) key(k string) tea.Cmd {
	if m.confirm.Shown() {
		switch k {
		case "left", "right", "tab":
			m.restart = !m.restart
		case "enter":
			return m.closeDialog(m.restart)
		case "esc":
			return m.closeDialog(false)
		}
		return nil
	}
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "up":
		m.selected = (m.selected + len(services) - 1) % len(services)
	case "down":
		m.selected = (m.selected + 1) % len(services)
	case "enter":
		m.restart = true
		m.focus.Push(paneDialog)
		return m.confirm.Show()
	case "ctrl+b":
		m.sidebar = !m.sidebar
	case "1", "2", "3", "4", "5", "6":
		m.effect = int(k[0] - '1')
		return m.toast.ShowFor(1500 * time.Millisecond)
	}
	return nil
}

func (m *model) closeDialog(restart bool) tea.Cmd {
	if restart {
		m.log(fmt.Sprintf("restarting %s…", services[m.selected]))
	}
	m.focus.Pop()
	return m.confirm.Hide()
}

func (m *model) log(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > 200 {
		m.logs = m.logs[len(m.logs)-200:]
	}
}

func (m *model) View() tea.View {
	start := time.Now()
	v := m.ui.View(m.w, m.h, m.layout())
	m.drawn = time.Since(start)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.BackgroundColor = bg
	return v
}

func (m *model) panel(id paneID) lipgloss.Style {
	if m.focus.Is(id) {
		return focused
	}
	return panel
}

func (m *model) layout() bloom.Child {
	title := fmt.Sprintf("◆ fleet  ·  %d services  ·  %s", len(services), services[m.selected])

	// The sidebar and the content beside it move together as the sidebar
	// springs open and shut.
	spring := []anim.Option{anim.Spring(14, 0.8), anim.FPS(120)}
	sidebarWidth := 0
	if m.sidebar {
		sidebarWidth = 22
	}
	sidebar := bloom.Fixed(sidebarWidth, bloom.Box(m.panel(paneSidebar), m.serviceList())).
		ID(paneSidebar).
		Focusable().
		Collapsible(1).
		Animate(spring...)

	requests := bloom.Fill(bloom.Box(m.panel(paneTable), bloom.Scroll(table(m.frame), m.selected))).
		ID(paneTable).
		Focusable().
		Min(30)
	logs := bloom.Fixed(46, bloom.Box(m.panel(paneLogs), m.logView())).
		ID(paneLogs).
		Focusable().
		Collapsible(2)
	content := bloom.Fill(bloom.Column(
		bloom.Row(m.cards()...).Gap(1),
		bloom.Fill(bloom.Row(requests, logs).Gap(1)),
	)).ID(paneMain).Min(40).Animate(spring...)

	// The key hints give way (and are cut off) before the timing.
	hints := "↑↓ select · enter restart · tab focus · ctrl+b sidebar · 1-6 effects · q quit"
	timing := fmt.Sprintf("frame %.2fms · %.1f%% of 120fps", ms(m.drawn), 100*ms(m.drawn)/8.33)
	statusBar := bloom.Row(
		bloom.Fill(bloom.Text(hints).Style(status).Truncate()),
		bloom.Text(timing).Style(status),
	)

	base := bloom.Column(
		bloom.Text(title).Style(header),
		bloom.Fill(bloom.Row(sidebar, content).Gap(1)),
		statusBar,
	)
	return bloom.Overlay(base, m.toastLayer(), m.confirmLayer())
}

// serviceList is the sidebar's list of services, which can be clicked.
func (m *model) serviceList() bloom.Child {
	items := make([]bloom.Child, 0, len(services)+1)
	for i, name := range services {
		st := item
		if i == m.selected {
			st = selected
		}
		items = append(items, bloom.Text("● "+name).Style(st).Truncate().
			On(on.Click(func() tea.Cmd { m.selected = i; return nil })))
	}
	items = append(items, bloom.Space())
	return bloom.Column(items...)
}

// cards are the metrics along the top, each with a live sparkline.
func (m *model) cards() []bloom.Child {
	names := []string{"requests/s", "p99 latency", "errors", "saturation"}
	cards := make([]bloom.Child, len(names))
	for i, name := range names {
		cards[i] = bloom.Fill(bloom.Box(panel, bloom.Column(
			bloom.Text(name).Style(label),
			bloom.Text(sparkline(m.frame, i, 40)).Style(spark),
		)))
	}
	return cards
}

func (m *model) toastLayer() bloom.LayerNode {
	e := toastEffects[m.effect]
	text := bloom.Text("✓ " + e.name + "\nan effect on a layer's cells")

	return bloom.Layer(bloom.Box(toast, text)).
		Anchor(bloom.End, bloom.End).
		Margin(2).
		Presence(m.toast, e.fx...)
}

func (m *model) confirmLayer() bloom.LayerNode {
	restart, cancel := button, active
	if m.restart {
		restart, cancel = active, button
	}

	body := bloom.Column(
		bloom.Text("Restart "+services[m.selected]+"?").Style(dialogTitle),
		bloom.Text("In-flight requests will be drained first.").Style(dialogText),
		bloom.Fixed(1, nil),
		bloom.Row(
			bloom.Text("restart").Style(restart).On(on.Click(func() tea.Cmd { return m.closeDialog(true) })),
			bloom.Text("cancel").Style(cancel).On(on.Click(func() tea.Cmd { return m.closeDialog(false) })),
		).Gap(2).Justify(flex.End),
	)

	return bloom.Layer(bloom.Box(dialog, body)).
		ID(paneDialog).
		Modal().
		On(on.ClickOutside(func() tea.Cmd { return m.closeDialog(false) })).
		Backdrop(fx.Dim{Amount: 0.55}, fx.Desaturate{Amount: 0.7}).
		Shadow(2, 1, fx.Dim{Amount: 0.6}).
		Presence(m.confirm, fx.Slide{From: fx.Bottom}, fx.Fade{})
}

// logView is the latest lines of the log, kept at the bottom of the panel.
func (m *model) logView() bloom.Child {
	recent := m.logs[max(len(m.logs)-60, 0):]
	return bloom.Text(strings.Join(recent, "\n")).Style(logText)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

var bars = []rune("▁▂▃▄▅▆▇█")

func sparkline(frame, series, n int) string {
	var b strings.Builder
	for i := range n {
		slow := math.Sin(float64(frame+i)/(4+float64(series)) + float64(series))
		fast := math.Sin(float64(frame+i*3) / 7)
		b.WriteRune(bars[int((slow+fast+2)/4*float64(len(bars)-1))])
	}
	return b.String()
}

func table(frame int) bloom.Child {
	// The rows are colored with escape codes rather than styles, as there are
	// many of them, redrawn every frame.
	const dim, green, reset = "\x1b[38;2;86;95;137m", "\x1b[38;2;158;206;106m", "\x1b[m"

	var b strings.Builder
	for i := range 120 {
		svc := services[i%len(services)]
		rps := 900 + (i*37+frame)%400
		latency := 5 + (i*13+frame)%90
		load := strings.Repeat("▪", 1+(i+frame/8)%12)

		fmt.Fprintf(&b, "%s%04d%s  %-8s  ", dim, i, reset, svc)
		fmt.Fprintf(&b, "%s%5d rps%s  %3dms  %s\n", green, rps, reset, latency, load)
	}
	return bloom.Text(strings.TrimSuffix(b.String(), "\n"))
}
