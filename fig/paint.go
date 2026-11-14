// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"image/color"
	"reflect"

	"bloomfx.dev/bloom/internal/same"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Paint chooses how each cell of a [Figure] is drawn. Paints made by this
// package are comparable, so a figure remembers what it rendered with each;
// one that is not, such as a [PaintFunc], is rendered each time.
type Paint interface {
	// Shade returns the style of c, a cell of f.
	Shade(f *Figure, c Cell) uv.Style
}

// PaintFunc adapts a function to a [Paint].
type PaintFunc func(f *Figure, c Cell) uv.Style

// Shade implements [Paint].
func (p PaintFunc) Shade(f *Figure, c Cell) uv.Style { return p(f, c) }

// shows reports whether a cell shows a character a paint should color.
func shows(c Cell) bool { return c.Kind == KindStroke || c.Kind == KindSeam }

// Solid draws every character in one color.
func Solid(c color.Color) Paint { return solid{c: c} }

type solid struct{ c color.Color }

func (p solid) Shade(_ *Figure, c Cell) uv.Style {
	if !shows(c) {
		return uv.Style{}
	}
	return uv.Style{Fg: p.c}
}

// Axis is what a [GradientPaint] runs along.
type Axis uint8

const (
	// ByColumn runs from the left column to the right. It is the default.
	ByColumn Axis = iota
	// ByGlyph colors each glyph in one color, from the first glyph to the
	// last.
	ByGlyph
	// ByRow runs from the top row to the bottom.
	ByRow
	// ByDiagonal runs from the top left corner to the bottom right, at 45
	// degrees as the terminal draws it, where a cell is twice as tall as it
	// is wide.
	ByDiagonal
)

// maxStops is how many colors a [GradientPaint] or [Palette] holds.
const maxStops = 16

// GradientPaint draws characters in colors blended from one stop to the
// next. Build one with [Gradient].
type GradientPaint struct {
	stops [maxStops]color.Color
	n     int
	axis  Axis
	blend bool
}

// Gradient returns a paint blending from the first of stops to the last,
// across the columns unless changed with [GradientPaint.By]. Up to 16 stops
// are used.
func Gradient(stops ...color.Color) GradientPaint {
	var g GradientPaint
	g.n = copy(g.stops[:], stops)
	return g
}

// By sets what the gradient runs along.
func (g GradientPaint) By(a Axis) GradientPaint { g.axis = a; return g }

// Blend colors the cells where glyphs met with the glyphs' colors mixed,
// rather than in the color of the glyph whose character the cell shows. It
// only applies [ByGlyph], as along the other axes, a cell's color follows
// from where it is.
func (g GradientPaint) Blend() GradientPaint { g.blend = true; return g }

// Shade implements [Paint].
func (g GradientPaint) Shade(f *Figure, c Cell) uv.Style {
	if g.n == 0 || !shows(c) {
		return uv.Style{}
	}
	var t float64
	switch g.axis {
	case ByColumn:
		t = frac(c.X, f.w-1)
	case ByRow:
		t = frac(c.Y, f.h-1)
	case ByDiagonal:
		t = frac(c.X+2*c.Y, f.w-1+2*(f.h-1))
	case ByGlyph:
		t = frac(f.visible[c.Glyph], f.shown-1)
		if s := f.seamAt(c.X, c.Y); s != nil && c.Kind == KindSeam {
			a, b := frac(f.visible[s.Earlier], f.shown-1), frac(f.visible[s.Later], f.shown-1)
			switch {
			case g.blend:
				t = (a + b) / 2
			case s.Winner == Earlier:
				t = a
			default:
				t = b
			}
		}
	}
	return uv.Style{Fg: g.at(t)}
}

// at returns the gradient's color at t, from 0 to 1.
func (g GradientPaint) at(t float64) color.Color {
	if g.n == 1 {
		return g.stops[0]
	}
	t = min(max(t, 0), 1) * float64(g.n-1)
	i := min(int(t), g.n-2)
	return mix(g.stops[i], g.stops[i+1], t-float64(i))
}

func frac(i, n int) float64 {
	if n <= 0 {
		return 0
	}
	return float64(i) / float64(n)
}

// mix returns the color t of the way from a to b.
func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	ch := func(x, y uint32) uint8 { return uint8((float64(x>>8) + (float64(y>>8)-float64(x>>8))*t) + 0.5) } //nolint:gosec // channels are 8 bits.
	return color.RGBA{R: ch(ar, br), G: ch(ag, bg), B: ch(ab, bb), A: 0xff}
}

// PalettePaint draws each glyph in the next of a list of colors. Build one
// with [Palette].
type PalettePaint struct {
	colors [maxStops]color.Color
	n      int
}

// Palette returns a paint drawing each glyph that shows something in the next
// of colors, starting over after the last. Up to 16 colors are used.
func Palette(colors ...color.Color) PalettePaint {
	var p PalettePaint
	p.n = copy(p.colors[:], colors)
	return p
}

// Shade implements [Paint].
func (p PalettePaint) Shade(f *Figure, c Cell) uv.Style {
	if p.n == 0 || !shows(c) {
		return uv.Style{}
	}
	return uv.Style{Fg: p.colors[f.visible[c.Glyph]%p.n]}
}

// Rainbow returns TOIlet's rainbow filter: bright colors in diagonal bands.
func Rainbow() Paint { return rainbow{} }

type rainbow struct{}

var rainbowColors = [...]color.Color{ansi.BrightMagenta, ansi.BrightRed, ansi.BrightYellow, ansi.BrightGreen, ansi.BrightCyan, ansi.BrightBlue}

func (rainbow) Shade(_ *Figure, c Cell) uv.Style {
	if !shows(c) {
		return uv.Style{}
	}
	return uv.Style{Fg: rainbowColors[(c.X/2+c.Y)%len(rainbowColors)]}
}

// Metal returns TOIlet's metal filter: shades of blue and gray in slanted
// bands.
func Metal() Paint { return metal{} }

type metal struct{}

var metalColors = [...]color.Color{ansi.BrightBlue, ansi.Blue, ansi.White, ansi.BrightBlack}

func (metal) Shade(_ *Figure, c Cell) uv.Style {
	if !shows(c) {
		return uv.Style{}
	}
	return uv.Style{Fg: metalColors[((c.Y+c.X/8)/2)%len(metalColors)]}
}

// Fill returns p with the blanks inside letters, such as the middle of an
// "O", filled with color inside, which makes outlined fonts look solid. p may
// be nil to fill without coloring characters.
func Fill(inside color.Color, p Paint) Paint { return fill{inside: inside, p: p} }

type fill struct {
	inside color.Color
	p      Paint
}

func (p fill) Shade(f *Figure, c Cell) uv.Style {
	if c.Kind == KindInside {
		return uv.Style{Bg: p.inside}
	}
	if p.p == nil {
		return uv.Style{}
	}
	return p.p.Shade(f, c)
}

// cacheable reports whether p can be a map key, and cannot change without
// changing p itself, so that what it rendered can be kept.
func cacheable(p Paint) bool {
	return same.Fixed(reflect.ValueOf(p))
}
