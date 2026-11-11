// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"cmp"
	"image"
	"image/color"
	"math"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

// decodeGlyphs are what the leading cell of a [Decode] column shows.
const decodeGlyphs = `0123456789ABCDEF:=+*<>|/\`

// decodeSteps is how many times per transition the leading glyphs change.
const decodeSteps = 48

// Decode reveals a node like digital rain settling into text: each lane, a
// column for rain from the top or bottom, a row from the left or right, or a
// diagonal from a corner, is revealed from From after a random delay. The
// leading cell of a lane shows a bright random glyph, the cells behind it
// their own content, and those ahead of it nothing. The delays are the same
// for the same Seed. Only single-width glyphs are replaced.
type Decode struct {
	// From is where the rain starts. The zero value is Top.
	From Direction
	Seed uint32
}

// Apply implements [Effect].
func (e Decode) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	sw := newSweep(cmp.Or(e.From, Top), s.Width(), s.Height())
	// The front runs a stride past the last cell, so each lane's leading
	// glyph leaves it before t=1.
	stride := float64(sw.stride())
	span := float64(sw.steps()) + stride
	step := uint32(max(t, 0) * decodeSteps)

	// Every lane finishes by t=1, the later it starts the faster it falls.
	lo, lanes := sw.lanes()
	var buf [512]float64
	fronts := buf[:0]
	if lanes > len(buf) {
		fronts = make([]float64, 0, lanes)
	}
	for i := range lanes {
		delay := hash(lo+i, -1, e.Seed) * 0.5
		fronts = append(fronts, (t-delay)/(1-delay)*span)
	}

	for y := range s.Height() {
		a, dAlong, lane, dLane := sw.row(y)
		for x := range s.Width() {
			front, along := fronts[lane-lo], float64(a)
			a, lane = a+dAlong, lane+dLane
			if along+stride <= front || !s.Visible(x, y) {
				continue
			}
			if along > front {
				s.Hide(x, y)
				continue
			}
			c := s.Cell(x, y)
			if c.Width != 1 {
				continue
			}
			c.Content = flicker(decodeGlyphs, x, y, e.Seed, step)
			c.Style.Fg = s.Env.Fg
			c.Style.Attrs |= uv.AttrBold
		}
	}
}

// glitchSteps is how many times per transition the glitch pattern changes.
const glitchSteps = 16

var (
	glitchRed  color.Color = color.RGBA{R: 0xff, G: 0x30, B: 0x50, A: 0xff}
	glitchCyan color.Color = color.RGBA{R: 0x20, G: 0xe0, B: 0xff, A: 0xff}
)

// Glitch shows a node as if the signal were breaking up: rows jitter
// sideways in short bursts, their colors split toward red and cyan, and some
// cells are hidden or garbled. The pattern changes in steps, so it flickers,
// and fades to nothing as t reaches 1. It is the same for the same Seed. Rows
// holding wide glyphs are not moved.
type Glitch struct {
	Seed uint32
}

// Apply implements [Effect].
func (e Glitch) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	step := uint32(max(t, 0) * glitchSteps)
	amount := 1 - float64(step)/glitchSteps
	seed := stepSeed(e.Seed, step)
	w := s.Width()

	for y := range s.Height() {
		if hash(-1, y, seed) < amount*0.4 {
			shift := 1 + int(hash(-2, y, seed)*amount*6)
			if hash(-3, y, seed) < 0.5 {
				shift = -shift
			}
			s.Move(image.Rect(0, y, w, y+1), image.Pt(shift, 0))
			split := glitchRed
			if y%2 != 0 {
				split = glitchCyan
			}
			for x := range w {
				if s.Visible(x, y) {
					c := s.Cell(x, y)
					c.Style.Fg = s.Mix(s.Fg(c), split, 0.3+0.5*amount)
				}
			}
		}
		for x := range w {
			if !s.Visible(x, y) {
				continue
			}
			u := hash(x, y, seed)
			switch c := s.Cell(x, y); {
			case u < amount*0.03:
				s.Hide(x, y)
			case u < amount*0.08 && c.Width == 1 && c.Content != " ":
				c.Content = shades[int(hash(y, x, seed)*float64(len(shades)))]
			}
		}
	}
}

// Shake moves a node from side to side with a decaying wobble, such as an
// input that rejected what was typed. t is the shake's progress: the node is
// still at 0 and at 1.
type Shake struct {
	// Amount is how far, in cells, the node moves at most; zero is 2.
	Amount int
	// Times is how many times the node swings each way; zero is 4.
	Times int
}

// Apply implements [Effect].
func (e Shake) Apply(s *canvas.Surface, t float64) {
	amount, times := e.Amount, e.Times
	if amount == 0 {
		amount = 2
	}
	if times <= 0 {
		times = 4
	}
	if t <= 0 || t >= 1 {
		return
	}
	s.Offset.X += int(math.Round(float64(amount) * math.Sin(2*math.Pi*float64(times)*t) * (1 - t)))
}

// Shimmer sweeps a highlight band across a node, diagonally by default, for
// placeholders shown while content loads. t is the band's position: it starts just off the
// node and ends just off the other side, so a value that runs from 0 to 1 and
// then jumps back to 0 loops seamlessly.
type Shimmer struct {
	// Color is what colors are mixed toward in the band; nil is the
	// terminal's foreground.
	Color color.Color
	// Width is how wide the band is, in columns, a row counting as two; zero
	// is 6.
	Width int
	// From is where the band starts. From a side, the band is parallel to
	// it, and from a corner it is diagonal. The zero value is TopLeft.
	From Direction
}

// shimmerStrength is how far colors in the middle of the band move toward
// its color.
const shimmerStrength = 0.6

// Apply implements [Effect].
func (e Shimmer) Apply(s *canvas.Surface, t float64) {
	to, width := e.Color, e.Width
	if to == nil {
		to = s.Env.Fg
	}
	if width <= 0 {
		width = 6
	}
	half := float64(width) / 2
	// The band's center travels across the surface in columns, rows counting
	// double, from just before the start to just past the far side. Only the
	// cells of a row within half of it are touched.
	sw := newSweep(cmp.Or(e.From, TopLeft), s.Width(), s.Height())
	center := -half + min(max(t, 0), 1)*(sw.extent()+float64(width))
	for y := range s.Height() {
		along, dAlong, _, _ := sw.row(y)
		lo, hi := 0, s.Width()-1
		if dAlong == 0 {
			// From the top or bottom, the band covers whole rows.
			if math.Abs(sw.cols*float64(along)-center) >= half {
				continue
			}
		} else {
			// The cells whose sweep step is within the band's reach, from the
			// start of the row, which is the first or the last cell.
			near, far := (center-half)/sw.cols-float64(along), (center+half)/sw.cols-float64(along)
			if dAlong < 0 {
				near, far = -far, -near
			}
			lo, hi = max(lo, int(math.Ceil(near))), min(hi, int(math.Floor(far)))
		}
		for x := lo; x <= hi; x++ {
			if !s.Visible(x, y) {
				continue
			}
			u := sw.cols * float64(along+dAlong*x)
			k := 1 - math.Abs(u-center)/half
			if k <= 0 {
				continue
			}
			f := shimmerStrength * smoothstep(k)
			c := s.Cell(x, y)
			c.Style.Fg = s.Mix(s.Fg(c), to, f)
			c.Style.Bg = s.Mix(s.Bg(c), to, f)
		}
	}
}
