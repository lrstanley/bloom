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

	card          lipgloss.Style
	heading       lipgloss.Style
	label         lipgloss.Style
	big           lipgloss.Style
	input         lipgloss.Style
	inputOn       lipgloss.Style
	inputOnHint   lipgloss.Style // the focused input, while it shows its placeholder
	hint          lipgloss.Style
	button        lipgloss.Style // the deploy button while it isn't focused
	buttonHover   lipgloss.Style
	buttonOn      lipgloss.Style
	buttonOnHover lipgloss.Style // the focused deploy button, under the pointer
	staged        lipgloss.Style
	live          lipgloss.Style
	failure       lipgloss.Style
	success       lipgloss.Style
	tour          lipgloss.Style
	status        lipgloss.Style
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or an input's well, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	// Both inputs are sunk into the background, and the focused one is
	// selected too, tinted toward the accent, so that it stands out.
	sunken, selected := f.On(roles.Sunken), f.On(roles.Sunken.Selected())

	input := f.Fill(roles.Sunken).Padding(0, 1)
	inputOn := f.Fill(roles.Sunken.Selected()).Padding(0, 1)
	return styles{
		fit: f,

		card:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 2),
		heading:       f.Text(roles.Accent).Bold(true),
		label:         f.Text(roles.Muted),
		big:           f.Text(roles.Fg).Bold(true),
		input:         input,
		inputOn:       inputOn,
		inputOnHint:   selected.Text(roles.Subtle).Padding(0, 1),
		hint:          sunken.Text(roles.Subtle).Padding(0, 1),
		button:        f.Fill(roles.Control).Padding(0, 2),
		buttonHover:   f.Fill(roles.Control.Hover()).Padding(0, 2),
		buttonOn:      f.Fill(roles.Accent).Bold(true).Padding(0, 2),
		buttonOnHover: f.Fill(roles.Accent.Hover()).Bold(true).Padding(0, 2),
		staged:        f.Text(roles.Muted).Padding(0, 1),
		live:          f.Fill(roles.Success).Bold(true).Padding(0, 1),
		failure:       f.Text(roles.Danger),
		success:       f.Text(roles.Success),
		tour:          lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(f.Color(roles.Accent)).Padding(0, 2),
		status:        f.Text(roles.Muted),
	}
}
