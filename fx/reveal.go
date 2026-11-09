// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"cmp"
	"image"
	"math"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/lrstanley/bloom/canvas"
)

// Unfold opens a node from its middle row outward, like a CRT turning on. A
// band of rows grows from the middle to the full height; its top row shows
// the node's first row and its bottom row the node's last, so a box's
// borders are drawn from the start, and the rows between show the node's
// middle.
type Unfold struct{}

// Apply implements [Effect].
func (Unfold) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	w, h := s.Width(), s.Height()
	if t <= 0 || h == 0 {
		for at := range s.Cells() {
			s.Hide(at.X, at.Y)
		}
		return
	}
	band := min(1+int(float64(h-1)*t), h)
	top := (h - band) / 2
	bottom := top + band - 1
	if band > 1 {
		// The first and last rows move to the band's edges; the rows moved
		// past them are dropped, and those left behind are outside the band.
		s.Move(image.Rect(0, 0, w, top+1), image.Pt(0, top))
		s.Move(image.Rect(0, bottom, w, h), image.Pt(0, bottom-(h-1)))
	}
	for y := range h {
		if y >= top && y <= bottom {
			continue
		}
		for x := range w {
			s.Hide(x, y)
		}
	}
}

// Iris reveals a node with a circle that grows from At until it covers the
// node, such as a dialog opening from the button that was pressed.
type Iris struct {
	// At is the circle's center, in frame coordinates, such as the rectangle
	// of a button from the previous frame. The zero point means the center of
	// the node.
	At image.Point
}

// Apply implements [Effect].
func (e Iris) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	w, h := s.Width(), s.Height()
	cx, cy := float64(w)/2, float64(h)/2
	if e.At != (image.Point{}) {
		at := e.At.Sub(s.Rect().Min)
		cx, cy = float64(at.X)+0.5, float64(at.Y)+0.5
	}
	// Cells are about twice as tall as they are wide, so rows count double
	// for the circle to look round.
	farX, farY := max(cx, float64(w)-cx), 2*max(cy, float64(h)-cy)
	r2 := (farX*farX + farY*farY) * max(t, 0) * max(t, 0)
	for at := range s.Cells() {
		x, y := at.X, at.Y
		dx, dy := float64(x)+0.5-cx, 2*(float64(y)+0.5-cy)
		if dx*dx+dy*dy >= r2 {
			s.Hide(x, y)
		}
	}
}

// Shade reveals a node's cells through shades of the frame beneath: each
// cell steps from blank to ░, ▒, ▓ and then its own glyph, the cells starting
// at random times, the same for the same Seed. Shades are drawn in the cell's
// background color, or its foreground color if it has none, over the
// background beneath, so a node dithers in. Wide glyphs, and blanks without a
// background, appear in one step, at the end of their cell's turn.
type Shade struct {
	Seed uint32
}

// shadeWindow is how much of a transition each cell spends stepping through
// its shades.
const shadeWindow = 0.4

// Apply implements [Effect].
func (e Shade) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	for at, c := range s.Cells() {
		x, y := at.X, at.Y
		if c.Width == 0 {
			if x > 0 && !s.Visible(x-1, y) {
				s.Hide(x, y)
			}
			continue
		}
		p := (t - hash(x, y, e.Seed)*(1-shadeWindow)) / shadeWindow
		if p >= 1 {
			continue
		}
		step := int(max(p, 0) * float64(len(shades)+1))
		if step == 0 || c.Width > 1 || (c.Content == " " && c.Style.Bg == nil) {
			s.Hide(x, y)
			continue
		}
		ink := c.Style.Bg
		if ink == nil {
			ink = s.Fg(c)
		}
		c.Content, c.Link = shades[step-1], uv.Link{}
		c.Style.Attrs, c.Style.Underline = 0, 0
		c.Style.Fg, c.Style.Bg = ink, s.Bg(s.Under(x, y))
	}
}

// Blinds reveals a node through slats, like opening window blinds: it is cut
// into slats of Size rows or columns, and every slat is revealed from its
// leading edge at the same time.
type Blinds struct {
	// From is the side or corner slats open from: Top and Bottom cut the node
	// into rows, Left and Right into columns, and a corner into diagonal
	// slats, Size columns across. The slats are counted from there. The zero
	// value is Bottom.
	From Direction
	// Size is the number of rows or columns in a slat. Zero means 3.
	Size int
}

// Apply implements [Effect].
func (e Blinds) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	size := e.Size
	if size <= 0 {
		size = 3
	}
	sw := newSweep(cmp.Or(e.From, Bottom), s.Width(), s.Height())
	shown := int(float64(size) * max(t, 0))
	for y := range s.Height() {
		// How far into its slat each cell is, kept by adding, as a step
		// along is -1, 0, or 1.
		along, dAlong, _, _ := sw.row(y)
		slat := along % size
		for x := range s.Width() {
			if slat >= shown && s.Visible(x, y) {
				s.Hide(x, y)
			}
			slat += dAlong
			if slat == size {
				slat = 0
			} else if slat < 0 {
				slat = size - 1
			}
		}
	}
}

// Wave reveals a node behind a front that sweeps in from a side or corner,
// wobbling like a wave. The cells just behind the front form a crest, mixed
// toward the terminal's foreground, which settles into their own colors.
type Wave struct {
	// From is where the front starts. The zero value is Bottom.
	From Direction
}

const (
	waveCrest  = 6.0  // how far behind the front the crest reaches, in columns
	waveLength = 18.0 // distance between the peaks of the front, in columns
	waveSpeed  = 0.75 // how many times the wobble cycles over a transition
)

// Apply implements [Effect].
func (e Wave) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	w, h := s.Width(), s.Height()
	sw := newSweep(cmp.Or(e.From, Bottom), w, h)

	// Measures are in columns, so rows count double, from the middle of a
	// cell, counted from the side or corner the front starts at. From a
	// corner, the front and the way it travels are turned 45 degrees.
	// Measures are in columns, from the middle of a cell. From a corner they
	// are taken square to the front, which runs at 45 degrees, so they are
	// shorter than along a row by √2.
	norm := 1.0
	if sw.diagonal() {
		norm = 1 / math.Sqrt2
	}
	length := sw.extent() * norm
	// How far a cell's middle is is linear in its sweep step.
	scale, shift := sw.cols*norm, sw.mid(0)*norm
	amp := min(3, length/6)
	front := -amp + max(t, 0)*(length+2*amp+waveCrest)
	phase := t * waveSpeed * 2 * math.Pi

	// The wobble of the front depends only on how far across it a cell is,
	// which is the same for every cell of a lane.
	lo, lanes := sw.lanes()
	var buf [512]float64
	wobble := buf[:0]
	if lanes > len(buf) {
		wobble = make([]float64, 0, lanes)
	}
	for i := range lanes {
		across := sw.laneMid(lo+i) * norm
		wobble = append(wobble, amp*math.Sin(across*2*math.Pi/waveLength+phase))
	}

	glow := s.Env.Fg
	for y := range h {
		along, dAlong, lane, dLane := sw.row(y)
		for x := range w {
			behind := front + wobble[lane-lo] - (float64(along)*scale + shift)
			along, lane = along+dAlong, lane+dLane
			if !s.Visible(x, y) {
				continue
			}
			switch {
			case behind < 0:
				s.Hide(x, y)
			case behind < waveCrest:
				c := s.Cell(x, y)
				strength := 1 - behind/waveCrest
				strength *= strength
				c.Style.Fg = s.Mix(s.Fg(c), glow, 0.85*strength)
				c.Style.Bg = s.Mix(s.Bg(c), glow, 0.4*strength)
			}
		}
	}
}
