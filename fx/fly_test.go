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
	before := reached(empty)
	Fly{}.Apply(empty, 0.5)
	if got := reached(empty); got != before {
		t.Fatalf("an empty node changed %q to %q", before, got)
	}

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
	// What is in flight is not yet where it lands.
	for x := range 4 {
		if rows := strings.Split(got, "\n"); rows[1][x] == "abcd"[x] || rows[2][x] == "efgh"[x] {
			t.Fatalf("t=0.44, column %d has landed:\n%s", x, got)
		}
	}
	// Plain, what is in flight is the node's own glyphs.
	s = flyLayer(text, node, from, 10)
	Fly{Seed: 1, Plain: true}.Apply(s, 0.44)
	if plain := reached(s); strings.Trim(strings.ReplaceAll(plain, "\n", ""), "~abcdefgh") != "" || plain == got {
		t.Fatalf("t=0.44, plain, want only the node's glyphs in flight:\n%s", plain)
	}
	s = flyLayer(text, node, from, 10)
	Fly{Seed: 1}.Apply(s, 0.56)
	got = reached(s)
	if rows := strings.Split(got, "\n"); rows[1][0] != 'a' || rows[2][0] != 'e' || rows[1][3] == 'd' || rows[2][3] == 'h' {
		t.Fatalf("t=0.56, want a and e landed, and d and h not yet:\n%s", got)
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

func TestFlyFrom(t *testing.T) {
	t.Parallel()

	node := image.Rect(2, 1, 6, 3)
	const text = "abcd\nefgh"

	tests := []struct {
		name string
		from Origin
		at   image.Point // the cell it all starts out in
	}{
		{"middle", Middle, image.Pt(4, 2)},
		{"top", Side(Top), image.Pt(4, 1)},
		{"bottom right", Side(BottomRight), image.Pt(5, 2)},
		{"a cell", At(image.Pt(3, 1)), image.Pt(3, 1)},
		{"a cell off it", At(image.Pt(0, 9)), image.Pt(2, 2)},
	}
	for _, tt := range tests {
		// It flies out of its spot whether it moved or not, starting out
		// squeezed into the one cell.
		for _, was := range []image.Rectangle{{}, image.Rect(6, 1, 10, 3)} {
			s := flyLayer(text, node, was, 10)
			Fly{From: tt.from, Seed: 1}.Apply(s, 0)
			rows := strings.Split(reached(s), "\n")
			for y, row := range rows {
				for x, c := range row {
					if at := image.Pt(x, y); (c != '~') != (at == tt.at) {
						t.Fatalf("%s, was %v, t=0 shows %q at %v:\n%s", tt.name, was, c, at, reached(s))
					}
				}
			}
			s = flyLayer(text, node, was, 10)
			Fly{From: tt.from, Seed: 1}.Apply(s, 1)
			if got, want := reached(s), "~~~~~~~~~~\n~~abcd~~~~\n~~efgh~~~~"; got != want {
				t.Fatalf("%s, was %v, t=1:\n%s\nwant:\n%s", tt.name, was, got, want)
			}
		}
	}
}

func TestOriginCell(t *testing.T) {
	t.Parallel()

	r := image.Rect(10, 5, 20, 9) // 10×4
	tests := []struct {
		o    Origin
		want image.Point
	}{
		{Origin{}, image.Pt(5, 2)},
		{Middle, image.Pt(5, 2)},
		{Side(Top), image.Pt(5, 0)},
		{Side(Bottom), image.Pt(5, 3)},
		{Side(Left), image.Pt(0, 2)},
		{Side(TopRight), image.Pt(9, 0)},
		{At(image.Pt(12, 6)), image.Pt(2, 1)},
		{At(image.Pt(0, 100)), image.Pt(0, 3)},
	}
	for _, tt := range tests {
		if got := tt.o.Cell(r); got != tt.want {
			t.Errorf("%+v: cell %v, want %v", tt.o, got, tt.want)
		}
	}
	if got := Middle.Cell(image.Rectangle{}); got != (image.Point{}) {
		t.Errorf("an empty node: cell %v", got)
	}
}
