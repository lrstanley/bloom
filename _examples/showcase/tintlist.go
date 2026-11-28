// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// tintList is a row for each tint, with a few of its colors, which switches
// to it when clicked.
type tintList struct {
	st      *styles
	current int                 // the tint in use, an index of tints.
	pick    func(i int) tea.Cmd // switches to tint i.
}

// Layout lists the tints, the one in use marked.
func (l tintList) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(tints)+1)
	for i, t := range tints {
		st, hover, name, mark := l.st.item, l.st.hovered, l.st.text, "  "
		if i == l.current {
			st, hover, name, mark = l.st.selected, l.st.selHover, l.st.current, "▸ "
		}
		row := []bloom.Child{bloom.Text(mark + t.DisplayName).Style(name), bloom.Space()}
		for j := range l.st.dots[i] {
			row = append(row, bloom.Text("●").Style(l.st.dots[i][j]))
		}
		rows = append(rows, bloom.Fixed(1, bloom.Box(st, bloom.Row(row...)).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			if i == l.current {
				return nil
			}
			return l.pick(i)
		}))))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}
