// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/on"
)

func TestScrollbarThumb(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		total, visible, offset, h int
		top, bottom               int
	}{
		{total: 10, visible: 10, h: 10},                                   // all in view
		{total: 5, visible: 10, h: 10},                                    // shorter than its view
		{total: 20, visible: 10, h: 10, top: 0, bottom: 5},                // at the top
		{total: 20, visible: 10, offset: 10, h: 10, top: 5, bottom: 10},   // at the bottom
		{total: 20, visible: 10, offset: 99, h: 10, top: 5, bottom: 10},   // past the bottom
		{total: 20, visible: 10, offset: -3, h: 10, top: 0, bottom: 5},    // above the top
		{total: 1000, visible: 10, offset: 500, h: 10, top: 5, bottom: 6}, // at least a row
		{total: 20, visible: 6, offset: 3, h: 6, top: 1, bottom: 3},       // rounded
		{total: 20, visible: 10, offset: 5},                               // no room
		{total: 20, offset: 5, h: 10},                                     // nothing in view
		{total: 100, visible: 99, h: 10, top: 0, bottom: 9},               // leaves a cell to move in
		{total: 100, visible: 99, offset: 1, h: 10, top: 1, bottom: 10},   // and moves into it
		{total: 100, visible: 99, offset: 1, h: 1, top: 0, bottom: 1},     // unless there is only one
	} {
		top, bottom := Scrollbar(tc.total, tc.visible, tc.offset).thumbSpan(tc.h)
		if top != tc.top || bottom != tc.bottom {
			t.Errorf("%+v: thumb %d-%d", tc, top, bottom)
		}
	}
}

func TestScrollbarOf(t *testing.T) {
	t.Parallel()

	lines := strings.TrimSuffix(strings.Repeat("line\n", 20), "\n")
	tree := func(text string, offset int) Child {
		return Box(rounded, Scroll(Text(text).ID("body"), offset)).
			Edge(Right, Stretch, ScrollbarOf("body").Thumb("#", lipgloss.NewStyle()))
	}
	// 4 of 20 rows in view, from row 10: a thumb of one row, 10/16 of the
	// way down the rest of the track.
	got := ansi.Strip(Render(8, 6, tree(lines, 10)))
	want := strings.Join([]string{
		"╭──────╮",
		"│line  │",
		"│line  │",
		"│line  #",
		"│line  │",
		"╰──────╯",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// Content that fits shows the border, with no scrollbar over it.
	if fits := ansi.Strip(Render(8, 6, tree("line", 0))); strings.Contains(fits, "#") {
		t.Fatalf("a scrollbar was drawn for content that fits:\n%s", fits)
	}
	// No such node, no scrollbar.
	missing := Box(rounded, Text("x")).Edge(Right, Stretch, ScrollbarOf("nope"))
	if none := ansi.Strip(Render(8, 6, missing)); strings.Contains(none, "┃") {
		t.Fatalf("a scrollbar was drawn for a missing node:\n%s", none)
	}

	// Nor for a scroll clipped out of view, here below the outer scroll's.
	clipped := Box(rounded, Scroll(Column(
		Fixed(3, Text("a\nb\nc")),
		Fixed(2, Scroll(Text(lines).ID("in"), 0)),
	), 0)).Edge(Right, Stretch, ScrollbarOf("in").Thumb("#", lipgloss.NewStyle()))
	if out := ansi.Strip(Render(8, 5, clipped)); strings.Contains(out, "#") {
		t.Fatalf("a scrollbar was drawn for a scroll out of view:\n%s", out)
	}

	// In a row, it takes a column, and draws its track.
	row := Row(Fill(Text("x")), Scrollbar(4, 2, 2).Track(".", lipgloss.NewStyle()))
	if inRow := ansi.Strip(Render(3, 4, row)); inRow != "x .\n  .\n  ┃\n  ┃" {
		t.Fatalf("got:\n%q", inRow)
	}
}

// scrollLayout is a Layouter that builds a scroll, which is built again for
// each size it is given while measuring and arranging.
type scrollLayout struct{ lines string }

func (scrollLayout) View() string { return "" }

func (l scrollLayout) Layout(Context) Child { return Scroll(Text(l.lines).ID("body"), 10) }

func TestScrollbarOfLayouter(t *testing.T) {
	t.Parallel()

	// The box is measured at its natural width before it is arranged, so
	// the layouter's scroll is built more than once; the scrollbar follows
	// the one on screen.
	lines := strings.TrimSuffix(strings.Repeat("line\n", 20), "\n")
	box := Box(rounded, scrollLayout{lines}).Edge(Right, Stretch, ScrollbarOf("body").Thumb("#", lipgloss.NewStyle()))
	if got := ansi.Strip(Render(12, 6, Row(box, Text("x")))); !strings.Contains(got, "#") {
		t.Fatalf("no scrollbar for a scroll a layouter built:\n%s", got)
	}
}

func TestScrollX(t *testing.T) {
	t.Parallel()

	long := Text("0123456789abcdef\nxy").ID("text")
	f := Solve(6, 1, Scroll(long, 1).X(3))
	expectRect(t, f, "text", image.Rect(-3, -1, 13, 1))
	if got := ansi.Strip(Render(6, 2, Scroll(long, 0).X(3))); strings.TrimSpace(got) != "345678" {
		t.Fatalf("got %q", got)
	}
	// Without X, the child gets the scroll's width.
	expectRect(t, Solve(6, 1, Scroll(long, 0)), "text", image.Rect(0, 0, 6, 2))
	if got := Debug(6, 1, Scroll(long, 1).X(3)); !strings.Contains(got, "Scroll 0,0 6x1 offset=1 x=3") {
		t.Fatalf("Debug:\n%s", got)
	}
}

func TestScrollbarHorizontal(t *testing.T) {
	t.Parallel()

	// 16 columns, 6 in view from column 5: the bottom edge's 4 cells, kept a
	// cell from the corners, have a thumb of 2 cells, 5/10 of the way along.
	text := Text("0123456789abcdef").ID("text")
	box := Box(rounded, Scroll(text, 0).X(5)).
		Edge(Bottom, Stretch, ScrollbarOf("text").Horizontal().Thumb("#", lipgloss.NewStyle()))
	got := ansi.Strip(Render(8, 3, box))
	if want := "╭──────╮\n│56789a│\n╰──##──╯"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// In a column, it takes a row.
	col := Column(Fill(Text("x")), Scrollbar(4, 2, 2).Horizontal())
	if inCol := ansi.Strip(Render(4, 2, col)); inCol != "x   \n  ━━" {
		t.Fatalf("got %q", inCol)
	}
}

func TestScrollbarOnScroll(t *testing.T) {
	t.Parallel()

	lines := strings.TrimSuffix(strings.Repeat("line\n", 20), "\n")
	offset, calls := 0, 0
	tree := func() Child {
		return Box(rounded, Scroll(Text(lines).ID("body"), offset)).
			Edge(Right, Stretch, ScrollbarOf("body").OnScroll(func(o int) tea.Cmd {
				offset, calls = o, calls+1
				return nil
			}))
	}
	r := New()
	frame := func() { r.Render(8, 6, tree()) }

	// The track is the right border's 4 rows, from y=1; 4 of 20 rows in view
	// make a thumb of one row, at the top.
	frame()
	r.Update(down(7, 1)) // on the thumb: grabbed, nothing moves yet
	if calls != 0 {
		t.Fatalf("pressing the thumb scrolled to %d", offset)
	}
	r.Update(dragTo(7, 3))
	if offset != 11 {
		t.Fatalf("dragged two rows down to offset %d, want 11", offset)
	}
	frame()
	r.Update(dragTo(7, 9)) // past the end
	if offset != 16 {
		t.Fatalf("dragged past the end to offset %d, want 16", offset)
	}
	frame()
	r.Update(release(7, 9))
	frame()

	// Pressing and releasing the thumb leaves an offset it cannot show
	// exactly as it is, and so does dragging away and back.
	offset, calls = 1, 0
	frame()
	r.Update(down(7, 1))
	r.Update(release(7, 1))
	frame()
	r.Update(down(7, 1))
	r.Update(dragTo(7, 3))
	frame()
	r.Update(dragTo(7, 1))
	frame()
	r.Update(release(7, 1))
	if offset != 1 {
		t.Fatalf("a press on the thumb, and a drag back to it, moved offset 1 to %d", offset)
	}

	// A press on the track jumps there, with the thumb centered on it.
	offset = 0
	frame()
	r.Update(down(7, 3))
	r.Update(release(7, 3))
	if offset != 11 {
		t.Fatalf("clicking the track scrolled to %d, want 11", offset)
	}

	// A scrollbar with nothing to scroll takes no clicks, which reach the box.
	clicked := false
	fits := Box(rounded, Scroll(Text("x").ID("body"), 0)).
		On(on.Click(func() tea.Cmd { clicked = true; return nil })).
		Edge(Right, Stretch, ScrollbarOf("body").OnScroll(func(int) tea.Cmd { t.Fatal("scrolled"); return nil }))
	r.Render(8, 6, fits)
	r.ClickAt(7, 2)
	if !clicked {
		t.Fatal("a click on a border with no scrollbar did not reach the box")
	}
}
