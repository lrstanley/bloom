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

	page        lipgloss.Style
	title       lipgloss.Style
	text        lipgloss.Style
	label       lipgloss.Style
	header      lipgloss.Style
	marker      lipgloss.Style
	tool        lipgloss.Style
	toolHover   lipgloss.Style
	toolOn      lipgloss.Style
	statusBar   lipgloss.Style
	statusMuted lipgloss.Style // the status line's hint, while tooltips float
	hint        lipgloss.Style

	healthy  lipgloss.Style
	degraded lipgloss.Style
	failed   lipgloss.Style
	rolling  lipgloss.Style

	// failedTip is the tooltip of a failed deploy, which stands out from
	// the others.
	failedTip lipgloss.Style
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a failed deploy, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	bar := f.On(roles.Surface) // the roles for drawing on the status line

	tool := f.Fill(roles.Surface).Padding(0, 1)
	statusBar := bar.Fill(roles.Surface).Padding(0, 1)
	return styles{
		fit: f,

		page:        lipgloss.NewStyle().Padding(1, 2, 0),
		title:       f.Text(roles.Accent).Bold(true),
		text:        f.Text(roles.Fg),
		label:       f.Text(roles.Muted),
		header:      f.Text(roles.Muted).Bold(true),
		marker:      f.Text(roles.Accent).Bold(true),
		tool:        tool,
		toolHover:   f.Fill(roles.Hover).Padding(0, 1),
		toolOn:      f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		statusBar:   statusBar,
		statusMuted: statusBar.Foreground(bar.Color(roles.Muted)),
		hint:        statusBar.Foreground(bar.Color(roles.Muted)).Padding(0, 1, 0, 0),

		healthy:  f.Text(roles.Success),
		degraded: f.Text(roles.Warning),
		failed:   f.Text(roles.Danger),
		rolling:  f.Text(roles.Info),

		failedTip: f.Fill(roles.Danger).Padding(0, 1),
	}
}
