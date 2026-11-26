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

// shadow is the shadow under a dragged pane: a role the example adds to
// bubbletint's, the background darkened by half, on a light tint too.
var shadow = roles.Define(roles.Spec{
	Name: "shadow",
	Kind: roles.KindSurface,
	Derive: func(p *roles.Palette) tint.Color {
		return p.Color(roles.Bg).Mix(tint.Color{A: 0xff}, 0.5)
	},
	Distinct: []roles.Role{roles.Bg},
	Fallback: roles.Fallback{Slots: []tint.Slot{tint.SlotBlack}},
})

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame. Only the screen,
// and what is raised above it, set a background; the rest are drawn over
// the screen's.
type styles struct {
	theme     *tint.Tint    // the tint the styles are made from, whose name and palette are shown
	fit       *roles.Fitted // the tint's roles, which the example's colors come from
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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised toast, from
// the tint, so the example reads on any tint, dark or light. The palette's
// swatches, and the dots beside each tint's name, show the tints' own colors.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	raised := f.On(roles.Raised) // the roles for drawing on the toast and the tour's note

	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	bg := func(cs ...color.Color) []lipgloss.Style {
		row := make([]lipgloss.Style, len(cs))
		for i, c := range cs {
			row[i] = lipgloss.NewStyle().Background(c)
		}
		return row
	}
	pane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 1)
	title := f.Text(roles.Accent).Bold(true).Padding(0, 1)
	item := lipgloss.NewStyle().Padding(0, 1)
	floating := raised.Fill(roles.Raised).BorderBackground(f.Color(roles.Bg)).Padding(0, 2)
	dots := make([][]lipgloss.Style, len(tints))
	for i, t := range tints {
		for _, c := range []color.Color{t.Red, t.Yellow, t.Green, t.Blue, t.Purple} {
			dots[i] = append(dots[i], fg(c))
		}
	}
	return styles{
		theme:     t,
		fit:       f,
		base:      f.Fill(roles.Bg),
		screen:    f.Fill(roles.Bg).Padding(0, 1),
		text:      f.Text(roles.Fg),
		muted:     f.Text(roles.Muted),
		logo:      f.Text(roles.Accent).Bold(true),
		badge:     f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		raised:    raised.Fill(roles.Raised),
		pane:      pane,
		lifted:    pane.BorderForeground(f.Color(roles.Accent)).Background(f.Color(roles.Bg)).BorderBackground(f.Color(roles.Bg)),
		title:     title,
		grabbed:   f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		item:      item,
		hovered:   item.Background(f.Color(roles.Hover)),
		selected:  item.Background(f.Color(roles.Selection)),
		highlight: f.Text(roles.Accent).Bold(true),
		toast:     floating.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Accent)),
		tour:      floating.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Info)),
		status:    [3]lipgloss.Style{f.Text(roles.Success), f.Text(roles.Warning), f.Text(roles.Danger)},
		shadow:    f.Color(shadow),
		bars:      []color.Color{f.Color(roles.Blue), f.Color(roles.Magenta), f.Color(roles.Red)},
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
