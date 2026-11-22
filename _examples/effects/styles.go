// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles. The model keeps them, so that nothing is a package
// global.
type styles struct {
	theme  *tint.Tint  // the tint the styles are made from
	accent color.Color // the color of the sweep effect's band

	frame  lipgloss.Style // the frame of a card
	name   lipgloss.Style // the name of a card's effect
	sample lipgloss.Style // the text the effects run on
	status lipgloss.Style // the status line
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	return styles{
		theme:  t,
		accent: t.Purple,
		frame:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack),
		name:   lipgloss.NewStyle().Bold(true).Foreground(t.Cyan),
		sample: lipgloss.NewStyle().Foreground(t.Fg).Background(lipgloss.Lighten(t.Bg, 0.1)).Padding(0, 1),
		status: lipgloss.NewStyle().Foreground(t.BrightBlack),
	}
}

// card is a frame with the name of its effect, around body.
func (st *styles) card(title string, body bloom.Child) bloom.Child {
	return bloom.Box(st.frame, bloom.Column(
		bloom.Text(title).Style(st.name),
		bloom.Fill(body),
	))
}
