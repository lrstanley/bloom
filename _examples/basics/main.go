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

	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/flex"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNight

var (
	accent = theme.Blue
	good   = theme.Green
	warn   = theme.Yellow
	dark   = theme.Bg
	prose  = theme.White

	border  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent)
	chip    = lipgloss.NewStyle().Foreground(dark).Background(good).Padding(0, 1)
	heading = lipgloss.NewStyle().Bold(true).Foreground(warn)
)

const paragraph = "Text with a style is rendered at the size it is given, so long " +
	"lines wrap to fit and the background fills the space."

// card is text in a rounded frame.
func card(s string) bloom.Child { return bloom.Box(border, bloom.Text(s)) }

// tag is text on a colored background.
func tag(s string) bloom.Child { return bloom.Text(s).Style(chip) }

type example struct {
	title string
	w, h  int // zero for the layout's natural size
	tree  bloom.Child
}

func examples() []example {
	justify := func(j flex.Justify, name string) bloom.Child {
		return bloom.Row(
			bloom.Fixed(14, bloom.Text(name)),
			bloom.Fill(bloom.Row(tag("a"), tag("b"), tag("c")).Justify(j)),
		)
	}
	stretched := chip.Align(lipgloss.Center, lipgloss.Center)

	return []example{
		{"Fixed, Fill, and Auto (a bare child is Auto)", 60, 3, bloom.Row(
			bloom.Fixed(12, card("fixed 12")),
			bloom.Fill(card("fill")),
			card("auto"),
		).Gap(1)},
		{"Grow weights and Percent", 60, 3, bloom.Row(
			bloom.Grow(1, card("grow 1")),
			bloom.Grow(2, card("grow 2")),
			bloom.Percent(25, card("25%")),
		)},
		{"Min and Max clamp a share", 60, 3, bloom.Row(
			bloom.Fill(card("max 14")).Max(14),
			bloom.Fill(card("fill")),
			bloom.Fill(card("min 30")).Min(30),
		)},
		{"Collapsible: dropped when minimums do not fit (here at 40 columns)", 40, 3, bloom.Row(
			bloom.Fixed(16, card("priority 1")).Collapsible(1),
			bloom.Fixed(16, card("priority 2")).Collapsible(2),
			bloom.Fill(card("min 20")).Min(20),
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
			card("tall\ncard\nhere"),
			bloom.Auto(tag("start")).Align(bloom.Start),
			tag("center"),
			bloom.Auto(tag("end")).Align(bloom.End),
			bloom.Fill(bloom.Text("stretch").Style(stretched)).Align(bloom.Stretch),
		).Align(bloom.Center).Gap(1)},
		{"Space takes what is left", 24, 6, bloom.Box(border, bloom.Column(
			bloom.Text("top"),
			bloom.Space(),
			bloom.Text("bottom"),
		))},
		{"Box with padding, around wrapping text", 36, 9, bloom.Box(
			border.Padding(1, 2),
			bloom.Text(paragraph).Style(lipgloss.NewStyle().Foreground(prose)),
		)},
		{"Fixed(n, nil) and Space(): fixed and growing gaps", 30, 3, bloom.Row(
			card("a"),
			bloom.Fixed(4, nil),
			card("b"),
			bloom.Space(),
			card("c"),
		)},
		{"Truncate: one line, cut off with an ellipsis", 30, 4, bloom.Box(border, bloom.Column(
			bloom.Text("A long line that will not fit").Truncate(),
			bloom.Text("A short one").Truncate(),
		))},
		{"Grid: equal columns, row by row (3 columns)", 40, 0, bloom.Grid(
			3,
			card("one"), card("two"), card("three"), card("four"), card("five"),
		).Gap(1, 0)},
		{"Wrap: natural sizes, flowing onto new lines", 36, 3, bloom.Wrap(
			tag("layout"), tag("flex"), tag("grid"),
			tag("wrap"), tag("truncate"), tag("spacer"),
		).Gap(1, 1)},
		{"Centered", 36, 5, bloom.Box(border, bloom.Centered(tag("in the middle")))},
		{"Natural size: no width or height given", 0, 0, bloom.Row(
			card("a"),
			card("bb\nbb"),
			card("ccc\nccc\nccc"),
		).Gap(1)},
		{"A layout is a string to Lip Gloss too", 0, 0, bloom.Text(lipgloss.JoinHorizontal(
			lipgloss.Center,
			bloom.Column(card("node"), card("tree")).View(),
			" → ",
			border.Render("lipgloss"),
		))},
	}
}

func render() string {
	var b strings.Builder
	for _, ex := range examples() {
		b.WriteString(heading.Render(ex.title))
		b.WriteString("\n")
		b.WriteString(bloom.Render(ex.w, ex.h, ex.tree))
		b.WriteString("\n\n")
	}
	return b.String()
}

func main() {
	fmt.Print(render())
}
