// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The colors and styles of the desktop, made from the theme by setTheme.
var (
	fg, muted, accent, hot, dark, winBg, raised, barBg color.Color

	paper, frame, frameOn, title, titleOn, grip, gripOn, control, ctlOver, ctlShut, corner,
	body, faint, row, rowHover, rowOn, display, key, keyOp, keyHover, keyEq, keyEqHover,
	outline, icon, iconHover, iconOn, menuItem, banner, bar, start, startOn, task, taskOn, taskMin, taskOver, status, clock lipgloss.Style
)

// setTheme makes the desktop's colors and styles from tint t.
func setTheme(t *tint.Tint) {
	theme = t
	fg, muted, accent, hot, dark = t.Fg, t.BrightBlack, t.Purple, t.Red, t.Bg
	winBg, raised, barBg = lipgloss.Lighten(t.Bg, 0.06), lipgloss.Lighten(t.Bg, 0.12), lipgloss.Darken(t.Bg, 0.25)

	paper = lipgloss.NewStyle().Foreground(lipgloss.Lighten(t.Bg, 0.14))

	frame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).BorderBackground(winBg).Background(winBg).Padding(0, 1)
	frameOn = frame.BorderForeground(accent)
	title = lipgloss.NewStyle().Foreground(muted).Background(winBg)
	titleOn = title.Foreground(dark).Background(accent).Bold(true)
	grip = lipgloss.NewStyle().Foreground(muted).Background(winBg)
	gripOn = grip.Foreground(accent)
	control = lipgloss.NewStyle().Foreground(fg).Background(winBg)
	ctlOver = control.Foreground(dark).Background(accent)
	ctlShut = control.Foreground(dark).Background(hot)
	corner = grip.Foreground(accent)

	body = lipgloss.NewStyle().Foreground(fg).Background(winBg)
	faint = body.Foreground(muted)
	row = body
	rowHover = row.Background(raised)
	rowOn = row.Foreground(dark).Background(t.Blue).Bold(true)

	display = lipgloss.NewStyle().Foreground(fg).Background(barBg).Bold(true).Padding(0, 1)
	key = lipgloss.NewStyle().Foreground(fg).Background(raised)
	keyOp = key.Foreground(t.Cyan)
	keyHover = key.Foreground(dark).Background(t.Cyan)
	keyEq = key.Foreground(dark).Background(accent).Bold(true)
	keyEqHover = keyEq.Background(t.BrightPurple)

	outline = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accent)

	icon = lipgloss.NewStyle().Foreground(fg).Width(12).Align(lipgloss.Center).Padding(0, 1)
	iconHover = icon.Background(raised)
	iconOn = icon.Foreground(dark).Background(accent)

	menuItem = lipgloss.NewStyle().Foreground(fg).Background(raised)
	banner = lipgloss.NewStyle().Foreground(dark).Background(accent).Bold(true)

	bar = lipgloss.NewStyle().Background(barBg)
	start = lipgloss.NewStyle().Foreground(dark).Background(accent).Bold(true)
	startOn = start.Background(t.BrightPurple)
	task = lipgloss.NewStyle().Foreground(fg).Background(raised)
	taskOn = task.Foreground(dark).Background(t.Blue).Bold(true)
	taskMin = task.Foreground(muted).Background(barBg)
	taskOver = task.Background(lipgloss.Lighten(t.Bg, 0.2))
	status = lipgloss.NewStyle().Foreground(muted).Background(barBg)
	clock = lipgloss.NewStyle().Foreground(fg).Background(raised)
}

// winID is the ID of a window's layer, which it keeps as it moves up and
// down the stack, for Animate to follow it. The nodes in the window are
// known by where they are in it, so a press on a button of a window behind
// the others, which brings the window to the front, still clicks it.
type winID int

// textID is the ID of the text of a notes window, for its scrollbar.
type textID int

// previewID is the ID of the outline showing where a window would snap.
type previewID struct{}

func (m *model) layout() bloom.Child {
	layers := make([]bloom.LayerNode, 0, len(m.windows)+2)
	layers = append(layers, m.icons())
	for i, w := range m.windows {
		if i == len(m.windows)-1 {
			// The outline of where a window would snap goes beneath the window
			// in front, which is the one being dragged.
			layers = append(layers, bloom.Layer(bloom.Box(outline, bloom.Space())).
				ID(previewID{}).Transparent().Place(m.target).
				Presence(m.preview, fx.Fade{}))
		}
		layers = append(layers, m.window(w, w == m.top()))
	}
	return bloom.Column(
		bloom.Fill(bloom.Overlay(bloom.Text(m.paper).Style(paper), layers...)),
		bloom.Fixed(1, m.taskbar()),
	)
}

// window returns the layer of window w, placed where the model says it is.
func (m *model) window(w *window, focused bool) bloom.LayerNode {
	r := m.place(w)
	st, ti, gr := frame, title, grip
	if focused {
		st, ti, gr = frameOn, titleOn, gripOn
	}
	glyph := "□"
	if w.snap != snapNone {
		glyph = "❐"
	}
	// The title bar, the title and the border beside it, drags the window,
	// and a double click on it maximizes the window. It is one edge child, as
	// the cell between two of them belongs to the box.
	bar := bloom.Row(
		bloom.Auto(bloom.Text(" "+w.app.glyph()+" "+w.title+" ").Style(ti).Truncate()),
		bloom.Fill(bloom.Text(strings.Repeat("─", max(r.Dx(), 0))).Style(gr)),
	).On(m.mover(w), on.DoubleClick(func() tea.Cmd { m.toggleMax(w); return nil }))
	box := bloom.Box(st, m.content(w)).
		Edge(bloom.Top, bloom.End, button("×", ctlShut, func() tea.Cmd { return m.close(w) })).
		Edge(bloom.Top, bloom.End, button(glyph, ctlOver, func() tea.Cmd { m.toggleMax(w); return nil })).
		Edge(bloom.Top, bloom.End, button("−", ctlOver, func() tea.Cmd { return m.minimize(w) })).
		Edge(bloom.Top, bloom.Stretch, bar).
		EdgeAt(bloom.Bottom, -1, bloom.Text("◢").Style(gr).HoverStyle(corner).On(m.resizer(w)))
	if w.app == appNotes {
		box = box.Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(textID(w.id)).
			Style(bloom.ScrollbarStyle{ThumbStyle: gr, TrackStyle: gr}))
	}

	layer := bloom.Layer(box).ID(winID(w.id)).Place(r).
		Shadow(2, 1, fx.Dim{Amount: 0.6}).
		// Any press on a window brings it to the front, including one on a
		// button or on its title bar as a drag starts.
		On(on.Down(func() tea.Cmd { m.raise(w); return nil }))
	// A window follows the pointer cell for cell while it is dragged, and
	// glides otherwise: to where it is maximized, snapped, or moved with the
	// keyboard, and from where it was dropped, should that be against an
	// edge, or off the screen.
	if g := m.grab; g == nil || g.win != w || !g.moved {
		layer = layer.Animate(anim.Spring(22, 1))
	}
	if w.slide {
		return layer.Presence(w.pres, fx.Slide{From: bloom.Bottom}, fx.Fade{})
	}
	return layer.Presence(w.pres, fx.Fade{})
}

// button is a control on a title bar.
func button(glyph string, hover lipgloss.Style, fn func() tea.Cmd) bloom.Child {
	return bloom.Text(" " + glyph + " ").Style(control).HoverStyle(hover).On(on.Click(fn))
}

// icons are the desktop's icons, one for each app: a click selects one, and a
// double click opens its app.
func (m *model) icons() bloom.LayerNode {
	items := make([]bloom.Child, len(apps))
	for i, a := range apps {
		st := icon
		if a == m.icon {
			st = iconOn
		}
		items[i] = bloom.Text(a.glyph()+"\n"+a.name()).Style(st).HoverStyle(iconHover).On(
			on.Click(func() tea.Cmd { m.icon = a; return nil }),
			on.DoubleClick(func() tea.Cmd { return m.launch(a) }),
		)
	}
	return bloom.Layer(bloom.Column(items...).Gap(1)).Anchor(bloom.Start, bloom.Start).Offset(1, 1).Transparent()
}

// taskbar is the bar along the bottom: a button for each window, and the
// status.
func (m *model) taskbar() bloom.Child {
	items := []bloom.Child{m.startMenu()}
	top := m.top()
	// The buttons stay in the order the windows opened in, rather than the
	// order they stack in, so that they don't move under the pointer.
	byID := slices.SortedFunc(slices.Values(m.windows), func(a, b *window) int { return a.id - b.id })
	for _, w := range byID {
		if w.shut {
			continue
		}
		st := task
		switch {
		case w == top:
			st = taskOn
		case !w.pres.Shown():
			st = taskMin
		}
		items = append(items, bloom.Text(" "+w.app.glyph()+" "+w.title+" ").Style(st).HoverStyle(taskOver).
			On(on.Click(func() tea.Cmd { return m.task(w) })))
	}
	items = append(items,
		bloom.Fill(bloom.Text(m.status).Style(status).Truncate()),
		bloom.Text(" "+m.now().Format(time.Kitchen)+" ").Style(clock),
	)
	return bloom.Box(bar, bloom.Row(items...).Gap(1))
}

// wallpaper is a w×h field of dots, for the desktop's background.
func wallpaper(w, h int) string {
	var b strings.Builder
	for y := range h {
		for x := range w {
			if (x+y*3)%8 == 0 && y%2 == 0 {
				b.WriteString("·")
			} else {
				b.WriteByte(' ')
			}
		}
		if y < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
