// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles. The model keeps them, so that nothing is a package
// global.
type styles struct {
	theme *tint.Tint // the tint the styles are made from

	panel    lipgloss.Style // a rounded frame
	heading  lipgloss.Style // the title at the top
	item     lipgloss.Style // an item of the list
	selected lipgloss.Style // the selected item
	marker   lipgloss.Style // the marker that follows the selected item
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	item := lipgloss.NewStyle().Padding(0, 1).Foreground(t.Fg)
	return styles{
		theme:    t,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack),
		heading:  lipgloss.NewStyle().Bold(true).Foreground(t.Blue),
		item:     item,
		selected: item.Background(lipgloss.Lighten(t.Bg, 0.12)),
		marker:   lipgloss.NewStyle().Foreground(t.Green).Bold(true),
	}
}
