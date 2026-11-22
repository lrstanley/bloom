// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
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

	panel     lipgloss.Style // a pane's rounded frame, padded
	title     lipgloss.Style // a pane's title, on its frame
	label     lipgloss.Style // a field's name, a timestamp, and the gutter
	value     lipgloss.Style // a field's value, and the text of the log
	caret     lipgloss.Style // the character under the note's caret
	status    lipgloss.Style // the keys, under the panes
	message   lipgloss.Style // the status line, above the keys
	selection lipgloss.Style // what is selected, wherever it is

	dialog      lipgloss.Style // the dialog's frame
	dialogTitle lipgloss.Style // the dialog's title, inside it
	token       lipgloss.Style // the token in the dialog

	levels map[string]lipgloss.Style // the levels of the log, by name
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent := t.Fg, t.BrightBlack, t.Blue
	title := lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 1)
	value := lipgloss.NewStyle().Foreground(fg)
	status := lipgloss.NewStyle().Foreground(muted)
	return styles{
		theme:     t,
		panel:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1),
		title:     title,
		label:     lipgloss.NewStyle().Foreground(muted),
		value:     value,
		caret:     value.Reverse(true),
		status:    status,
		message:   status.Foreground(fg),
		selection: lipgloss.NewStyle().Background(t.SelectionBg),
		dialog: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).
			Padding(1, 2).Background(lipgloss.Lighten(t.Bg, 0.06)),
		dialogTitle: title.Padding(0),
		token:       lipgloss.NewStyle().Foreground(t.Green).Bold(true),
		levels: map[string]lipgloss.Style{
			info:  lipgloss.NewStyle().Foreground(t.Blue),
			warn:  lipgloss.NewStyle().Foreground(t.Yellow),
			fail:  lipgloss.NewStyle().Foreground(t.Red),
			debug: lipgloss.NewStyle().Foreground(muted),
		},
	}
}

// pane frames body, with title on the top of its frame.
func (st *styles) pane(title string, body bloom.Child) bloom.Child {
	return bloom.Box(st.panel, body).Edge(bloom.Top, bloom.Start, bloom.Text(title).Style(st.title))
}
