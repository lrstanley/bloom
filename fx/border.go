// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image/color"
	"math"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

// The effects in this file change only the cells around the edge of their
// surface, which, given to a box's border effects, are its border. They
// loop: t is the phase, as from anim.NewLoop, and each looks the same at
// t=0 and t=1, so the loop is seamless.

// perimeter calls fn for each shown cell around the edge of s, clockwise from
// the top-left corner, with i its index along the edge, of n.
func perimeter(s *canvas.Surface, fn func(c *uv.Cell, i, n int)) {
	w, h := s.Width(), s.Height()
	if w <= 0 || h <= 0 {
		return
	}
	n := 2*(w+h) - 4
	switch {
	case h == 1:
		n = w
	case w == 1:
		n = h
	}
	visit := func(x, y, i int) {
		if s.Visible(x, y) {
			fn(s.Cell(x, y), i, n)
		}
	}
	for x := range w {
		visit(x, 0, x)
	}
	for y := 1; y < h; y++ {
		visit(w-1, y, w-1+y)
	}
	if w > 1 && h > 1 {
		for x := w - 2; x >= 0; x-- {
			visit(x, h-1, w+h-2+w-1-x)
		}
		for y := h - 2; y > 0; y-- {
			visit(0, y, 2*w+h-3+h-1-y)
		}
	}
}

// phase returns where t is in its loop, from 0 up to 1.
func phase(t float64) float64 { return t - math.Floor(t) }

// Chase colors the edge with a gradient from From to To and back, which
// travels clockwise around it as t goes from 0 to 1, such as a border with
// a rotating gradient.
type Chase struct {
	// From and To are the colors of the gradient; a nil From is each cell's
	// own color, and a nil To the terminal's foreground.
	From, To color.Color
	// Cycles is how many times the gradient repeats around the edge; zero
	// is 1. More cycles make a ripple of narrower bands.
	Cycles int
}

// Apply implements [Effect].
func (e Chase) Apply(s *canvas.Surface, t float64) {
	to, cycles, at := e.To, float64(max(e.Cycles, 1)), phase(t)
	if to == nil {
		to = s.Env.Fg
	}
	perimeter(s, func(c *uv.Cell, i, n int) {
		from := e.From
		if from == nil {
			from = s.Fg(c)
		}
		k := cycles*float64(i)/float64(n) - at
		c.Style.Fg = s.Mix(from, to, 0.5-0.5*math.Cos(2*math.Pi*k))
	})
}

// Pulse moves the edge's colors toward Color and back as t goes from 0 to 1,
// all of it at once, such as a border drawing attention to itself.
type Pulse struct {
	// Color is what the edge's colors move toward; nil is the terminal's
	// foreground. They reach it at t=0.5.
	Color color.Color
}

// Apply implements [Effect].
func (e Pulse) Apply(s *canvas.Surface, t float64) {
	to, f := e.Color, math.Sin(math.Pi*phase(t))
	if to == nil {
		to = s.Env.Fg
	}
	if f *= f; f <= 0 {
		return
	}
	perimeter(s, func(c *uv.Cell, _, _ int) {
		c.Style.Fg = s.Mix(s.Fg(c), to, f)
	})
}

// Comet sends a bright head with a fading tail once around the edge,
// clockwise from the top-left corner, as t goes from 0 to 1, such as a box
// that is busy.
type Comet struct {
	// Color is the color of the head; nil is the terminal's foreground.
	Color color.Color
	// Length is how many cells the tail fades over, head included; zero
	// is 8.
	Length int
}

// Apply implements [Effect].
func (e Comet) Apply(s *canvas.Surface, t float64) {
	to, length, at := e.Color, e.Length, phase(t)
	if to == nil {
		to = s.Env.Fg
	}
	if length <= 0 {
		length = 8
	}
	perimeter(s, func(c *uv.Cell, i, n int) {
		behind := math.Mod(at*float64(n)-float64(i)+float64(n), float64(n))
		if k := 1 - behind/float64(min(length, n)); k > 0 {
			c.Style.Fg = s.Mix(s.Fg(c), to, smoothstep(k))
		}
	})
}

// dashed is the dashed glyph for each line glyph that has one.
var dashed = map[string]string{"─": "╌", "━": "╍", "│": "╎", "┃": "╏"}

// March draws the edge as marching ants: runs of its line glyphs alternate
// with dashed ones, and travel clockwise around it by one run of each as t
// goes from 0 to 1. Corners, and glyphs with no dashed form, such as double
// lines, are left as they are.
type March struct {
	// Length is how many cells each run takes; zero is 2.
	Length int
}

// Apply implements [Effect].
func (e March) Apply(s *canvas.Surface, t float64) {
	length := e.Length
	if length <= 0 {
		length = 2
	}
	period := 2 * length
	shift := int(phase(t) * float64(period))
	perimeter(s, func(c *uv.Cell, i, _ int) {
		if ((i-shift)%period+period)%period < length {
			return
		}
		if d, ok := dashed[c.Content]; ok {
			c.Content = d
		}
	})
}
