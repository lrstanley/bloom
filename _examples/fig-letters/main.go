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

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	intro   anim.Value // 0 to 1, the title's reveal
	clock   clock
	anagram anagram
}

func newModel() *model {
	m := &model{
		st:    newStyles(tint.TintDracula),
		intro: anim.NewValue(0, anim.Duration(1800*time.Millisecond), anim.Ease(anim.Linear), anim.FPS(60)),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	m.clock = newClock()
	m.clock.setStyles(m.st)
	m.anagram = anagram{st: m.st, hover: -1}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.intro.Play(0, 1), tick())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		cmds = append(cmds, tick(), m.clock.advance())
	case tea.KeyPressMsg:
		if k := msg.String(); k == "q" || k == "ctrl+c" {
			return m, tea.Quit
		}
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, anim.Update(msg, &m.intro, &m.clock.morph))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

// key shows the next anagram, replays the title, or pauses the clock.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "space":
		m.anagram.word = (m.anagram.word + 1) % len(anagrams)
	case "i":
		return m.intro.Play(0, 1)
	case "p":
		m.clock.paused = !m.clock.paused
	}
	return nil
}

// layout places the title over the clock and the anagram, with the
// anagrams under them and the keys at the bottom.
func (m *model) layout() bloom.Child {
	title := fig.Text("letters").
		Font(fonts.Slant(), fonts.Small()).
		Paint(m.st.titlePaint).
		Each(m.intro.Float(), fx.Slide{From: fx.Top}, fx.Fade{}).
		Spread(0.6, fig.Outward)

	return bloom.Column(
		bloom.Centered(title),
		bloom.Centered(bloom.Text("FIGlet text, animated glyph by glyph").Style(m.st.dim)),
		bloom.Row(
			// The clock is not centered, so that its digits stay put when the
			// time gets wider or narrower.
			bloom.Fill(&m.clock),
			bloom.Fill(&m.anagram),
		),
		bloom.Centered(m.anagram.words()),
		bloom.Space(),
		m.st.keys("space", "next anagram", "i", "replay title", "p", "pause clock", "q", "quit"),
	)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
