// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

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

	// The colors the border effects are drawn in.
	accent color.Color
	pink   color.Color
	warn   color.Color
	good   color.Color
	bad    color.Color

	panel lipgloss.Style // a rounded frame
	pane  lipgloss.Style // the frame of the list and the log, padded
	item  lipgloss.Style // the text inside an effect's box
	text  lipgloss.Style // the lines of the list and the log
	title lipgloss.Style // a box's name, on its top edge
	label lipgloss.Style // quieter text, such as the status line
	tab   lipgloss.Style // a filter tab
	onTab lipgloss.Style // the filter tab chosen, or under the pointer
	icon  lipgloss.Style // an icon down the list's side
	onTip lipgloss.Style // an icon under the pointer
	side  lipgloss.Style // the log's name down its side

	scrollbar bloom.ScrollbarStyle // a scrollbar's thumb
	onBar     bloom.ScrollbarStyle // a scrollbar's thumb under the pointer
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, pink := t.Fg, t.BrightBlack, t.Blue, t.Purple
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	tab := lipgloss.NewStyle().Foreground(muted)
	thumb := lipgloss.NewStyle().Foreground(accent)
	icon := lipgloss.NewStyle().Foreground(fg)
	return styles{
		theme:  t,
		accent: accent,
		pink:   pink,
		warn:   t.Yellow,
		good:   t.Green,
		bad:    t.Red,

		panel: panel,
		pane:  panel.Padding(0, 1),
		item:  lipgloss.NewStyle().Foreground(fg).Padding(0, 1),
		text:  lipgloss.NewStyle().Foreground(fg),
		title: lipgloss.NewStyle().Foreground(fg).Bold(true).Padding(0, 1),
		label: lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
		tab:   tab,
		onTab: tab.Foreground(accent).Bold(true),
		icon:  icon,
		onTip: icon.Foreground(t.Yellow).Bold(true),
		side:  lipgloss.NewStyle().Foreground(pink).Bold(true),

		scrollbar: bloom.ScrollbarStyle{ThumbStyle: thumb},
		onBar:     bloom.ScrollbarStyle{ThumbStyle: thumb.Foreground(pink)},
	}
}
