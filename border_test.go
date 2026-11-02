// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

var rounded = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

func TestBoxEdges(t *testing.T) {
	t.Parallel()

	box := Box(rounded, Text("body").ID("body")).
		Edge(Top, Start, Text("title").ID("title")).
		Edge(Top, End, Text("1/9").ID("count")).
		Edge(Top, End, Text("!").ID("flag")).
		Edge(Bottom, Center, Text("help").ID("help")).
		Edge(Right, Stretch, Text("x\nx\nx").ID("bar"))
	f := Solve(box, 20, 5)
	expectRect(t, f, "body", image.Rect(1, 1, 19, 4))
	expectRect(t, f, "title", image.Rect(2, 0, 7, 1))
	expectRect(t, f, "count", image.Rect(15, 0, 18, 1))
	expectRect(t, f, "flag", image.Rect(13, 0, 14, 1))
	expectRect(t, f, "help", image.Rect(8, 4, 12, 1+4))
	expectRect(t, f, "bar", image.Rect(19, 1, 20, 4))

	want := strings.Join([]string{
		"╭─title──────!─1/9─╮",
		"│body              x",
		"│                  x",
		"│                  x",
		"╰───────help───────╯",
	}, "\n")
	if got := ansi.Strip(Render(box, 20, 5)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBoxEdgesShareWhatIsLeft(t *testing.T) {
	t.Parallel()

	// The ends are placed first and given what they want while there is
	// room; the middle gets what is left, centered as far as the ends allow.
	box := Box(rounded, nil).
		Edge(Top, Center, Text("mid").ID("mid")).
		Edge(Top, Start, Text("abcdef").ID("a")).
		Edge(Top, End, Text("xyz").ID("z"))
	f := Solve(box, 14, 3)
	expectRect(t, f, "a", image.Rect(2, 0, 8, 1))
	expectRect(t, f, "z", image.Rect(9, 0, 12, 1))
	if _, ok := f.Rect("mid"); ok {
		t.Fatal("a middle child with no room left was shown")
	}

	f = Solve(box, 20, 3)
	expectRect(t, f, "mid", image.Rect(9, 0, 12, 1))

	// Clipped to the room there is, rather than drawn over the corner.
	if got := ansi.Strip(Render(Box(rounded, nil).Edge(Top, Start, Text("abcdefgh")), 8, 3)); !strings.HasPrefix(got, "╭─abcd─╮") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestBoxEdgesNeedABorder(t *testing.T) {
	t.Parallel()

	noTop := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true)
	f := Solve(Box(noTop, Text("x")).Edge(Top, Start, Text("t").ID("t")), 10, 3)
	if _, ok := f.Rect("t"); ok {
		t.Fatal("a child on a side with no border was shown")
	}

	// A box too small for its frame shows nothing on its edges either.
	box := Box(rounded, Text("x")).Edge(Top, Start, Text("t").ID("t"))
	if got := Render(Row(Fixed(1, box)), 4, 3); strings.Contains(got, "t") {
		t.Fatalf("a cramped box showed its edge:\n%s", got)
	}

	// Margins are outside the border.
	margins := rounded.Margin(1, 2)
	f = Solve(Box(margins, Text("x")).Edge(Top, Start, Text("t").ID("t")).Edge(Left, Start, Text("l").ID("l")), 12, 6)
	expectRect(t, f, "t", image.Rect(4, 1, 5, 2))
	expectRect(t, f, "l", image.Rect(2, 2, 3, 3))
}

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
	got := ansi.Strip(Render(tree(lines, 10), 8, 6))
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
	if fits := ansi.Strip(Render(tree("line", 0), 8, 6)); strings.Contains(fits, "#") {
		t.Fatalf("a scrollbar was drawn for content that fits:\n%s", fits)
	}
	// No such node, no scrollbar.
	missing := Box(rounded, Text("x")).Edge(Right, Stretch, ScrollbarOf("nope"))
	if none := ansi.Strip(Render(missing, 8, 6)); strings.Contains(none, "┃") {
		t.Fatalf("a scrollbar was drawn for a missing node:\n%s", none)
	}

	// Nor for a scroll clipped out of view, here below the outer scroll's.
	clipped := Box(rounded, Scroll(Column(
		Fixed(3, Text("a\nb\nc")),
		Fixed(2, Scroll(Text(lines).ID("in"), 0)),
	), 0)).Edge(Right, Stretch, ScrollbarOf("in").Thumb("#", lipgloss.NewStyle()))
	if out := ansi.Strip(Render(clipped, 8, 5)); strings.Contains(out, "#") {
		t.Fatalf("a scrollbar was drawn for a scroll out of view:\n%s", out)
	}

	// In a row, it takes a column, and draws its track.
	row := Row(Fill(Text("x")), Scrollbar(4, 2, 2).Track(".", lipgloss.NewStyle()))
	if inRow := ansi.Strip(Render(row, 3, 4)); inRow != "x .\n  .\n  ┃\n  ┃" {
		t.Fatalf("got:\n%q", inRow)
	}
}

func TestBorderFx(t *testing.T) {
	t.Parallel()

	red := color.RGBA{R: 0xff, A: 0xff}
	var given image.Rectangle
	spy := fx.Func(func(s *canvas.Surface, _ float64) { given = s.Rect() })
	box := Box(rounded.Margin(1), Text("in")).
		Edge(Top, Start, Text("t")).
		BorderFx(1, spy, fx.Recolor{Fg: red})
	out := Render(box, 8, 6)

	// The effects are given the box less its margins, whose edge is the
	// border.
	if want := image.Rect(1, 1, 7, 5); given != want {
		t.Fatalf("effects were given %v, want %v", given, want)
	}
	c := canvas.Parse(out, ansi.GraphemeWidth)
	if got := c.CellAt(1, 1); got.Content != "╭" || got.Style.Fg != red {
		t.Fatalf("the border was not recolored: %+v", got)
	}
	for _, p := range []image.Point{{2, 2}, {3, 1}} {
		if got := c.CellAt(p.X, p.Y); got.Style.Fg != nil {
			t.Fatalf("what is inside or on an edge was recolored at %v: %+v", p, got)
		}
	}
}

func TestBorderFxUnchangedFrame(t *testing.T) {
	t.Parallel()

	tree := func(t float64) Child {
		return Box(rounded, Text("x")).BorderFx(t, fx.Chase{To: color.RGBA{R: 0xff, A: 0xff}})
	}
	r := New()
	first := r.Render(10, 4, tree(0.25))
	if again := r.Render(10, 4, tree(0.25)); unsafe.StringData(first) != unsafe.StringData(again) {
		t.Fatal("an unchanged frame with border effects was drawn again")
	}
	if changed := r.Render(10, 4, tree(0.5)); changed == first {
		t.Fatal("a frame whose border effects moved on was not drawn")
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
	if got := ansi.Strip(Render(Row(box, Text("x")), 12, 6)); !strings.Contains(got, "#") {
		t.Fatalf("no scrollbar for a scroll a layouter built:\n%s", got)
	}
}

func TestBorderFxNoBorder(t *testing.T) {
	t.Parallel()

	// A box that is all margin has no frame for its effects to change.
	box := Box(lipgloss.NewStyle().Margin(1), nil).BorderFx(1, fx.Recolor{Bg: color.RGBA{R: 0xff, A: 0xff}})
	if got := Render(box, 2, 2); strings.Contains(got, "\x1b[") {
		t.Fatalf("effects changed the margins: %q", got)
	}
}

func TestScrollX(t *testing.T) {
	t.Parallel()

	long := Text("0123456789abcdef\nxy").ID("text")
	f := Solve(Scroll(long, 1).X(3), 6, 1)
	expectRect(t, f, "text", image.Rect(-3, -1, 13, 1))
	if got := ansi.Strip(Render(Scroll(long, 0).X(3), 6, 2)); strings.TrimSpace(got) != "345678" {
		t.Fatalf("got %q", got)
	}
	// Without X, the child gets the scroll's width.
	expectRect(t, Solve(Scroll(long, 0), 6, 1), "text", image.Rect(0, 0, 6, 2))
	if got := Debug(Scroll(long, 1).X(3), 6, 1); !strings.Contains(got, "Scroll 0,0 6x1 offset=1 x=3") {
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
	got := ansi.Strip(Render(box, 8, 3))
	if want := "╭──────╮\n│56789a│\n╰──##──╯"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// In a column, it takes a row.
	col := Column(Fill(Text("x")), Scrollbar(4, 2, 2).Horizontal())
	if inCol := ansi.Strip(Render(col, 4, 2)); inCol != "x   \n  ━━" {
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

func TestBoxEdgeTextRunsDownTheSides(t *testing.T) {
	t.Parallel()

	box := Box(rounded, Text("")).
		Edge(Left, Start, Text("AB").Truncate()).
		Edge(Right, End, Text("\x1b[31m✕\x1b[m"))
	got := Render(box, 4, 4)
	c := canvas.Parse(got, ansi.GraphemeWidth)
	if a, b := c.CellAt(0, 1).Content, c.CellAt(0, 2).Content; a != "A" || b != "B" {
		t.Fatalf("left edge shows %q, %q:\n%s", a, b, ansi.Strip(got))
	}
	if x := c.CellAt(3, 2); x.Content != "✕" || x.Style.Fg == nil {
		t.Fatalf("right edge shows %+v:\n%s", x, ansi.Strip(got))
	}
	if stacked := stack("a\x1b[1mb\nc\x1b[m"); stacked != "a\x1b[1m\nb\nc\x1b[m" {
		t.Fatalf("stack: %q", stacked)
	}
}

func TestSameStyle(t *testing.T) {
	t.Parallel()

	a := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	b := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	c := a.Bold(true)
	upper := a.Transform(strings.ToUpper)
	switch {
	case !sameStyle(&a, &a):
		t.Fatal("a style differs from itself")
	case !sameStyle(&a, &b):
		t.Fatal("equal styles built apart differ")
	case sameStyle(&a, &c):
		t.Fatal("different styles match")
	case sameStyle(&upper, &upper):
		t.Fatal("a style with a transform matches")
	}
}

func TestFrameFollowsStyleChanges(t *testing.T) {
	t.Parallel()

	// The frame of a box's style is kept from the last frame only while the
	// style is the same.
	r := New()
	tree := func(pad int) Child {
		return Row(Box(rounded.Padding(0, pad), Text("x").ID("x")), Space())
	}
	r.Render(20, 3, tree(0))
	r.Render(20, 3, tree(3))
	expectRect(t, r.Frame(), "x", image.Rect(4, 1, 5, 2))
	// Frames no box used are dropped, along with their styles.
	r.Render(20, 3, Text("plain"))
	for i, m := range r.r.frameMemos {
		if m.ok {
			t.Fatalf("the frame of box %d is still kept", i)
		}
	}
}

// BenchmarkBorderFrame draws a 200×60 box with a title, a counter, and a
// scrollbar that can be dragged on its border, whose border chases round a
// little more each frame.
func BenchmarkBorderFrame(b *testing.B) {
	lines := strings.TrimSuffix(strings.Repeat("a line of the list\n", 400), "\n")
	r := New()
	tree := func(t float64) Child {
		return Box(rounded, Scroll(Text(lines).ID("body"), 100)).
			Edge(Top, Start, Text(" title ")).
			Edge(Top, End, Text("101-158 of 400")).
			Edge(Right, Stretch, ScrollbarOf("body").OnScroll(func(int) tea.Cmd { return nil })).
			BorderFx(t, fx.Chase{To: color.RGBA{R: 0xff, A: 0xff}})
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.Render(benchW, benchH, tree(float64(i%100)/100))
	}
}
