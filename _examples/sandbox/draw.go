// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"math"
	"time"

	"bloomfx.dev/bloom"
	uv "github.com/charmbracelet/ultraviolet"
)

// sandShades is how many shades of each of sand's hues there are, so that a
// band of one hue looks grainy.
const sandShades = 3

// palette is the colors of each material's shades, made once, as colors in
// interfaces, so that drawing a frame doesn't allocate one for every cell.
var palette = func() (p [steam + 1][]color.Color) {
	for h := range hues {
		for _, l := range [sandShades]float64{0.62, 0.55, 0.48} {
			p[sand] = append(p[sand], hsl(float64(h)/hues, 0.7, l))
		}
	}
	p[water] = rgbs(0x2f6fd0, 0x3a7be0, 0x356fc8, 0x4688e6, 0x2c66c0)
	p[stone] = rgbs(0x6e6a86, 0x625e78, 0x58546c)
	p[lava] = rgbs(0xff5a1f, 0xff7a1a, 0xe8401a, 0xffa424, 0xd03010)
	p[steam] = rgbs(0xc8cad8, 0xb4b6c6, 0xdcdde6)
	return p
}()

// shadesOf returns how many shades material m has.
func shadesOf(m material) int { return max(len(palette[m]), 1) }

// worldView draws a world as half blocks: each cell is two pixels, the top
// one in the cell's foreground and the bottom one in its background. Empty
// pixels are left alone, so what is behind the world, here the panel's
// background, shows through. It draws whatever size it is given, and has
// no natural size of its own, so it goes in a slot that sizes it, such as
// Fill.
type worldView struct {
	w     *world
	brush brush
	ghost color.Color    // the color of the brush's outline
	took  *time.Duration // set to how long Draw took, for the meters
}

// brush is the pointer over the world, in pixels, drawn as a ring of radius
// r while shown.
type brush struct {
	x, y, r int
	shown   bool
}

var _ uv.Drawable = worldView{}

func (v worldView) Draw(scr uv.Screen, area uv.Rectangle) {
	start := time.Now()
	cell := uv.Cell{Width: 1}
	for row := range area.Dy() {
		for col := range area.Dx() {
			top, bottom := v.color(col, 2*row), v.color(col, 2*row+1)
			switch {
			case top == nil && bottom == nil:
				continue
			case top == nil:
				cell.Content, cell.Style = "▄", uv.Style{Fg: bottom}
			default:
				cell.Content, cell.Style = "▀", uv.Style{Fg: top, Bg: bottom}
			}
			scr.SetCell(area.Min.X+col, area.Min.Y+row, &cell)
		}
	}
	if v.took != nil {
		*v.took = time.Since(start)
	}
}

// color returns the color of the pixel at x, y, or nil for none: an empty
// pixel, or one past the world's edge, as it is before it is first sized.
func (v worldView) color(x, y int) color.Color {
	wd := v.w
	if x >= wd.w || y >= wd.h {
		return nil
	}
	if p := wd.px[y*wd.w+x]; p.mat != empty {
		return palette[p.mat][p.shade] // set keeps shade within the palette
	}
	if b := v.brush; b.shown {
		dx, dy := x-b.x, y-b.y
		if d := dx*dx + dy*dy; d <= b.r*b.r && d > (b.r-1)*(b.r-1) {
			return v.ghost
		}
	}
	return nil
}

// graphScale is the frame time at the top of the graph. A slower frame's
// bar is clipped to it, so the scale stays put.
const graphScale = 4 * time.Millisecond

// graph draws the last frame times as bars, the newest on the right, on a
// scale from nothing to graphScale at its full height. The bars are drawn
// over a gradient up the graph, from ramp's first color at the bottom to
// its last at the top, so a slow frame's bar reaches the hotter colors.
type graph struct {
	samples []time.Duration // oldest first
	ramp    []color.Color   // bottom to top, made once
}

var _ bloom.Measurer = graph{}

// Measure asks for a column for each sample, and four rows: 32 levels of
// bar, in eighths of a cell. Given more rows, it draws taller bars.
func (g graph) Measure(avail bloom.Size) bloom.Size {
	return bloom.Size{W: min(avail.W, len(g.samples)), H: min(avail.H, 4)}
}

var eighths = [...]string{"", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

func (g graph) Draw(scr uv.Screen, area uv.Rectangle) {
	h := area.Dy()
	samples := g.samples[max(len(g.samples)-area.Dx(), 0):]
	x0 := area.Max.X - len(samples)
	cell := uv.Cell{Width: 1}
	for i, d := range samples {
		// At least a sliver, so a fast frame still shows.
		level := min(max(int(math.Round(float64(d)/float64(graphScale)*float64(h*8))), 1), h*8)
		for row := range h {
			n := min(max(level-row*8, 0), 8)
			if n == 0 {
				break
			}
			// A cell takes one color: the ramp's at the middle of what of
			// the bar it holds.
			cell.Style.Fg = g.ramp[(16*row+n)*(len(g.ramp)-1)/(16*h)]
			cell.Content = eighths[n]
			scr.SetCell(x0+i, area.Max.Y-1-row, &cell)
		}
	}
}

// heat returns n colors for the frame time graph, from 0 to graphScale:
// green to 1ms, then through yellow at 2ms and orange at 3ms to red at
// 4ms. It sweeps the hue, as blending green into yellow goes through gray.
func heat(n int) []color.Color {
	// The hue at each millisecond, in degrees.
	stops := [...]float64{120, 120, 55, 30, 0}
	out := make([]color.Color, n)
	for i := range n {
		at := float64(i) / float64(n-1) * float64(len(stops)-1)
		k := min(int(at), len(stops)-2)
		hue := stops[k] + (stops[k+1]-stops[k])*(at-float64(k))
		out[i] = hsl(hue/360, 0.65, 0.62)
	}
	return out
}

// hsl returns the color of hue h, saturation s, and lightness l, each from
// 0 to 1.
func hsl(h, s, l float64) color.Color {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * min(l, 1-l)
		return uint8(math.Round(255 * (l - a*max(-1, min(k-3, 9-k, 1)))))
	}
	return color.RGBA{R: f(0), G: f(8), B: f(4), A: 0xff}
}

// rgbs returns colors for hex values such as 0xff8800.
func rgbs(hex ...uint32) []color.Color {
	out := make([]color.Color, len(hex))
	for i, v := range hex {
		out[i] = color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff} //nolint:gosec // bytes of the value.
	}
	return out
}
