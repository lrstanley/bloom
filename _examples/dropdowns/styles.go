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

	heading  lipgloss.Style
	text     lipgloss.Style
	label    lipgloss.Style
	panel    lipgloss.Style
	dialog   lipgloss.Style
	input    lipgloss.Style
	inputOn  lipgloss.Style
	button   lipgloss.Style
	buttonOn lipgloss.Style
	status   lipgloss.Style
	backdrop fx.Dim

	// A list's border joins its field's sides, and the edge facing the
	// field is left open: listBelow is a list that opens below its field,
	// and listAbove one that opens above it.
	listBelow    lipgloss.Style
	listAbove    lipgloss.Style
	option       lipgloss.Style
	optionPicked lipgloss.Style
	optionCursor lipgloss.Style
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg := t.Fg
	muted := t.BrightBlack
	accent := t.Purple
	dark := t.Bg
	field := lipgloss.Lighten(t.Bg, 0.06)
	fieldOn := lipgloss.Lighten(t.Bg, 0.16)
	listBg := lipgloss.Lighten(t.Bg, 0.1)

	input := lipgloss.NewStyle().Foreground(fg).Background(field).Padding(0, 1)
	button := lipgloss.NewStyle().Foreground(fg).Background(muted).Padding(0, 2)
	list := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(accent).Background(listBg).BorderBackground(listBg)
	option := lipgloss.NewStyle().Foreground(fg).Background(listBg).Padding(0, 1)
	return styles{
		theme: t,

		heading:  lipgloss.NewStyle().Bold(true).Foreground(accent),
		text:     lipgloss.NewStyle().Foreground(fg),
		label:    lipgloss.NewStyle().Foreground(muted),
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(1, 2),
		dialog:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2),
		input:    input,
		inputOn:  input.Background(fieldOn),
		button:   button,
		buttonOn: button.Foreground(dark).Background(accent).Bold(true),
		status:   lipgloss.NewStyle().Foreground(muted),
		backdrop: fx.Dim{Amount: 0.5},

		listBelow:    list.BorderTop(false),
		listAbove:    list.BorderBottom(false),
		option:       option,
		optionPicked: option.Foreground(accent).Bold(true),
		optionCursor: option.Foreground(dark).Background(accent),
	}
}
