// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
)

// load is requests per second over the last minutes, oldest first.
var load = []int{12, 18, 25, 22, 30, 41, 38, 52, 47, 60, 58, 71, 66, 80, 74, 69, 85, 92, 88, 95}

// chart is a bar chart of its values, as wide and tall as it is given, the
// latest values at the right.
type chart struct {
	st   *styles
	vals []int
}

func (c chart) Layout(ctx bloom.Context) bloom.Child {
	// A bar every other column, as many of the latest values as fit.
	vals := c.vals[max(len(c.vals)-(ctx.Width()+1)/2, 0):]
	peak := 1
	for _, v := range vals {
		peak = max(peak, v)
	}
	const eighths = " ▁▂▃▄▅▆▇█"
	levels := []rune(eighths)
	rows := make([]string, ctx.Height())
	for y := range rows {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", max(ctx.Width()-2*len(vals)+1, 0)))
		for i, v := range vals {
			// How many eighths of this row the bar fills, from the bottom.
			fill := v*ctx.Height()*8/peak - (ctx.Height()-1-y)*8
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(levels[min(max(fill, 0), 8)])
		}
		// Rows toward the top, reached only under heavy load, are warmer.
		t := 1 - float64(y)/float64(max(ctx.Height()-1, 1))
		rows[y] = c.st.onBg.Foreground(blend(c.st.theme.Green, c.st.theme.Red, t*t)).Render(b.String())
	}
	return bloom.Text(strings.Join(rows, "\n"))
}
