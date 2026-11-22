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

	card    lipgloss.Style // the frame of a card
	name    lipgloss.Style // a card's name, and a dialog's title
	rowEven lipgloss.Style // the even rows of a list, from the first
	rowOdd  lipgloss.Style // the odd rows of a list
	panel   lipgloss.Style // the bordered panel a card transitions
	status  lipgloss.Style // quieter text, such as the status line
	button  lipgloss.Style // a key that opens a dialog
	dialog  lipgloss.Style // the frame of a dialog
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	ink, muted, accent, dark := t.Fg, t.BrightBlack, t.Cyan, t.Bg
	surface := lipgloss.Lighten(t.Bg, 0.1)
	raised := lipgloss.Lighten(t.Bg, 0.15)

	rowEven := lipgloss.NewStyle().Foreground(ink).Background(surface)
	return styles{
		theme:   t,
		card:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		name:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		rowEven: rowEven,
		rowOdd:  rowEven.Background(raised),
		panel:   rowEven.Border(lipgloss.NormalBorder()).BorderForeground(t.Blue).Padding(0, 1),
		status:  lipgloss.NewStyle().Foreground(muted),
		button:  lipgloss.NewStyle().Foreground(dark).Background(accent).Padding(0, 1),
		dialog:  lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(t.Yellow).Padding(1, 3),
	}
}
