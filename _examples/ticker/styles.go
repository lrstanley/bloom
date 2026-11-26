// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

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

	// good and bad are a stock's change, up or down, and goodOn and badOn
	// the same on the title bar and the selected row, which share its fill.
	// The live indicator pulses from dim to goodOn, on the title bar.
	good   color.Color
	bad    color.Color
	goodOn color.Color
	badOn  color.Color
	dim    color.Color

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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a bar's panel, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	panel := f.On(roles.Surface) // the roles for drawing on the headline bar and the banner
	title := f.On(roles.Accent)  // the roles for drawing on the title bar and the selected row

	onBg := f.Fill(roles.Bg)
	onPanel := panel.Fill(roles.Surface)
	header := f.Fill(roles.Accent).Bold(true).Padding(0, 1)
	item := onBg.Padding(0, 1)
	return styles{
		fit:    f,
		good:   f.Color(roles.Success),
		bad:    f.Color(roles.Danger),
		goodOn: title.Color(roles.Success),
		badOn:  title.Color(roles.Danger),
		dim:    title.Color(roles.Muted),

		onPanel: onPanel,
		header:  header,
		pulse:   header.Padding(0),
		live:    header.PaddingLeft(0),
		bar:     onPanel.Padding(0, 1),
		badge:   f.Fill(roles.Warning).Bold(true).Padding(0, 1),
		panel:   onBg.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		status:  onBg.Foreground(f.Color(roles.Muted)).Padding(0, 1),
		banner:  onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(panel.Color(roles.Warning)).Padding(0, 1),

		item:          item,
		selected:      header,
		itemRight:     item.Align(lipgloss.Right),
		selectedRight: header.Align(lipgloss.Right),
	}
}

// blend returns the color t of the way from a to b.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x) + (float64(y)-float64(x))*t) / 257) }
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 255}
}
