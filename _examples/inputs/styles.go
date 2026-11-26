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

	panel   lipgloss.Style // the frame of a box, padded
	focused lipgloss.Style // the frame of the box with focus
	hint    lipgloss.Style // the line of help under the boxes
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as a border or the focused box, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 1)
	return styles{
		fit:     f,
		panel:   panel,
		focused: panel.BorderForeground(f.Color(roles.Accent)),
		hint:    f.Text(roles.Subtle).Padding(0, 1),
	}
}
