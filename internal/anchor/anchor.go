// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package anchor places a box against a side or corner of another, as a
// layer anchored to a node is, flipping it to the opposite side when it does
// not fit on the one asked for.
package anchor

import (
	"image"

	"bloomfx.dev/bloom/fx"
)

// Place returns where a box of size goes against side of target, moved by
// offset, inside area, and the side it goes on. The box goes beyond the side
// or corner, against the way from it across target, and lines up with
// target's start otherwise. A box that does not fit on side goes on the
// opposite one if it fits there, each way on its own for a corner, with
// offset mirrored. One that fits on neither is shifted to stay inside area,
// over target.
func Place(target, area image.Rectangle, size image.Point, side fx.Direction, offset image.Point) (image.Point, fx.Direction) {
	dx, dy := side.Vector()
	x, flipX := axis(target.Min.X, target.Max.X, area.Min.X, area.Max.X, size.X, dx, offset.X, true)
	y, flipY := axis(target.Min.Y, target.Max.Y, area.Min.Y, area.Max.Y, size.Y, dy, offset.Y, true)
	if flipX {
		dx = -dx
	}
	if flipY {
		dy = -dy
	}
	return image.Pt(x, y), sideOf(dx, dy)
}

// On returns where a box of size goes against placed, a side [Place] chose
// for a box asked to go on asked, without flipping it again: offset is
// mirrored on the axes placed flips, and a box that does not fit is shifted
// to stay inside area.
func On(target, area image.Rectangle, size image.Point, asked, placed fx.Direction, offset image.Point) image.Point {
	ax, ay := asked.Vector()
	dx, dy := placed.Vector()
	if dx != ax {
		offset.X = -offset.X
	}
	if dy != ay {
		offset.Y = -offset.Y
	}
	x, _ := axis(target.Min.X, target.Max.X, area.Min.X, area.Max.X, size.X, dx, offset.X, false)
	y, _ := axis(target.Min.Y, target.Max.Y, area.Min.Y, area.Max.Y, size.Y, dy, offset.Y, false)
	return image.Pt(x, y)
}

// axis returns where a box n cells long goes on one axis against a node
// from lo to hi, in an area from start to end, and whether it flipped to
// the other side of the node, which it may only with flip. A d of 0 lines
// it up with lo, less than 0 places it after hi, and more than 0 before lo.
func axis(lo, hi, start, end, n, d, offset int, flip bool) (at int, flipped bool) {
	if d == 0 {
		return min(max(lo+offset, start), end-n), false
	}
	// at is on the side asked for, and other on the opposite one.
	at, other := hi+offset, lo-n-offset
	if d > 0 {
		at, other = lo-n+offset, hi-offset
	}
	fits := func(p int) bool { return p >= start && p+n <= end }
	if flip && !fits(at) && fits(other) {
		return other, true
	}
	return min(max(at, start), end-n), false
}

// sideOf returns the side or corner whose vector is dx, dy.
func sideOf(dx, dy int) fx.Direction {
	for _, d := range [...]fx.Direction{fx.Bottom, fx.Top, fx.Left, fx.Right, fx.TopLeft, fx.TopRight, fx.BottomLeft, fx.BottomRight} {
		if x, y := d.Vector(); x == dx && y == dy {
			return d
		}
	}
	return 0
}
