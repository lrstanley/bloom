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
// rather than copying them into every frame. Only the screen,
// and what is raised above it, set a background; the rest are drawn over
// the screen's.
type styles struct {
	theme     *tint.Tint // the tint the styles are made from
	base      lipgloss.Style
	screen    lipgloss.Style
	text      lipgloss.Style
	muted     lipgloss.Style
	logo      lipgloss.Style
	badge     lipgloss.Style
	raised    lipgloss.Style
	pane      lipgloss.Style
	lifted    lipgloss.Style
	title     lipgloss.Style
	grabbed   lipgloss.Style
	item      lipgloss.Style
	hovered   lipgloss.Style
	selected  lipgloss.Style
	highlight lipgloss.Style
	toast     lipgloss.Style
	tour      lipgloss.Style
	status    [3]lipgloss.Style // by a service's status.
	shadow    color.Color
	bars      []color.Color       // the activity bars' gradient.
	swatches  [2][]lipgloss.Style // the tint's colors, then its bright ones.
	dots      [][]lipgloss.Style  // a few of each of tints' colors.
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	bg := func(cs ...color.Color) []lipgloss.Style {
		row := make([]lipgloss.Style, len(cs))
		for i, c := range cs {
			row[i] = lipgloss.NewStyle().Background(c)
		}
		return row
	}
	raised := lipgloss.Lighten(t.Bg, 0.06)
	pane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack).Padding(0, 1)
	title := fg(t.Purple).Bold(true).Padding(0, 1)
	item := lipgloss.NewStyle().Padding(0, 1)
	floating := lipgloss.NewStyle().Background(raised).BorderBackground(t.Bg).Padding(0, 2)
	dots := make([][]lipgloss.Style, len(tints))
	for i, t := range tints {
		for _, c := range []color.Color{t.Red, t.Yellow, t.Green, t.Blue, t.Purple} {
			dots[i] = append(dots[i], fg(c))
		}
	}
	return styles{
		theme:     t,
		base:      lipgloss.NewStyle().Background(t.Bg),
		screen:    fg(t.Fg).Background(t.Bg).Padding(0, 1),
		text:      fg(t.Fg),
		muted:     fg(t.BrightBlack),
		logo:      fg(t.Purple).Bold(true),
		badge:     fg(t.Bg).Background(t.Purple).Bold(true).Padding(0, 1),
		raised:    lipgloss.NewStyle().Background(raised),
		pane:      pane,
		lifted:    pane.BorderForeground(t.Purple).Background(t.Bg).BorderBackground(t.Bg),
		title:     title,
		grabbed:   title.Foreground(t.Bg).Background(t.Purple),
		item:      item,
		hovered:   item.Background(raised),
		selected:  item.Background(lipgloss.Lighten(t.Bg, 0.12)),
		highlight: fg(t.Purple).Bold(true),
		toast:     floating.Border(lipgloss.RoundedBorder()).BorderForeground(t.Purple),
		tour:      floating.Border(lipgloss.RoundedBorder()).BorderForeground(t.Blue),
		status:    [3]lipgloss.Style{fg(t.Green), fg(t.Yellow), fg(t.Red)},
		shadow:    lipgloss.Darken(t.Bg, 0.5),
		bars:      []color.Color{t.Blue, t.Purple, t.Red},
		swatches: [2][]lipgloss.Style{
			bg(t.Black, t.Red, t.Green, t.Yellow, t.Blue, t.Purple, t.Cyan, t.White),
			bg(
				t.BrightBlack, t.BrightRed, t.BrightGreen, t.BrightYellow,
				t.BrightBlue, t.BrightPurple, t.BrightCyan, t.BrightWhite,
			),
		},
		dots: dots,
	}
}
