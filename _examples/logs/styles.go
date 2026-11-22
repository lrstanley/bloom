// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

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

	panel   lipgloss.Style // the log's rounded frame
	title   lipgloss.Style // the log's name, on the frame
	counter lipgloss.Style // how many lines the log has, on the frame
	status  lipgloss.Style // the status line
	hint    lipgloss.Style // where the log is, and the keys
	numbers lipgloss.Style // the line numbers

	// The scrollbars on the log's frame, and while the pointer is over
	// them.
	bar      bloom.ScrollbarStyle
	barHover bloom.ScrollbarStyle

	match   lipgloss.Style // a match of the search
	current lipgloss.Style // the match found
	focused lipgloss.Style // the line of the match found

	// The styles of a log line, as the escape sequences that start them, made
	// once rather than rendered for each line.
	stamp  string
	levels map[string]string
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
		bar:      bloom.ScrollbarStyle{ThumbStyle: thumb},
		barHover: bloom.ScrollbarStyle{ThumbStyle: thumb.Foreground(t.Purple)},
		match:    lipgloss.NewStyle().Background(t.Yellow).Foreground(t.Bg),
		current:  lipgloss.NewStyle().Background(t.Purple).Foreground(t.Bg).Bold(true),
		focused:  lipgloss.NewStyle().Background(lipgloss.Lighten(t.Bg, 0.06)),
		stamp:    sgr(lipgloss.NewStyle().Foreground(muted)),
		levels: map[string]string{
			info:  sgr(lipgloss.NewStyle().Foreground(t.Blue)),
			warn:  sgr(lipgloss.NewStyle().Foreground(t.Yellow)),
			fail:  sgr(lipgloss.NewStyle().Foreground(t.Red).Bold(true)),
			debug: sgr(lipgloss.NewStyle().Foreground(muted)),
		},
	}
}

// sgr returns the escape sequence that starts style.
func sgr(style lipgloss.Style) string {
	s, _, _ := strings.Cut(style.Render("x"), "x")
	return s
}
