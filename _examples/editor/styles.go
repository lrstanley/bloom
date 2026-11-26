// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a scrollbar's thumb,
// from the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	muted := f.Text(roles.Muted)
	thumb := f.Text(roles.BorderStrong)
	return styles{
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		title:    f.Text(roles.Accent).Bold(true).Padding(0, 1),
		counter:  muted.Padding(0, 1),
		status:   f.Text(roles.Fg),
		hint:     muted,
		numbers:  f.Text(roles.Subtle),
		current:  lipgloss.NewStyle().Background(f.Color(roles.Surface)),
		bar:      bloom.ScrollbarStyle{ThumbStyle: thumb},
		barHover: bloom.ScrollbarStyle{ThumbStyle: thumb.Foreground(f.Color(roles.Accent))},
	}
}
