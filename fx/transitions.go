// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"cmp"
	"image"

	"github.com/lrstanley/bloom/canvas"
)

// Fade crossfades a node in over the frame beneath it. Backgrounds blend
// from the one beneath to the node's own; until t=0.5 each cell shows the
// glyph beneath, fading into that background, and after it the node's own,
// fading in. At t=0 the node looks exactly like what is beneath it. Cells
// over or holding wide glyphs keep the node's glyph throughout.
type Fade struct{}

// Apply implements [Effect].
func (Fade) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	for at, c := range s.Cells() {
		under := s.Under(at.X, at.Y)
		bg := s.Mix(s.Bg(under), s.Bg(c), t)
		if t < 0.5 && under != nil && under.Width == 1 && c.Width == 1 {
			c.Content, c.Link = under.Content, under.Link
			c.Style.Attrs, c.Style.Underline = under.Style.Attrs, under.Style.Underline
			c.Style.Fg = s.Mix(s.Fg(under), bg, 2*t)
		} else {
			c.Style.Fg = s.Mix(bg, s.Fg(c), max(2*t-1, 0))
		}
		c.Style.Bg = bg
	}
}

// Slide moves a node in from beyond a side or corner of its viewport, such
// as a dialog rising from the bottom of the screen.
type Slide struct {
	// From is where the node comes from. The zero value is Bottom.
	From Direction
}

// Apply implements [Effect].
func (e Slide) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	r, vp := s.Rect(), s.Viewport
	// The node moves against the way from its side or corner.
	dx, dy := cmp.Or(e.From, Bottom).Vector()
	var off image.Point
	switch {
	case dx > 0:
		off.X = lerpInt(vp.Min.X-r.Max.X, 0, t)
	case dx < 0:
		off.X = lerpInt(vp.Max.X-r.Min.X, 0, t)
	}
	switch {
	case dy > 0:
		off.Y = lerpInt(vp.Min.Y-r.Max.Y, 0, t)
	case dy < 0:
		off.Y = lerpInt(vp.Max.Y-r.Min.Y, 0, t)
	}
	s.Offset = s.Offset.Add(off)
}

// Wipe reveals a node from one side or corner to the opposite one.
type Wipe struct {
	// From is where the reveal starts. The zero value is Bottom.
	From Direction
}

// Apply implements [Effect].
func (e Wipe) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	sw := newSweep(cmp.Or(e.From, Bottom), s.Width(), s.Height())
	shown := int(float64(sw.steps()) * t)
	for y := range s.Height() {
		along, dAlong, _, _ := sw.row(y)
		for x := range s.Width() {
			if along >= shown && s.Visible(x, y) {
				s.Hide(x, y)
			}
			along += dAlong
		}
	}
}

// Dissolve reveals a node's cells in a random order, the same for the same
// Seed.
type Dissolve struct {
	Seed uint32
}

// Apply implements [Effect].
func (e Dissolve) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	for at := range s.Cells() {
		x, y := at.X, at.Y
		if hash(x, y, e.Seed) >= t {
			s.Hide(x, y)
		}
	}
}

// Typewriter reveals a node's cells in reading order.
type Typewriter struct{}

// Apply implements [Effect].
func (Typewriter) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	shown := int(float64(s.Width()*s.Height()) * t)
	for at := range s.Cells() {
		x, y := at.X, at.Y
		if y*s.Width()+x >= shown {
			s.Hide(x, y)
		}
	}
}

// scrambleGlyphs are what [Scramble] shows in place of a cell's own glyph.
const scrambleGlyphs = `!#$%&*+-/<=>?@\^_|~0123456789`

// Scramble shows random glyphs that settle into a node's own, in a random
// order, the same for the same Seed. Only single-width glyphs are scrambled.
type Scramble struct {
	Seed uint32
}

// scrambleSteps is how many times per transition the random glyphs change.
const scrambleSteps = 24

// Apply implements [Effect].
func (e Scramble) Apply(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	step := uint32(t * scrambleSteps)
	for at, c := range s.Cells() {
		x, y := at.X, at.Y
		if c.Width != 1 || c.Content == " " || hash(x, y, e.Seed) < t {
			continue
		}
		c.Content = flicker(scrambleGlyphs, x, y, e.Seed, step)
	}
}
