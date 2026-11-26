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

	card    lipgloss.Style // the frame of a card
	name    lipgloss.Style // a card's name, and a dialog's title
	rowEven lipgloss.Style // the even rows of a list, from the first
	rowOdd  lipgloss.Style // the odd rows of a list
	panel   lipgloss.Style // the bordered panel a card transitions
	status  lipgloss.Style // quieter text, such as the status line
	button  lipgloss.Style // a key that opens a dialog
	dialog  lipgloss.Style // the frame of a dialog
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised panel, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})

	rowEven := f.Fill(roles.Surface)
	return styles{
		fit:     f,
		card:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		name:    f.Text(roles.Accent).Bold(true),
		rowEven: rowEven,
		rowOdd:  f.Fill(roles.Raised),
		panel:   rowEven.Border(lipgloss.NormalBorder()).BorderForeground(f.On(roles.Surface).Color(roles.Accent2)).Padding(0, 1),
		status:  f.Text(roles.Muted),
		button:  f.Fill(roles.Accent).Padding(0, 1),
		dialog:  lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(f.Color(roles.Accent)).Padding(1, 3),
	}
}
