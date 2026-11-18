// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anchor

import (
	"image"
	"slices"
)

// Cell is what a cell of the screen shows, which a box placed over it
// hides.
type Cell uint8

const (
	// Blank shows nothing: a space with no background, underline, or other
	// attribute that shows.
	Blank Cell = iota
	// Painted shows no glyph, but a background or another attribute that
	// shows, such as the fill of a panel.
	Painted
	// Glyph shows a glyph, or part of a wide one.
	Glyph
)

// What hiding a cell costs, by what it shows.
var cellCost = [...]int{Blank: 0, Painted: 1, Glyph: 4}

// Scene is what is on a screen, for [Scene.Fit] to place boxes beside
// others where they hide the least of it.
//
// To place several boxes that must not cover each other, such as tooltips
// shown together, fit them one at a time in the order they matter, adding
// each to Avoid once placed, and drop one that does not fit:
//
//	s := anchor.Scene{Screen: screen, Cell: cells, Busy: busy, Avoid: targets}
//	for _, t := range tips {
//		r, ok := s.Fit(t.anchor, t.size, t.near)
//		if !ok {
//			continue // covers another, or its own target, wherever it goes
//		}
//		s.Avoid = append(s.Avoid, r)
//		place(t, r)
//	}
type Scene struct {
	// Screen is where boxes go: each candidate is shifted inside it.
	Screen image.Rectangle
	// Cell returns what the cell at p shows. A nil Cell is a blank screen.
	Cell func(p image.Point) Cell
	// Busy are areas that take input, such as the nodes that can be
	// clicked or focused: hiding a cell inside one costs double.
	Busy []image.Rectangle
	// Avoid are areas a box must not cover at all, such as the node it is
	// about, the cell under the pointer, the cursor's cell, and other boxes
	// already placed.
	Avoid []image.Rectangle
}

// Fit returns where a box of size goes beside anchor, inside the screen,
// hiding the least of what is drawn there, and whether it goes somewhere
// that covers nothing in Avoid.
//
// The candidates are the box against each side of anchor, above and below
// on the next row, or left and right with a blank cell between, in that
// order of preference, each lined up with the start, the middle, and the
// end of anchor, and shifted to stay inside the screen. One that then
// covers anchor, or an area in Avoid, is out, as is a box bigger than the
// screen. Of the rest, the one that costs least wins: 4 for each cell with
// a glyph it hides, 1 for a cell without one that still shows something,
// such as a background, and nothing for a blank cell, counted twice inside
// an area of Busy, plus 1 for each side it is down the order of preference.
// A tie goes to the candidate whose middle is nearest near, such as where
// the pointer is, and then to the one first in order.
//
// When every candidate is out, Fit returns the first, above anchor and
// lined up with its start, shifted inside the screen, and false.
func (s *Scene) Fit(anchor image.Rectangle, size, near image.Point) (image.Rectangle, bool) {
	var (
		best             image.Rectangle
		bestCost, bestTo int
		found            bool
	)
	for side, pos := range [...]image.Point{
		{0, -1}, // above
		{0, 1},  // below
		{1, 0},  // right
		{-1, 0}, // left
	} {
		for align := range 3 {
			r := s.shift(candidate(anchor, size, pos, align))
			if !r.In(s.Screen) || r.Overlaps(anchor) || slices.ContainsFunc(s.Avoid, r.Overlaps) {
				continue
			}
			cost, to := s.cost(r)+side, distance(r, near)
			if !found || cost < bestCost || (cost == bestCost && to < bestTo) {
				best, bestCost, bestTo, found = r, cost, to, true
			}
		}
	}
	if !found {
		return s.shift(candidate(anchor, size, image.Pt(0, -1), 0)), false
	}
	return best, true
}

// candidate returns a box of size against the side of anchor that pos
// points to, lined up with anchor's start for an align of 0, its middle for
// 1, and its end for 2. A box left or right of anchor has a blank cell
// between them.
func candidate(anchor image.Rectangle, size, pos image.Point, align int) image.Rectangle {
	var at image.Point
	line := func(lo, hi, n int) int {
		switch align {
		case 1:
			return lo + (hi-lo-n)/2
		case 2:
			return hi - n
		}
		return lo
	}
	switch {
	case pos.Y < 0:
		at = image.Pt(line(anchor.Min.X, anchor.Max.X, size.X), anchor.Min.Y-size.Y)
	case pos.Y > 0:
		at = image.Pt(line(anchor.Min.X, anchor.Max.X, size.X), anchor.Max.Y)
	case pos.X > 0:
		at = image.Pt(anchor.Max.X+1, line(anchor.Min.Y, anchor.Max.Y, size.Y))
	default:
		at = image.Pt(anchor.Min.X-1-size.X, line(anchor.Min.Y, anchor.Max.Y, size.Y))
	}
	return image.Rectangle{Min: at, Max: at.Add(size)}
}

// shift moves r inside the screen, or to its top-left corner when r is
// bigger.
func (s *Scene) shift(r image.Rectangle) image.Rectangle {
	scr := s.Screen
	d := image.Pt(
		max(min(r.Min.X, scr.Max.X-r.Dx()), scr.Min.X)-r.Min.X,
		max(min(r.Min.Y, scr.Max.Y-r.Dy()), scr.Min.Y)-r.Min.Y,
	)
	return r.Add(d)
}

// cost returns what placing a box at r hides, before its side's place in
// the order of preference.
func (s *Scene) cost(r image.Rectangle) int {
	if s.Cell == nil {
		return 0
	}
	r = r.Intersect(s.Screen)
	// Only the busy areas r meets are looked at for each cell.
	var near [8]image.Rectangle
	busy := near[:0]
	for _, b := range s.Busy {
		if b.Overlaps(r) {
			busy = append(busy, b)
		}
	}
	total := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := image.Pt(x, y)
			c := cellCost[s.Cell(p)]
			if c > 0 && slices.ContainsFunc(busy, p.In) {
				c *= 2
			}
			total += c
		}
	}
	return total
}

// distance returns how far the middle of r is from the middle of the cell
// at p, in half cells, across and down.
func distance(r image.Rectangle, p image.Point) int {
	abs := func(n int) int { return max(n, -n) }
	return abs(r.Min.X+r.Max.X-2*p.X-1) + abs(r.Min.Y+r.Max.Y-2*p.Y-1)
}
