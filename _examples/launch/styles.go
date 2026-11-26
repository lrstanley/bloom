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

// control is the deploy button while it isn't focused: a role the example
// adds to bubbletint's, well clear of the background it sits on, while its
// text still reads.
var control = roles.Define(roles.Spec{
	Name: "control",
	Kind: roles.KindSurface,
	Derive: func(p *roles.Palette) tint.Color {
		fg := p.Color(roles.Fg)
		return p.Color(roles.Bg).MixReadable(fg, fg, p.ByDark(0.22, 0.17), 4.5)
	},
	Distinct: []roles.Role{roles.Bg},
	Fallback: roles.Fallback{Slots: []tint.Slot{tint.SlotBrightBlack}},
})

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	card        lipgloss.Style
	heading     lipgloss.Style
	label       lipgloss.Style
	big         lipgloss.Style
	input       lipgloss.Style
	inputOn     lipgloss.Style
	inputOnHint lipgloss.Style // the focused input, while it shows its placeholder
	hint        lipgloss.Style
	button      lipgloss.Style
	buttonOn    lipgloss.Style
	staged      lipgloss.Style
	live        lipgloss.Style
	failure     lipgloss.Style
	success     lipgloss.Style
	tour        lipgloss.Style
	status      lipgloss.Style
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or an input's well, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	// The focused input takes the selection's highlight, so that it stands
	// out from the unfocused one, sunk into the background.
	sunken, selection := f.On(roles.Sunken), f.On(roles.Selection)

	input := sunken.Fill(roles.Sunken).Padding(0, 1)
	inputOn := selection.Fill(roles.Selection).Padding(0, 1)
	return styles{
		fit: f,

		card:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)).Padding(0, 2),
		heading:     f.Text(roles.Accent).Bold(true),
		label:       f.Text(roles.Muted),
		big:         f.Text(roles.Fg).Bold(true),
		input:       input,
		inputOn:     inputOn,
		inputOnHint: selection.Text(roles.Subtle).Padding(0, 1),
		hint:        sunken.Text(roles.Subtle).Padding(0, 1),
		button:      f.Fill(control).Padding(0, 2),
		buttonOn:    f.Fill(roles.Accent).Bold(true).Padding(0, 2),
		staged:      f.Text(roles.Muted).Padding(0, 1),
		live:        f.Fill(roles.Success).Bold(true).Padding(0, 1),
		failure:     f.Text(roles.Danger),
		success:     f.Text(roles.Success),
		tour:        lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(f.Color(roles.Accent)).Padding(0, 2),
		status:      f.Text(roles.Muted),
	}
}
