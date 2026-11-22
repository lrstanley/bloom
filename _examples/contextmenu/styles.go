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
	item     lipgloss.Style // a file
	hovered  lipgloss.Style // a file under the pointer
	selected lipgloss.Style // the selected file
	status   lipgloss.Style // the status line

	menuItem        lipgloss.Style // an item of a menu
	menuHover       lipgloss.Style // an item of a menu under the pointer
	menuDanger      lipgloss.Style // an item of a menu that destroys something
	menuDangerHover lipgloss.Style // an item that destroys something, under the pointer
	menuRule        lipgloss.Style // a rule between items of a menu
}

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	fg, muted, accent, danger, dark := t.Fg, t.BrightBlack, t.Blue, t.Red, t.Bg
	raised := lipgloss.Lighten(t.Bg, 0.08)
	// A menu has no border, so its background sets it apart from the list.
	menuBg := lipgloss.Lighten(t.Bg, 0.14)

	item := lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	menuItem := item.Background(menuBg).Width(menuWidth)
	menuHover := menuItem.Foreground(dark).Background(accent)
	return styles{
		theme:    t,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		item:     item,
		hovered:  item.Background(raised),
		selected: item.Foreground(dark).Background(accent).Bold(true),
		status:   lipgloss.NewStyle().Foreground(muted),

		menuItem:        menuItem,
		menuHover:       menuHover,
		menuDanger:      menuItem.Foreground(danger),
		menuDangerHover: menuHover.Background(danger),
		menuRule:        menuItem.Foreground(muted),
	}
}
