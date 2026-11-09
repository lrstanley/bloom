// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command fig-mission is a dashboard under a FIGlet header that steps down to
// smaller fonts as the window narrows, and to plain text when none fits,
// with a shine sweeping across its strokes now and then: a [fig.PaintFunc]
// over a gradient. Deploying shows a FIGlet
// toast over the panels, sliding in from the top: a transparent layer, so the
// panels show through between its strokes, or an opaque one, to compare.
//
// Keys: d deploys, t toggles the toast's transparency, q quits.
package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fig"
	"github.com/lrstanley/bloom/fig/fonts"
	"github.com/lrstanley/bloom/fx"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintCatppuccinMocha

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	good   = theme.Green
	bad    = theme.Red
	warn   = theme.Yellow
	bg     = theme.Bg

	onBg   = lipgloss.NewStyle().Background(bg)
	panel  = onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.45)).Padding(0, 1)
	title  = onBg.Foreground(accent).Bold(true)
	text   = onBg.Foreground(fg)
	dim    = onBg.Foreground(muted)
	keycap = onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1)
	plain  = onBg.Foreground(accent).Bold(true).Padding(0, 1)
)

const heading = "Mission Control"

// headerFonts are the fonts the header steps down through.
var headerFonts = []*fig.Font{fonts.ANSICompact(), fonts.Doom(), fonts.Small(), fonts.Mini()}

var (
	headerPaint = fig.Gradient(theme.Purple, theme.Blue, theme.Cyan).By(fig.ByDiagonal)
	toastPaint  = fig.Gradient(theme.Green, theme.Yellow).By(fig.ByColumn)
)

type service struct {
	name    string
	up      bool
	latency int // ms
}

var services = []service{
	{"api-gateway", true, 42},
	{"auth", true, 18},
	{"billing", false, 0},
	{"search", true, 97},
	{"notifications", true, 31},
	{"media-transcode", true, 260},
	{"queue", true, 9},
	{"cdn-edge", true, 64},
	{"recommendations", true, 143},
}

// load is requests per second over the last minutes, oldest first.
var load = []int{12, 18, 25, 22, 30, 41, 38, 52, 47, 60, 58, 71, 66, 80, 74, 69, 85, 92, 88, 95}

type model struct {
	ui      *bloom.Root
	w, h    int
	version int
	deploys []string
	opaque  bool

	shimmer anim.Sequence // 0 to 1 across the header, then a pause, over and over
	toast   anim.Presence
}

func newModel() *model {
	return &model{
		ui:      bloom.New(bloom.WithColors(fg, bg)),
		version: 41,
		deploys: []string{
			"v1.4.35 rolled out", "v1.4.36 rolled out", "v1.4.37 rolled out", "v1.4.38 rolled out",
			"v1.4.39 rolled back", "v1.4.40 rolled out", "v1.4.41 rolled out",
		},
		shimmer: anim.NewSequence(0,
			anim.Jump(0),
			anim.To(1, anim.Duration(1400*time.Millisecond)),
			anim.Hold(2*time.Second),
		).With(anim.Ease(anim.Linear), anim.FPS(60)),
		toast: anim.NewPresence(anim.Duration(450*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60)),
	}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd { return m.shimmer.Loop() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "d":
			m.version++
			m.deploys = append(m.deploys, fmt.Sprintf("v1.4.%d rolled out", m.version))
			cmds = append(cmds, m.toast.ShowFor(2200*time.Millisecond))
		case "t":
			m.opaque = !m.opaque
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.shimmer, &m.toast))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.BackgroundColor = bg
	return v
}

func (m *model) layout() bloom.Child {
	header := fig.Text(heading).
		Font(headerFonts...).
		Paint(shine(m.shimmer.Float())).
		Fallback(bloom.Text(strings.ToUpper(heading)).Style(plain).Truncate())

	up := 0
	for _, s := range services {
		if s.up {
			up++
		}
	}
	summary := bloom.Text(fmt.Sprintf("%s %d of %d services up  ·  us-east-1  ·  v1.4.%d live",
		onBg.Foreground(warn).Render("●"), up, len(services), m.version)).Style(dim)

	body := bloom.Row(
		bloom.Fill(card("services", fmt.Sprintf("%d/%d up", up, len(services)), m.services())).Min(30),
		bloom.Fill(card("requests/s", fmt.Sprintf("%d now", load[len(load)-1]), chart(load))).Min(26).Collapsible(2),
		bloom.Fill(card("deploys", fmt.Sprintf("%d today", len(m.deploys)), m.log())).Min(28).Collapsible(1),
	)

	footer := bloom.Row(
		bloom.Fill(keys("d", "deploy", "t", "transparency", "q", "quit")),
		bloom.Text(fmt.Sprintf("header: %s · toast: %s", m.headerFont(), m.toastKind())).Style(dim),
	)

	base := bloom.Column(
		bloom.Centered(header),
		bloom.Centered(summary),
		bloom.Fill(body),
		footer,
	)

	// The toast dims the screen beneath it, which shows through the blanks
	// between its strokes unless it is opaque.
	toast := fig.Text("Deployed!").Font(fonts.Doom(), fonts.Small(), fonts.Mini()).Paint(toastPaint)
	layer := bloom.Layer(toast).
		Anchor(bloom.Center, bloom.Center).
		Backdrop(fx.Dim{Amount: 0.7}).
		Presence(m.toast, fx.Slide{From: fx.Top})
	if !m.opaque {
		layer = layer.Transparent()
	}
	return bloom.Overlay(base, layer)
}

// card is a panel with a title, and a note on the right, on its border.
func card(name, note string, child bloom.Child) bloom.Child {
	return bloom.Box(panel, child).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+name+" ").Style(title)).
		Edge(bloom.Top, bloom.End, bloom.Text(" "+note+" ").Style(dim))
}

// keys is a footer of keys and what they do, as pairs.
func keys(pairs ...string) bloom.Child {
	parts := make([]bloom.Child, 0, len(pairs))
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, bloom.Row(
			bloom.Text(pairs[i]).Style(keycap),
			bloom.Text(" "+pairs[i+1]).Style(dim),
		))
	}
	return bloom.Wrap(parts...).Gap(2, 0)
}

// shine returns the header's paint with a band of light t of the way across
// it, diagonally, brightening the strokes it passes over.
func shine(t float64) fig.Paint {
	if t <= 0 || t >= 1 {
		return headerPaint
	}
	return fig.PaintFunc(func(f *fig.Figure, c fig.Cell) uv.Style {
		st := headerPaint.Shade(f, c)
		w, h := f.Size()
		const band = 6.0
		at := -band + t*(float64(w+2*h)+2*band)
		if d := math.Abs(float64(c.X+2*(h-1-c.Y)) - at); d < band && st.Fg != nil {
			st.Fg = blend(st.Fg, theme.BrightWhite, 0.85*(1-d/band))
		}
		return st
	})
}

// blend returns the color t of the way from a to b.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x) + (float64(y)-float64(x))*t) / 257) }
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 255}
}

// headerFont returns the name of the font the header is shown in at the
// window's width.
func (m *model) headerFont() string {
	for _, f := range headerFonts {
		if w, _ := f.Figure(heading, fig.Options{}).Size(); w <= m.w {
			return f.Name()
		}
	}
	return "plain text"
}

func (m *model) toastKind() string {
	if m.opaque {
		return "opaque"
	}
	return "transparent"
}

// services is a row for each service: its status, name, and latency, with
// a bar for the latency.
func (m *model) services() bloom.Child {
	rows := make([]bloom.Child, 0, len(services)+1)
	for _, s := range services {
		hue, lat := good, fmt.Sprintf("%4dms", s.latency)
		switch {
		case !s.up:
			hue, lat = bad, "  down"
		case s.latency > 200:
			hue = warn
		}
		rows = append(rows, bloom.Row(
			bloom.Text("● ").Style(onBg.Foreground(hue)),
			bloom.Fill(bloom.Text(s.name).Style(text).Truncate()),
			bloom.Text(" "+meter(s.latency, 300, 6)).Style(onBg.Foreground(hue)),
			bloom.Text(lat).Style(dim),
		))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}

// meter is a bar w cells wide, filled v of the way to most, in eighths.
func meter(v, most, w int) string {
	const eighths = " ▏▎▍▌▋▊▉█"
	levels := []rune(eighths)
	fill := min(v, most) * w * 8 / most
	var b strings.Builder
	for range w {
		b.WriteRune(levels[min(max(fill, 0), 8)])
		fill -= 8
	}
	return b.String()
}

// log is the deploys, the latest first, each marked by how it went.
func (m *model) log() bloom.Child {
	rows := make([]bloom.Child, 0, len(m.deploys)+1)
	for i, d := range slices.Backward(m.deploys) {
		mark, st := "✓", onBg.Foreground(good)
		if strings.HasSuffix(d, "back") {
			mark, st = "↺", onBg.Foreground(bad)
		}
		at := fmt.Sprintf("%02d:%02d ", 9+i/2, (i*17)%60)
		rows = append(rows, bloom.Row(
			bloom.Text(at).Style(dim),
			bloom.Text(mark+" ").Style(st),
			bloom.Fill(bloom.Text(d).Style(text).Truncate()),
		))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}

// chart is a bar chart of its values, as wide and tall as it is given, the
// latest values at the right.
type chart []int

func (c chart) View() string { return bloom.Render(len(c), 6, c) }

func (c chart) Layout(ctx bloom.Context) bloom.Child {
	// A bar every other column, as many of the latest values as fit.
	vals := c[max(len(c)-(ctx.Width+1)/2, 0):]
	peak := 1
	for _, v := range vals {
		peak = max(peak, v)
	}
	const eighths = " ▁▂▃▄▅▆▇█"
	levels := []rune(eighths)
	rows := make([]string, ctx.Height)
	for y := range rows {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", max(ctx.Width-2*len(vals)+1, 0)))
		for i, v := range vals {
			// How many eighths of this row the bar fills, from the bottom.
			fill := v*ctx.Height*8/peak - (ctx.Height-1-y)*8
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(levels[min(max(fill, 0), 8)])
		}
		// Rows toward the top, reached only under heavy load, are warmer.
		t := 1 - float64(y)/float64(max(ctx.Height-1, 1))
		rows[y] = onBg.Foreground(blend(theme.Green, theme.Red, t*t)).Render(b.String())
	}
	return bloom.Text(strings.Join(rows, "\n"))
}
