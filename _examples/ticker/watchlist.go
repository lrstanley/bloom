// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

type stock struct {
	name   string
	price  float64
	change float64 // percent
}

var stocks = []stock{
	{"ACME Corp", 128.40, 1.2},
	{"Globex International Holdings Group", 412.05, -0.8},
	{"Initech", 33.17, 0.4},
	{"Umbrella Pharmaceuticals and Biotechnology Ltd.", 87.92, -2.6},
	{"Hooli", 215.60, 3.1},
	{"Stark Industries Advanced Research Division", 530.00, 0.0},
	{"Wayne Enterprises", 301.75, 1.9},
	{"Soylent Nutritional Sciences, Incorporated", 19.48, -1.1},
}

// watchlist is a row for each stock, which can be clicked. Long names are
// cut off, except for the selected one, which scrolls.
type watchlist struct {
	st       *styles
	selected int     // index into stocks
	phase    float64 // 0 to 1, how far the selected name has scrolled

	pick func(i int) tea.Cmd // selects stock i
}

func (l watchlist) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(stocks)+1)
	for i, s := range stocks {
		st, right, phase, good, bad := l.st.item, l.st.itemRight, 0.0, l.st.good, l.st.bad
		if i == l.selected {
			st, right, phase, good, bad = l.st.selected, l.st.selectedRight, l.phase, l.st.goodOn, l.st.badOn
		}
		trend := good
		if s.change < 0 {
			trend = bad
		}

		price := bloom.Text(fmt.Sprintf("%.2f", s.price)).Style(right)
		change := bloom.Text(fmt.Sprintf("%+.1f%%", s.change)).Style(right.Foreground(trend))
		rows = append(rows, bloom.Row(
			bloom.Fill(bloom.Marquee(s.name, phase).Style(st)),
			bloom.Fixed(11, price),
			bloom.Fixed(9, change),
		).On(on.Click(func() tea.Cmd { return l.pick(i) })))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}
