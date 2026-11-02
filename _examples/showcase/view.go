// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

const hints = "drag a title to move a pane · double click it to zoom · t tint · ? tour · q quit"

// styles are made from a tint. Only the screen, and what is raised above
// it, set a background; the rest are drawn over the screen's.
type styles struct {
	t                                  *tint.Tint
	base, screen, text, muted, logo    lipgloss.Style
	badge, raised                      lipgloss.Style
	pane, lifted, title, grabbed       lipgloss.Style
	item, hovered, selected, highlight lipgloss.Style
	toast, tour                        lipgloss.Style
	status                             [3]lipgloss.Style // by a service's status.
	shadow                             color.Color
}

func newStyles(t *tint.Tint) styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	raised := lipgloss.Lighten(t.Bg, 0.06)
	pane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack).Padding(0, 1)
	title := fg(t.Purple).Bold(true).Padding(0, 1)
	item := lipgloss.NewStyle().Padding(0, 1)
	floating := lipgloss.NewStyle().Background(raised).BorderBackground(t.Bg).Padding(0, 2)
	return styles{
		t:         t,
		base:      lipgloss.NewStyle().Background(t.Bg),
		screen:    fg(t.Fg).Background(t.Bg).Padding(0, 1),
		text:      fg(t.Fg),
		muted:     fg(t.BrightBlack),
		logo:      fg(t.Purple).Bold(true),
		badge:     fg(t.Bg).Background(t.Purple).Bold(true).Padding(0, 1),
		raised:    lipgloss.NewStyle().Background(raised),
		pane:      pane,
		lifted:    pane.BorderForeground(t.Purple).Background(t.Bg).BorderBackground(t.Bg),
		title:     title,
		grabbed:   title.Foreground(t.Bg).Background(t.Purple),
		item:      item,
		hovered:   item.Background(raised),
		selected:  item.Background(lipgloss.Lighten(t.Bg, 0.12)),
		highlight: fg(t.Purple).Bold(true),
		toast:     floating.Border(lipgloss.RoundedBorder()).BorderForeground(t.Purple),
		tour:      floating.Border(lipgloss.RoundedBorder()).BorderForeground(t.Blue),
		status:    [3]lipgloss.Style{fg(t.Green), fg(t.Yellow), fg(t.Red)},
		shadow:    lipgloss.Darken(t.Bg, 0.5),
	}
}

// paneSizes are how wide the panes are: fixed, or sharing the width by
// weight down to a minimum. Those with a priority collapse as the terminal
// narrows, the lowest first.
var paneSizes = map[paneID]struct{ fixed, grow, min, collapse int }{
	paneTints:    {fixed: 30, collapse: 2},
	paneActivity: {grow: 4, min: 30},
	panePalette:  {grow: 3, min: 28, collapse: 1},
	paneServices: {grow: 3, min: 26},
}

// Panes glide to their new places and sizes as they are moved and zoomed,
// and a dragged pane trails the pointer a little.
var glide = anim.Spring(14, 0.85)

// titleID is the ID of a pane's title, by which bloom follows the drag of
// it as the pane moves from the row to its own layer.
type titleID paneID

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the tint's background too, around and beneath the
	// layout, where terminals let programs set it.
	v.BackgroundColor = m.st.t.Bg
	// The bars slow down while the terminal is not focused.
	v.ReportFocus = true
	// All motion, not only while a button is held, for the hover styles.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

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
		bloom.Text(m.st.t.DisplayName).Style(m.st.badge),
	).Gap(1)

	screen := bloom.Box(m.st.screen, bloom.Column(
		header,
		bloom.Fill(body),
		bloom.Text(hints).Style(m.st.muted).Truncate(),
	))
	layers := []bloom.LayerNode{m.toastLayer(), m.tourLayer()}
	if m.dragging != "" {
		layers = append([]bloom.LayerNode{m.dragLayer()}, layers...)
	}
	return bloom.Overlay(screen, layers...)
}

// dragLayer floats the dragged pane under the pointer, held where it was
// grabbed, over a shadow. It has the pane's ID, so it carries on from where
// the pane was, and the pane carries on from it when dropped.
func (m *model) dragLayer() bloom.LayerNode {
	at := m.at.Sub(m.grab)
	return bloom.Layer(m.pane(m.dragging)).
		Anchor(bloom.Start, bloom.Start).
		Offset(at.X, at.Y).
		Size(m.size.X, m.size.Y).
		Shadow(2, 1, fx.Recolor{Fg: m.st.shadow, Bg: m.st.shadow}).
		ID(m.dragging).
		Animate(glide)
}

// slot is pane id's place in the row: sharing the width, all of it while
// zoomed, or none while another pane is. While the pane is dragged, its
// place is left empty, for the other panes to glide over as they move.
func (m *model) slot(id paneID) bloom.SlotNode {
	if id == m.dragging {
		size := paneSizes[id]
		return bloom.Grow(size.grow, nil).Min(size.min)
	}

	pane, size := m.pane(id), paneSizes[id]
	s := bloom.Fill(pane)
	switch {
	case m.zoomed == "" && size.fixed > 0:
		s = bloom.Fixed(size.fixed, pane)
	case m.zoomed == "":
		s = bloom.Grow(size.grow, pane).Min(size.min)
	case m.zoomed != id:
		s = bloom.Fixed(0, pane)
	}
	if m.zoomed == "" && size.collapse > 0 {
		s = s.Collapsible(size.collapse)
	}
	return s.ID(id).Animate(glide)
}

// pane boxes the pane's content under its title, which, except for the
// sidebar's, can be dragged to move the pane, and double clicked to zoom.
func (m *model) pane(id paneID) bloom.Child {
	var body bloom.Child
	switch id {
	case paneTints:
		body = m.tintList()
	case paneActivity:
		body = m.activity()
	case panePalette:
		body = m.palette()
	case paneServices:
		body = m.services()
	}

	box, title := m.st.pane, m.st.title
	if id == m.dragging {
		// Lifted, it brings its own background over the panes beneath.
		box, title = m.st.lifted, m.st.grabbed
	}

	head := bloom.Text(string(id)).Style(title)
	if id != paneTints {
		head = bloom.Text("⠿ "+string(id)).Style(title).HoverStyle(m.st.grabbed).ID(titleID(id)).On(
			on.Drag(func(e on.Event) tea.Cmd { return m.drag(id, e) }),
			on.DoubleClick(func() tea.Cmd { return m.zoom(id) }),
		)
	}
	return bloom.Box(box, bloom.Column(head, bloom.Fixed(1, nil), bloom.Fill(body)))
}

// tintList is a row for each tint, with a few of its colors, which switches
// to it when clicked.
func (m *model) tintList() bloom.Child {
	rows := make([]bloom.Child, 0, len(tints)+1)
	for i, t := range tints {
		st, hover, name, mark := m.st.item, m.st.hovered, m.st.text, "  "
		if i == m.tint {
			st, hover, name, mark = m.st.selected, m.st.selected, m.st.highlight, "▸ "
		}
		row := []bloom.Child{bloom.Text(mark + t.DisplayName).Style(name), bloom.Space()}
		for _, c := range []color.Color{t.Red, t.Yellow, t.Green, t.Blue, t.Purple} {
			row = append(row, bloom.Text("●").Style(lipgloss.NewStyle().Foreground(c)))
		}
		rows = append(rows, bloom.Fixed(1, bloom.Box(st, bloom.Row(row...)).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			if i == m.tint {
				return nil
			}
			return m.switchTint(i)
		}))))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}

// activity is a bar chart, its bars rising and falling as the loop runs, in
// a gradient through the tint's colors.
func (m *model) activity() bloom.Child {
	t := m.st.t
	return bloom.Column(
		bloom.Text("requests per second").Style(m.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Fill(bars{phase: m.bars.Float(), stops: []color.Color{t.Blue, t.Purple, t.Red}}),
	)
}

// bars is a [bloom.Layouter], so it draws as many bars as fit the space it
// is given.
type bars struct {
	phase float64
	stops []color.Color
}

func (b bars) View() string { return "" }

func (b bars) Layout(ctx bloom.Context) bloom.Child {
	// Each bar is two cells wide, with a cell between them.
	n := max((ctx.Width+1)/3, 1)
	colors := lipgloss.Blend1D(n, b.stops...)
	cols := make([]bloom.Child, n)
	for i := range n {
		x := float64(i) / float64(n)
		level := 0.55 + 0.3*math.Sin(2*math.Pi*(b.phase+x)) + 0.15*math.Sin(2*math.Pi*(2*b.phase-1.7*x))
		style := lipgloss.NewStyle().Foreground(colors[i])
		cols[i] = bloom.Fixed(2, bloom.Text(bar(level, ctx.Height)).Style(style))
	}
	return bloom.Row(cols...).Gap(1)
}

// eighths are the blocks that fill a cell from the bottom, in eighths.
var eighths = []rune(" ▁▂▃▄▅▆▇█")

// bar is a column h cells tall, filled from the bottom to level, from 0 to
// 1, in eighths of a cell.
func bar(level float64, h int) string {
	filled := int(math.Round(min(max(level, 0), 1) * float64(h*8)))
	lines := make([]string, h)
	for y := range h {
		// How much of this line is filled, counting lines from the bottom.
		n := min(max(filled-(h-1-y)*8, 0), 8)
		lines[y] = strings.Repeat(string(eighths[n]), 2)
	}
	return strings.Join(lines, "\n")
}

// palette shows the tint's sixteen colors, as swatches that share the width.
func (m *model) palette() bloom.Child {
	t := m.st.t
	swatches := func(cs ...color.Color) bloom.Child {
		row := make([]bloom.Child, len(cs))
		for i, c := range cs {
			row[i] = bloom.Fill(bloom.Box(lipgloss.NewStyle().Background(c), bloom.Text("")))
		}
		return bloom.Row(row...).Gap(1)
	}
	about := []string{string(t.Hue)}
	if t.Tone != "" {
		about = append(about, string(t.Tone))
	}
	return bloom.Column(
		bloom.Text(t.DisplayName).Style(m.st.highlight).Truncate(),
		bloom.Text(strings.Join(about, " · ")).Style(m.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Fixed(3, swatches(t.Black, t.Red, t.Green, t.Yellow, t.Blue, t.Purple, t.Cyan, t.White)),
		bloom.Fixed(1, nil),
		bloom.Fixed(3, swatches(
			t.BrightBlack, t.BrightRed, t.BrightGreen, t.BrightYellow,
			t.BrightBlue, t.BrightPurple, t.BrightCyan, t.BrightWhite,
		)),
	)
}

// services are the services listed, with their status: 0 is healthy, 1
// slow, and 2 down.
var services = []struct {
	name       string
	status, ms int
}{
	{"api", 0, 24},
	{"auth", 0, 18},
	{"billing", 1, 212},
	{"gateway", 0, 31},
	{"search", 2, 0},
	{"storage", 0, 47},
	{"worker", 1, 164},
}

var statusNames = [...]string{"healthy", "slow", "down"}

// services is a row for each service, which is selected when clicked.
func (m *model) services() bloom.Child {
	rows := make([]bloom.Child, 0, len(services)+2)
	for i, s := range services {
		st, hover := m.st.item, m.st.hovered
		if i == m.selected {
			st, hover = m.st.selected, m.st.selected
		}
		latency := fmt.Sprintf("%dms", s.ms)
		if s.status == 2 {
			latency = "—"
		}
		row := bloom.Row(
			bloom.Text("●").Style(m.st.status[s.status]),
			bloom.Text(s.name).Style(m.st.text),
			bloom.Space(),
			bloom.Text(latency).Style(m.st.muted),
		).Gap(1)
		rows = append(rows, bloom.Fixed(1, bloom.Box(st, row).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			m.selected = i
			return nil
		}))))
	}
	s := services[m.selected]
	rows = append(
		rows,
		bloom.Space(),
		bloom.Text(s.name+" is "+statusNames[s.status]).Style(m.st.status[s.status]).Truncate(),
	)
	return bloom.Column(rows...)
}

// toastLayer slides the toast in from the right, decoding its text.
func (m *model) toastLayer() bloom.LayerNode {
	text := bloom.Transition(
		m.toastText.Float(),
		bloom.Box(m.st.raised, bloom.Text(m.note).Style(m.st.text)),
		fx.Decode{Seed: m.notes},
	)
	return bloom.Layer(bloom.Box(m.st.toast, text)).
		Anchor(bloom.End, bloom.End).
		Margin(2).
		Presence(m.toast, fx.Slide{From: fx.Right})
}

// tourLayer is the tour's: a note at the bottom, over a backdrop that
// leaves only the pane it explains lit.
func (m *model) tourLayer() bloom.LayerNode {
	// Past the last step, the last note stays while it fades out.
	step := tourSteps[min(max(m.step, 1), len(tourSteps))-1]
	note := bloom.Column(
		bloom.Text(step.title).Style(m.st.highlight),
		bloom.Text(step.text).Style(m.st.text),
	)
	decode := fx.Decode{Seed: uint32(m.step)} //nolint:gosec // the step is small.

	l := bloom.Layer(bloom.Box(m.st.tour, bloom.Transition(m.tourText.Float(), bloom.Box(m.st.raised, note), decode))).
		Anchor(bloom.Center, bloom.End).
		Margin(2).
		Presence(m.tourOn, fx.Fade{})
	if r, ok := m.ui.Frame().Rect(step.pane); ok {
		l = l.Backdrop(fx.Spotlight{Rect: r, Soft: 4})
	}
	return l
}
