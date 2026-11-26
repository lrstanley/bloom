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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a running clock, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	hue := f.Color // the tint's hues, which the figures are painted in

	onBg := f.Fill(roles.Bg)
	muted := onBg.Foreground(f.Color(roles.Muted))
	return styles{
		fit:     f,
		onBg:    onBg,
		panel:   onBg.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 2),
		label:   onBg.Foreground(f.Color(roles.Accent)).Bold(true),
		dim:     muted,
		chip:    muted.Padding(0, 1),
		chipOn:  f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		keycap:  f.Fill(roles.Raised).Padding(0, 1),
		running: onBg.Foreground(f.Color(roles.Success)),
		paused:  onBg.Foreground(f.Color(roles.Warning)),

		titlePaint: fig.Gradient(hue(roles.Accent), hue(roles.Magenta), hue(roles.Cyan)).By(fig.ByDiagonal),
		clockPaint: fig.Gradient(hue(roles.Cyan), hue(roles.Green)).By(fig.ByRow),
		wordPaint:  fig.Palette(hue(roles.Magenta), hue(roles.Red), hue(roles.Yellow), hue(roles.Green), hue(roles.Cyan), hue(roles.Blue)),
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
