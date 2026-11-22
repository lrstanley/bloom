// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fig"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	theme *tint.Tint // the tint the styles are made from

	onBg    lipgloss.Style // the background, and nothing else
	panel   lipgloss.Style // the frame of a card
	label   lipgloss.Style // a card's title
	dim     lipgloss.Style // notes, and the keys' descriptions
	chip    lipgloss.Style // an anagram under the cards
	chipOn  lipgloss.Style // the anagram shown
	keycap  lipgloss.Style // a key in the footer
	running lipgloss.Style // the clock's state, while it runs
	paused  lipgloss.Style // the clock's state, while it is paused

	titlePaint fig.Paint
	clockPaint fig.Paint
	wordPaint  fig.Paint
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, bg := t.Fg, t.BrightBlack, t.Purple, t.Bg

	onBg := lipgloss.NewStyle().Background(bg)
	return styles{
		theme:   t,
		onBg:    onBg,
		panel:   onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.35)).Padding(0, 2),
		label:   onBg.Foreground(accent).Bold(true),
		dim:     onBg.Foreground(muted),
		chip:    onBg.Foreground(muted).Padding(0, 1),
		chipOn:  onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1),
		keycap:  onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1),
		running: onBg.Foreground(t.Green),
		paused:  onBg.Foreground(t.Yellow),

		titlePaint: fig.Gradient(t.BrightPurple, t.Purple, t.Cyan).By(fig.ByDiagonal),
		clockPaint: fig.Gradient(t.Cyan, t.Green).By(fig.ByRow),
		wordPaint:  fig.Palette(t.BrightPurple, t.Red, t.Yellow, t.Green, t.Cyan, t.Purple),
	}
}

// card is a panel with a title on its border, 8 rows tall inside.
func (st *styles) card(title string, child bloom.Child) bloom.BoxNode {
	return bloom.Box(st.panel, bloom.Column(bloom.Fixed(8, child))).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+title+" ").Style(st.label))
}

// keys is a footer of keys and what they do, as pairs.
func (st *styles) keys(pairs ...string) bloom.Child {
	parts := make([]bloom.Child, 0, len(pairs))
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, bloom.Row(
			bloom.Text(pairs[i]).Style(st.keycap),
			bloom.Text(" "+pairs[i+1]).Style(st.dim),
		))
	}
	return bloom.Wrap(parts...).Gap(2, 0)
}
