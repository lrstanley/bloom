// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"
	"strings"
	"testing"
)

// sizeProbe records the sizes it is laid out at, and wraps a long line above
// a last row.
type sizeProbe struct {
	sizes *[]Size
}

func (p sizeProbe) Layout(ctx Context) Child {
	*p.sizes = append(*p.sizes, ctx.Size())
	return Box(rounded, Column(Text(strings.Repeat("word ", 20)), Text("LAST")))
}

func TestLayerFixedWidthFitsHeight(t *testing.T) {
	t.Parallel()
	base := Text(strings.Repeat(strings.Repeat(".", 120)+"\n", 20))
	tests := []struct {
		name  string
		w     int
		want  image.Rectangle
		width int
	}{
		// 100 cells of text wrap to 3 rows in 38, plus "LAST" and borders.
		{"fixed width", 40, image.Rect(40, 7, 80, 13), 40},
		// A width past the screen is clamped to it before measuring.
		{"clamped width", 200, image.Rect(0, 8, 120, 12), 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var sizes []Size
			f := Solve(120, 20, Overlay(base, Layer(sizeProbe{sizes: &sizes}).Size(tt.w, 0).ID("l")))
			expectRect(t, f, "l", tt.want)
			if len(sizes) == 0 || slices.ContainsFunc(sizes, func(s Size) bool { return s.W != tt.width }) {
				t.Fatalf("laid out at %v; want only width %d", sizes, tt.width)
			}
			if got := Render(120, 20, Overlay(base, Layer(sizeProbe{sizes: &sizes}).Size(tt.w, 0))); !strings.Contains(got, "LAST") {
				t.Fatalf("last row cut off:\n%s", got)
			}
		})
	}
}

func TestLayerFixedHeightMeasuresAtHeight(t *testing.T) {
	t.Parallel()
	var sizes []Size
	f := Solve(120, 20, Overlay(Text("base"), Layer(sizeProbe{sizes: &sizes}).Size(0, 3).ID("l")))
	if r, ok := f.Rect("l"); !ok || r.Dy() != 3 {
		t.Fatalf("layer at %v, in the frame %v; want 3 rows", r, ok)
	}
	if len(sizes) == 0 || slices.ContainsFunc(sizes, func(s Size) bool { return s.H != 3 }) {
		t.Fatalf("laid out at %v; want only height 3", sizes)
	}
}

// TestLayerPlace puts layers at rectangles of their overlay, one of them
// anchored to a node before, which Place undoes.
func TestLayerPlace(t *testing.T) {
	t.Parallel()
	r := New()
	r.Render(20, 10, Overlay(Text("base").ID("base"),
		Layer(Text("window")).ID("window").Place(image.Rect(3, 2, 13, 6)),
		Layer(Text("moved")).ID("moved").AnchorTo("base", Bottom).Place(image.Rect(-2, 8, 6, 12)),
		Layer(Text("natural")).ID("natural").Place(image.Rect(1, 1, 1, 1)),
	))
	expectRect(t, r.Frame(), "window", image.Rect(3, 2, 13, 6))
	expectRect(t, r.Frame(), "moved", image.Rect(-2, 8, 6, 12))
	expectRect(t, r.Frame(), "natural", image.Rect(1, 1, 8, 2))
}
