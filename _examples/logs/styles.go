// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or an error's level, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	muted := f.Text(roles.Muted)
	return styles{
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		title:    f.Text(roles.Accent).Bold(true).Padding(0, 1),
		counter:  muted.Padding(0, 1),
		status:   f.Text(roles.Fg),
		hint:     muted,
		numbers:  muted,
		bar:      bloom.ScrollbarStyle{ThumbStyle: f.Text(roles.BorderStrong)},
		barHover: bloom.ScrollbarStyle{ThumbStyle: f.Text(roles.Accent)},
		match:    f.Fill(roles.Yellow),
		current:  f.Fill(roles.Accent).Bold(true),
		focused:  lipgloss.NewStyle().Background(f.Color(roles.Bg.Selected())),
		stamp:    sgr(muted),
		levels: map[string]string{
			info:  sgr(f.Text(roles.Info)),
			warn:  sgr(f.Text(roles.Warning)),
			fail:  sgr(f.Text(roles.Danger).Bold(true)),
			debug: sgr(muted),
		},
	}
}

// sgr returns the escape sequence that starts style.
func sgr(style lipgloss.Style) string {
	s, _, _ := strings.Cut(style.Render("x"), "x")
	return s
}
