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

	panel      lipgloss.Style // the frame of the font list
	stage      lipgloss.Style // the frame of the font shown
	title      lipgloss.Style // a title on a frame's border
	dim        lipgloss.Style // notes, and the keys' descriptions
	item       lipgloss.Style // a font in the list
	selected   lipgloss.Style // the font shown, in the list
	paintLabel lipgloss.Style // the label before the paints
	chip       lipgloss.Style // a paint
	chipOn     lipgloss.Style // the paint in use
	keycap     lipgloss.Style // a key in the footer

	// paints are the paints shown, in order.
	paints []paint
}

// paint is one of fig's paints, by name.
type paint struct {
	name  string
	paint fig.Paint
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, bg := t.Fg, t.BrightBlack, t.Blue, t.Bg

	onBg := lipgloss.NewStyle().Background(bg)
	panel := onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.45))
	title := onBg.Foreground(accent).Bold(true)
	text := onBg.Foreground(fg)

	pink, peach := lipgloss.Color("#ff6f91"), lipgloss.Color("#ffc29b")
	sky, violet := t.Cyan, t.Purple

	return styles{
		theme:      t,
		panel:      panel,
		stage:      panel.BorderForeground(accent).Padding(0, 2),
		title:      title,
		dim:        onBg.Foreground(muted),
		item:       text.Padding(0, 1),
		selected:   onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1),
		paintLabel: title.PaddingRight(1),
		chip:       onBg.Foreground(muted).Padding(0, 1),
		chipOn:     onBg.Foreground(bg).Background(t.Purple).Bold(true).Padding(0, 1),
		keycap:     onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1),
		paints: []paint{
			{"column", fig.Gradient(pink, peach)},
			{"diagonal", fig.Gradient(sky, violet, pink).By(fig.ByDiagonal)},
			{"row", fig.Gradient(peach, pink, violet).By(fig.ByRow)},
			{"glyph", fig.Gradient(sky, pink).By(fig.ByGlyph)},
			{"palette", fig.Palette(t.Red, t.Yellow, t.Green, t.Cyan, t.Blue)},
			{"rainbow", fig.Rainbow()},
			{"metal", fig.Metal()},
			{"filled", fig.Fill(lipgloss.Darken(violet, 0.55), fig.Solid(peach))},
		},
	}
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
