// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anchor

import (
	"image"
	"testing"

	"bloomfx.dev/bloom/fx"
)

// area is the 30×10 screen the tests place boxes in.
var area = image.Rect(0, 0, 30, 10)

func TestPlace(t *testing.T) {
	t.Parallel()

	rect := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	tests := []struct {
		name   string
		target image.Rectangle
		w, h   int
		side   fx.Direction
		want   image.Point
		placed fx.Direction
	}{
		// Room on the side asked for: no flip.
		{"bottom fits", rect(10, 3, 4, 1), 6, 3, fx.Bottom, image.Pt(10, 4), fx.Bottom},
		{"top fits", rect(10, 5, 4, 1), 6, 3, fx.Top, image.Pt(10, 2), fx.Top},
		{"left fits", rect(10, 3, 4, 1), 6, 3, fx.Left, image.Pt(4, 3), fx.Left},
		{"right fits", rect(10, 3, 4, 1), 6, 3, fx.Right, image.Pt(14, 3), fx.Right},

		// No room: the opposite side.
		{"bottom flips up", rect(10, 8, 4, 1), 6, 3, fx.Bottom, image.Pt(10, 5), fx.Top},
		{"top flips down", rect(10, 1, 4, 1), 6, 3, fx.Top, image.Pt(10, 2), fx.Bottom},
		{"left flips right", rect(3, 3, 4, 1), 6, 3, fx.Left, image.Pt(7, 3), fx.Right},
		{"right flips left", rect(22, 3, 4, 1), 6, 3, fx.Right, image.Pt(16, 3), fx.Left},

		// A corner flips each way on its own.
		{"bottom right flips up", rect(10, 8, 4, 1), 6, 3, fx.BottomRight, image.Pt(14, 5), fx.TopRight},
		{"bottom right flips left", rect(22, 3, 4, 1), 6, 3, fx.BottomRight, image.Pt(16, 4), fx.BottomLeft},
		{"bottom right flips both", rect(22, 8, 4, 1), 6, 3, fx.BottomRight, image.Pt(16, 5), fx.TopLeft},
		{"top left flips both", rect(3, 1, 4, 1), 6, 3, fx.TopLeft, image.Pt(7, 2), fx.BottomRight},

		// Room on neither side: shifted over the node, on the side asked for.
		{"bottom fits nowhere", rect(10, 4, 4, 1), 6, 6, fx.Bottom, image.Pt(10, 4), fx.Bottom},
		{"top fits nowhere", rect(10, 4, 4, 1), 6, 6, fx.Top, image.Pt(10, 0), fx.Top},
		{"right fits nowhere", rect(12, 3, 4, 1), 15, 3, fx.Right, image.Pt(15, 3), fx.Right},
		{"left fits nowhere", rect(12, 3, 4, 1), 15, 3, fx.Left, image.Pt(0, 3), fx.Left},

		// Along the side, the box lines up with the node's start, and is
		// shifted to stay inside the area rather than flipped.
		{"bottom shifted left", rect(26, 3, 4, 1), 6, 3, fx.Bottom, image.Pt(24, 4), fx.Bottom},
	}
	for _, tt := range tests {
		at, placed := Place(tt.target, area, image.Pt(tt.w, tt.h), tt.side, image.Point{})
		if at != tt.want || placed != tt.placed {
			t.Errorf("%s: at %v on side %d, want %v on side %d", tt.name, at, placed, tt.want, tt.placed)
		}
	}
}

func TestPlaceMirrorsOffset(t *testing.T) {
	t.Parallel()

	gap, size := image.Pt(0, 1), image.Pt(6, 3)
	tests := []struct {
		name   string
		target image.Rectangle
		want   image.Point
		placed fx.Direction
	}{
		// A gap of a row below the node, kept above it once flipped.
		{"below", image.Rect(10, 2, 14, 3), image.Pt(10, 4), fx.Bottom},
		{"flipped", image.Rect(10, 7, 14, 8), image.Pt(10, 3), fx.Top},
		// The offset counts toward fitting: the box fits below without it.
		{"offset past the edge", image.Rect(10, 6, 14, 7), image.Pt(10, 2), fx.Top},
	}
	for _, tt := range tests {
		at, placed := Place(tt.target, area, size, fx.Bottom, gap)
		if at != tt.want || placed != tt.placed {
			t.Errorf("%s: at %v on side %d, want %v on side %d", tt.name, at, placed, tt.want, tt.placed)
		}
		// On places it the same, on the side Place chose.
		if got := On(tt.target, area, size, fx.Bottom, placed, gap); got != tt.want {
			t.Errorf("%s: On placed it at %v, want %v", tt.name, got, tt.want)
		}
	}

	// On does not flip back a box that no longer fits where it was put.
	if got, want := On(image.Rect(10, 1, 14, 2), area, size, fx.Bottom, fx.Top, gap), image.Pt(10, 0); got != want {
		t.Errorf("On flipped back: at %v, want %v", got, want)
	}
}
