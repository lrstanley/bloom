// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"fmt"
	"image"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// shown returns the text of s's shown cells, one line per row, with hidden
// cells as '_' and the tails of wide glyphs left out.
func shown(s *Surface) string {
	var b strings.Builder
	for y := range s.Height() {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := range s.Width() {
			c := s.Cell(x, y)
			switch {
			case c.Width == 0:
			case !s.Visible(x, y):
				b.WriteByte('_')
			default:
				b.WriteString(c.Content)
			}
		}
	}
	return b.String()
}

func layerOf(src string) *Surface {
	c := Parse(src, method)
	return c.Layer(c.Bounds().Add(image.Pt(2, 1)), image.Rect(0, 0, 40, 20), Parse("", method), DefaultEnv, &Mixer{})
}

func TestSub(t *testing.T) {
	t.Parallel()

	s := layerOf("abcd\nefgh\nijkl")
	s.Offset = image.Pt(1, 1)
	sub := s.Sub(image.Rect(1, 1, 3, 3))

	if sub.Width() != 2 || sub.Height() != 2 {
		t.Fatalf("size = %dx%d, want 2x2", sub.Width(), sub.Height())
	}
	if want := image.Rect(3, 2, 5, 4); sub.Rect() != want {
		t.Fatalf("rect = %v, want %v", sub.Rect(), want)
	}
	if sub.Offset != s.Offset || sub.Viewport != s.Viewport {
		t.Fatalf("offset %v viewport %v, want the parent's", sub.Offset, sub.Viewport)
	}
	if c := sub.Cell(0, 0); c == nil || c.Content != "f" {
		t.Fatalf("cell 0,0 = %v, want f", c)
	}
	if sub.Cell(2, 0) != nil || sub.Visible(0, 2) {
		t.Fatal("the sub surface reaches past its bounds")
	}

	// Hiding and offsets: shared cells and mask, but not the offset.
	sub.Hide(1, 1) // k
	sub.Offset = image.Pt(5, 5)
	if s.Visible(2, 2) || s.Offset != image.Pt(1, 1) {
		t.Fatalf("parent: visible(2,2)=%v offset=%v", s.Visible(2, 2), s.Offset)
	}
	sub.Cell(0, 0).Content = "F"
	if s.Cell(1, 1).Content != "F" {
		t.Fatal("the cells are not shared")
	}

	// Nested, and clipped to the parent.
	inner := sub.Sub(image.Rect(1, 0, 9, 9))
	if inner.Width() != 1 || inner.Height() != 2 || inner.Visible(0, 1) || !inner.Visible(0, 0) {
		t.Fatalf("inner = %dx%d, visible %v/%v", inner.Width(), inner.Height(), inner.Visible(0, 0), inner.Visible(0, 1))
	}
	if empty := s.Sub(image.Rect(9, 9, 12, 12)); empty.Width() != 0 || empty.Height() != 0 {
		t.Fatal("a sub surface outside the parent is not empty")
	}
}

func TestMove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		hide []image.Point
		r    image.Rectangle
		d    image.Point
		want string
	}{
		{"right", "abcd\nefgh", nil, image.Rect(0, 0, 4, 2), image.Pt(1, 0), "_abc\n_efg"},
		{"left", "abcd\nefgh", nil, image.Rect(0, 0, 4, 2), image.Pt(-2, 0), "cd__\ngh__"},
		{"down", "abc\ndef\nghi", nil, image.Rect(0, 0, 3, 3), image.Pt(0, 1), "___\nabc\ndef"},
		{"up and left", "abc\ndef\nghi", nil, image.Rect(0, 0, 3, 3), image.Pt(-1, -1), "ef_\nhi_\n___"},
		{"only inside r", "abcd\nefgh", nil, image.Rect(1, 0, 3, 1), image.Pt(1, 0), "a_bd\nefgh"},
		{"all the way out", "abcd", nil, image.Rect(0, 0, 4, 1), image.Pt(-9, 0), "____"},
		{"out of the surface rect", "ab", nil, image.Rect(-2, -2, 9, 9), image.Pt(1, 0), "_a"},
		{"hidden moves along", "abcd", []image.Point{{1, 0}}, image.Rect(0, 0, 4, 1), image.Pt(1, 0), "_a_c"},
		{"wide glyph moves whole", "a世cd", nil, image.Rect(0, 0, 5, 1), image.Pt(1, 0), "_a世c"},
		{"wide glyph cut at the right", "a世cd", nil, image.Rect(0, 0, 5, 1), image.Pt(3, 0), "___a "},
		{"wide glyph cut at the left", "ab世d", nil, image.Rect(0, 0, 5, 1), image.Pt(-3, 0), " d___"},
		{"wide glyph cut by r", "ab世d", nil, image.Rect(0, 0, 3, 1), image.Pt(-1, 0), "b _ d"},
		{"wide glyph outside r loses its tail", "ab世d", nil, image.Rect(3, 0, 5, 1), image.Pt(1, 0), "ab _ "},
		{"wide glyphs move down", "世a\nb世", nil, image.Rect(0, 0, 3, 2), image.Pt(0, 1), "___\n世a"},
		{"no move", "abc", nil, image.Rect(0, 0, 3, 1), image.Point{}, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := layerOf(tt.src)
			for _, p := range tt.hide {
				s.Hide(p.X, p.Y)
			}
			s.Move(tt.r, tt.d)
			if got := shown(s); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			assertWhole(t, s.c)
		})
	}

	t.Run("on a region", func(t *testing.T) {
		t.Parallel()
		c := Parse("abcd\nefgh", method)
		s := c.Region(c.Bounds(), DefaultEnv, &Mixer{})
		s.Move(c.Bounds(), image.Pt(1, 1))
		if got, want := c.String(), "\n abc"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if !s.Visible(0, 0) {
			t.Fatal("a region cannot hide cells")
		}
	})

	t.Run("through a sub surface", func(t *testing.T) {
		t.Parallel()
		s := layerOf("abcd\nefgh")
		band := s.Sub(image.Rect(0, 1, 4, 2))
		band.Move(image.Rect(0, 0, 4, 1), image.Pt(2, 0))
		if got, want := shown(s), "abcd\n__ef"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestMoveInSubKeepsGlyphsWhole(t *testing.T) {
	t.Parallel()

	for _, d := range []image.Point{{1, 0}, {-1, 0}} {
		s := layerOf("世ab世")
		s.Sub(image.Rect(1, 0, 5, 1)).Move(image.Rect(0, 0, 4, 1), d)
		for x := 0; x < s.Width(); {
			w := s.Cell(x, 0).Width
			if w == 0 {
				t.Fatalf("move by %v: tail at %d without its glyph: %q", d, x, shown(s))
			}
			for i := 1; i < w; i++ {
				if x+i >= s.Width() || s.Cell(x+i, 0).Width != 0 {
					t.Fatalf("move by %v: glyph at %d lost its tail: %q", d, x, shown(s))
				}
			}
			x += w
		}
	}
}

func TestTravelLayer(t *testing.T) {
	t.Parallel()

	// A node's cells, abc, drawn at 5,0 of cells covering 0,0 to 10,1, as
	// for a node moving across them, composited at the same place.
	c := Parse("     abc  ", method)
	frame := Parse("..........", method)
	s := c.TravelLayer(image.Rect(5, 0, 8, 1), image.Rect(5, 0, 8, 1), frame.Bounds(), frame, DefaultEnv, &Mixer{})

	if s.Width() != 3 || s.Height() != 1 {
		t.Fatalf("size = %dx%d, want the node's, 3x1", s.Width(), s.Height())
	}
	if want := image.Rect(-5, 0, 5, 1); s.Reach() != want {
		t.Fatalf("reach = %v, want %v", s.Reach(), want)
	}
	if got := shown(s); got != "abc" {
		t.Fatalf("shown = %q, want the node's cells", got)
	}
	if s.Visible(-1, 0) || s.Cell(-1, 0) == nil || s.Cell(5, 0) != nil {
		t.Fatal("cells around the node are not reachable and hidden")
	}

	// Set draws anywhere in the reach, and shows the cell; Hide hides it.
	x := uv.Cell{Content: "x", Width: 1}
	s.Set(-5, 0, &x)
	s.Set(4, 0, &x)
	s.Set(5, 0, &x) // out of reach
	s.Hide(1, 0)
	if !s.Visible(-5, 0) || !s.Visible(4, 0) || s.Visible(1, 0) {
		t.Fatal("Set did not show cells, or Hide did not hide one")
	}
	frame.Composite(s, frame.Bounds())
	if got, want := frame.String(), "x....a.c.x"; got != want {
		t.Fatalf("composited %q, want %q", got, want)
	}

	// Layer is a travel layer reaching only its own cells.
	l := layerOf("ab")
	if want := image.Rect(0, 0, 2, 1); l.Reach() != want {
		t.Fatalf("layer reach = %v, want %v", l.Reach(), want)
	}
}

func TestSetClipsToReach(t *testing.T) {
	t.Parallel()

	// A wide glyph set at the edge of a region, or of part of a layer, is
	// drawn as blanks inside it rather than over the cell beside it.
	c := Parse("abcd", method)
	wide := uv.Cell{Content: "世", Width: 2}
	c.Region(image.Rect(1, 0, 2, 1), DefaultEnv, &Mixer{}).Set(0, 0, &wide)
	if got, want := c.String(), "a cd"; got != want {
		t.Fatalf("region: %q, want %q", got, want)
	}
	s := layerOf("abcd")
	s.Sub(image.Rect(1, 0, 3, 1)).Set(1, 0, &wide)
	if got := shown(s); got != "ab d" {
		t.Fatalf("sub: %q, want %q", got, "ab d")
	}
	s.Set(0, 0, &wide)
	if got := shown(s); got != "世 d" {
		t.Fatalf("whole: %q, want %q", got, "世 d")
	}
}

func TestSubFrom(t *testing.T) {
	t.Parallel()

	// A part of a moving node was the matching part of where it was, scaled
	// if its size changed.
	s := layerOf("abcd\nefgh") // at 2,1, 4×2
	s.From = image.Rect(10, 5, 14, 7)
	if got, want := s.Sub(image.Rect(0, 1, 4, 2)).From, image.Rect(10, 6, 14, 7); got != want {
		t.Fatalf("row 1 was at %v, want %v", got, want)
	}
	s.From = image.Rect(10, 5, 18, 9)
	if got, want := s.Sub(image.Rect(2, 1, 4, 2)).From, image.Rect(14, 7, 18, 9); got != want {
		t.Fatalf("resized, part was at %v, want %v", got, want)
	}
	s.From = image.Rectangle{}
	if got := s.Sub(image.Rect(0, 0, 2, 1)).From; !got.Empty() {
		t.Fatalf("not moving, part was at %v", got)
	}
}

func TestCells(t *testing.T) {
	t.Parallel()

	s := layerOf("ab\ncd")
	s.Hide(1, 0)
	var got []string
	for at, c := range s.Cells() {
		got = append(got, fmt.Sprintf("%d,%d=%s", at.X, at.Y, c.Content))
		// A cell hidden before it comes is skipped.
		s.Hide(0, 1)
	}
	if want := "0,0=a 1,1=d"; strings.Join(got, " ") != want {
		t.Fatalf("Cells gave %q, want %q", strings.Join(got, " "), want)
	}
	// Breaking out early stops it.
	for range s.Cells() {
		break
	}
}
