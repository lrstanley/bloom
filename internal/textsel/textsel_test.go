// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// row returns a function giving the cells of the first row of s.
func row(s string) (cell func(x int) *uv.Cell, width int) {
	c := canvas.Parse(s, ansi.GraphemeWidth)
	return func(x int) *uv.Cell { return c.CellAt(x, 0) }, c.Width()
}

func TestWordsAt(t *testing.T) {
	t.Parallel()

	ws := Words{Chars: "@-./_~?&=%+#"}
	for _, tc := range []struct {
		s      string
		at     int
		x0, x1 int
		ok     bool
	}{
		{"open ~/src/main.go now", 8, 5, 17, true},
		{"open ~/src/main.go now", 4, 0, 0, false},
		{"say 日本語 ok", 6, 4, 9, true}, // the second half of a wide character
		{"a, b", 1, 0, 0, false},
		{"emoji 🎉 here", 6, 6, 7, true},
		{"short", 9, 0, 0, false},
	} {
		cell, w := row(tc.s)
		x0, x1, ok := ws.At(w, tc.at, cell)
		if ok != tc.ok || ok && (x0 != tc.x0 || x1 != tc.x1) {
			t.Errorf("%q at %d: %d-%d %v, want %d-%d %v", tc.s, tc.at, x0, x1, ok, tc.x0, tc.x1, tc.ok)
		}
	}
}

func TestRowText(t *testing.T) {
	t.Parallel()

	c := canvas.Parse("a日本b", ansi.GraphemeWidth)
	// Starting on the second half of a wide character takes it whole.
	if got := RowText(c.CellAt, 2, 4, 0); got != "日本" {
		t.Fatalf("got %q", got)
	}
	if got := RowText(c.CellAt, 0, 9, 0); got != "a日本b    " {
		t.Fatalf("past the end: got %q", got)
	}
}

func TestCutText(t *testing.T) {
	t.Parallel()

	full := "the quick brown fox"
	src := canvas.Parse("the quic…", ansi.GraphemeWidth)
	for _, tc := range []struct {
		x0, x1 int
		want   string
	}{
		{0, 2, "the"},
		{4, 7, "quick"}, // ends just before the "…": the rest of the word
		{4, 8, "quick brown fox"},
	} {
		got, ok := CutText(src, full, 0, tc.x0, tc.x1)
		if !ok || got != tc.want {
			t.Errorf("%d-%d: %q %v, want %q", tc.x0, tc.x1, got, ok, tc.want)
		}
	}
	if _, ok := CutText(canvas.Parse("not cut", ansi.GraphemeWidth), full, 0, 0, 3); ok {
		t.Error("a row with no … was cut")
	}
}

func TestWrapJoins(t *testing.T) {
	t.Parallel()

	src := canvas.Parse("hello\nworld\nabcde\nfgh\nnext", ansi.GraphemeWidth)
	got := WrapJoins(src, "hello world abcdefgh\nnext")
	want := []Join{JoinSpace, JoinSpace, JoinNone, JoinBreak, JoinBreak}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRowSpan(t *testing.T) {
	t.Parallel()

	from, to := image.Pt(3, 1), image.Pt(2, 3)
	for y, want := range [][2]int{1: {3, 9}, 2: {0, 9}, 3: {0, 2}} {
		if y == 0 {
			continue
		}
		if x0, x1 := RowSpan(from, to, y, 10); x0 != want[0] || x1 != want[1] {
			t.Errorf("row %d: %d-%d, want %v", y, x0, x1, want)
		}
	}
}

func TestEdgeStep(t *testing.T) {
	t.Parallel()

	// Content from 0 to 100, of which 10 to 20 is shown.
	for p, want := range map[int]int{5: -1, 10: 0, 15: 0, 20: 1, 30: 1} {
		if got := EdgeStep(p, 10, 20, 0, 100); got != want {
			t.Errorf("at %d: %d, want %d", p, got, want)
		}
	}
	// All of it shown: nothing to scroll to.
	if got := EdgeStep(5, 10, 20, 10, 20); got != 0 {
		t.Errorf("all shown: %d", got)
	}
}

func TestContrast(t *testing.T) {
	t.Parallel()

	black, white := color.Black, color.White
	if c := Contrast(black, white); c < 20.9 || c > 21.1 {
		t.Errorf("black on white: %.2f, want 21", c)
	}
	if c := Contrast(white, white); c != 1 {
		t.Errorf("white on white: %.2f, want 1", c)
	}
}
