// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"image/color"
	"math/rand/v2"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// randomStyle returns a style with a mix of every kind of color and
// attribute, often unset, so pairs of them differ in few or many ways.
func randomStyle(r *rand.Rand) uv.Style {
	pick := func() color.Color {
		switch r.IntN(6) {
		case 0:
			return ansi.BasicColor(r.IntN(18)) // including two past the bright colors
		case 1:
			return ansi.IndexedColor(r.IntN(256))
		case 2:
			return color.RGBA{R: uint8(r.IntN(3) * 127), G: uint8(r.IntN(256)), B: 7, A: 255}
		case 3:
			return ansi.RGBColor{R: uint8(r.IntN(256)), G: uint8(r.IntN(256)), B: uint8(r.IntN(256))}
		}
		return nil
	}
	a, b := uint8(r.IntN(256)), uint8(r.IntN(256)) // fewer attributes, by and-ing two
	return uv.Style{
		Fg: pick(), Bg: pick(), UnderlineColor: pick(),
		Underline: ansi.Underline(r.IntN(6)),
		Attrs:     a & b,
	}
}

func TestAppendStyleDiffMatchesUltraviolet(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	var buf []byte
	for range 200_000 {
		from, to := randomStyle(r), randomStyle(r)
		if r.IntN(3) == 0 {
			to = from
			to.Attrs ^= 1 << r.IntN(8)
		}
		if styleEqual(&from, &to) != from.Equal(&to) {
			t.Fatalf("styleEqual(%+v, %+v) = %v", from, to, !from.Equal(&to))
		}
		if from.Equal(&to) || to.IsZero() {
			continue
		}
		buf = appendStyleDiff(buf[:0], &from, &to)
		if want := uv.StyleDiff(&from, &to); string(buf) != want {
			t.Fatalf("from %+v to %+v: got %q, want %q", from, to, buf, want)
		}
	}
}

func BenchmarkRenderStyled(b *testing.B) {
	r := rand.New(rand.NewPCG(3, 4))
	c := New(200, 60, ansi.GraphemeWidth)
	for y := range 60 {
		for x := range 200 {
			s := randomStyle(r)
			c.SetCell(x, y, &uv.Cell{Content: "x", Width: 1, Style: s})
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = c.Render()
	}
}
