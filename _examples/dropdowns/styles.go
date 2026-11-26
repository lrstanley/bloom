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

// Roles the example adds to bubbletint's: the focused field, and a neutral
// button.
var (
	// fieldOn is the focused field: lifted from the other fields toward the
	// text, so the one in use stands out.
	fieldOn = roles.Define(roles.Spec{
		Name: "field-on",
		Kind: roles.KindSurface,
		Derive: func(p *roles.Palette) tint.Color {
			return p.Color(roles.Sunken).Mix(p.Color(roles.Fg), p.ByDark(0.16, 0.12))
		},
		Distinct: []roles.Role{roles.Sunken},
		Fallback: roles.Fallback{Slots: []tint.Slot{tint.SlotBrightBlack}},
	})
	// control is a neutral button: well clear of the background it sits on,
	// while its text still reads.
	control = roles.Define(roles.Spec{
		Name: "control",
		Kind: roles.KindSurface,
		Derive: func(p *roles.Palette) tint.Color {
			fg := p.Color(roles.Fg)
			return p.Color(roles.Bg).MixReadable(fg, fg, p.ByDark(0.3, 0.2), 4.5)
		},
		Distinct: []roles.Role{roles.Bg},
		Fallback: roles.Fallback{Slots: []tint.Slot{tint.SlotBrightBlack}},
	})
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	heading  lipgloss.Style
	text     lipgloss.Style
	label    lipgloss.Style
	panel    lipgloss.Style
	dialog   lipgloss.Style
	input    lipgloss.Style
	inputOn  lipgloss.Style
	button   lipgloss.Style
	buttonOn lipgloss.Style
	status   lipgloss.Style
	backdrop fx.Dim

	// A list's border joins its field's sides, and the edge facing the
	// field is left open: listBelow is a list that opens below its field,
	// and listAbove one that opens above it.
	listBelow    lipgloss.Style
	listAbove    lipgloss.Style
	option       lipgloss.Style
	optionPicked lipgloss.Style
	optionCursor lipgloss.Style
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as a sunken field or a floating list,
// from the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	overlay := f.On(roles.Overlay) // the roles for drawing on a list

	button := f.Fill(control).Padding(0, 2)
	list := overlay.Fill(roles.Overlay).Border(lipgloss.NormalBorder()).
		BorderForeground(overlay.Color(roles.Accent)).BorderBackground(overlay.Color(roles.Overlay))
	option := overlay.Fill(roles.Overlay).Padding(0, 1)
	return styles{
		fit: f,

		heading:  f.Text(roles.Accent).Bold(true),
		text:     f.Text(roles.Fg),
		label:    f.Text(roles.Muted),
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(1, 2),
		dialog:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Accent)).Padding(1, 2),
		input:    f.Fill(roles.Sunken).Padding(0, 1),
		inputOn:  f.Fill(fieldOn).Padding(0, 1),
		button:   button,
		buttonOn: f.Fill(roles.Accent).Bold(true).Padding(0, 2),
		status:   f.Text(roles.Muted),
		backdrop: fx.Dim{Amount: 0.5},

		listBelow:    list.BorderTop(false),
		listAbove:    list.BorderBottom(false),
		option:       option,
		optionPicked: overlay.Text(roles.Accent).Bold(true).Padding(0, 1),
		optionCursor: overlay.Fill(roles.Accent).Padding(0, 1),
	}
}
