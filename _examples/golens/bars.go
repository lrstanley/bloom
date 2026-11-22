// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"slices"
	"strings"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// topbar is the path to the selected function, and the project.
func (m *model) topbar() bloom.Child {
	right := m.st.muted.Render(m.prog.module) + "  " + m.st.accent.Render("Go Lens")
	room := m.w - lipgloss.Width(right) - 3
	sep := m.st.faint.Render(" › ")

	var parts []string
	width := 0
	for k := m.depth(); k >= 0; k-- {
		name := m.nodeAt(k).name
		w := ansi.StringWidth(name) + 3
		if width+w > room && len(parts) > 0 {
			parts = append(parts, m.st.faint.Render("…"))
			break
		}
		width += w
		if k == m.depth() {
			parts = append(parts, m.st.bright.Render(name))
		} else {
			parts = append(parts, m.st.muted.Render(name))
		}
	}
	slices.Reverse(parts)
	return bloom.Row(
		bloom.Fill(bloom.Text(" "+strings.Join(parts, sep)).Truncate()),
		bloom.Text(right+" "),
	)
}

// footer shows key hints, given as pairs of keys and what they do.
func (st *styles) footer(hints ...string) bloom.Child {
	var b strings.Builder
	b.WriteString(" ")
	for i := 0; i+1 < len(hints); i += 2 {
		if i > 0 {
			b.WriteString(st.faint.Render("  ·  "))
		}
		b.WriteString(st.text.Render(hints[i]) + " " + st.muted.Render(hints[i+1]))
	}
	return bloom.Text(b.String()).Truncate()
}
