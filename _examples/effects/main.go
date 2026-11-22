// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command effects is a gallery of cell effects. Every card runs its effect
// as a transition, in and out on a loop driven by one animated value; two of
// them are custom effects written with fx.Func, and one card is a
// uv.Drawable placed with bloom.Drawable. The backdrops example shows the
// backdrop effects.
//
// Keys: space pauses, q quits.
package main

import (
	"fmt"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

const sampleText = "The quick brown fox\njumps over the lazy\ndog. 0123456789 ✓"

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	gallery []galleryCard // the cards, each with its effects
	t       anim.Sequence // the gallery's progress, in and out, holding at each end
	phase   anim.Sequence // the plasma's phase
	paused  bool
}

func newModel() *model {
	m := &model{
		st: newStyles(tint.TintDracula),
		t: anim.NewSequence(0,
			anim.To(1), anim.Hold(700*time.Millisecond),
			anim.To(0), anim.Hold(700*time.Millisecond),
		).With(anim.Duration(1400*time.Millisecond), anim.Ease(anim.InOutCubic)),
		phase: anim.NewLoop(26 * time.Second),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg))
	m.gallery = newGallery(m.st.accent)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return tea.Batch(m.t.Loop(), m.phase.Loop()) }

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
				m.t.Stop()
				m.phase.Stop()
			} else {
				cmds = append(cmds, m.t.Loop(), m.phase.Loop())
			}
		}
	}
	return m, tea.Batch(append(cmds, anim.Update(msg, &m.t, &m.phase))...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

// layout places the cards in a grid, over the status line.
func (m *model) layout() bloom.Child {
	t := m.t.Float()
	cards := make([]bloom.Child, 0, len(m.gallery)+1)
	for _, g := range m.gallery {
		text := bloom.Text(sampleText).Style(m.st.sample)
		cards = append(cards, m.st.card(g.name, bloom.Transition(t, text, g.effects...)))
	}
	drawing := bloom.Drawable(plasma{t: m.phase.Float()})
	cards = append(cards, m.st.card("Drawable + Fade", bloom.Transition(t, drawing, fx.Fade{})))

	hints := fmt.Sprintf(" t = %.2f · space: pause · q: quit", t)
	return bloom.Column(
		bloom.Fill(bloom.Grid(3, cards...).Gap(1, 0)),
		bloom.Text(hints).Style(m.st.status).Truncate(),
	)
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithFPS(60)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
