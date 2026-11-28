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
// tint by newStyles. The model keeps them, so that nothing is a package
// global.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	panel    lipgloss.Style // a rounded frame
	heading  lipgloss.Style // the title at the top
	item     lipgloss.Style // an item of the list
	selected lipgloss.Style // the selected item
	marker   lipgloss.Style // the marker that follows the selected item
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as a border or the selected item, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	return styles{
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		heading:  f.Text(roles.Accent).Bold(true),
		item:     f.Text(roles.Fg).Padding(0, 1),
		selected: f.Fill(roles.Bg.Selected()).Padding(0, 1),
		marker:   f.Text(roles.Accent2).Bold(true),
	}
}
