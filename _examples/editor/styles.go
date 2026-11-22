// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
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

	panel   lipgloss.Style // the text's rounded frame
	title   lipgloss.Style // the text's name, on the frame
	counter lipgloss.Style // how many lines the text has, on the frame
	status  lipgloss.Style // the status line
	hint    lipgloss.Style // where the caret is, and the keys
	numbers lipgloss.Style // the line numbers
	current lipgloss.Style // the line the caret is on

	// The scrollbars on the text's frame, and while the pointer is over
	// them.
	bar      bloom.ScrollbarStyle
	barHover bloom.ScrollbarStyle
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	muted, accent := t.BrightBlack, t.Blue
	thumb := lipgloss.NewStyle().Foreground(accent)
	return styles{
		theme:    t,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		title:    lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 1),
		counter:  lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
		status:   lipgloss.NewStyle().Foreground(t.Fg),
		hint:     lipgloss.NewStyle().Foreground(muted),
		numbers:  lipgloss.NewStyle().Foreground(muted),
		current:  lipgloss.NewStyle().Background(lipgloss.Lighten(t.Bg, 0.05)),
		bar:      bloom.ScrollbarStyle{ThumbStyle: thumb},
		barHover: bloom.ScrollbarStyle{ThumbStyle: thumb.Foreground(t.Purple)},
	}
}
