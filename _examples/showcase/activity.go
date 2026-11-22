// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"math"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// activity is a bar chart, its bars rising and falling as the loop runs, in
// a gradient through the tint's colors.
type activity struct {
	st   styles
	bars anim.Sequence // loops, moving the activity bars.
}

// Update moves the bars along, and takes the new styles of a themeMsg.
func (a *activity) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(themeMsg); ok {
		a.st = *msg.st
	}
	return a.bars.Update(msg)
}

// Layout is the chart under its caption.
func (a *activity) Layout(bloom.Context) bloom.Child {
	return bloom.Column(
		bloom.Text("requests per second").Style(a.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Fill(bars{phase: a.bars.Float(), stops: a.st.bars}),
	)
}

// bars is a component with a Layout, so it draws as many bars as fit the space it
// is given.
type bars struct {
	phase float64
	stops []color.Color
}

func (b bars) Layout(ctx bloom.Context) bloom.Child {
	// Each bar is two cells wide, with a cell between them.
	n := max((ctx.Width()+1)/3, 1)
	colors := lipgloss.Blend1D(n, b.stops...)
	cols := make([]bloom.Child, n)
	for i := range n {
		x := float64(i) / float64(n)
		level := 0.55 + 0.3*math.Sin(2*math.Pi*(b.phase+x)) + 0.15*math.Sin(2*math.Pi*(2*b.phase-1.7*x))
		style := lipgloss.NewStyle().Foreground(colors[i])
		cols[i] = bloom.Fixed(2, bloom.Text(bar(level, ctx.Height())).Style(style))
	}
	return bloom.Row(cols...).Gap(1)
}

// eighths are the blocks that fill a cell from the bottom, in eighths.
var eighths = []rune(" ▁▂▃▄▅▆▇█")

// bar is a column h cells tall, filled from the bottom to level, from 0 to
// 1, in eighths of a cell.
func bar(level float64, h int) string {
	filled := int(math.Round(min(max(level, 0), 1) * float64(h*8)))
	lines := make([]string, h)
	for y := range h {
		// How much of this line is filled, counting lines from the bottom.
		n := min(max(filled-(h-1-y)*8, 0), 8)
		lines[y] = strings.Repeat(string(eighths[n]), 2)
	}
	return strings.Join(lines, "\n")
}
