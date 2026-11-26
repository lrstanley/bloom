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
	pane     lipgloss.Style // the frame of the reader and the details, padded
	bar      lipgloss.Style // the toolbar and the status bar
	title    lipgloss.Style // the toolbar's title
	action   lipgloss.Style // an action on the toolbar
	item     lipgloss.Style // an article in the list
	selected lipgloss.Style // the article open
	prose    lipgloss.Style // the reader's text
	chip     lipgloss.Style // a tag
	bold     lipgloss.Style // the article's title, in the details
	note     lipgloss.Style // quieter text, in the details
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a bar's panel, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	onBar := f.On(roles.Surface) // the roles for drawing on the toolbar and the status bar

	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border))
	bar := onBar.Fill(roles.Surface)
	item := f.Text(roles.Fg).Padding(0, 1)
	return styles{
		fit:      f,
		panel:    panel,
		pane:     panel.Padding(0, 1),
		bar:      bar,
		title:    onBar.Text(roles.Accent).Bold(true).Padding(0, 1),
		action:   bar.Padding(0, 1),
		item:     item,
		selected: f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		prose:    f.Text(roles.Fg),
		chip:     f.Fill(roles.Accent2).Padding(0, 1),
		bold:     lipgloss.NewStyle().Bold(true),
		note:     f.Text(roles.Muted),
	}
}
