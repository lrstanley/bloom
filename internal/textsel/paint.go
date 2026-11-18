// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"image"
	"image/color"
	"math"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	// minContrast is the least contrast, as WCAG measures it, that a
	// selected cell's own foreground is kept at against the selection's
	// background.
	minContrast = 3
	// selectionTint is how far toward the terminal's default foreground the
	// default selection background is from its default background.
	selectionTint = 0.3
)

// Style is how selected cells are drawn, as a style given for it is
// applied to them: with its foreground and background, those it sets, and
// its attributes. Without Set, cells get a background between the
// terminal's default background and foreground.
type Style struct {
	Set       bool
	Fg, Bg    color.Color
	Attrs     uint8
	Underline bool
	Reverse   bool
}

// NewStyle returns the Style of the cells of style c, or the default one
// when not set.
func NewStyle(c uv.Style, set bool) Style {
	s := Style{Set: set}
	if !set {
		return s
	}
	s.Fg, s.Bg, s.Attrs = c.Fg, c.Bg, c.Attrs&^uv.AttrReverse
	s.Underline, s.Reverse = c.Underline != 0, c.Attrs&uv.AttrReverse != 0
	return s
}

// Painter draws cells as selected, in Style, in a terminal whose default
// colors are those of an env, mixing colors with a mixer.
type Painter struct {
	Style Style
	memo  readMemo
}

// Highlight draws cell c as selected.
func (p *Painter) Highlight(c *uv.Cell, env *canvas.Env, mixer *canvas.Mixer) {
	s := &p.Style
	c.Style.Attrs &^= uv.AttrReverse
	if s.Reverse && s.Fg == nil && s.Bg == nil {
		c.Style.Attrs |= uv.AttrReverse | s.Attrs
		return
	}
	switch {
	case s.Bg != nil:
		c.Style.Bg = s.Bg
	case !s.Set:
		c.Style.Bg = mixer.Mix(env.Bg, env.Fg, selectionTint)
	}
	c.Style.Attrs |= s.Attrs
	if s.Underline {
		c.Style.Underline = uv.UnderlineSingle
	}
	if s.Fg != nil {
		c.Style.Fg = s.Fg
		return
	}
	p.readable(c, env)
}

// readable gives cell c the terminal's default foreground or background,
// whichever reads better, when its own foreground is hard to read on its
// background.
func (p *Painter) readable(c *uv.Cell, env *canvas.Env) {
	fg, bg := c.Style.Fg, c.Style.Bg
	if fg == nil {
		fg = env.Fg
	}
	if bg == nil {
		bg = env.Bg
	}
	m := &p.memo
	key := [4][4]uint32{rgbaOf(fg), rgbaOf(bg), rgbaOf(env.Fg), rgbaOf(env.Bg)}
	if !m.ok || m.key != key {
		m.key, m.ok, m.swap = key, true, readKeep
		if Contrast(fg, bg) < minContrast {
			m.swap = readFg
			if Contrast(env.Bg, bg) > Contrast(env.Fg, bg) {
				m.swap = readBg
			}
		}
	}
	switch m.swap {
	case readFg:
		c.Style.Fg = env.Fg
	case readBg:
		c.Style.Fg = env.Bg
	case readKeep:
	}
}

// readMemo is what [Painter.readable] decided for the colors it was last
// given, as the cells a selection covers tend to share them, and working out
// their contrast is slow.
type readMemo struct {
	key  [4][4]uint32 // the foreground, background, and the terminal's two
	swap readSwap
	ok   bool
}

// readSwap is the foreground [Painter.readable] gives a cell.
type readSwap uint8

const (
	readKeep readSwap = iota // its own
	readFg                   // the terminal's foreground
	readBg                   // the terminal's background
)

func rgbaOf(c color.Color) [4]uint32 {
	r, g, b, a := c.RGBA()
	return [4]uint32{r, g, b, a}
}

// Flash draws cell c, at at, as a band swept across area draws it at t,
// from 0 to 1, such as over the text just copied: as selected where the
// band is, fading out to either side. The band runs diagonally, from the
// top-left of area to its bottom-right.
func (p *Painter) Flash(c *uv.Cell, at image.Point, t float64, area image.Rectangle, env *canvas.Env, mixer *canvas.Mixer) {
	// A cell is about twice as tall as it is wide, so a row counts as two
	// columns for the band to lean at 45 degrees.
	diagonal := func(p image.Point) float64 { return float64(p.X + 2*p.Y) }
	start, end := diagonal(area.Min), diagonal(area.Max.Sub(image.Pt(1, 1)))
	width := max((end-start)/4, 4)
	center := start - width + t*(end-start+2*width)
	k := 1 - math.Abs(diagonal(at)-center)/width
	if k <= 0 {
		return
	}
	plain := c.Style
	p.Highlight(c, env, mixer)
	if k >= 0.5 {
		return
	}
	// The band's edges blend the background back to the cell's own.
	from, to := plain.Bg, c.Style.Bg
	if from == nil {
		from = env.Bg
	}
	if to == nil {
		to = env.Bg
	}
	c.Style = plain
	c.Style.Bg = mixer.Mix(from, to, k*2)
	p.readable(c, env)
}
