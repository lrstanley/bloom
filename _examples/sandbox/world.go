// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import "math/rand/v2"

// material is what a pixel of the world is made of.
type material uint8

const (
	empty material = iota
	sand
	water
	stone
	lava
	steam
)

// hues is how many hues sand cycles through as it is poured, so that what
// is poured settles in bands of color.
const hues = 48

// world is a falling-sand simulation: a grid of pixels, each of one
// material, that fall, flow, and react a step at a time. Its pixels are
// half a cell tall, so a world for an area of w×h cells is w×2h pixels.
type world struct {
	w, h int
	px   []pixel // row by row, from the top left
	step uint8
	rng  *rand.Rand

	grains int // the pixels that are not empty
}

// pixel is a pixel of the world.
type pixel struct {
	mat   material
	shade uint8 // its color within its material's palette
	speed uint8 // how fast it is falling, in sixteenths of a pixel a step
	moved uint8 // the step it last moved in, so it moves once a step
}

func newWorld(seed uint64) *world {
	return &world{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} //nolint:gosec // not for security.
}

// resize changes the size of the world to w×h pixels, keeping what it
// holds against the bottom left, where it has settled.
func (wd *world) resize(w, h int) {
	w, h = max(w, 0), max(h, 0)
	if w == wd.w && h == wd.h {
		return
	}
	px := make([]pixel, w*h)
	for y := range min(h, wd.h) {
		from, to := (wd.h-1-y)*wd.w, (h-1-y)*w
		n := min(w, wd.w)
		copy(px[to:to+n], wd.px[from:from+n])
	}
	wd.w, wd.h, wd.px, wd.grains = w, h, px, 0
	for _, p := range px {
		if p.mat != empty {
			wd.grains++
		}
	}
}

// clear empties the world.
func (wd *world) clear() {
	clear(wd.px)
	wd.grains = 0
}

// at returns the material at x, y. Outside the world is stone, so nothing
// falls or flows out of it.
func (wd *world) at(x, y int) material {
	if x < 0 || y < 0 || x >= wd.w || y >= wd.h {
		return stone
	}
	return wd.px[y*wd.w+x].mat
}

// set makes the pixel at x, y m, in a shade picked for it.
func (wd *world) set(x, y int, m material, hue int) {
	if x < 0 || y < 0 || x >= wd.w || y >= wd.h {
		return
	}
	p := &wd.px[y*wd.w+x]
	switch {
	case p.mat == empty && m != empty:
		wd.grains++
	case p.mat != empty && m == empty:
		wd.grains--
	}
	shade := wd.rng.IntN(shadesOf(m))
	if m == sand {
		shade = (hue%hues)*sandShades + wd.rng.IntN(sandShades)
	}
	p.mat, p.shade, p.speed = m, uint8(shade), 0 //nolint:gosec // palettes have fewer than 256 colors.
}

// paint pours m into the circle of radius r around x, y. Loose materials
// are sprinkled, so that they pour rather than land as a block, and only
// into empty pixels; stone and empty, the eraser, fill the circle.
func (wd *world) paint(x, y, r int, m material, hue int) {
	for py := y - r; py <= y+r; py++ {
		for px := x - r; px <= x+r; px++ {
			if dx, dy := px-x, py-y; dx*dx+dy*dy > r*r {
				continue
			}
			if m == empty || m == stone {
				wd.set(px, py, m, hue)
			} else if wd.at(px, py) == empty && wd.rng.IntN(4) == 0 {
				wd.set(px, py, m, hue)
			}
		}
	}
}

// rain sprinkles loose materials across the top row: rainbow sand, water,
// and now and then a drop of lava.
func (wd *world) rain(hue int) {
	for x := range wd.w {
		switch n := wd.rng.IntN(100); {
		case n < 6:
			wd.set(x, 0, sand, hue+x/8)
		case n < 10:
			wd.set(x, 0, water, 0)
		case n == 10:
			wd.set(x, 0, lava, 0)
		}
	}
}

// update moves every pixel once: it scans from the bottom up, so falling
// pixels fall into room made this step, alternating the direction of each
// row so that nothing drifts one way.
func (wd *world) update() {
	wd.step++
	for y := wd.h - 1; y >= 0; y-- {
		ltr := (int(wd.step)+y)%2 == 0
		for k := range wd.w {
			x := k
			if !ltr {
				x = wd.w - 1 - k
			}
			p := &wd.px[y*wd.w+x]
			m := p.mat
			if m == empty || m == stone || p.moved == wd.step {
				continue
			}
			switch m { //nolint:exhaustive // empty and stone don't move, and were skipped.
			case sand:
				wd.fall(x, y, sand, 0)
			case water:
				wd.fall(x, y, water, 4)
			case lava:
				if wd.quench(x, y) {
					continue
				}
				// Lava glows, and flows slowly.
				if wd.rng.IntN(6) == 0 {
					p.shade = uint8(wd.rng.IntN(shadesOf(lava))) //nolint:gosec // a few shades.
				}
				if wd.step%3 == 0 {
					wd.fall(x, y, lava, 1)
				}
			case steam:
				wd.rise(x, y)
			}
		}
	}
}

// sinks reports whether m sinks through what is at x, y.
func (wd *world) sinks(m material, x, y int) bool {
	switch wd.at(x, y) {
	case empty, steam:
		return true
	case water:
		return m == sand || m == lava
	case sand, stone, lava:
		return false
	}
	return false
}

// drift is how often, one step in so many, a pixel falling freely of each
// material falls a pixel to the side as well, so that a stream fans out as
// it falls, rather than falling as a column. Zero never drifts.
var drift = [steam + 1]uint32{sand: 4, water: 3, lava: 8}

// gravity is how much faster a pixel falls each step it falls freely, and
// fastest how fast it can fall, both in sixteenths of a pixel a step. A
// pixel falls at least a pixel a step, so gravity speeds it up after eight
// steps, and from rest, it falls 60 pixels in about half a second.
const (
	gravity = 2
	fastest = 5 * 16
)

// fall moves the pixel at x, y of material m down, or down a slope, or for
// a liquid, up to spread pixels to the side. Falling through empty pixels,
// it speeds up, by gravity each step; through anything else, such as sand
// through water, it sinks a pixel a step.
func (wd *world) fall(x, y int, m material, spread int) {
	p := &wd.px[y*wd.w+x]
	if wd.sinks(m, x, y+1) {
		// One draw picks whether to drift, which way, and how to round the
		// distance.
		r := wd.rng.Uint32()
		tx := x
		if n := drift[m]; n > 0 && r%(2*n) < 2 {
			if d := 1 - 2*int(r%2); wd.sinks(m, x+d, y+1) && wd.sinks(m, x+d, y) {
				tx = x + d
			}
		}
		if wd.at(tx, y+1) != empty {
			p.speed = 0
			wd.swap(x, y, tx, y+1)
			return
		}
		v := min(p.speed+gravity, fastest)
		far := int(v >> 4)
		if r>>28 < uint32(v&15) {
			far++ // the fraction of a pixel, on average
		}
		far = max(far, 1)
		ty := y + 1
		for ty-y < far && wd.at(tx, ty+1) == empty {
			ty++
		}
		// Short of far, it lands, on whatever is under it, and falls on at
		// that one's speed: nothing, unless it is falling too.
		switch j := (ty+1)*wd.w + tx; {
		case ty-y == far:
		case ty+1 < wd.h && wd.px[j].moved == wd.step:
			v = wd.px[j].speed
		default:
			v = 0
		}
		p.speed = v
		wd.swap(x, y, tx, ty)
		return
	}
	p.speed = 0
	d := 1 - 2*wd.rng.IntN(2)
	ways := [2]int{d, -d} // which way to try first, and then
	for _, dir := range ways {
		if wd.sinks(m, x+dir, y+1) && wd.sinks(m, x+dir, y) {
			wd.swap(x, y, x+dir, y+1)
			return
		}
	}
	// A liquid flows as far as it can to the side, up to spread.
	for _, dir := range ways {
		far := 0
		for k := 1; k <= spread && wd.at(x+dir*k, y) == empty; k++ {
			far = k
		}
		if far > 0 {
			wd.swap(x, y, x+dir*far, y)
			return
		}
	}
}

// rise moves the steam at x, y up, or up a slope, or to the side, and now
// and then lets it vanish.
func (wd *world) rise(x, y int) {
	if wd.rng.IntN(90) == 0 {
		wd.px[y*wd.w+x].mat = empty
		wd.grains--
		return
	}
	d := 1 - 2*wd.rng.IntN(2)
	for _, to := range [...][2]int{{x, y - 1}, {x + d, y - 1}, {x - d, y - 1}, {x + d, y}} {
		if wd.at(to[0], to[1]) == empty {
			wd.swap(x, y, to[0], to[1])
			return
		}
	}
}

// quench turns the lava at x, y to stone, and the water beside it to steam,
// when there is water beside it.
func (wd *world) quench(x, y int) bool {
	hit := false
	for _, n := range [...][2]int{{x, y - 1}, {x - 1, y}, {x + 1, y}, {x, y + 1}} {
		if wd.at(n[0], n[1]) == water {
			wd.set(n[0], n[1], steam, 0)
			wd.px[n[1]*wd.w+n[0]].moved = wd.step
			hit = true
		}
	}
	if hit {
		wd.set(x, y, stone, 0)
	}
	return hit
}

// swap trades the pixels at x1, y1 and x2, y2, which have both moved this
// step.
func (wd *world) swap(x1, y1, x2, y2 int) {
	i, j := y1*wd.w+x1, y2*wd.w+x2
	wd.px[i], wd.px[j] = wd.px[j], wd.px[i]
	wd.px[i].moved, wd.px[j].moved = wd.step, wd.step
}

// scene lays out a few stone ledges and a basin for things to pour over.
func (wd *world) scene() {
	w, h := wd.w, wd.h
	ledge := func(x0, y0, x1, y1 int) {
		steps := max(abs(x1-x0), abs(y1-y0), 1)
		for s := 0; s <= steps; s++ {
			x, y := x0+(x1-x0)*s/steps, y0+(y1-y0)*s/steps
			wd.set(x, y, stone, 0)
			wd.set(x, y+1, stone, 0)
		}
	}
	ledge(w/8, h/3, w*3/8, h/2)
	ledge(w*7/8, h/4, w*5/8, h*2/5)
	// A basin along the floor, open at the top.
	ledge(w/3, h-h/5, w/3+w/10, h-2)
	ledge(w*2/3, h-h/5, w*2/3-w/10, h-2)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
