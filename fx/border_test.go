// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
)

// boxSurface returns a surface over a 6×4 rounded box with a dot inside.
func boxSurface() *canvas.Surface {
	return textLayer("╭────╮\n│.  .│\n│.  .│\n╰────╯")
}

func TestPerimeter(t *testing.T) {
	t.Parallel()

	for _, size := range []image.Point{{6, 4}, {1, 3}, {4, 1}, {1, 1}, {2, 2}} {
		c := canvas.New(size.X, size.Y, ansi.GraphemeWidth)
		s := c.Layer(c.Bounds(), c.Bounds(), c, env, &canvas.Mixer{})
		seen := map[*uv.Cell]bool{}
		next := 0
		perimeter(s, func(cell *uv.Cell, i, n int) {
			if i != next || seen[cell] || i >= n {
				t.Fatalf("%v: cell %d of %d out of order or seen twice", size, i, n)
			}
			seen[cell] = true
			next++
		})
		w, h := size.X, size.Y
		want := w * h
		if w > 2 && h > 2 {
			want -= (w - 2) * (h - 2)
		}
		if next != want {
			t.Fatalf("%v: visited %d cells, want %d", size, next, want)
		}
	}

	// Clockwise from the top-left corner: the cell after the top row is
	// below the top-right corner, and the last is above the bottom-left.
	s := boxSurface()
	var order []*uv.Cell
	perimeter(s, func(c *uv.Cell, _, _ int) { order = append(order, c) })
	if order[6] != s.Cell(5, 1) || order[len(order)-1] != s.Cell(0, 1) {
		t.Fatal("the edge is not walked clockwise")
	}
}

func TestBorderEffectsChangeOnlyTheEdge(t *testing.T) {
	t.Parallel()

	for name, e := range map[string]Effect{
		"chase": Chase{From: red, To: blue},
		"pulse": Pulse{Color: red},
		"comet": Comet{Color: red},
		"march": March{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := boxSurface()
			e.Apply(s, 0.4)
			for _, p := range []image.Point{{1, 1}, {4, 2}} {
				if c := s.Cell(p.X, p.Y); c.Content != "." || c.Style.Fg != nil {
					t.Fatalf("changed the cell inside at %v: %+v", p, c)
				}
			}
			// The loop is seamless.
			a, b := boxSurface(), boxSurface()
			e.Apply(a, 0)
			e.Apply(b, 1)
			for i := range 6 {
				ca, cb := a.Cell(i, 0), b.Cell(i, 0)
				if ca.Content != cb.Content || rgba(a.Fg(ca)) != rgba(b.Fg(cb)) {
					t.Fatalf("t=0 and t=1 differ at %d,0", i)
				}
			}
		})
	}
}

func TestComet(t *testing.T) {
	t.Parallel()

	// The edge is 16 cells, so at t=0.25 the head is at cell 4 of the top
	// row, with the tail fading behind it.
	s := boxSurface()
	Comet{Color: red, Length: 3}.Apply(s, 0.25)
	if got := rgba(s.Fg(s.Cell(4, 0))); got != rgba(red) {
		t.Fatalf("head is %v, want red", got)
	}
	if got := rgba(s.Fg(s.Cell(3, 0))); got == rgba(red) || got == rgba(white) {
		t.Fatalf("tail is %v, want a mix of red and white", got)
	}
	if got := rgba(s.Fg(s.Cell(5, 0))); got != rgba(white) {
		t.Fatalf("ahead of the head is %v, want the cell's own white", got)
	}
}

func TestMarch(t *testing.T) {
	t.Parallel()

	s := boxSurface()
	March{Length: 1}.Apply(s, 0)
	if got := rowText(s, 0); got != "╭╌─╌─╮" {
		t.Fatalf("top edge is %q", got)
	}
	if got := s.Cell(5, 1).Content; got != "│" {
		t.Fatalf("right edge at index 6 is %q, want a solid line", got)
	}
	if got := s.Cell(5, 2).Content; got != "╎" {
		t.Fatalf("right edge at index 7 is %q, want a dashed line", got)
	}
}
