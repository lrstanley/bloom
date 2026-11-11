// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"image"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// renderLimit is how many renderings a figure keeps, besides room for every
// glyph's layers in one paint.
const renderLimit = 64

type renderKey struct {
	p     Paint
	glyph int // -1 for the whole figure
	rest  bool
}

// Render returns the figure with ANSI styles from p, for Lip Gloss or any
// other program that prints strings. A nil p is the same as [Figure.String].
func (f *Figure) Render(p Paint) string {
	if p == nil {
		return f.cached(renderKey{glyph: -1}, f.String)
	}
	return f.cached(renderKey{p: p, glyph: -1}, func() string {
		c := canvas.New(f.w, f.h, ansi.GraphemeWidth)
		for y := range f.h {
			for x := range f.w {
				if f.cells[y*f.w+x].s != subCont {
					f.draw(c, x, y, f.At(x, y), p)
				}
			}
		}
		return c.Render()
	})
}

// draw sets cell x, y of c to fc, styled by p.
func (f *Figure) draw(c *canvas.Canvas, x, y int, fc Cell, p Paint) {
	uc := uv.Cell{Content: fc.Content, Width: max(ansi.StringWidth(fc.Content), 1)}
	if p != nil {
		uc.Style = p.Shade(f, fc)
	}
	c.SetCell(x, y, &uc)
}

func (f *Figure) cached(k renderKey, fn func() string) string {
	f.mu.Lock()
	if k.p != f.fixed {
		if !cacheable(k.p) {
			f.mu.Unlock()
			return fn()
		}
		f.fixed = k.p
	}
	s, ok := f.renders[k]
	f.mu.Unlock()
	if ok {
		return s
	}
	s = fn()
	f.mu.Lock()
	if f.renders == nil || len(f.renders) >= renderLimit+2*len(f.glyphs) {
		f.renders = make(map[renderKey]string, 8)
	}
	f.renders[k] = s
	f.mu.Unlock()
	return s
}

// layer returns what glyph i draws on its own, and where: its own
// sub-characters, the blanks inside it, and, when rest is true, the cells
// where it is drawn on top of glyphs it met, as the figure shows them. Drawn
// over one another in [Figure.order], the layers of a figure at rest make
// the figure itself. That holds for fonts whose glyphs' rows are all as
// wide, as every bundled font's are: rows of different widths, which figlet
// joins as C strings, can be split or cut in ways the layers do not follow.
func (f *Figure) layer(i int, p Paint, rest bool) (string, image.Point) {
	r := f.glyphs[i].Rect
	if r.Empty() {
		return "", image.Point{}
	}
	s := f.cached(renderKey{p: p, glyph: i, rest: rest}, func() string {
		c := canvas.New(r.Dx(), r.Dy(), ansi.GraphemeWidth)
		for _, sc := range f.strokes(f.marks[i]).cells {
			if sc.s == subCont {
				continue
			}
			kind := KindStroke
			if sc.s == subHard {
				kind = KindHardblank
			}
			f.draw(c, sc.x-r.Min.X, sc.y-r.Min.Y, Cell{X: sc.x, Y: sc.y, Content: f.content(sc.s), Kind: kind, Glyph: i}, p)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				fc := f.cells[y*f.w+x]
				switch {
				case fc.s == subCont:
					continue
				case fc.kind == KindSeam && int(fc.g) == i && rest:
				case fc.kind == KindInside && int(fc.g) == i:
				default:
					continue
				}
				f.draw(c, x-r.Min.X, y-r.Min.Y, f.At(x, y), p)
			}
		}
		return c.Render()
	})
	return s, r.Min
}

// order returns the glyphs in the order their layers are drawn, the one on
// top last.
func (f *Figure) order() []int {
	out := make([]int, len(f.glyphs))
	for i := range out {
		out[i] = i
		if f.stack == EarlierOnTop {
			out[i] = len(out) - 1 - i
		}
	}
	return out
}

// over returns the glyph drawn on top whose bounds hold x, y, or -1.
func (f *Figure) over(x, y int) int {
	top := -1
	for _, i := range f.order() {
		if (image.Point{X: x, Y: y}).In(f.glyphs[i].Rect) {
			top = i
		}
	}
	return top
}
