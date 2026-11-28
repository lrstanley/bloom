// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
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

	// On the screen's background.
	panel    lipgloss.Style
	focused  lipgloss.Style
	header   lipgloss.Style
	item     lipgloss.Style
	selected lipgloss.Style
	label    lipgloss.Style
	spark    lipgloss.Style
	logText  lipgloss.Style

	// On the raised background of the status bar, dialog, and toast.
	status      lipgloss.Style
	dialog      lipgloss.Style
	dialogTitle lipgloss.Style
	dialogText  lipgloss.Style
	button      lipgloss.Style
	buttonHover lipgloss.Style
	active      lipgloss.Style
	activeHover lipgloss.Style
	toast       lipgloss.Style

	// The escape sequences that start the table's dim and green text. The
	// table colors its rows with these rather than with styles, as there are
	// many of them, redrawn every frame.
	tableDim  string
	tableGood string
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised dialog, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	raised := f.On(roles.Raised) // the roles for drawing on the status bar, dialog, and toast

	onBg := f.Fill(roles.Bg)
	panel := onBg.Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border))
	item := onBg.Padding(0, 1)

	onPanel := raised.Fill(roles.Raised)

	return styles{
		fit: f,

		panel:    panel,
		focused:  panel.BorderForeground(f.Color(roles.Accent)),
		header:   f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		item:     item,
		selected: f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		label:    onBg.Foreground(f.Color(roles.Muted)),
		spark:    onBg.Foreground(f.Color(roles.Success)),
		logText:  onBg.AlignVertical(lipgloss.Bottom),

		status:      onPanel.Padding(0, 1),
		dialog:      onPanel.Border(lipgloss.ThickBorder()).BorderForeground(raised.Color(roles.Warning)).Padding(1, 2),
		dialogTitle: onPanel.Bold(true),
		dialogText:  raised.Text(roles.Muted),
		button:      raised.Fill(roles.Control).Padding(0, 2),
		buttonHover: raised.Fill(roles.Control.Hover()).Padding(0, 2),
		active:      raised.Fill(roles.Warning).Bold(true).Padding(0, 2),
		activeHover: raised.Fill(roles.Warning.Hover()).Bold(true).Padding(0, 2),
		toast:       onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(raised.Color(roles.Success)).Padding(0, 1),

		tableDim:  ansi.NewStyle().ForegroundColor(f.Color(roles.Muted)).String(),
		tableGood: ansi.NewStyle().ForegroundColor(f.Color(roles.Success)).String(),
	}
}
