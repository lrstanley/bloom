// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command basics prints layouts without a Bubble Tea program, using
// bloom.Render, one for each way children share a row or column: fixed,
// filling, weighted, percentage, natural, and clamped sizes, gaps,
// justification, alignment, spacers, frames, wrapping, and centering; and
// the grid, flow, spacer, and truncating text helpers. The last shows that a
// layout is also just a string to Lip Gloss.
package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/flex"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

const paragraph = "Text with a style is rendered at the size it is given, so long " +
	"lines wrap to fit and the background fills the space."

type example struct {
	title string
	w, h  int // zero for the layout's natural size
	tree  bloom.Child
}

// examples are the layouts printed, made with the styles st.
func examples(st *styles) []example {
	justify := func(j flex.Justify, name string) bloom.Child {
		return bloom.Row(
			bloom.Fixed(14, bloom.Text(name)),
			bloom.Fill(bloom.Row(st.tag("a"), st.tag("b"), st.tag("c")).Justify(j)),
		)
	}

	return []example{
		{"Fixed, Fill, and Auto (a bare child is Auto)", 60, 3, bloom.Row(
			bloom.Fixed(12, st.card("fixed 12")),
			bloom.Fill(st.card("fill")),
			st.card("auto"),
		).Gap(1)},
		{"Grow weights and Percent", 60, 3, bloom.Row(
			bloom.Grow(1, st.card("grow 1")),
			bloom.Grow(2, st.card("grow 2")),
			bloom.Percent(25, st.card("25%")),
		)},
		{"Min and Max clamp a share", 60, 3, bloom.Row(
			bloom.Fill(st.card("max 14")).Max(14),
			bloom.Fill(st.card("fill")),
			bloom.Fill(st.card("min 30")).Min(30),
		)},
		{"Collapsible: dropped when minimums do not fit (here at 40 columns)", 40, 3, bloom.Row(
			bloom.Fixed(16, st.card("priority 1")).Collapsible(1),
			bloom.Fixed(16, st.card("priority 2")).Collapsible(2),
			bloom.Fill(st.card("min 20")).Min(20),
		)},
		{"Justify, when nothing grows", 46, 6, bloom.Column(
			justify(flex.Start, "Start"),
			justify(flex.Center, "Center"),
			justify(flex.End, "End"),
			justify(flex.SpaceBetween, "SpaceBetween"),
			justify(flex.SpaceAround, "SpaceAround"),
			justify(flex.SpaceEvenly, "SpaceEvenly"),
		)},
		{"Align on the cross axis: the row centers, slots override it", 60, 5, bloom.Row(
			st.card("tall\ncard\nhere"),
			bloom.Auto(st.tag("start")).Align(bloom.Start),
			st.tag("center"),
			bloom.Auto(st.tag("end")).Align(bloom.End),
			bloom.Fill(bloom.Text("stretch").Style(st.stretched)).Align(bloom.Stretch),
		).Align(bloom.Center).Gap(1)},
		{"Space takes what is left", 24, 6, bloom.Box(st.border, bloom.Column(
			bloom.Text("top"),
			bloom.Space(),
			bloom.Text("bottom"),
		))},
		{"Box with padding, around wrapping text", 36, 9, bloom.Box(
			st.padded,
			bloom.Text(paragraph).Style(st.prose),
		)},
		{"Fixed(n, nil) and Space(): fixed and growing gaps", 30, 3, bloom.Row(
			st.card("a"),
			bloom.Fixed(4, nil),
			st.card("b"),
			bloom.Space(),
			st.card("c"),
		)},
		{"Truncate: one line, cut off with an ellipsis", 30, 4, bloom.Box(st.border, bloom.Column(
			bloom.Text("A long line that will not fit").Truncate(),
			bloom.Text("A short one").Truncate(),
		))},
		{"Grid: equal columns, row by row (3 columns)", 40, 0, bloom.Grid(
			3,
			st.card("one"), st.card("two"), st.card("three"), st.card("four"), st.card("five"),
		).Gap(1, 0)},
		{"Wrap: natural sizes, flowing onto new lines", 36, 3, bloom.Wrap(
			st.tag("layout"), st.tag("flex"), st.tag("grid"),
			st.tag("wrap"), st.tag("truncate"), st.tag("spacer"),
		).Gap(1, 1)},
		{"Centered", 36, 5, bloom.Box(st.border, bloom.Centered(st.tag("in the middle")))},
		{"Natural size: no width or height given", 0, 0, bloom.Row(
			st.card("a"),
			st.card("bb\nbb"),
			st.card("ccc\nccc\nccc"),
		).Gap(1)},
		{"A layout is a string to Lip Gloss too", 0, 0, bloom.Text(lipgloss.JoinHorizontal(
			lipgloss.Center,
			bloom.Render(0, 0, bloom.Column(st.card("node"), st.card("tree"))),
			" → ",
			st.border.Render("lipgloss"),
		))},
	}
}

// render is every example, under its title, in the styles made from the
// example's tint.
func render() string {
	st := newStyles(tint.TintTokyoNight)
	var b strings.Builder
	for _, ex := range examples(&st) {
		b.WriteString(st.heading.Render(ex.title))
		b.WriteString("\n")
		b.WriteString(bloom.Render(ex.w, ex.h, ex.tree))
		b.WriteString("\n\n")
	}
	return b.String()
}

func main() {
	fmt.Print(render())
}
