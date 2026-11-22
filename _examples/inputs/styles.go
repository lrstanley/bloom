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

	panel   lipgloss.Style // the frame of a box, padded
	focused lipgloss.Style // the frame of the box with focus
	hint    lipgloss.Style // the line of help under the boxes
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.BrightBlack).Padding(0, 1)
	return styles{
		theme:   t,
		panel:   panel,
		focused: panel.BorderForeground(t.Green),
		hint:    lipgloss.NewStyle().Foreground(t.BrightBlack).Padding(0, 1),
	}
}
