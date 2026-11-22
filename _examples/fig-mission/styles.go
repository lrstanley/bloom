// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

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

	onBg   lipgloss.Style // the background, and nothing else
	panel  lipgloss.Style // the frame of a card
	title  lipgloss.Style // a card's title
	text   lipgloss.Style // the names of services and deploys
	dim    lipgloss.Style // notes, and the keys' descriptions
	keycap lipgloss.Style // a key in the footer
	plain  lipgloss.Style // the header, when no font fits

	// good, bad, and warn color what is up, down, or slow.
	good lipgloss.Style
	bad  lipgloss.Style
	warn lipgloss.Style

	headerPaint fig.Paint
	toastPaint  fig.Paint
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, bg := t.Fg, t.BrightBlack, t.Blue, t.Bg

	onBg := lipgloss.NewStyle().Background(bg)
	return styles{
		theme:  t,
		onBg:   onBg,
		panel:  onBg.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.45)).Padding(0, 1),
		title:  onBg.Foreground(accent).Bold(true),
		text:   onBg.Foreground(fg),
		dim:    onBg.Foreground(muted),
		keycap: onBg.Foreground(fg).Background(lipgloss.Lighten(bg, 0.12)).Padding(0, 1),
		plain:  onBg.Foreground(accent).Bold(true).Padding(0, 1),
		good:   onBg.Foreground(t.Green),
		bad:    onBg.Foreground(t.Red),
		warn:   onBg.Foreground(t.Yellow),

		headerPaint: fig.Gradient(t.Purple, t.Blue, t.Cyan).By(fig.ByDiagonal),
		toastPaint:  fig.Gradient(t.Green, t.Yellow).By(fig.ByColumn),
	}
}

// card is a panel with a title, and a note on the right, on its border.
func (st *styles) card(title, note string, child bloom.Child) bloom.BoxNode {
	return bloom.Box(st.panel, child).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+title+" ").Style(st.title)).
		Edge(bloom.Top, bloom.End, bloom.Text(" "+note+" ").Style(st.dim))
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

// blend returns the color t of the way from a to b.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x) + (float64(y)-float64(x))*t) / 257) }
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 255}
}
