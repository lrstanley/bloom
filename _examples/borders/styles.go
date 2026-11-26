// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

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

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a border, from the
// tint, so the example reads on any tint, dark or light. Pink is the tint's
// magenta, which the example uses for its own sake.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	accent, pink := f.Color(roles.Accent), f.Color(roles.Magenta)
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border))
	tab := f.Text(roles.Muted)
	thumb := f.Text(roles.Accent)
	icon := f.Text(roles.Fg)
	return styles{
		fit:    f,
		accent: accent,
		pink:   pink,
		warn:   f.Color(roles.Warning),
		good:   f.Color(roles.Success),
		bad:    f.Color(roles.Danger),

		panel: panel,
		pane:  panel.Padding(0, 1),
		item:  f.Text(roles.Fg).Padding(0, 1),
		text:  f.Text(roles.Fg),
		title: f.Text(roles.Fg).Bold(true).Padding(0, 1),
		label: f.Text(roles.Muted).Padding(0, 1),
		tab:   tab,
		onTab: tab.Foreground(accent).Bold(true),
		icon:  icon,
		onTip: icon.Foreground(accent).Bold(true),
		side:  f.Text(roles.Magenta).Bold(true),

		scrollbar: bloom.ScrollbarStyle{ThumbStyle: thumb},
		onBar:     bloom.ScrollbarStyle{ThumbStyle: thumb.Foreground(pink)},
	}
}
