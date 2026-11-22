// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
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

	card        lipgloss.Style
	heading     lipgloss.Style
	label       lipgloss.Style
	big         lipgloss.Style
	input       lipgloss.Style
	inputOn     lipgloss.Style
	inputOnHint lipgloss.Style // the focused input, while it shows its placeholder
	hint        lipgloss.Style
	button      lipgloss.Style
	buttonOn    lipgloss.Style
	staged      lipgloss.Style
	live        lipgloss.Style
	failure     lipgloss.Style
	success     lipgloss.Style
	tour        lipgloss.Style
	status      lipgloss.Style
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg := t.Fg
	muted := t.BrightBlack
	accent := t.Blue
	good := t.Green
	warn := t.Red
	dark := t.Bg

	field := lipgloss.Lighten(t.Bg, 0.05)
	fieldOn := lipgloss.Lighten(t.Bg, 0.16)

	input := lipgloss.NewStyle().Foreground(fg).Background(field).Padding(0, 1)
	inputOn := input.Background(fieldOn)
	button := lipgloss.NewStyle().Foreground(fg).Background(muted).Padding(0, 2)
	return styles{
		theme: t,

		card:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 2),
		heading:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		label:       lipgloss.NewStyle().Foreground(muted),
		big:         lipgloss.NewStyle().Bold(true).Foreground(fg),
		input:       input,
		inputOn:     inputOn,
		inputOnHint: inputOn.Foreground(muted),
		hint:        input.Foreground(muted),
		button:      button,
		buttonOn:    button.Foreground(dark).Background(accent).Bold(true),
		staged:      lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
		live:        lipgloss.NewStyle().Foreground(dark).Background(good).Bold(true).Padding(0, 1),
		failure:     lipgloss.NewStyle().Foreground(warn),
		success:     lipgloss.NewStyle().Foreground(good),
		tour:        lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(accent).Padding(0, 2),
		status:      lipgloss.NewStyle().Foreground(muted),
	}
}
