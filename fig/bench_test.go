// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig_test

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/fx"
	uv "github.com/charmbracelet/ultraviolet"
)

func BenchmarkParse(b *testing.B) {
	data, err := os.ReadFile("fonts/data/standard.flf.gz")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, perr := fig.Parse(data); perr != nil {
			b.Fatal(perr)
		}
	}
}

// BenchmarkLayout lays out a new line of text each time, as a font only
// remembers so many.
func BenchmarkLayout(b *testing.B) {
	f := fonts.Standard()
	texts := make([]string, 1000)
	for i := range texts {
		texts[i] = fmt.Sprintf("Hello, World! %03d", i)
	}
	i := 0
	for b.Loop() {
		f.Figure(texts[i%len(texts)], fig.Options{})
		i++
	}
}

func BenchmarkRenderCached(b *testing.B) {
	f := fonts.Standard().Figure("Hello, World!", fig.Options{})
	p := fig.Gradient(color.White, color.Black).By(fig.ByDiagonal)
	for b.Loop() {
		f.Render(p)
	}
}

// BenchmarkRenderPaintFunc draws a figure with a paint that is called for
// every cell, every time.
func BenchmarkRenderPaintFunc(b *testing.B) {
	f := fonts.Standard().Figure("Hello, World!", fig.Options{})
	p := fig.PaintFunc(func(_ *fig.Figure, c fig.Cell) uv.Style {
		return uv.Style{Fg: color.Gray{Y: uint8(c.X * 4)}}
	})
	for b.Loop() {
		f.Render(p)
	}
}

// BenchmarkEachFrame draws a frame of text revealed letter by letter: a
// layer per glyph, each with its own effects.
func BenchmarkEachFrame(b *testing.B) {
	r := bloom.New()
	p := fig.Gradient(color.White, color.Black)
	i := 0
	for b.Loop() {
		t := float64(i%100) / 100
		r.Render(120, 10, fig.Text("Hello, World!").Paint(p).Each(t, fx.Slide{From: bloom.Top}, fx.Fade{}))
		i++
	}
}
