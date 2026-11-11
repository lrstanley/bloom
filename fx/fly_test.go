// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image"
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	"github.com/charmbracelet/x/ansi"
)

// flyLayer returns a surface for text drawn at node, in a w-column frame,
// moving there from from.
func flyLayer(text string, node, from image.Rectangle, w int) *canvas.Surface {
	frame := canvas.New(w, node.Max.Y, ansi.GraphemeWidth)
	cells := canvas.New(w, node.Max.Y, ansi.GraphemeWidth)
	cells.Blit(canvas.Parse(text, ansi.GraphemeWidth), node, node)
	s := cells.TravelLayer(node, node, frame.Bounds(), frame, env, &canvas.Mixer{})
	s.From = from
	return s
}

// reached returns what s shows across its reach, row by row, with hidden
// cells as a "~".
func reached(s *canvas.Surface) string {
	r := s.Reach()
	var b strings.Builder
	for y := r.Min.Y; y < r.Max.Y; y++ {
		if y > r.Min.Y {
			b.WriteByte('\n')
		}
		for x := r.Min.X; x < r.Max.X; x++ {
			c := s.Cell(x, y)
			switch {
			case c.Width == 0:
			case !s.Visible(x, y):
				b.WriteByte('~')
			default:
				b.WriteString(c.Content)
			}
		}
	}
	return b.String()
}

func TestFly(t *testing.T) {
	t.Parallel()

	node, from := image.Rect(0, 1, 4, 3), image.Rect(6, 1, 10, 3)
	const text = "abcd\nefgh"

	// A node that is not moving is left as it is.
	for _, still := range []image.Rectangle{{}, node} {
		s := flyLayer(text, node, still, 10)
		before := reached(s)
		Fly{}.Apply(s, 0.5)
		if got := reached(s); got != before {
			t.Fatalf("from %v changed %q to %q", still, before, got)
		}
	}

	// A node with nothing left of it, such as a slot shrunk to nothing, has
	// nothing to fly.
	empty := flyLayer("", image.Rect(4, 1, 4, 3), from, 10)
	Fly{}.Apply(empty, 0.5)

	// It starts out whole where it was, and ends where it is.
	s := flyLayer(text, node, from, 10)
	Fly{Seed: 1}.Apply(s, 0)
	if got, want := reached(s), "~~~~~~~~~~\n~~~~~~abcd\n~~~~~~efgh"; got != want {
		t.Fatalf("t=0:\n%s\nwant:\n%s", got, want)
	}
	s = flyLayer(text, node, from, 10)
	Fly{Seed: 1}.Apply(s, 1)
	if got, want := reached(s), "~~~~~~~~~~\nabcd~~~~~~\nefgh~~~~~~"; got != want {
		t.Fatalf("t=1:\n%s\nwant:\n%s", got, want)
	}

	// The trailing column waits until the others are on their way, and the
	// leading one lands before the last have.
	s = flyLayer(text, node, from, 10)
	Fly{Seed: 1}.Apply(s, 0.44)
	got := reached(s)
	if rows := strings.Split(got, "\n"); rows[1][9] != 'd' || rows[2][9] != 'h' {
		t.Fatalf("t=0.44, want d and h waiting:\n%s", got)
	}
	if strings.Trim(strings.ReplaceAll(got, "\n", ""), "~dh") == "" {
		t.Fatalf("t=0.44 shows nothing in flight:\n%s", got)
	}
	s = flyLayer(text, node, from, 10)
	Fly{Seed: 1}.Apply(s, 0.56)
	got = reached(s)
	if rows := strings.Split(got, "\n"); rows[1][0] != 'a' || rows[2][0] != 'e' {
		t.Fatalf("t=0.56, want a and e landed:\n%s", got)
	}

	// A node that changed size starts out filling where it was, and nothing
	// else, whether it grew or shrank.
	for _, was := range []image.Rectangle{image.Rect(6, 1, 8, 2), image.Rect(4, 0, 10, 3)} {
		s = flyLayer(text, node, was, 10)
		Fly{Seed: 1}.Apply(s, 0)
		shown := reached(s)
		for y, row := range strings.Split(shown, "\n") {
			for x, c := range row {
				if in := image.Pt(x, y).In(was); in == (c == '~') {
					t.Fatalf("resized from %v, t=0 shows %q at %d,%d:\n%s", was, c, x, y, shown)
				}
			}
		}
	}

	// Wide glyphs stay whole, landed, waiting, or flown as a narrow glyph.
	for i := range 21 {
		w := flyLayer("世界ab\nc世界d", image.Rect(0, 1, 6, 3), image.Rect(8, 1, 14, 3), 14)
		Fly{Seed: 2}.Apply(w, float64(i)/20)
		r := w.Reach()
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				c := w.Cell(x, y)
				if c.Width == 2 && (x+1 >= r.Max.X || w.Cell(x+1, y).Width != 0) {
					t.Fatalf("t=%.2f: wide glyph at %d,%d lost its tail:\n%s", float64(i)/20, x, y, reached(w))
				}
				if c.Width == 0 && (x == r.Min.X || w.Cell(x-1, y).Width != 2) {
					t.Fatalf("t=%.2f: tail at %d,%d lost its glyph:\n%s", float64(i)/20, x, y, reached(w))
				}
			}
		}
	}
}

func BenchmarkFlyFullScreen(b *testing.B) {
	// A 100×60 node, half a 200-column screen, flying across it.
	node, from := image.Rect(0, 0, 100, 60), image.Rect(100, 0, 200, 60)
	text := strings.TrimSuffix(strings.Repeat(strings.Repeat("abcdefghij", 10)+"\n", 60), "\n")
	frame := canvas.New(200, 60, ansi.GraphemeWidth)
	src := canvas.Parse(text, ansi.GraphemeWidth)
	cells := canvas.New(200, 60, ansi.GraphemeWidth)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		cells.Resize(200, 60)
		cells.Blit(src, node, node)
		s := cells.TravelLayer(node, node, frame.Bounds(), frame, env, &canvas.Mixer{})
		s.From = from
		Fly{Seed: 1}.Apply(s, float64(i%20)/20)
	}
}
