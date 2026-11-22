// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"math"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
)

// rainbow is a custom effect: glyphs take a hue by column that fades to
// their own color as t reaches 1.
var rainbow = fx.Func(func(s *canvas.Surface, t float64) {
	for y := range s.Height() {
		for x := range s.Width() {
			c := s.Cell(x, y)
			if c == nil || c.Content == " " {
				continue
			}
			hue := math.Mod(float64(x)/float64(max(s.Width(), 1))+t, 1)
			c.Style.Fg = s.Mix(hsv(hue), s.Fg(c), t)
		}
	}
})

// sweep is a custom effect: a bright band crosses the node diagonally,
// leaving it as it was once it is past. The band's background is tinted
// with accent.
func sweep(accent color.Color) fx.Effect {
	return fx.Func(func(s *canvas.Surface, t float64) {
		pos := t*float64(s.Width()+s.Height()+6) - 3
		for y := range s.Height() {
			for x := range s.Width() {
				if d := math.Abs(float64(x+y) - pos); d < 3 {
					c := s.Cell(x, y)
					c.Style.Fg = s.Mix(s.Fg(c), color.White, 1-d/3)
					c.Style.Bg = s.Mix(s.Bg(c), accent, (1-d/3)*0.6)
				}
			}
		}
	})
}

// galleryCard is a card of the gallery: the effects it runs together, and
// their name.
type galleryCard struct {
	name    string
	effects []fx.Effect
}

// newGallery returns the gallery's cards, with the sweep effect's band
// tinted with accent.
func newGallery(accent color.Color) []galleryCard {
	return []galleryCard{
		{"Fade", []fx.Effect{fx.Fade{}}},
		{"Slide from the left", []fx.Effect{fx.Slide{From: fx.Left}}},
		{"Wipe from the top", []fx.Effect{fx.Wipe{From: fx.Top}}},
		{"Dissolve", []fx.Effect{fx.Dissolve{Seed: 11}}},
		{"Typewriter", []fx.Effect{fx.Typewriter{}}},
		{"Scramble + Fade", []fx.Effect{fx.Scramble{Seed: 5}, fx.Fade{}}},
		{"Custom: rainbow", []fx.Effect{rainbow}},
		{"Custom: sweep", []fx.Effect{sweep(accent)}},
	}
}
