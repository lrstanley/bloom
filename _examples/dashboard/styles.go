// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	theme *tint.Tint // the bubbletint tint the example's colors come from

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
	active      lipgloss.Style
	toast       lipgloss.Style

	// The escape sequences that start the table's dim and green text. The
	// table colors its rows with these rather than with styles, as there are
	// many of them, redrawn every frame.
	tableDim  string
	tableGood string
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	var (
		fg      = t.Fg
		text    = t.White
		accent  = t.Blue
		muted   = t.BrightBlack
		good    = t.Green
		warn    = t.Yellow
		bg      = t.Bg
		panelBg = lipgloss.Lighten(t.Bg, 0.05)
	)

	onBg := lipgloss.NewStyle().Background(bg)
	panel := onBg.Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	item := onBg.Foreground(text).Padding(0, 1)

	onPanel := lipgloss.NewStyle().Background(panelBg)
	button := onPanel.Foreground(fg).Background(muted).Padding(0, 2)

	return styles{
		theme: t,

		panel:    panel,
		focused:  panel.BorderForeground(accent),
		header:   onBg.Foreground(bg).Background(accent).Bold(true).Padding(0, 1),
		item:     item,
		selected: item.Foreground(bg).Background(accent).Bold(true),
		label:    onBg.Foreground(muted),
		spark:    onBg.Foreground(good),
		logText:  onBg.Foreground(text).AlignVertical(lipgloss.Bottom),

		status:      onPanel.Foreground(text).Padding(0, 1),
		dialog:      onPanel.Border(lipgloss.ThickBorder()).BorderForeground(warn).Padding(1, 2),
		dialogTitle: onPanel.Bold(true),
		dialogText:  onPanel.Foreground(muted),
		button:      button,
		active:      button.Foreground(bg).Background(warn).Bold(true),
		toast:       onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(good).Padding(0, 1),

		tableDim:  ansi.NewStyle().ForegroundColor(muted).String(),
		tableGood: ansi.NewStyle().ForegroundColor(good).String(),
	}
}
