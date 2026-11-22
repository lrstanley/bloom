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
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fig/fonts"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// gallery is every bundled font, by name. The gallery starts on figlet's
// standard font.
var (
	gallery  = fonts.All()
	standard = slices.Index(fonts.Names, "standard")
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	font  int // index into gallery, the font shown
	paint int // index into st.paints, the paint in use

	reveal anim.Value // 0 to 1, the glyphs of a new font appearing
}

func newModel() *model {
	m := &model{
		st:     newStyles(tint.TintTokyoNight),
		font:   standard,
		reveal: anim.NewValue(1, anim.Duration(300*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60)),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg))
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.reveal.Play(0, 1) }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if k := msg.String(); k == "q" || k == "ctrl+c" {
			return m, tea.Quit
		}
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, anim.Update(msg, &m.reveal))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

// key changes the font or the paint.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "down", "j":
		return m.stepFont(1)
	case "up", "k":
		return m.stepFont(-1)
	case "right", "l":
		m.paint = (m.paint + 1) % len(m.st.paints)
	case "left", "h":
		m.paint = (m.paint + len(m.st.paints) - 1) % len(m.st.paints)
	}
	return nil
}

// pickFont shows font i, wrapping around the gallery, and reveals it.
func (m *model) pickFont(i int) tea.Cmd {
	m.font = (i + len(gallery)) % len(gallery)
	return m.reveal.Play(0, 1)
}

// stepFont shows the font by fonts on from the one shown.
func (m *model) stepFont(by int) tea.Cmd { return m.pickFont(m.font + by) }

// pickPaint uses paint i.
func (m *model) pickPaint(i int) tea.Cmd {
	m.paint = i
	return nil
}

// layout places the font list beside the stage, with the paints under the
// stage and the keys at the bottom.
func (m *model) layout() bloom.Child {
	list := fontList{st: &m.st, font: m.font, pick: m.pickFont, step: m.stepFont}
	shown := stage{st: &m.st, font: m.font, paint: m.st.paints[m.paint], reveal: m.reveal.Float()}
	paints := paintBar{st: &m.st, paint: m.paint, pick: m.pickPaint}

	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fixed(30, list).Collapsible(1),
			bloom.Fill(bloom.Column(
				bloom.Fill(shown),
				paints,
			)).Min(40),
		)),
		m.st.keys("↑↓", "font", "←→", "paint", "click", "pick", "q", "quit"),
	)
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
