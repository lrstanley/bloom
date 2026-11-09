// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command fig-letters animates FIGlet text glyph by glyph, with package fig:
//
//   - a title revealed a letter at a time, from the middle out, with
//     [fig.TextNode.Each];
//   - a clock whose digits roll over as the seconds change, with a
//     [fig.Morph]: new digits slide in from the top as old ones leave from
//     the bottom, and those that stay put do not move;
//   - an anagram whose letters fly to their places in the next word, with
//     [fig.TextNode.Animate] and [fx.Fly], and light up under the pointer,
//     with [fig.TextNode.OnGlyph] and a [fig.PaintFunc].
//
// Keys: space shows the next anagram, i replays the title, p pauses the
// clock, q quits. Letters of the anagram, and the words under it, can be
// clicked, too.
package main

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fig"
	"github.com/lrstanley/bloom/fig/fonts"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintDracula

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Purple
	bg     = theme.Bg

	onBg   = lipgloss.NewStyle().Background(bg)
	panel  = onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.35)).Padding(0, 2)
	label  = onBg.Foreground(accent).Bold(true)
	dim    = onBg.Foreground(muted)
	chip   = onBg.Foreground(muted).Padding(0, 1)
	chipOn = onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1)
	keycap = onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1)
)

var (
	titlePaint = fig.Gradient(theme.BrightPurple, theme.Purple, theme.Cyan).By(fig.ByDiagonal)
	clockPaint = fig.Gradient(theme.Cyan, theme.Green).By(fig.ByRow)
	wordPaint  = fig.Palette(theme.BrightPurple, theme.Red, theme.Yellow, theme.Green, theme.Cyan, theme.Purple)
)

// anagrams are the words shown, one after the other.
var anagrams = []string{"listen", "silent", "enlist", "tinsel", "inlets"}

// start is the time the clock starts at: it rolls over a minute, and an
// hour, soon after.
var start = time.Date(2026, 11, 8, 9, 59, 54, 0, time.UTC)

type tickMsg struct{}

type model struct {
	ui     *bloom.Root
	w, h   int
	now    time.Time
	paused bool
	word   int
	hover  int // the glyph of the anagram under the pointer, or -1
	clicks int

	intro anim.Value // 0 to 1, the title's reveal
	clock fig.Morph
}

func newModel() *model {
	return &model{
		ui:    bloom.New(bloom.WithColors(fg, bg)),
		now:   start,
		hover: -1,
		intro: anim.NewValue(0, anim.Duration(1800*time.Millisecond), anim.Ease(anim.Linear), anim.FPS(60)),
		clock: fig.NewMorph(start.Format("15:04:05"), anim.Duration(450*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60)),
	}
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.intro.Play(0, 1), tick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		cmds = append(cmds, tick())
		if !m.paused {
			m.now = m.now.Add(time.Second)
			cmds = append(cmds, m.clock.Set(m.now.Format("15:04:05")))
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space", " ":
			m.word = (m.word + 1) % len(anagrams)
		case "i":
			cmds = append(cmds, m.intro.Play(0, 1))
		case "p":
			m.paused = !m.paused
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.intro, &m.clock))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.BackgroundColor = bg
	return v
}

func (m *model) layout() bloom.Child {
	title := fig.Text("letters").
		Font(fonts.Slant(), fonts.Small()).
		Paint(titlePaint).
		Each(m.intro.Float(), fx.Slide{From: fx.Top}, fx.Fade{}).
		Spread(0.6, fig.Outward)

	clock := m.clock.Text().
		Font(fonts.Big(), fonts.Small()).
		Paint(clockPaint).
		Key(fig.ByPosition).
		Enter(fx.Slide{From: fx.Top}, fx.Fade{}).
		Leave(fx.Slide{From: fx.Bottom}, fx.Fade{}).
		Spread(0.3, fig.Backward)

	word := fig.Text(anagrams[m.word]).
		Font(fonts.Big(), fonts.Small()).
		Paint(m.wordPaint()).
		ID("anagram").
		Animate(anim.Duration(900*time.Millisecond), anim.Ease(anim.Linear)).
		MoveWith(fx.Fly{Seed: 3}).
		OnGlyph(m.glyphHandlers)

	state := onBg.Foreground(theme.Green).Render(" ● running ")
	if m.paused {
		state = onBg.Foreground(theme.Yellow).Render(" ❚❚ paused ")
	}
	clockCard := card("clock", bloom.Row(bloom.Fixed(2, bloom.Space()), bloom.Fill(clock))).
		Edge(bloom.Top, bloom.End, bloom.Text(state).Style(onBg)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(m.now.Format(" Mon 2 Jan 2006 ")).Style(dim))
	wordCard := card("anagram", bloom.Centered(word)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf(" %d/%d ", m.word+1, len(anagrams))).Style(dim)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf(" hover or click a letter · %d clicks ", m.clicks)).Style(dim))

	return bloom.Column(
		bloom.Centered(title),
		bloom.Centered(bloom.Text("FIGlet text, animated glyph by glyph").Style(dim)),
		bloom.Row(
			// The clock is not centered, so that its digits stay put when the
			// time gets wider or narrower.
			bloom.Fill(clockCard),
			bloom.Fill(wordCard),
		),
		bloom.Centered(m.words()),
		bloom.Space(),
		keys("space", "next anagram", "i", "replay title", "p", "pause clock", "q", "quit"),
	)
}

// card is a panel with a title on its border, 8 rows tall inside.
func card(title string, child bloom.Child) bloom.BoxNode {
	return bloom.Box(panel, bloom.Column(bloom.Fixed(8, child))).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+title+" ").Style(label))
}

// words are the anagrams, the one shown highlighted, each shown by a click.
func (m *model) words() bloom.Child {
	chips := make([]bloom.Child, len(anagrams))
	for i, w := range anagrams {
		st := chip
		if i == m.word {
			st = chipOn
		}
		chips[i] = bloom.Text(w).Style(st).On(on.Click(func() tea.Cmd {
			m.word = i
			return nil
		}))
	}
	return bloom.Row(chips...).Gap(1)
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

// glyphHandlers lights up a letter of the anagram while the pointer is over
// it, and counts clicks on it.
func (m *model) glyphHandlers(g fig.Glyph) []on.Handler {
	return []on.Handler{
		on.Hover(func(over bool) tea.Cmd {
			switch {
			case over:
				m.hover = g.Index
			case m.hover == g.Index:
				m.hover = -1
			}
			return nil
		}),
		on.Click(func() tea.Cmd {
			m.clicks++
			return nil
		}),
	}
}

// wordPaint colors each letter of the anagram, and the one under the
// pointer bold and white.
func (m *model) wordPaint() fig.Paint {
	if m.hover < 0 {
		return wordPaint
	}
	hover := m.hover
	return fig.PaintFunc(func(f *fig.Figure, c fig.Cell) uv.Style {
		st := wordPaint.Shade(f, c)
		if c.Glyph == hover {
			st.Fg = theme.BrightWhite
			st.Attrs |= uv.AttrBold
		}
		return st
	})
}
