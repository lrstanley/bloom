// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
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

	panel    lipgloss.Style // a rounded frame
	item     lipgloss.Style // a file
	hovered  lipgloss.Style // a file under the pointer
	selected lipgloss.Style // the selected file
	selHover lipgloss.Style // the selected file, under the pointer
	status   lipgloss.Style // the status line

	menuItem        lipgloss.Style // an item of a menu
	menuHover       lipgloss.Style // an item of a menu under the pointer
	menuDanger      lipgloss.Style // an item of a menu that destroys something
	menuDangerHover lipgloss.Style // an item that destroys something, under the pointer
	menuRule        lipgloss.Style // a rule between items of a menu
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as a menu or the row under the
// pointer, from the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	// A menu has no border, so its overlay background sets it apart from
	// the list.
	menu := f.On(roles.Overlay) // the roles for drawing on a menu

	menuItem := menu.Fill(roles.Overlay).Padding(0, 1).Width(menuWidth)
	menuHover := menu.Fill(roles.Accent).Padding(0, 1).Width(menuWidth)
	return styles{
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		item:     f.Text(roles.Fg).Padding(0, 1),
		hovered:  f.Fill(roles.Bg.Hover()).Padding(0, 1),
		selected: f.Fill(roles.Bg.Selected()).Bold(true).Padding(0, 1),
		selHover: f.Fill(roles.Bg.Selected().Hover()).Bold(true).Padding(0, 1),
		status:   f.Text(roles.Muted),

		menuItem:        menuItem,
		menuHover:       menuHover,
		menuDanger:      menuItem.Foreground(menu.Color(roles.Danger)),
		menuDangerHover: menu.Fill(roles.Danger).Padding(0, 1).Width(menuWidth),
		menuRule:        menuItem.Foreground(menu.Color(roles.Border)),
	}
}
