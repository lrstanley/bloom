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

	panel    lipgloss.Style // a rounded frame
	item     lipgloss.Style // a file in the list
	hovered  lipgloss.Style // a file under the pointer
	selected lipgloss.Style // the selected file
	divider  lipgloss.Style // the line between the panes
	grabbed  lipgloss.Style // the line between the panes, under the pointer
	heading  lipgloss.Style // the name of the opened file, padded
	page     lipgloss.Style // the padding around the file's text
	code     lipgloss.Style // the file's text
	status   lipgloss.Style // the status line
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	item := lipgloss.NewStyle().Foreground(t.Fg).Padding(0, 1)
	divider := lipgloss.NewStyle().Foreground(t.BrightBlack)
	return styles{
		theme:    t,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack),
		item:     item,
		hovered:  item.Background(lipgloss.Lighten(t.Bg, 0.08)),
		selected: item.Foreground(t.Bg).Background(t.Blue).Bold(true),
		divider:  divider,
		grabbed:  divider.Foreground(t.Blue),
		heading:  lipgloss.NewStyle().Bold(true).Foreground(t.Blue).Padding(0, 1),
		page:     lipgloss.NewStyle().Padding(0, 1),
		code:     lipgloss.NewStyle().Foreground(t.Fg),
		status:   lipgloss.NewStyle().Foreground(t.BrightBlack),
	}
}
