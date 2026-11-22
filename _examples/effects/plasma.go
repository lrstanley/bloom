// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"math"

	"bloomfx.dev/bloom"
	uv "github.com/charmbracelet/ultraviolet"
)

// plasma is a [uv.Drawable] painting a moving color field with half blocks,
// and a [bloom.Measurer] for its natural size.
type plasma struct{ t float64 } // t is the loop's phase, from 0 to 1

func (p plasma) Measure(avail bloom.Size) bloom.Size {
	return bloom.Size{W: min(avail.W, 28), H: min(avail.H, 4)}
}

func (p plasma) Draw(scr uv.Screen, area uv.Rectangle) {
	phase := p.t * 20 * math.Pi // whole cycles of every wave, so the loop is seamless
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			// v is the field's value at this column, for a row of half
			// blocks: three waves, across, diagonally, and down.
			v := func(row float64) float64 {
				col := float64(x)
				across := math.Sin(col/4 + phase)
				diagonal := math.Sin((col+row)/6 + phase*1.3)
				down := math.Sin(row/3 - phase)
				return across + diagonal + down
			}
			top, bottom := v(float64(2*y)), v(float64(2*y+1))
			scr.SetCell(x, y, &uv.Cell{Content: "▀", Width: 1, Style: uv.Style{
				Fg: hsv(math.Mod((top+3)/6+p.t*6, 1)),
				Bg: hsv(math.Mod((bottom+3)/6+p.t*6, 1)),
			}})
		}
	}
}

// hsv returns a saturated color of hue h, from 0 to 1.
func hsv(h float64) color.Color {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*6, 6)
		return uint8(255 * (1 - 0.65*max(0, min(k, 4-k, 1))))
	}
	return color.RGBA{R: f(5), G: f(3), B: f(1), A: 0xff}
}
