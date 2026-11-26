// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
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

	frame  lipgloss.Style // the frame of a card
	name   lipgloss.Style // the name of a card's effect
	sample lipgloss.Style // the text the effects run on
	status lipgloss.Style // the status line
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a raised panel, from
// the tint, so the example reads on any tint, dark or light.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	return styles{
		fit:    f,
		frame:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Border)),
		name:   f.Text(roles.Accent2).Bold(true),
		sample: f.Fill(roles.Raised).Padding(0, 1),
		status: f.Text(roles.Muted),
	}
}

// card is a frame with the name of its effect, around body.
func (st *styles) card(title string, body bloom.Child) bloom.Child {
	return bloom.Box(st.frame, bloom.Column(
		bloom.Text(title).Style(st.name),
		bloom.Fill(body),
	))
}
