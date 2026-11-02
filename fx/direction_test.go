// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"fmt"
	"testing"
)

var directions = []Direction{Bottom, Top, Left, Right, TopLeft, TopRight, BottomLeft, BottomRight}

func TestDirection(t *testing.T) {
	t.Parallel()

	if dx, dy := Direction(0).Vector(); dx != 0 || dy != 0 {
		t.Errorf("zero vector = %d, %d", dx, dy)
	}
	// Opposites have opposite vectors, and corners combine their sides.
	for _, pair := range [][2]Direction{{Bottom, Top}, {Left, Right}, {TopLeft, BottomRight}, {TopRight, BottomLeft}} {
		a, b := pair[0], pair[1]
		ax, ay := a.Vector()
		bx, by := b.Vector()
		if ax != -bx || ay != -by || (ax == 0 && ay == 0) {
			t.Errorf("%v is (%d, %d), %v is (%d, %d)", a, ax, ay, b, bx, by)
		}
	}
	// Each is a corner, then its vertical and horizontal sides.
	for _, c := range [][3]Direction{
		{TopLeft, Top, Left}, {TopRight, Top, Right}, {BottomLeft, Bottom, Left}, {BottomRight, Bottom, Right},
	} {
		corner := c[0]
		cx, cy := corner.Vector()
		_, vy := c[1].Vector()
		hx, _ := c[2].Vector()
		if cx != hx || cy != vy {
			t.Errorf("%v is (%d, %d), want (%d, %d)", corner, cx, cy, hx, vy)
		}
	}
}

// nearFar counts the cells of s shown in the half, or for a corner the
// quarter, of it nearest d, and in the one farthest from it.
func nearFar(s interface {
	Width() int
	Height() int
	Visible(x, y int) bool
}, d Direction,
) (near, far int) {
	dx, dy := d.Vector()
	w, h := s.Width(), s.Height()
	side := func(v, n, dir int) (isNear, isFar bool) {
		switch {
		case dir > 0:
			return v < n/2, v >= n-n/2
		case dir < 0:
			return v >= n-n/2, v < n/2
		}
		return true, true
	}
	for y := range h {
		ny, fy := side(y, h, dy)
		for x := range w {
			nx, fx := side(x, w, dx)
			if !s.Visible(x, y) {
				continue
			}
			if nx && ny {
				near++
			}
			if fx && fy {
				far++
			}
		}
	}
	return near, far
}

func TestDirections(t *testing.T) {
	t.Parallel()

	effects := map[string]struct {
		make func(Direction) Effect
		mid  float64 // when the near part is well ahead of the far one.
	}{
		"wipe": {func(d Direction) Effect { return Wipe{From: d} }, 0.5},
		// One slat, larger than the surface, six steps open.
		"blinds": {func(d Direction) Effect { return Blinds{From: d, Size: 100} }, 0.06},
		"wave":   {func(d Direction) Effect { return Wave{From: d} }, 0.5},
		"decode": {func(d Direction) Effect { return Decode{From: d, Seed: 9} }, 0.7},
	}
	for name, tt := range effects {
		for _, d := range directions {
			t.Run(fmt.Sprintf("%s from %d", name, d), func(t *testing.T) {
				t.Parallel()
				const w, h = 30, 12
				e := tt.make(d)

				s := textLayer(block(w, h))
				e.Apply(s, 0)
				if n := hidden(s); n != w*h {
					t.Fatalf("t=0 hides %d of %d cells", n, w*h)
				}
				s = textLayer(block(w, h))
				e.Apply(s, 1)
				if n := hidden(s); n != 0 {
					t.Fatalf("t=1 hides %d cells", n)
				}
				s = textLayer(block(w, h))
				e.Apply(s, tt.mid)
				if near, far := nearFar(s, d); near <= far || near == 0 {
					t.Fatalf("t=%v shows %d cells near the start and %d far from it:\n%s", tt.mid, near, far, rows(s))
				}
			})
		}
	}
}

func TestDecodeLanes(t *testing.T) {
	t.Parallel()

	const w, h = 20, 8
	for _, d := range directions {
		for _, at := range []float64{0.3, 0.6, 0.9} {
			s := textLayer(block(w, h))
			Decode{From: d, Seed: 4}.Apply(s, at)
			// Within a lane, a cell is shown only if the one before it,
			// toward the start, is shown too.
			dx, dy := d.Vector()
			px, py := -dx, -dy
			if dx != 0 && dy != 0 {
				px = -2 * dx // a row on a diagonal lane is two columns.
			}
			for y := range h {
				for x := range w {
					bx, by := x+px, y+py
					if bx < 0 || bx >= w || by < 0 || by >= h || !s.Visible(x, y) {
						continue
					}
					if !s.Visible(bx, by) {
						t.Fatalf("from %d at t=%v: %d,%d is shown but %d,%d before it is not:\n%s", d, at, x, y, bx, by, rows(s))
					}
				}
			}
		}
	}

	// The default is from the top.
	a, b := textLayer(block(w, h)), textLayer(block(w, h))
	Decode{Seed: 4}.Apply(a, 0.5)
	Decode{From: Top, Seed: 4}.Apply(b, 0.5)
	if rows(a) != rows(b) {
		t.Fatalf("default:\n%s\nfrom the top:\n%s", rows(a), rows(b))
	}
}

func TestShimmerDirections(t *testing.T) {
	t.Parallel()

	const w, h = 30, 8
	changed := func(d Direction, at float64) [h][w]bool {
		s := textLayer(block(w, h))
		Shimmer{From: d, Width: 4}.Apply(s, at)
		var out [h][w]bool
		for y := range h {
			for x := range w {
				out[y][x] = s.Cell(x, y).Style.Fg != nil
			}
		}
		return out
	}
	changedAny := func(c [h][w]bool) bool {
		for y := range h {
			for x := range w {
				if c[y][x] {
					return true
				}
			}
		}
		return false
	}

	// From a side, the band is parallel to it: from the left every row is
	// changed in the same columns, and from the top every column in the same
	// rows.
	for _, d := range []Direction{Left, Right} {
		c := changed(d, 0.5)
		if !changedAny(c) {
			t.Fatalf("from %d changed nothing", d)
		}
		for y := 1; y < h; y++ {
			if c[y] != c[0] {
				t.Fatalf("from %d, row %d differs from row 0", d, y)
			}
		}
	}
	for _, d := range []Direction{Top, Bottom} {
		c := changed(d, 0.5)
		if !changedAny(c) {
			t.Fatalf("from %d changed nothing", d)
		}
		for y := range h {
			for x := 1; x < w; x++ {
				if c[y][x] != c[y][0] {
					t.Fatalf("from %d, row %d is not changed whole", d, y)
				}
			}
		}
	}
	// It starts near where it comes from.
	if c := changed(Left, 0.1); !c[0][0] || c[0][w-1] {
		t.Fatal("from the left, the band does not start at the left")
	}
	if c := changed(Right, 0.1); c[0][0] || !c[0][w-1] {
		t.Fatal("from the right, the band does not start at the right")
	}
	if c := changed(Bottom, 0.1); c[0][0] || !c[h-1][0] {
		t.Fatal("from the bottom, the band does not start at the bottom")
	}
	if c := changed(BottomRight, 0.05); c[0][0] || !c[h-1][w-1] {
		t.Fatal("from the bottom right, the band does not start there")
	}
	if changed(0, 0.4) != changed(TopLeft, 0.4) {
		t.Fatal("the default is not from the top left")
	}
}

func TestStaggerCorner(t *testing.T) {
	t.Parallel()

	for _, c := range [][2]Direction{{TopLeft, Top}, {TopRight, Top}, {BottomLeft, Bottom}, {BottomRight, Bottom}} {
		corner, side := c[0], c[1]
		a, b := textLayer(block(12, 6)), textLayer(block(12, 6))
		Stagger{Effect: Wipe{From: Left}, From: corner}.Apply(a, 0.5)
		Stagger{Effect: Wipe{From: Left}, From: side}.Apply(b, 0.5)
		if rows(a) != rows(b) {
			t.Fatalf("from %d:\n%s\nfrom %d:\n%s", corner, rows(a), side, rows(b))
		}
	}
}
