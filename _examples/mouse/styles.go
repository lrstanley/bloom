// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as the selected file or the row under
// the pointer, from the tint, so the example reads on any tint, dark or
// light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	divider := f.Text(roles.Border)
	return styles{
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		item:     f.Text(roles.Fg).Padding(0, 1),
		hovered:  f.Fill(roles.Hover).Padding(0, 1),
		selected: f.Fill(roles.Selection).Bold(true).Padding(0, 1),
		divider:  divider,
		grabbed:  divider.Foreground(f.Color(roles.Accent)),
		heading:  f.Text(roles.Accent).Bold(true).Padding(0, 1),
		page:     lipgloss.NewStyle().Padding(0, 1),
		code:     f.Text(roles.Fg),
		status:   f.Text(roles.Muted),
	}
}
