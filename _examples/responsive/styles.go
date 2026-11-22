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

	panel    lipgloss.Style // a rounded frame
	pane     lipgloss.Style // the frame of the reader and the details, padded
	bar      lipgloss.Style // the toolbar and the status bar
	title    lipgloss.Style // the toolbar's title
	action   lipgloss.Style // an action on the toolbar
	item     lipgloss.Style // an article in the list
	selected lipgloss.Style // the article open
	prose    lipgloss.Style // the reader's text
	chip     lipgloss.Style // a tag
	bold     lipgloss.Style // the article's title, in the details
	note     lipgloss.Style // quieter text, in the details
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, text, accent, muted, dark := t.Fg, t.White, t.Yellow, t.BrightBlack, t.Bg
	barBg := lipgloss.Lighten(t.Bg, 0.05)

	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	bar := lipgloss.NewStyle().Foreground(fg).Background(barBg)
	item := lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	return styles{
		theme:    t,
		panel:    panel,
		pane:     panel.Padding(0, 1),
		bar:      bar,
		title:    bar.Bold(true).Foreground(accent).Padding(0, 1),
		action:   bar.Foreground(text).Padding(0, 1),
		item:     item,
		selected: item.Foreground(dark).Background(accent).Bold(true),
		prose:    lipgloss.NewStyle().Foreground(fg),
		chip:     lipgloss.NewStyle().Foreground(dark).Background(t.Purple).Padding(0, 1),
		bold:     lipgloss.NewStyle().Bold(true),
		note:     lipgloss.NewStyle().Foreground(muted),
	}
}
