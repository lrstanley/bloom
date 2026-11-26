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
// tint by newStyles. render makes them, and passes them to the examples, so
// that nothing is a package global.
type styles struct {
	border    lipgloss.Style // a rounded frame
	padded    lipgloss.Style // the rounded frame, padded, around the paragraph
	chip      lipgloss.Style // text on a colored background
	stretched lipgloss.Style // a chip, centered in the space it is stretched to
	heading   lipgloss.Style // the title of each example
	prose     lipgloss.Style // the paragraph's text
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as an accent or body text, from the
// tint, so the example reads on any tint, dark or light. The headings are the
// tint's yellow, apart from the frames and chips they title.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(f.Color(roles.Accent))
	chip := f.Fill(roles.Accent2).Padding(0, 1)
	return styles{
		border:    border,
		padded:    border.Padding(1, 2),
		chip:      chip,
		stretched: chip.Align(lipgloss.Center, lipgloss.Center),
		heading:   f.Text(roles.Yellow).Bold(true),
		prose:     f.Text(roles.Fg),
	}
}

// card is text in a rounded frame.
func (st *styles) card(s string) bloom.Child { return bloom.Box(st.border, bloom.Text(s)) }

// tag is text on a colored background.
func (st *styles) tag(s string) bloom.Child { return bloom.Text(s).Style(st.chip) }
