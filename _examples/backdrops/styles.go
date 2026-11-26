// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
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

	card     lipgloss.Style // the frame of the panel
	title    lipgloss.Style // the panel's title
	text     lipgloss.Style // what a backdrop does, and blank lines
	dim      lipgloss.Style // notes
	label    lipgloss.Style // the name before a row of tabs
	item     lipgloss.Style // a tab
	selected lipgloss.Style // the screen's backdrop, in its tabs
	picked   lipgloss.Style // the dialog's backdrop, in its tabs
	hover    lipgloss.Style // a tab under the pointer
	status   lipgloss.Style // the line of keys at the bottom

	dialog      lipgloss.Style // the frame of the dialog
	dialogTitle lipgloss.Style // the dialog's title
	dialogText  lipgloss.Style // what the dialog's backdrop does, and blank lines
	dialogDim   lipgloss.Style // the dialog's notes
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised dialog, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	panel := f.On(roles.Surface) // the roles for drawing on the panel
	raised := f.On(roles.Raised) // the roles for drawing on the dialog

	onPanel := panel.Fill(roles.Surface)
	onDialog := raised.Fill(roles.Raised)
	dim := panel.Text(roles.Muted)
	item := onPanel.Padding(0, 1)
	return styles{
		fit:      f,
		card:     onPanel.Border(lipgloss.RoundedBorder()).BorderForeground(panel.Color(roles.Border)).BorderBackground(panel.Color(roles.Surface)).Padding(1, 3),
		title:    panel.Text(roles.Accent).Bold(true),
		text:     onPanel,
		dim:      dim,
		label:    dim.Width(8),
		item:     item,
		selected: panel.Fill(roles.Accent2).Bold(true).Padding(0, 1),
		picked:   panel.Fill(roles.Accent).Bold(true).Padding(0, 1),
		hover:    item.Background(panel.Color(roles.Hover)),
		status:   f.Text(roles.Muted),

		dialog:      onDialog.Border(lipgloss.RoundedBorder()).BorderForeground(raised.Color(roles.Accent)).BorderBackground(raised.Color(roles.Raised)).Padding(1, 4),
		dialogTitle: raised.Text(roles.Accent).Bold(true),
		dialogText:  onDialog,
		dialogDim:   raised.Text(roles.Muted),
	}
}

// tabs is a row of clickable backdrop names after name, with current drawn
// in style.
func (st *styles) tabs(name string, list []backdrop, current int, style lipgloss.Style, pick func(i int) tea.Cmd) bloom.Child {
	items := make([]bloom.Child, 0, len(list)+1)
	items = append(items, bloom.Text(name).Style(st.label))
	for i, d := range list {
		s := st.item
		if i == current {
			s = style
		}
		items = append(items, bloom.Text(d.name).Style(s).HoverStyle(st.hover).On(on.Click(func() tea.Cmd { return pick(i) })))
	}
	return bloom.Row(items...).Gap(1)
}
