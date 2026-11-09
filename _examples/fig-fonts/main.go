// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command fig-fonts is a gallery of every FIGlet and TOIlet font bundled with
// package fig, each drawing a line of text with one of fig's paints:
// gradients across columns, rows, the diagonal or glyph by glyph, a palette,
// a rainbow, metal, and a fill for the cells inside letters. A
// [fig.TextNode] is a bloom node like any other, so it is centered on the
// stage, revealed glyph by glyph with [fig.TextNode.Each] when the font
// changes, and replaced by a note when the window is too small for its font.
//
// Keys: up/down change the font, left/right change the paint, q quits. Fonts
// and paints can be clicked, and the wheel scrolls through the fonts.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fig"
	"github.com/lrstanley/bloom/fig/fonts"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNight

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue
	bg     = theme.Bg

	onBg     = lipgloss.NewStyle().Background(bg)
	panel    = onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.45))
	stage    = panel.BorderForeground(accent).Padding(0, 2)
	title    = onBg.Foreground(accent).Bold(true)
	text     = onBg.Foreground(fg)
	dim      = onBg.Foreground(muted)
	item     = text.Padding(0, 1)
	selected = onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1)
	chip     = onBg.Foreground(muted).Padding(0, 1)
	chipOn   = onBg.Foreground(bg).Background(theme.Purple).Bold(true).Padding(0, 1)
	keycap   = onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1)
)

// gallery is every bundled font, by name. The gallery starts on figlet's
// standard font.
var (
	gallery  = fonts.All()
	standard = slices.Index(fonts.Names, "standard")
)

type paint struct {
	name  string
	paint fig.Paint
}

var (
	pink, peach = lipgloss.Color("#ff6f91"), lipgloss.Color("#ffc29b")
	sky, violet = theme.Cyan, theme.Purple
)

// paints are the paints shown, in order.
var paints = []paint{
	{"column", fig.Gradient(pink, peach)},
	{"diagonal", fig.Gradient(sky, violet, pink).By(fig.ByDiagonal)},
	{"row", fig.Gradient(peach, pink, violet).By(fig.ByRow)},
	{"glyph", fig.Gradient(sky, pink).By(fig.ByGlyph)},
	{"palette", fig.Palette(theme.Red, theme.Yellow, theme.Green, theme.Cyan, theme.Blue)},
	{"rainbow", fig.Rainbow()},
	{"metal", fig.Metal()},
	{"filled", fig.Fill(lipgloss.Darken(violet, 0.55), fig.Solid(peach))},
}

type model struct {
	ui    *bloom.Root
	w, h  int
	font  int
	paint int

	reveal anim.Value // 0 to 1, the glyphs of a new font appearing
}

func newModel() *model {
	return &model{
		ui:     bloom.New(bloom.WithColors(fg, bg)),
		font:   standard,
		reveal: anim.NewValue(1, anim.Duration(300*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60)),
	}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd { return m.reveal.Play(0, 1) }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "down", "j":
			cmds = append(cmds, m.pickFont(m.font+1))
		case "up", "k":
			cmds = append(cmds, m.pickFont(m.font-1))
		case "right", "l":
			m.paint = (m.paint + 1) % len(paints)
		case "left", "h":
			m.paint = (m.paint + len(paints) - 1) % len(paints)
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.reveal))
	return m, tea.Batch(cmds...)
}

// pickFont shows font i, wrapping around the gallery, and reveals it.
func (m *model) pickFont(i int) tea.Cmd {
	m.font = (i + len(gallery)) % len(gallery)
	return m.reveal.Play(0, 1)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.BackgroundColor = bg
	return v
}

func (m *model) layout() bloom.Child {
	font, p := gallery[m.font], paints[m.paint]
	info := font.Info()

	sample := fig.Text("Bloom!").
		Font(font).
		Paint(p.paint).
		Each(m.reveal.Float(), fx.Decode{Seed: uint32(m.font)}, fx.Fade{}). //nolint:gosec // a font's index.
		Spread(0.5, fig.Forward).
		Fallback(bloom.Text("too large for the window: make it bigger").Style(dim))

	shown := bloom.Box(stage, bloom.Centered(sample)).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+font.Name()+" ").Style(title)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf(" %d rows · %s · %s paint ", info.Height, layoutName(info.Horizontal), p.name)).Style(dim)).
		Edge(bloom.Bottom, bloom.Center, bloom.Text(" "+credit(info.Comments)+" ").Style(dim).Truncate())

	list := bloom.Box(panel, fontList{m: m}).
		Edge(bloom.Top, bloom.Start, bloom.Text(" fonts ").Style(title)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf(" %d/%d ", m.font+1, len(gallery))).Style(dim)).
		On(on.Scroll(func(_, dy int) tea.Cmd { return m.pickFont(m.font + dy) }))

	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fixed(30, list).Collapsible(1),
			bloom.Fill(bloom.Column(
				bloom.Fill(shown),
				bloom.Row(bloom.Text("paint").Style(title.PaddingRight(1)), bloom.Fill(m.chips())),
			)).Min(40),
		)),
		keys("↑↓", "font", "←→", "paint", "click", "pick", "q", "quit"),
	)
}

// chips are the paints, the one in use highlighted, each picked by a click.
func (m *model) chips() bloom.Child {
	chips := make([]bloom.Child, len(paints))
	for i, p := range paints {
		st := chip
		if i == m.paint {
			st = chipOn
		}
		chips[i] = bloom.Text(p.name).Style(st).On(on.Click(func() tea.Cmd {
			m.paint = i
			return nil
		}))
	}
	return bloom.Wrap(chips...).Gap(1, 0)
}

// fontList is the fonts' names, scrolled to keep the one shown in the
// middle, as tall as it is given.
type fontList struct{ m *model }

func (l fontList) View() string { return bloom.Render(28, 10, l) }

func (l fontList) Layout(ctx bloom.Context) bloom.Child {
	n := min(ctx.Height, len(gallery))
	first := min(max(l.m.font-n/2, 0), len(gallery)-n)
	rows := make([]bloom.Child, 0, n)
	for i := first; i < first+n; i++ {
		st, name := item, fonts.Names[i]
		if i == l.m.font {
			st, name = selected, "› "+name
		}
		rows = append(rows, bloom.Text(name).Style(st).Truncate().On(on.Click(func() tea.Cmd {
			return l.m.pickFont(i)
		})))
	}
	return bloom.Column(rows...)
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

// layoutName describes how a font's glyphs are put side by side.
func layoutName(m fig.Mode) string {
	switch m { //nolint:exhaustive // A font's own layout is never Default.
	case fig.Smushed, fig.Universal:
		return "smushed"
	case fig.Fitted:
		return "fitted"
	}
	return "full width"
}

// credit returns the first line of a font's comments that says who made it.
func credit(comments []string) string {
	for _, c := range comments {
		c = strings.TrimSpace(c)
		if strings.ContainsFunc(c, func(r rune) bool { return r >= 'A' && r <= 'z' }) && !strings.HasPrefix(c, "SPDX") {
			return c
		}
	}
	return "no credit given"
}
