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
	page     lipgloss.Style // the frame of the reader, padded
	item     lipgloss.Style // a sidebar item
	selected lipgloss.Style // the sidebar item of the page open
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as a border or the open page's item,
// from the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border))
	item := lipgloss.NewStyle().Padding(0, 1)
	return styles{
		fit:      f,
		panel:    panel,
		page:     panel.Padding(0, 1),
		item:     item,
		selected: f.Fill(roles.Accent).Padding(0, 1),
	}
}
