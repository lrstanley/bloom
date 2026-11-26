// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fig"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or the focused frame,
// from the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})

	onBg := f.Fill(roles.Bg)
	panel := onBg.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border))
	title := onBg.Foreground(f.Color(roles.Accent)).Bold(true)
	muted := onBg.Foreground(f.Color(roles.Muted))

	pink, peach := lipgloss.Color("#ff6f91"), lipgloss.Color("#ffc29b")
	sky, violet := f.Color(roles.Cyan), f.Color(roles.Magenta)

	return styles{
		fit:        f,
		panel:      panel,
		stage:      panel.BorderForeground(f.Color(roles.Accent)).Padding(0, 2),
		title:      title,
		dim:        muted,
		item:       onBg.Padding(0, 1),
		selected:   f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		paintLabel: title.PaddingRight(1),
		chip:       muted.Padding(0, 1),
		chipOn:     f.Fill(roles.Accent2).Bold(true).Padding(0, 1),
		keycap:     f.Fill(roles.Raised).Padding(0, 1),
		paints: []paint{
			{"column", fig.Gradient(pink, peach)},
			{"diagonal", fig.Gradient(sky, violet, pink).By(fig.ByDiagonal)},
			{"row", fig.Gradient(peach, pink, violet).By(fig.ByRow)},
			{"glyph", fig.Gradient(sky, pink).By(fig.ByGlyph)},
			{"palette", fig.Palette(f.Color(roles.Red), f.Color(roles.Yellow), f.Color(roles.Green), sky, f.Color(roles.Blue))},
			{"rainbow", fig.Rainbow()},
			{"metal", fig.Metal()},
			{"filled", fig.Fill(f.Color(roles.Magenta.Soft()), fig.Solid(peach))},
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
