// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles. The model keeps them, so that nothing is a package
// global.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	panel   lipgloss.Style // the frame around the world and the sidebar
	title   lipgloss.Style // the sidebar's title
	heading lipgloss.Style // a heading in the sidebar
	item    lipgloss.Style // a material to pick
	picked  lipgloss.Style // the material picked
	hovered lipgloss.Style // a material under the pointer
	label   lipgloss.Style // a meter's name
	value   lipgloss.Style // a meter's value
	muted   lipgloss.Style // notes in the sidebar
	status  lipgloss.Style // the status line
	ghost   color.Color    // the brush's outline over the world
	ramp    []color.Color  // the frame time graph's gradient, fast to slow

	// swatches are a sample of each material's color, one for each of sand's
	// hues, and one for each other material.
	swatches [steam + 1][]lipgloss.Style
}

// swatch returns the sample of material m's color, for sand in hue.
func (st *styles) swatch(m material, hue int) lipgloss.Style {
	s := st.swatches[m]
	if len(s) == 0 {
		return st.label
	}
	return s[hue%len(s)]
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or the row under the
// pointer, from the tint, so the example reads on any tint, dark or light.
// The materials keep their own colors, from palette.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	item := f.Text(roles.Fg).Padding(0, 1)
	var swatches [steam + 1][]lipgloss.Style
	for m, colors := range palette {
		step := 1
		if material(m) == sand {
			step = sandShades // the middle shade of each hue
		}
		for i := step / 2; i < len(colors); i += step {
			swatches[m] = append(swatches[m], lipgloss.NewStyle().Foreground(colors[i]))
		}
	}
	return styles{
		swatches: swatches,
		fit:      f,
		panel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		title:    f.Text(roles.Accent).Bold(true),
		heading:  f.Text(roles.Accent2).Bold(true).MarginTop(1),
		item:     item,
		picked:   f.Fill(roles.Accent).Bold(true).Padding(0, 1),
		hovered:  f.Fill(roles.Hover).Padding(0, 1),
		label:    f.Text(roles.Muted),
		value:    f.Text(roles.Fg),
		muted:    f.Text(roles.Muted).Italic(true),
		status:   f.Text(roles.Muted),
		ghost:    f.Color(roles.BorderStrong),
		ramp:     heat(64),
	}
}
