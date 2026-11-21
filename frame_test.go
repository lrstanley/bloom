// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestStretchedFrame checks that a box draws its frame as Lip Gloss draws
// it at the box's size, whether the frame is stretched or drawn whole.
func TestStretchedFrame(t *testing.T) {
	t.Parallel()

	red, blue, gray := lipgloss.Color("#ff0000"), lipgloss.Color("#0000ff"), lipgloss.Color("#303030")
	styles := map[string]struct {
		style   lipgloss.Style
		stretch bool
	}{
		"rounded":      {lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(red), true},
		"background":   {lipgloss.NewStyle().Border(lipgloss.ThickBorder()).Background(gray).Padding(1, 2), true},
		"margins":      {lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Margin(1, 2).MarginBackground(blue).Padding(0, 1), true},
		"sides":        {lipgloss.NewStyle().Border(lipgloss.DoubleBorder(), true, false, true, true).BorderTopForeground(red).BorderLeftForeground(blue), true},
		"reverse":      {lipgloss.NewStyle().Border(lipgloss.BlockBorder()).Reverse(true).Foreground(red), true},
		"padding only": {lipgloss.NewStyle().Padding(1).Background(blue), true},
		"pattern":      {lipgloss.NewStyle().Border(lipgloss.Border{Top: "-=", Bottom: "-=", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}), false},
		"wide":         {lipgloss.NewStyle().Border(lipgloss.Border{Top: "世", Bottom: "世", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}), false},
		"blend":        {lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForegroundBlend(red, blue), false},
		"repeating":    {lipgloss.NewStyle().Border(lipgloss.Border{Top: "aab", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}), false},
		"combining":    {lipgloss.NewStyle().Border(lipgloss.Border{Top: "\u0301", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}), false},
		"transform":    {lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Transform(func(string) string { return "xx\nxx" }), false},
		"own width":    {lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(12), false},
	}
	for name, tt := range styles {
		for _, size := range []image.Point{{40, 12}, {17, 9}, {9, 7}, {7, 5}, {3, 3}} {
			t.Run(fmt.Sprintf("%s %dx%d", name, size.X, size.Y), func(t *testing.T) {
				t.Parallel()
				r := newRenderer(ansi.GraphemeWidth)
				got := r.render(size.X, size.Y, Box(tt.style, nil))
				if r.boxes[0].cramped {
					t.Skip("a box smaller than its frame is not drawn")
				}
				want, _ := textOf(TextNode{style: tt.style, styled: true}, size.X, size.Y, ansi.GraphemeWidth)
				if a, b := cellsOf(got, size), cellsOf(want, size); a != b {
					t.Fatalf("got:\n%s\nwant:\n%s", a, b)
				}
				stretched := false
				for _, o := range r.prevOps {
					stretched = stretched || o.kind == opStretch
				}
				f := frameOf(tt.style)
				if fits := size.X >= f.l+f.r+2 && size.Y >= f.t+f.b+2; stretched != (tt.stretch && fits) {
					t.Fatalf("stretched = %v", stretched)
				}
			})
		}
	}
}

// cellsOf returns s drawn on a canvas of size, as text with the style of
// each cell, for comparing frames that may be written with different
// escape sequences.
func cellsOf(s string, size image.Point) string {
	c := canvas.New(size.X, size.Y, ansi.GraphemeWidth)
	c.Blit(canvas.Parse(s, ansi.GraphemeWidth), c.Bounds(), c.Bounds())
	var out strings.Builder
	for y := range size.Y {
		for x := range size.X {
			cell := c.CellAt(x, y)
			fmt.Fprintf(&out, "%q%v%v%d|", cell.Content, cell.Style.Fg, cell.Style.Bg, cell.Style.Attrs)
		}
		out.WriteByte('\n')
	}
	return out.String()
}

// TestRenderAfterSolve checks that drawing a frame after Solve, which ends
// frames of a renderer's cache without drawing them, draws the new text
// rather than taking the frame for the last one drawn, whose canvases that
// cache would have parsed other text into had Solve used Render's renderers.
func TestRenderAfterSolve(t *testing.T) {
	if got := ansi.Strip(Render(5, 1, Text("aaaaa"))); got != "aaaaa" {
		t.Fatalf("first frame: %q", got)
	}
	for range 2 * 64 {
		Solve(5, 1, Text("x"))
	}
	if got := ansi.Strip(Render(5, 1, Text("bbbbb"))); got != "bbbbb" {
		t.Fatalf("after Solve: got %q, want %q", got, "bbbbb")
	}
}
