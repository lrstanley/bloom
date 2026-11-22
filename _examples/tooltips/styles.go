// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	theme *tint.Tint // the tint the styles are made from

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

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg := t.Fg
	muted := t.BrightBlack
	accent := t.Purple
	dark := t.Bg
	button := lipgloss.Lighten(t.Bg, 0.1)

	tool := lipgloss.NewStyle().Foreground(fg).Background(button).Padding(0, 1)
	statusBar := lipgloss.NewStyle().Foreground(fg).Background(lipgloss.Lighten(t.Bg, 0.05)).Padding(0, 1)
	return styles{
		theme: t,

		page:        lipgloss.NewStyle().Padding(1, 2, 0),
		title:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		text:        lipgloss.NewStyle().Foreground(fg),
		label:       lipgloss.NewStyle().Foreground(muted),
		header:      lipgloss.NewStyle().Foreground(muted).Bold(true),
		marker:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		tool:        tool,
		toolHover:   tool.Background(lipgloss.Lighten(t.Bg, 0.2)),
		toolOn:      tool.Foreground(dark).Background(accent).Bold(true),
		statusBar:   statusBar,
		statusMuted: statusBar.Foreground(muted),
		hint:        statusBar.Foreground(muted).Padding(0, 1, 0, 0),

		healthy:  lipgloss.NewStyle().Foreground(t.Green),
		degraded: lipgloss.NewStyle().Foreground(t.Yellow),
		failed:   lipgloss.NewStyle().Foreground(t.Red),
		rolling:  lipgloss.NewStyle().Foreground(t.Blue),

		failedTip: lipgloss.NewStyle().Foreground(dark).Background(t.Red).Padding(0, 1),
	}
}
