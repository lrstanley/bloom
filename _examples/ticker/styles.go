// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

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

	// muted, good, and bad are the live indicator's colors, and a stock's
	// change, up or down.
	muted color.Color
	good  color.Color
	bad   color.Color

	onPanel lipgloss.Style // the background of the headline bar and the banner
	header  lipgloss.Style // the title bar
	pulse   lipgloss.Style // the live indicator, in the title bar
	live    lipgloss.Style // the word after the live indicator
	bar     lipgloss.Style // the headline bar
	badge   lipgloss.Style // the label before the headlines
	panel   lipgloss.Style // the frame of the watchlist
	status  lipgloss.Style // the line of keys at the bottom
	banner  lipgloss.Style // the frame of the banner

	// item and selected are a row of the watchlist, and the selected one,
	// and itemRight and selectedRight are those aligned to the right, for
	// the numbers.
	item          lipgloss.Style
	selected      lipgloss.Style
	itemRight     lipgloss.Style
	selectedRight lipgloss.Style
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, warn, bg := t.Fg, t.BrightBlack, t.Blue, t.Yellow, t.Bg
	panelBg := lipgloss.Lighten(t.Bg, 0.05)

	onBg := lipgloss.NewStyle().Background(bg)
	onPanel := lipgloss.NewStyle().Background(panelBg)
	header := onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1)
	item := onBg.Foreground(fg).Padding(0, 1)
	selected := item.Foreground(bg).Background(accent).Bold(true)
	return styles{
		theme: t,
		muted: muted,
		good:  t.Green,
		bad:   t.Red,

		onPanel: onPanel,
		header:  header,
		pulse:   header.Padding(0),
		live:    header.PaddingLeft(0),
		bar:     onPanel.Foreground(fg).Padding(0, 1),
		badge:   onBg.Foreground(bg).Background(warn).Bold(true).Padding(0, 1),
		panel:   onBg.Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		status:  onBg.Foreground(muted).Padding(0, 1),
		banner:  onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(warn).Padding(0, 1),

		item:          item,
		selected:      selected,
		itemRight:     item.Align(lipgloss.Right),
		selectedRight: selected.Align(lipgloss.Right),
	}
}

// blend returns the color t of the way from a to b.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x) + (float64(y)-float64(x))*t) / 257) }
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 255}
}
