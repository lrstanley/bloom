// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package fx has cell effects: functions that transform a node's rendered
// cells, or the frame beneath a dialog, for a progress value.
//
// The same [Effect] serves two purposes. Given to a transition, it is applied
// to the node's own cells, and t runs from 0, as the node starts to enter, to
// 1, when it is fully shown; at 1 a transition effect leaves the cells as
// they are. Given to a backdrop, it is applied to everything drawn beneath a
// layer, and t is the strength, following the layer's transition.
//
// See the [documentation] for guides and examples. It is also available for
// LLMs and agents as [llms.txt].
//
// [documentation]: https://bloomfx.dev/
// [llms.txt]: https://bloomfx.dev/llms.txt
package fx

import (
	"image/color"
	"math"

	"github.com/lrstanley/bloom/canvas"
)

// Effect transforms the cells of a surface for progress t, from 0 to 1.
//
// The layout engine skips compositing a frame identical to the previous one,
// which it decides by comparing effects with ==. Effects should be comparable
// values, as all of this package's are; effects that are not, and effects
// behind pointers, which can change in place, are assumed to change on every
// frame.
type Effect interface {
	Apply(s *canvas.Surface, t float64)
}

// Func adapts a function to an [Effect].
type Func func(s *canvas.Surface, t float64)

// Apply implements [Effect].
func (f Func) Apply(s *canvas.Surface, t float64) { f(s, t) }

// Direction is a side or a corner of a node or of the screen, such as where
// an effect starts from. What a corner means is up to what it is given to,
// which each documents: a front that sweeps across a node from a corner runs
// diagonally, at 45 degrees as the terminal draws it, where a cell is twice
// as tall as it is wide.
//
// The zero value is the default of whatever it is given to, which each
// documents.
type Direction uint8

const (
	_ Direction = iota // the default.

	// Bottom is the bottom side.
	Bottom
	// Top is the top side.
	Top
	// Left is the left side.
	Left
	// Right is the right side.
	Right
	// TopLeft is the top left corner.
	TopLeft
	// TopRight is the top right corner.
	TopRight
	// BottomLeft is the bottom left corner.
	BottomLeft
	// BottomRight is the bottom right corner.
	BottomRight
)

// Vector returns the way from d's side or corner toward the opposite one,
// as -1, 0, or 1 on each axis, y growing down: (1, 0) from Left, and (-1, -1)
// from BottomRight. The zero value has no way, (0, 0).
func (d Direction) Vector() (dx, dy int) {
	switch d {
	case Bottom:
		return 0, -1
	case Top:
		return 0, 1
	case Left:
		return 1, 0
	case Right:
		return -1, 0
	case TopLeft:
		return 1, 1
	case TopRight:
		return -1, 1
	case BottomLeft:
		return 1, -1
	case BottomRight:
		return -1, -1
	}
	return 0, 0
}

// sweep measures cells of a w×h surface by how far a front starting at a
// side or corner, and travelling to the opposite one, has to go to reach
// them. Along a side, a cell is a step; across a corner, a column is a step
// and a row two, so the front runs at 45 degrees as the terminal draws it.
//
// Measures are taken from lx and ly, a cell's column and row counted from
// the side or corner the sweep starts at. Effects that measure in columns,
// with rows counting double, convert steps and lanes with cols and laneCols.
type sweep struct {
	dx, dy, w, h int
	ax, ay       int // along = ax*lx + ay*ly
	px, py       int // lane = px*lx + py*ly

	// How many columns a step along, and a lane across, are. From a corner
	// both are already counted in columns.
	cols, laneCols float64
}

func newSweep(d Direction, w, h int) sweep {
	s := sweep{w: w, h: h}
	s.dx, s.dy = d.Vector()
	switch {
	case s.dx != 0 && s.dy != 0:
		s.ax, s.ay, s.px, s.py = 1, 2, 1, -2
		s.cols, s.laneCols = 1, 1
	case s.dx != 0:
		s.ax, s.py = 1, 1
		s.cols, s.laneCols = 1, 2
	default:
		s.ay, s.px = 1, 1
		s.cols, s.laneCols = 2, 1
	}
	return s
}

// diagonal reports whether the sweep starts at a corner.
func (s sweep) diagonal() bool { return s.dx != 0 && s.dy != 0 }

// local returns x and y counted from the side or corner the sweep starts at.
func (s sweep) local(x, y int) (lx, ly int) {
	if s.dx < 0 {
		x = s.w - 1 - x
	}
	if s.dy < 0 {
		y = s.h - 1 - y
	}
	return x, y
}

// row returns, for the first cell of row y, how many steps from the start
// it is, ax*lx + ay*ly, and which lane it is on, px*lx + py*ly: its column
// for a sweep from the top or bottom, its row for one from a side, and its
// diagonal for one from a corner. It also returns how much each changes from
// one cell of the row to the next, so that a loop over the row can keep them
// by adding.
func (s sweep) row(y int) (along, dAlong, lane, dLane int) {
	lx, ly := s.local(0, y)
	along, lane = s.ax*lx+s.ay*ly, s.px*lx+s.py*ly
	dAlong, dLane = s.ax, s.px
	if s.dx < 0 {
		dAlong, dLane = -dAlong, -dLane
	}
	return along, dAlong, lane, dLane
}

// steps returns how many steps the sweep takes to cover the surface: one
// more than the farthest cell's.
func (s sweep) steps() int { return s.ax*(s.w-1) + s.ay*(s.h-1) + 1 }

// extent returns how far, in columns, the front travels from the start to
// the far side of the last cell.
func (s sweep) extent() float64 { return s.cols * float64(s.ax*s.w+s.ay*s.h) }

// mid returns how far, in columns, the middle of a cell along steps from the
// start is.
func (s sweep) mid(along int) float64 { return s.cols * (float64(along) + float64(s.ax+s.ay)/2) }

// laneMid returns where, in columns across the front, the middles of the
// cells of a lane are.
func (s sweep) laneMid(lane int) float64 {
	return s.laneCols * (float64(lane) + float64(s.px+s.py)/2)
}

// lanes returns the lowest lane, and how many lanes there are.
func (s sweep) lanes() (lo, n int) {
	lo = min(s.px*(s.w-1), 0) + min(s.py*(s.h-1), 0)
	hi := max(s.px*(s.w-1), 0) + max(s.py*(s.h-1), 0)
	return lo, hi - lo + 1
}

// stride returns how many steps apart neighboring cells of a lane are.
func (s sweep) stride() int {
	if s.diagonal() {
		return 4
	}
	return 1
}

// each calls fn for every shown cell of s.
func each(s *canvas.Surface, fn func(x, y int)) {
	for y := range s.Height() {
		for x := range s.Width() {
			if s.Visible(x, y) {
				fn(x, y)
			}
		}
	}
}

// recolor sets the colors of every shown cell of s from its current colors,
// resolved to the terminal's defaults when unset. A nil fg or bg leaves that
// color alone, and keepUnsetBg leaves unset backgrounds unset.
func recolor(s *canvas.Surface, fg, bg func(color.Color) color.Color, keepUnsetBg bool) {
	each(s, func(x, y int) {
		c := s.Cell(x, y)
		if fg != nil {
			c.Style.Fg = fg(s.Fg(c))
		}
		if bg != nil && (c.Style.Bg != nil || !keepUnsetBg) {
			c.Style.Bg = bg(s.Bg(c))
		}
	})
}

// hash returns a well-mixed value in [0, 1) for a cell and seed.
func hash(x, y int, seed uint32) float64 {
	h := uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xc2b2ae3d27d4eb4f ^ uint64(seed)*0x165667b19e3779f9 //nolint:gosec // wrapping is fine for a hash.
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return float64(h>>11) / (1 << 53)
}

func lerpInt(a, b int, t float64) int { return int(math.Round(float64(a) + float64(b-a)*t)) }
