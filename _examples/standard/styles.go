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
	page     lipgloss.Style // the frame of the reader, padded
	item     lipgloss.Style // a sidebar item
	selected lipgloss.Style // the sidebar item of the page open
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack)
	item := lipgloss.NewStyle().Padding(0, 1)
	return styles{
		theme:    t,
		panel:    panel,
		page:     panel.Padding(0, 1),
		item:     item,
		selected: item.Foreground(t.Bg).Background(t.Green),
	}
}
