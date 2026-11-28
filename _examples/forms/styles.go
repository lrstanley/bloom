// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// fieldOn is the focused input, a role the example adds to bubbletint's:
// lifted from the other inputs toward the text, so the one being typed in
// stands out.
var fieldOn = roles.Define(roles.Spec{
	Name: "field-on",
	Kind: roles.KindSurface,
	Derive: func(p *roles.Palette) tint.Color {
		return p.Color(roles.Sunken).Mix(p.Color(roles.Fg), p.ByDark(0.16, 0.12))
	},
	Distinct: []roles.Role{roles.Sunken},
	Fallback: roles.Fallback{Slots: []tint.Slot{tint.SlotBrightBlack}},
})

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	panel         lipgloss.Style
	heading       lipgloss.Style
	headingWarn   lipgloss.Style // the heading of the overwrite dialog
	label         lipgloss.Style
	input         lipgloss.Style
	inputOn       lipgloss.Style
	inputOnHint   lipgloss.Style // the focused input, while it shows its placeholder
	hint          lipgloss.Style
	button        lipgloss.Style
	buttonHover   lipgloss.Style
	buttonOn      lipgloss.Style
	buttonOnHover lipgloss.Style // the focused button, under the pointer
	dropdown      lipgloss.Style
	option        lipgloss.Style
	dialog        lipgloss.Style
	danger        lipgloss.Style
	toastBox      lipgloss.Style
	statusBar     lipgloss.Style
	backdrop      fx.Recolor
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a sunken input, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	sunken := f.On(roles.Sunken)   // the roles for drawing in an input
	focused := f.On(fieldOn)       // the roles for drawing in the focused input
	overlay := f.On(roles.Overlay) // the roles for drawing on the dropdown

	rounded := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	heading := f.Text(roles.Accent).Bold(true)
	input := f.Fill(roles.Sunken).Padding(0, 1)
	inputOn := f.Fill(fieldOn).Padding(0, 1)
	dialog := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(f.Color(roles.Accent)).Padding(1, 3)
	return styles{
		fit: f,

		panel:         rounded.BorderForeground(f.Color(roles.Border)).Padding(1, 2),
		heading:       heading,
		headingWarn:   f.Text(roles.Danger).Bold(true),
		label:         f.Text(roles.Muted),
		input:         input,
		inputOn:       inputOn,
		inputOnHint:   focused.Text(roles.Subtle).Padding(0, 1),
		hint:          sunken.Text(roles.Subtle).Padding(0, 1),
		button:        f.Fill(roles.Control).Padding(0, 2),
		buttonHover:   f.Fill(roles.Control.Hover()).Padding(0, 2),
		buttonOn:      f.Fill(roles.Accent).Bold(true).Padding(0, 2),
		buttonOnHover: f.Fill(roles.Accent.Hover()).Bold(true).Padding(0, 2),
		dropdown:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(overlay.Color(roles.Accent)).BorderBackground(overlay.Color(roles.Overlay)),
		option:        f.Fill(roles.Overlay).Padding(0, 1),
		dialog:        dialog,
		danger:        dialog.BorderForeground(f.Color(roles.Danger)),
		toastBox:      rounded.BorderForeground(f.Color(roles.Success)).Padding(0, 1),
		statusBar:     f.Text(roles.Muted),
		backdrop:      fx.Recolor{Fg: f.Color(roles.Border), Bg: f.Color(roles.Sunken)},
	}
}
