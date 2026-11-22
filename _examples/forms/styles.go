// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom/fx"
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

	panel       lipgloss.Style
	heading     lipgloss.Style
	headingWarn lipgloss.Style // the heading of the overwrite dialog
	label       lipgloss.Style
	input       lipgloss.Style
	inputOn     lipgloss.Style
	inputOnHint lipgloss.Style // the focused input, while it shows its placeholder
	hint        lipgloss.Style
	button      lipgloss.Style
	buttonOn    lipgloss.Style
	dropdown    lipgloss.Style
	option      lipgloss.Style
	dialog      lipgloss.Style
	danger      lipgloss.Style
	toastBox    lipgloss.Style
	statusBar   lipgloss.Style
	backdrop    fx.Recolor
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg := t.Fg
	muted := t.BrightBlack
	accent := t.Purple
	warn := t.Red
	good := t.Cyan
	dark := t.Bg
	field := lipgloss.Lighten(t.Bg, 0.05)
	fieldOn := lipgloss.Lighten(t.Bg, 0.16)

	rounded := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	heading := lipgloss.NewStyle().Bold(true).Foreground(accent)
	input := lipgloss.NewStyle().Foreground(fg).Background(field).Padding(0, 1)
	inputOn := input.Background(fieldOn)
	button := lipgloss.NewStyle().Foreground(fg).Background(muted).Padding(0, 2)
	dialog := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(accent).Padding(1, 3)
	return styles{
		theme: t,

		panel:       rounded.BorderForeground(muted).Padding(1, 2),
		heading:     heading,
		headingWarn: heading.Foreground(warn),
		label:       lipgloss.NewStyle().Foreground(muted),
		input:       input,
		inputOn:     inputOn,
		inputOnHint: inputOn.Foreground(muted),
		hint:        input.Foreground(muted),
		button:      button,
		buttonOn:    button.Foreground(dark).Background(accent).Bold(true),
		dropdown:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(accent),
		option:      lipgloss.NewStyle().Foreground(fg).Padding(0, 1),
		dialog:      dialog,
		danger:      dialog.BorderForeground(warn),
		toastBox:    rounded.BorderForeground(good).Padding(0, 1),
		statusBar:   lipgloss.NewStyle().Foreground(muted),
		backdrop:    fx.Recolor{Fg: lipgloss.Lighten(t.Bg, 0.15), Bg: lipgloss.Darken(t.Bg, 0.3)},
	}
}
