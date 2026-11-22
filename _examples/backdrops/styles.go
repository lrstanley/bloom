// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
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

	card     lipgloss.Style // the frame of the panel
	title    lipgloss.Style // the panel's and the dialog's titles
	text     lipgloss.Style // what a backdrop does, and blank lines
	dim      lipgloss.Style // notes
	label    lipgloss.Style // the name before a row of tabs
	item     lipgloss.Style // a tab
	selected lipgloss.Style // the screen's backdrop, in its tabs
	picked   lipgloss.Style // the dialog's backdrop, in its tabs
	hover    lipgloss.Style // a tab under the pointer
	status   lipgloss.Style // the line of keys at the bottom
	dialog   lipgloss.Style // the frame of the dialog
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	surface := lipgloss.Darken(t.Bg, 0.25)
	muted := t.BrightBlack

	dim := lipgloss.NewStyle().Foreground(muted).Background(surface)
	item := lipgloss.NewStyle().Foreground(t.Fg).Background(surface).Padding(0, 1)
	return styles{
		theme:    t,
		card:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).BorderBackground(surface).Background(surface).Padding(1, 3),
		title:    lipgloss.NewStyle().Bold(true).Foreground(t.Purple).Background(surface),
		text:     lipgloss.NewStyle().Foreground(t.Fg).Background(surface),
		dim:      dim,
		label:    dim.Width(8),
		item:     item,
		selected: item.Foreground(t.Bg).Background(t.Cyan).Bold(true),
		picked:   item.Foreground(t.Bg).Background(t.Purple).Bold(true),
		hover:    item.Background(lipgloss.Lighten(surface, 0.12)),
		status:   lipgloss.NewStyle().Foreground(muted),
		dialog:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Purple).BorderBackground(surface).Background(surface).Padding(1, 4),
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
