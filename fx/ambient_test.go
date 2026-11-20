// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom/canvas"
	"github.com/charmbracelet/x/ansi"
)

// ambientFrame returns a 60×20 region of an empty frame, whose top-left
// cell is at 5,3 of a larger one, with "hello world" written at 10,5 of the
// region and a red-backgrounded blank at 30,10.
func ambientFrame() *canvas.Surface {
	frame := canvas.New(70, 26, ansi.GraphemeWidth)
	text := canvas.Parse("hello world", ansi.GraphemeWidth)
	for x := range text.Width() {
		frame.SetCell(5+10+x, 3+5, text.CellAt(x, 0))
	}
	block := canvas.Parse("\x1b[48;2;255;0;0m \x1b[m", ansi.GraphemeWidth)
	frame.SetCell(5+30, 3+10, block.CellAt(0, 0))
	return frame.Region(image.Rect(5, 3, 65, 23), env, &canvas.Mixer{})
}

// render returns the surface's glyphs, row by row.
func render(s *canvas.Surface) string {
	var b strings.Builder
	for y := range s.Height() {
		for x := range s.Width() {
			c := s.Cell(x, y).Content
			if c == "" {
				c = " "
			}
			b.WriteString(c)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// drawn counts the cells the effect drew into.
func drawn(s *canvas.Surface) int {
	n := 0
	for y := range s.Height() {
		for x := range s.Width() {
			if c := s.Cell(x, y); c.Content != " " && c.Content != "" {
				n++
			}
		}
	}
	// Less the text and the block, which were there before.
	return n - len("helloworld")
}

// dropsOf returns drops holding each of list.
func dropsOf(list ...Drop) Drops {
	var d Drops
	for _, drop := range list {
		d.Add(drop)
	}
	return d
}

func ambientEffects() map[string]Effect {
	at := 2 * time.Second
	return map[string]Effect{
		"ripple": Ripple{Time: at, Drops: dropsOf(Drop{At: image.Pt(35, 13), Time: at - 400*time.Millisecond}), Seed: 3},
		"rain":   Ripple{Time: at, Rain: 4, Seed: 3},
		"drift":  Drift{Time: at, Cover: 0.6, Seed: 3},
		"focus":  Drift{Time: at, Cover: 0.01, Focus: image.Pt(35, 13), Radius: 12, Seed: 3},
		"stream": Rain{Time: at, Density: 1, Seed: 3},
	}
}

func TestAmbient(t *testing.T) {
	t.Parallel()

	for name, e := range ambientEffects() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := ambientFrame()
			before := render(s)
			e.Apply(s, 0)
			if render(s) != before {
				t.Fatal("drew at strength 0")
			}

			e.Apply(s, 1)
			if drawn(s) == 0 {
				t.Fatal("drew nothing")
			}
			// What was drawn is left alone, with a cell of room around it.
			for x := 9; x <= 21; x++ {
				want := " "
				if x >= 10 && x < 21 {
					want = before[5*61+x : 5*61+x+1]
				}
				if got := s.Cell(x, 5).Content; got != want {
					t.Fatalf("cell %d,5 is %q, want %q:\n%s", x, got, want, render(s))
				}
			}
			if c := s.Cell(30, 10); c.Content != " " {
				t.Fatalf("drew %q over a background", c.Content)
			}

			// The same input draws the same.
			again := ambientFrame()
			e.Apply(again, 1)
			if render(again) != render(s) {
				t.Fatal("drew differently for the same input")
			}
		})
	}
}

func TestRippleRing(t *testing.T) {
	t.Parallel()

	drop := image.Pt(35, 13) // frame coordinates: the region's 30,10
	for _, age := range []time.Duration{300 * time.Millisecond, 700 * time.Millisecond} {
		s := ambientFrame()
		e := Ripple{Time: 5 * time.Second, Drops: dropsOf(Drop{At: drop, Time: 5*time.Second - age})}
		e.Apply(s, 1)

		// The front is where the ring has travelled to, at 26 columns a
		// second, with rows counting double.
		radius := 26 * age.Seconds()
		far, near := 0, 0
		for y := range s.Height() {
			for x := range s.Width() {
				if y == 5 || s.Cell(x, y).Content == " " {
					continue // row 5 holds the text
				}
				r := math.Hypot(float64(x-30), 2*float64(y-10))
				switch {
				case r > radius+2:
					far++
				case r > radius-5:
					near++
				}
			}
		}
		if far > 0 || near == 0 {
			t.Fatalf("at %v: %d cells drawn past the front, %d at it:\n%s", age, far, near, render(s))
		}
	}

	// Drops older than their life, and those yet to fall, draw nothing.
	s := ambientFrame()
	Ripple{Time: 5 * time.Second, Drops: dropsOf(
		Drop{At: drop, Time: 2 * time.Second},
		Drop{At: drop, Time: 6 * time.Second},
	)}.Apply(s, 1)
	if n := drawn(s); n != 0 {
		t.Fatalf("drew %d cells", n)
	}
}

func TestDriftFocus(t *testing.T) {
	t.Parallel()

	// With almost no cover, glyphs gather only around the focus.
	s := ambientFrame()
	Drift{Time: time.Second, Cover: 0.01, Focus: image.Pt(35, 13), Radius: 10}.Apply(s, 1)
	for y := range s.Height() {
		for x := range s.Width() {
			if c := s.Cell(x, y); c.Content != " " && math.Hypot(float64(x-30), 2*float64(y-10)) > 10 &&
				y != 5 {
				t.Fatalf("drew %q at %d,%d, outside the focus:\n%s", c.Content, x, y, render(s))
			}
		}
	}
	if drawn(s) == 0 {
		t.Fatal("drew nothing around the focus")
	}
}

func TestDrops(t *testing.T) {
	t.Parallel()

	// Past what it holds, each drop added takes the place of the oldest.
	var d Drops
	for i := range maxDrops + 2 {
		d.Add(Drop{At: image.Pt(i, 0)})
	}
	if d.list[0].At.X != maxDrops || d.list[1].At.X != maxDrops+1 || d.list[2].At.X != 2 {
		t.Fatalf("kept %v", d.list[:3])
	}
	// A ripple holding them compares equal while nothing changes.
	if a, b := (Ripple{Drops: d}), (Ripple{Drops: d}); a != b {
		t.Fatal("ripples with the same drops differ")
	}
}

func TestGlyphSet(t *testing.T) {
	t.Parallel()

	g := newGlyphSet("·•●")
	if g.pick(0.4) != "•" || g.pick(0.99) != "●" {
		t.Fatalf("unicode set: %q %q", g.pick(0.4), g.pick(0.99))
	}
	if g = newGlyphSet(""); g.pick(0) != "a" || g.pick(0.999) != "9" {
		t.Fatalf("default set: %q %q", g.pick(0), g.pick(0.999))
	}
}

func BenchmarkAmbient(b *testing.B) {
	frame := canvas.New(200, 60, ansi.GraphemeWidth)
	mixer := &canvas.Mixer{}
	var drops Drops
	for i := range 12 {
		drops.Add(Drop{At: image.Pt(i*15, i*5), Time: time.Duration(i) * 100 * time.Millisecond})
	}
	effects := map[string]func(time.Duration) Effect{
		"ripple": func(at time.Duration) Effect {
			return Ripple{Time: at, Drops: drops, Rain: 1}
		},
		"drift": func(at time.Duration) Effect { return Drift{Time: at, Focus: image.Pt(100, 30), Radius: 16} },
		"rain":  func(at time.Duration) Effect { return Rain{Time: at} },
	}
	for name, e := range effects {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				frame.Clear()
				e(time.Duration(i)*16*time.Millisecond).Apply(frame.Region(frame.Bounds(), env, mixer), 1)
			}
		})
	}
}
