// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anchor

import (
	"image"
	"testing"
)

// grid returns a scene of rows, where "." is a blank cell, "#" one that
// shows only a background, and anything else a glyph.
func grid(rows ...string) *Scene {
	return &Scene{
		Screen: image.Rect(0, 0, len(rows[0]), len(rows)),
		Cell: func(p image.Point) Cell {
			switch rows[p.Y][p.X] {
			case '.':
				return Blank
			case '#':
				return Painted
			}
			return Glyph
		},
	}
}

func rect(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

func TestFitCost(t *testing.T) {
	t.Parallel()

	s := grid(
		"xx#..",
		"x#.#.",
	)
	tests := []struct {
		name string
		busy []image.Rectangle
		r    image.Rectangle
		want int
	}{
		{"glyphs", nil, rect(0, 0, 2, 1), 8},
		{"painted", nil, rect(2, 0, 1, 1), 1},
		{"blank", nil, rect(3, 0, 2, 1), 0},
		{"all of it", nil, rect(0, 0, 5, 2), 4 + 4 + 1 + 4 + 1 + 1},
		{"double inside a busy area", []image.Rectangle{rect(0, 0, 1, 2)}, rect(0, 0, 3, 1), 8 + 4 + 1},
		{"busy areas that overlap count once", []image.Rectangle{rect(0, 0, 2, 1), rect(1, 0, 2, 1)}, rect(0, 0, 3, 1), 8 + 8 + 2},
		{"blank stays free inside a busy area", []image.Rectangle{rect(3, 0, 2, 2)}, rect(3, 0, 2, 2), 2},
		{"cut to the screen", nil, rect(3, 1, 4, 3), 1},
	}
	for _, tt := range tests {
		s.Busy = tt.busy
		if got := s.cost(tt.r); got != tt.want {
			t.Errorf("%s: cost %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestFit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scene  *Scene
		anchor image.Rectangle
		size   image.Point
		near   image.Point
		want   image.Rectangle
		ok     bool
	}{
		{
			"blank cells over text",
			grid(
				"xxxxxxxxxx",
				"...aa.....",
				"..........",
			),
			rect(3, 1, 2, 1), image.Pt(2, 1), image.Pt(3, 1),
			rect(3, 2, 2, 1), true,
		},
		{
			// Above costs 1, below costs nothing but is a step down.
			"a tie goes up the order of preference",
			grid(
				"...#......",
				"...aa.....",
				"..........",
			),
			rect(3, 1, 2, 1), image.Pt(2, 1), image.Pt(3, 1),
			rect(3, 0, 2, 1), true,
		},
		{
			"a glyph outweighs the order of preference",
			grid(
				"...x......",
				"...aa.....",
				"..........",
			),
			rect(3, 1, 2, 1), image.Pt(2, 1), image.Pt(3, 1),
			rect(3, 2, 2, 1), true,
		},
		{
			"right before left",
			grid(
				"xxxxxxxxxxx",
				"....aa.....",
				"xxxxxxxxxxx",
			),
			rect(4, 1, 2, 1), image.Pt(3, 1), image.Pt(4, 1),
			rect(7, 1, 3, 1), true,
		},
		{
			"left once right shows more",
			grid(
				"xxxxxxxxxxx",
				"....aa.x...",
				"xxxxxxxxxxx",
			),
			rect(4, 1, 2, 1), image.Pt(3, 1), image.Pt(4, 1),
			rect(0, 1, 3, 1), true,
		},
		{
			"the nearer of equal candidates",
			grid(
				"............",
				"..aaaaaaaa..",
				"............",
			),
			rect(2, 1, 8, 1), image.Pt(3, 1), image.Pt(9, 1),
			rect(7, 0, 3, 1), true,
		},
		{
			"lined up with the middle when nearest it",
			grid(
				"............",
				"..aaaaaaaa..",
				"............",
			),
			rect(2, 1, 8, 1), image.Pt(2, 1), image.Pt(5, 1),
			rect(5, 0, 2, 1), true,
		},
		{
			// Shifted down inside the screen, above would cover the anchor.
			"not over the anchor",
			grid(
				"..aa......",
				"..........",
			),
			rect(2, 0, 2, 1), image.Pt(2, 1), image.Pt(2, 0),
			rect(2, 1, 2, 1), true,
		},
		{
			// Shifted left at the right edge, above covers the glyphs at 5
			// and 6, which it would not where it started.
			"scored once shifted inside the screen",
			grid(
				".....xx...",
				"........aa",
				"..........",
			),
			rect(8, 1, 2, 1), image.Pt(5, 1), image.Pt(8, 1),
			rect(5, 2, 5, 1), true,
		},
		{
			"a box taller than the screen fits nowhere",
			grid(
				"..........",
				"..aa......",
				"..........",
			),
			rect(2, 1, 2, 1), image.Pt(2, 4), image.Pt(2, 1),
			rect(2, 0, 2, 4), false,
		},
	}
	for _, tt := range tests {
		got, ok := tt.scene.Fit(tt.anchor, tt.size, tt.near)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: placed at %v (%t), want %v (%t)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFitAvoids(t *testing.T) {
	t.Parallel()

	blank := func() *Scene {
		return grid(
			"..........",
			"..........",
			"...aa.....",
			"..........",
			"..........",
		)
	}
	anchor, size := rect(3, 2, 2, 1), image.Pt(2, 1)
	tests := []struct {
		name  string
		avoid []image.Rectangle
		want  image.Rectangle
	}{
		{"nothing in the way", nil, rect(3, 1, 2, 1)},
		{"the pointer's cell", []image.Rectangle{rect(4, 1, 1, 1)}, rect(3, 3, 2, 1)},
		{"the cursor's cell above and below", []image.Rectangle{rect(3, 1, 2, 1), rect(3, 3, 2, 1)}, rect(6, 2, 2, 1)},
		{"a target to the right as well", []image.Rectangle{rect(3, 1, 2, 1), rect(3, 3, 2, 1), rect(6, 2, 4, 1)}, rect(0, 2, 2, 1)},
	}
	for _, tt := range tests {
		s := blank()
		s.Avoid = tt.avoid
		if got, ok := s.Fit(anchor, size, anchor.Min); got != tt.want || !ok {
			t.Errorf("%s: placed at %v (%t), want %v", tt.name, got, ok, tt.want)
		}
	}

	// With every candidate out, the first, shifted inside the screen.
	s := blank()
	s.Avoid = []image.Rectangle{s.Screen}
	if got, ok := s.Fit(rect(8, 0, 2, 1), image.Pt(4, 1), image.Pt(8, 0)); got != rect(6, 0, 4, 1) || ok {
		t.Errorf("every candidate out: placed at %v (%t), want %v (false)", got, ok, rect(6, 0, 4, 1))
	}
	// A box bigger than the screen starts at its corner.
	s = blank()
	if got, _ := s.Fit(anchor, image.Pt(12, 1), anchor.Min); got.Min != image.Pt(0, 1) {
		t.Errorf("a box wider than the screen placed at %v", got)
	}
	// Without cells, the screen is blank.
	s = &Scene{Screen: image.Rect(0, 0, 10, 5)}
	if got, ok := s.Fit(anchor, size, anchor.Min); got != rect(3, 1, 2, 1) || !ok {
		t.Errorf("without cells: placed at %v (%t)", got, ok)
	}
}

// TestFitSeveral places boxes one at a time, each avoiding the others and
// the anchors of the rest, as tooltips shown together are.
func TestFitSeveral(t *testing.T) {
	t.Parallel()

	s := grid(
		"..........",
		"aa..bb....",
		"..........",
	)
	a, b := rect(0, 1, 2, 1), rect(4, 1, 2, 1)
	s.Avoid = []image.Rectangle{a, b}
	size := image.Pt(5, 1)

	first, ok := s.Fit(a, size, a.Min)
	if !ok || first != rect(0, 0, 5, 1) {
		t.Fatalf("first placed at %v (%t)", first, ok)
	}
	s.Avoid = append(s.Avoid, first)
	second, ok := s.Fit(b, size, b.Min)
	if !ok || second.Overlaps(first) || second != rect(3, 2, 5, 1) {
		t.Fatalf("second placed at %v (%t), over the first at %v", second, ok, first)
	}
	s.Avoid = append(s.Avoid, second)
	// Above and below both are taken, and the sides cover a or b.
	if third, fits := s.Fit(a, size, a.Min); fits {
		t.Fatalf("third placed at %v, want it dropped", third)
	}
}
