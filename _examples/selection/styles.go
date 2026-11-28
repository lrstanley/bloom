// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
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
	dialogText  lipgloss.Style // the dialog's note, inside it
	token       lipgloss.Style // the token in the dialog

	levels map[string]lipgloss.Style // the levels of the log, by name
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised dialog, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	raised := f.On(roles.Raised) // the roles for drawing on the dialog

	title := f.Text(roles.Accent).Bold(true).Padding(0, 1)
	value := f.Text(roles.Fg)
	status := f.Text(roles.Muted)
	return styles{
		fit:       f,
		panel:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 1),
		title:     title,
		label:     f.Text(roles.Muted),
		value:     value,
		caret:     value.Reverse(true),
		status:    status,
		message:   f.Text(roles.Fg),
		selection: lipgloss.NewStyle().Background(f.Color(roles.Bg.Selected())),
		dialog: raised.Fill(roles.Raised).Border(lipgloss.RoundedBorder()).
			BorderForeground(raised.Color(roles.Accent)).Padding(1, 2),
		dialogTitle: raised.Text(roles.Accent).Bold(true),
		dialogText:  raised.Text(roles.Muted),
		token:       raised.Text(roles.Success).Bold(true),
		levels: map[string]lipgloss.Style{
			info:  f.Text(roles.Info),
			warn:  f.Text(roles.Warning),
			fail:  f.Text(roles.Danger),
			debug: f.Text(roles.Muted),
		},
	}
}

// pane frames body, with title on the top of its frame.
func (st *styles) pane(title string, body bloom.Child) bloom.Child {
	return bloom.Box(st.panel, body).Edge(bloom.Top, bloom.Start, bloom.Text(title).Style(st.title))
}
