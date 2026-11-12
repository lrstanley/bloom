// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMinSize(t *testing.T) {
	t.Parallel()

	app := Text("app").ID("app")
	f := Solve(80, 24, MinSize(80, 24, app))
	expectRect(t, f, "app", image.Rect(0, 0, 80, 24))

	for _, size := range []image.Point{{79, 24}, {80, 23}, {10, 2}} {
		n := MinSize(80, 24, app)
		if _, ok := Solve(size.X, size.Y, n).Rect("app"); ok {
			t.Fatalf("%v: the child was laid out in a space too small for it", size)
		}
		if got := ansi.Strip(Render(size.X, size.Y, n)); !strings.Contains(got, "window") {
			t.Fatalf("%v: no note:\n%s", size, got)
		}
	}

	// No minimum on an axis is never too small on it.
	expectRect(t, Solve(5, 1, MinSize(0, 0, app)), "app", image.Rect(0, 0, 5, 1))
	expectRect(t, Solve(80, 1, MinSize(80, -1, app)), "app", image.Rect(0, 0, 80, 1))
}

func TestMinSizeNote(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		w, h, minW, minH int
		want             string
	}{
		{40, 10, 30, 24, "window too small, min 30x24"},
		{40, 10, 0, 24, "window too small, min 24h"},
		{30, 10, 40, 0, "window too small, min 40w"},
		{20, 10, 30, 0, "window too small"},
		{26, 10, 80, 24, "window too small"},
		{27, 10, 80, 24, "window too small, min 80x24"},
		{10, 10, 80, 24, "window to…"},
	} {
		got := ansi.Strip(Render(c.w, c.h, MinSize(c.minW, c.minH, Text("app"))))
		lines := strings.Split(got, "\n")
		if len(lines) != c.h {
			t.Fatalf("%dx%d: %d lines", c.w, c.h, len(lines))
		}
		// One line, in the middle.
		mid := lines[(c.h-1)/2]
		lines[(c.h-1)/2] = ""
		if strings.TrimSpace(mid) != c.want || strings.TrimSpace(strings.Join(lines, "")) != "" {
			t.Fatalf("%dx%d with a minimum of %dx%d: want %q in the middle:\n%s", c.w, c.h, c.minW, c.minH, c.want, got)
		}
		if lead, trail := len(mid)-len(strings.TrimLeft(mid, " ")), len(mid)-len(strings.TrimRight(mid, " ")); lead-trail > 0 || trail-lead > 1 {
			t.Fatalf("%dx%d: the note is not centered: %q", c.w, c.h, mid)
		}
	}
}

func TestMinSizeNoteFaded(t *testing.T) {
	t.Parallel()

	// The note is drawn in a gray between the terminal's foreground and its
	// background, white on black until the terminal reports its own.
	got := Render(40, 1, MinSize(80, 24, Text("app")))
	var gray int
	at := strings.Index(got, "\x1b[38;2;")
	if at < 0 {
		t.Fatalf("the note has no color of its own: %q", got)
	}
	if _, err := fmt.Sscanf(got[at:], "\x1b[38;2;%d;%d;%dm", &gray, &gray, &gray); err != nil || gray <= 0x40 || gray >= 0xc0 {
		t.Fatalf("the note is not faded: %q", got)
	}
}

func TestMinSizeNaturalSize(t *testing.T) {
	t.Parallel()

	// Drawn at its natural height, it is given its minimum height.
	got := Render(30, 0, MinSize(0, 5, Text("app")))
	if lines := strings.Split(got, "\n"); len(lines) != 5 || !strings.HasPrefix(lines[0], "app") {
		t.Fatalf("natural height:\n%s", got)
	}
	if got = MinSize(6, 2, Text("app")).View(); got != "app   \n      " {
		t.Fatalf("view: %q", got)
	}

	// In a slot sized by its content, it asks for its minimum where it fits.
	f := Solve(40, 3, Row(Auto(MinSize(12, 0, Text("app"))).ID("side"), Fill(Text("main"))))
	expectRect(t, f, "side", image.Rect(0, 0, 12, 3))
	f = Solve(40, 3, Row(Auto(MinSize(5, 0, Text("a long label"))).ID("side"), Fill(Text("main"))))
	expectRect(t, f, "side", image.Rect(0, 0, 12, 3))
}
