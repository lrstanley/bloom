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
// The scrollbars can be dragged, or clicked to jump.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNightStorm

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	pink   = theme.Purple
	warn   = theme.Yellow
	good   = theme.Green
	bad    = theme.Red

	panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	item  = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	title = lipgloss.NewStyle().Foreground(fg).Bold(true).Padding(0, 1)
	label = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	tab   = lipgloss.NewStyle().Foreground(muted)
	onTab = tab.Foreground(accent).Bold(true)
	thumb = lipgloss.NewStyle().Foreground(accent)
	icon  = lipgloss.NewStyle().Foreground(fg)
	onTip = icon.Foreground(warn).Bold(true)
	side  = lipgloss.NewStyle().Foreground(pink).Bold(true)
)

const (
	paneList = "list" // the content of the scrolled list, which its scrollbar follows
	paneBox  = "box"  // the list's box, for the size of a page
	paneLog  = "log"  // the content of the log, scrolled sideways
	paneLogs = "logs" // the log's box, for the width in view
)

// filters are the tabs over the list, each a prefix of the paths it shows.
var filters = []string{"all", "kv", "pki", "db"}

type model struct {
	ui     *bloom.Root
	w, h   int
	phase  anim.Sequence
	paused bool
	filter int
	offset int // rows the list is scrolled
	logX   int // columns the log is scrolled
	status string
	paths  []string
	log    string
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
	return &model{
		ui:     bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		phase:  anim.NewLoop(4 * time.Second),
		paths:  paths,
		log:    strings.Join(log, "\n"),
		status: "drag a scrollbar, or click an icon on the list's left side",
	}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
		case "up":
			m.scrollTo(m.offset - 1)
		case "down":
			m.scrollTo(m.offset + 1)
		case "left":
			m.scrollLogTo(m.logX - 4)
		case "right":
			m.scrollLogTo(m.logX + 4)
		case "tab":
			m.setFilter((m.filter + 1) % len(filters))
		case "space", " ":
			m.paused = !m.paused
			if m.paused {
				m.phase.Stop()
			} else {
				cmds = append(cmds, m.phase.Loop())
			}
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.phase))
	return m, tea.Batch(cmds...)
}

// shown returns the paths the filter shows.
func (m *model) shown() []string {
	if m.filter == 0 {
		return m.paths
	}
	var out []string
	for _, p := range m.paths {
		if strings.HasPrefix(p, filters[m.filter]+"/") {
			out = append(out, p)
		}
	}
	return out
}

// page returns how many rows of the list are in view, from the last frame.
func (m *model) page() int {
	r, _ := m.ui.Frame().Rect(paneBox)
	return max(r.Dy()-2, 1)
}

// scrollTo scrolls the list to row offset, keeping its last page in view.
func (m *model) scrollTo(offset int) {
	m.offset = min(max(offset, 0), max(len(m.shown())-m.page(), 0))
}

// scrollLogTo scrolls the log to column x, keeping the ends of its longest
// line in view, from the last frame.
func (m *model) scrollLogTo(x int) {
	content, _ := m.ui.Frame().Rect(paneLog)
	view, _ := m.ui.Frame().Rect(paneLogs)
	m.logX = min(max(x, 0), max(content.Dx()-(view.Dx()-2), 0))
}

func (m *model) setFilter(i int) {
	m.filter, m.offset = i, 0
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.BackgroundColor = theme.Bg
	// The loop slows down while the terminal is not focused.
	v.ReportFocus = true
	return v
}

func (m *model) layout() bloom.Child {
	t := m.phase.Float()
	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(bloom.Column(bloom.Fill(m.list(t)), bloom.Fixed(7, m.logBox()))),
			bloom.Fixed(34, bloom.Column(
				bloom.Fill(effectBox("alert", "fx.Pulse", "draws attention to itself", t, fx.Pulse{Color: bad})),
				bloom.Fill(effectBox("syncing", "fx.Comet", "busy, once around per loop", t, fx.Comet{Color: warn, Length: 12})),
				// Marching ants move one run per loop, so they are sped up.
				bloom.Fill(effectBox("selected", "fx.March", "marching ants", 8*t, fx.March{})),
				bloom.Fill(effectBox("ripple", "fx.Chase", "three cycles of a gradient", t, fx.Chase{To: good, Cycles: 3})),
			)),
		).Gap(1)),
		bloom.Text(m.status).Style(label).Truncate(),
	)
}

// list is the scrolled list of paths, with its title, filter tabs, and
// position on its border, icons down its left side, and a scrollbar on its
// right side that can be dragged.
func (m *model) list(t float64) bloom.Child {
	paths := m.shown()
	box := bloom.Box(panel, bloom.Scroll(
		bloom.Text(strings.Join(paths, "\n")).Style(item).ID(paneList),
		m.offset,
	)).ID(paneBox).
		On(on.Scroll(func(_, dy int) tea.Cmd { m.scrollTo(m.offset + dy); return nil })).
		Edge(bloom.Top, bloom.Start, bloom.Text("◆ secrets").Style(title)).
		Edge(bloom.Top, bloom.End, m.tabs()).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf("%d-%d of %d",
			min(m.offset+1, len(paths)), min(m.offset+m.page(), len(paths)), len(paths))).Style(label)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneList).Thumb("┃", thumb).
			OnScroll(func(offset int) tea.Cmd { m.scrollTo(offset); return nil }))
	// Single glyphs down the side, each one a button.
	for _, a := range []struct{ glyph, does string }{{"+", "new secret"}, {"✎", "edit"}, {"⧉", "copy path"}, {"✕", "delete"}} {
		box = box.Edge(bloom.Left, bloom.Start, bloom.Text(a.glyph).Style(icon).HoverStyle(onTip).
			On(on.Click(func() tea.Cmd { m.status = "clicked " + a.glyph + ": " + a.does; return nil })))
	}
	return box.BorderFx(t, fx.Chase{From: accent, To: pink})
}

// tabs are the filters, which can be clicked.
func (m *model) tabs() bloom.Child {
	kids := make([]bloom.Child, 0, len(filters))
	for i, f := range filters {
		st := tab
		if i == m.filter {
			st = onTab
		}
		kids = append(kids, bloom.Text(f).Style(st).HoverStyle(onTab).
			On(on.Click(func() tea.Cmd { m.setFilter(i); return nil })))
	}
	return bloom.Row(kids...).Gap(1)
}

// logBox is a log of long lines, which scroll sideways rather than wrap,
// with a horizontal scrollbar on its bottom edge and its name down its side.
func (m *model) logBox() bloom.Child {
	return bloom.Box(panel, bloom.Scroll(bloom.Text(m.log).Style(item).ID(paneLog), 0).X(m.logX)).ID(paneLogs).
		Edge(bloom.Left, bloom.Center, bloom.Text("LOG").Style(side)).
		Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneLog).Horizontal().Thumb("━", thumb).
			OnScroll(func(x int) tea.Cmd { m.scrollLogTo(x); return nil }))
}

// effectBox is a box with one looping border effect, at phase t, described
// inside it and named on its border.
func effectBox(name, code, about string, t float64, e fx.Effect) bloom.Child {
	return bloom.Box(panel, bloom.Text(about).Style(item)).
		Edge(bloom.Top, bloom.Start, bloom.Text(name).Style(title)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(code).Style(label)).
		BorderFx(t, e)
}
