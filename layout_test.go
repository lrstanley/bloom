// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
)

var rounded = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

func expectRect(t *testing.T, f *Frame, id string, want image.Rectangle) {
	t.Helper()
	if got, ok := f.Rect(id); !ok || got != want {
		t.Fatalf("%s rect = %v, %v; want %v", id, got, ok, want)
	}
}

// meteredLeaf counts how often it is measured.
type meteredLeaf struct{ n *int }

func (meteredLeaf) View() string { return "abcdefghij" }

func (l meteredLeaf) Measure(avail Size) Size {
	*l.n++
	return Size{W: min(10, avail.W), H: 1}
}

func nested(depth int, leaf Child) Child {
	c := leaf
	for d := range depth {
		if d%2 == 0 {
			c = Row(c, Text("x"))
		} else {
			c = Column(c, Text("x"))
		}
	}
	return c
}

func TestNestedMeasureIsLinear(t *testing.T) {
	t.Parallel()

	var n int
	Solve(nested(16, meteredLeaf{n: &n}), 200, 60)
	if n > 200 {
		t.Fatalf("the leaf was measured %d times at depth 16", n)
	}
}

func TestGrid(t *testing.T) {
	t.Parallel()
	children := []Child{Text("a").ID("a"), Text("bb").ID("b"), Text("c").ID("c")}
	n := Grid(2, children...).Gap(1, 1)
	f := Solve(n, 11, 7)
	expectRect(t, f, "a", image.Rect(0, 0, 5, 3))
	expectRect(t, f, "b", image.Rect(6, 0, 11, 3))
	expectRect(t, f, "c", image.Rect(0, 4, 5, 7))
	if got := n.View(); got != "a  bb\n     \nc    " {
		t.Fatalf("natural: %q", got)
	}
	f = Solve(Grid(2, Fixed(2, Text("a")).ID("fixed"), Fill(Text("b")).ID("fill")), 10, 2)
	expectRect(t, f, "fixed", image.Rect(0, 0, 2, 2))
	expectRect(t, f, "fill", image.Rect(5, 0, 10, 2))
	expectRect(t, Solve(Grid(0, Text("a").ID("a")), 4, 2), "a", image.Rect(0, 0, 4, 2))
	if got := Grid(2).View(); got != "" {
		t.Fatalf("empty: %q", got)
	}
	n = Grid(2, Text("a\nb"), Text("c"), Text("d\ne\nf")).Gap(0, 1)
	if got := Render(Column(n, Text("end").ID("end")), 10, 0); len(strings.Split(got, "\n")) != 7 {
		t.Fatalf("natural row heights: %q", got)
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		node  WrapNode
		width int
		want  string
	}{
		{"one line", Wrap(Text("aa"), Text("bbb")), 6, "aabbb"},
		{"wrap", Wrap(Text("aa"), Text("bbb")).Gap(1, 1), 5, "aa \n   \nbbb"},
		{"overwide", Wrap(Text("abcdefgh"), Text("bb")).Gap(1, 0), 4, "abcd\nbb  "},
		{"tall", Wrap(Text("a\nb"), Text("c"), Text("dd")).Gap(1, 1), 3, "a c\nb  \n   \ndd "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ansi.Strip(Render(tt.node, tt.width, 0)); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	n := Wrap(Text("aaa").ID("a"), Text("bbb").ID("b")).Gap(1, 1)
	expectRect(t, Solve(Column(Auto(n), Text("end").ID("end")), 5, 10), "end", image.Rect(0, 3, 5, 4))
	if got := n.View(); got != "aaa bbb" {
		t.Fatalf("natural width: %q", got)
	}
	if got := Wrap().View(); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestWrapMeasureCache(t *testing.T) {
	t.Parallel()
	var calls int
	children := make([]Child, 120)
	for i := range children {
		children[i] = meteredLeaf{n: &calls}
	}
	Solve(Column(Auto(Wrap(children...).Gap(1, 1)), Text("footer")), 40, 60)
	if calls != len(children) {
		t.Fatalf("measured %d times for %d children", calls, len(children))
	}
}

func TestGridNaturalRows(t *testing.T) {
	t.Parallel()
	n := Grid(2, Text("a"), Text("b"), Text("c\nd\ne").ID("tall")).Gap(1, 1)
	expectRect(t, Solve(n, 9, 0), "tall", image.Rect(0, 2, 4, 5))
}

func TestPresent(t *testing.T) {
	t.Parallel()
	p := anim.NewPresence()
	for _, vertical := range []bool{false, true} {
		children := []Child{Text("a").ID("a"), Fill(Present(p, Text("hidden").ID("hidden"))).ID("slot"), Text("b").ID("b")}
		n := Row(children...).Gap(2)
		if vertical {
			n = Column(children...).Gap(2)
		}
		f := Solve(n, 10, 10)
		if _, ok := f.Rect("hidden"); ok {
			t.Fatal("hidden presence has an entry")
		}
		if _, ok := f.Rect("slot"); ok {
			t.Fatal("hidden slot has an entry")
		}
		want := image.Rect(3, 0, 4, 10)
		if vertical {
			want = image.Rect(0, 3, 10, 4)
		}
		expectRect(t, f, "b", want)
	}
	if got := Render(Present(p, Text("hidden")), 0, 0); got != "" {
		t.Fatalf("absent root: %q", got)
	}
	p.Show()
	f := Solve(Row(Present(p, Text("shown").ID("shown")), Text("b").ID("b")).Gap(1), 10, 1)
	expectRect(t, f, "b", image.Rect(6, 0, 7, 1))
	expectRect(t, f, "shown", image.Rect(0, 0, 5, 1))
}

func TestLayerPresence(t *testing.T) {
	t.Parallel()
	p := anim.NewPresence()
	r := New()
	tree := func() Child {
		return Overlay(Text("base").ID("base"), Layer(Text("dialog").ID("child")).ID("layer").Modal().Backdrop(fx.Dim{}).Shadow(1, 1, fx.Dim{}).Presence(p, fx.Fade{}))
	}
	base := r.Render(12, 3, Text("base").ID("base"))
	if got := r.Render(12, 3, tree()); got != base {
		t.Fatalf("hidden layer changed rendering: %q", got)
	}
	for _, id := range []string{"child", "layer"} {
		if _, ok := r.Frame().Rect(id); ok {
			t.Fatalf("hidden %s has entry", id)
		}
	}
	if hit, _ := r.Frame().hit(image.Pt(1, 1)); hit < 0 || r.Frame().entries[hit].id != "base" {
		t.Fatal("hidden layer took hit")
	}
	p.Show()
	r.Render(12, 3, tree())
	if _, ok := r.Frame().Rect("layer"); !ok {
		t.Fatal("visible layer missing")
	}
	p.Hide()
	r.Render(12, 3, tree())
	if _, ok := r.Frame().Rect("layer"); !ok {
		t.Fatal("exiting layer omitted before presence finished")
	}
	p = anim.NewPresence()
	r.Render(12, 3, tree())
	if _, ok := r.Frame().Rect("layer"); ok {
		t.Fatal("hidden layer cached as visible")
	}
}

func BenchmarkWrapFrame(b *testing.B) {
	children := make([]Child, 120)
	for i := range children {
		children[i] = Text("service running").Style(benchItem)
	}
	tree := Wrap(children...).Gap(1, 1)
	r := New()
	b.ReportAllocs()
	for b.Loop() {
		r.View(benchW, benchH, tree)
	}
}

func BenchmarkGridFrame(b *testing.B) {
	children := make([]Child, 40)
	for i := range children {
		children[i] = Box(benchBorder, Text("service\n100 req/s"))
	}
	tree := Grid(5, children...).Gap(1, 1)
	r := New()
	b.ReportAllocs()
	for b.Loop() {
		r.View(benchW, benchH, tree)
	}
}

func TestBoxTooSmallForItsFrame(t *testing.T) {
	t.Parallel()

	box := Box(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()), Text("x"))
	if got := Render(Row(Fixed(1, box), Text("y")), 4, 3); strings.ContainsAny(got, "╭│╰") {
		t.Fatalf("a box one cell wide drew part of its border:\n%s", got)
	}
	if got := Render(Row(Fixed(2, box)), 4, 3); !strings.Contains(got, "╭╮") {
		t.Fatalf("a box just big enough for its border was not drawn:\n%s", got)
	}
}

func TestEmptySlotTakesNoGap(t *testing.T) {
	t.Parallel()
	for _, n := range []Child{Fixed(0, Text("x")), Auto(Text(""))} {
		f := Solve(Row(n, Text("b").ID("b"), n, Text("c").ID("c"), n).Gap(2), 10, 1)
		expectRect(t, f, "b", image.Rect(0, 0, 1, 1))
		expectRect(t, f, "c", image.Rect(3, 0, 4, 1))
		if got := Render(Row(n, Text("b"), n, Text("c")).Gap(2), 0, 0); got != "b  c" {
			t.Fatalf("natural size: %q", got)
		}
	}
	// A growing slot with no content still takes its gap, so measuring
	// leaves room for it rather than wrapping the text.
	if got := Render(Row(Text("a b"), Space()).Gap(1), 0, 0); got != "a b " {
		t.Fatalf("natural size with Space: %q", got)
	}
}

// TestFillNaturalSize checks that growing children, which share space by
// weight rather than by size, each get at least their natural size when
// their row is at its natural size.
func TestFillNaturalSize(t *testing.T) {
	t.Parallel()
	wraps := lipgloss.NewStyle() // Styled text wraps where plain text is cut.
	row := Row(
		Fill(Text("ab").Style(wraps)).ID("a"),
		Fill(Text("abcdef").Style(wraps)).ID("b"),
		Grow(2, Text("abcd").Style(wraps)).ID("c"),
	).Gap(1)
	// 22 cells, plus the gaps, is the least in which b gets its 6.
	f := Solve(row, 0, 0)
	expectRect(t, f, "a", image.Rect(0, 0, 5, 1))
	expectRect(t, f, "b", image.Rect(6, 0, 12, 1))
	expectRect(t, f, "c", image.Rect(13, 0, 24, 1))

	// A child held to its Min or Max leaves the others the rest, as
	// sharing out space does, so the row needs no more than their sizes.
	for want, row := range map[int]Child{
		11: Row(Fill(Text("x").Style(wraps)).Max(1), Fill(Text("abcdefghij").Style(wraps))),
		30: Row(Fill(Text("x").Style(wraps)).Min(20), Fill(Text("abcdefghij").Style(wraps))),
	} {
		r, _ := Solve(Column(row, Text("end").ID("end")), 0, 0).Rect("end")
		if r.Dx() != want || r.Min.Y != 1 {
			t.Fatalf("row of %d cells measured %v", want, r)
		}
	}

	// A column measures the row's height at that size, so the row is as tall
	// stretched across the column as it is at its natural size.
	for _, w := range []int{0, 40} {
		if r, _ := Solve(Column(row, Text("end").ID("end")), w, 0).Rect("end"); r.Min.Y != 1 {
			t.Fatalf("at width %d, the row is %d lines tall; want 1", w, r.Min.Y)
		}
	}
}

func TestGridRowHeights(t *testing.T) {
	t.Parallel()
	grid := Grid(1, Text("a").ID("a"), Text("b\nc\nd").ID("b"))
	// Extra height is shared on top of each row's own height, so no row
	// loses content it had at its natural height.
	for h, want := range map[int][2]image.Rectangle{
		4: {image.Rect(0, 0, 1, 1), image.Rect(0, 1, 1, 4)},
		6: {image.Rect(0, 0, 1, 2), image.Rect(0, 2, 1, 6)},
		7: {image.Rect(0, 0, 1, 2), image.Rect(0, 2, 1, 7)},
		// Short of height, rows give it up in proportion to their own.
		2: {image.Rect(0, 0, 1, 1), image.Rect(0, 1, 1, 2)},
	} {
		f := Solve(grid, 1, h)
		expectRect(t, f, "a", want[0])
		expectRect(t, f, "b", want[1])
	}
	if got := Render(grid, 1, 5); !strings.Contains(got, "d") {
		t.Fatalf("height 5 lost a line:\n%s", got)
	}
}

func TestAnchorToDirections(t *testing.T) {
	t.Parallel()

	// The anchor is 4×1 at 10,3, and the layer 2×1.
	tests := []struct {
		side Direction
		want image.Point
	}{
		{0, image.Pt(10, 4)}, // the default, Bottom.
		{Bottom, image.Pt(10, 4)},
		{Top, image.Pt(10, 2)},
		{Left, image.Pt(8, 3)},
		{Right, image.Pt(14, 3)},
		{TopLeft, image.Pt(8, 2)},
		{TopRight, image.Pt(14, 2)},
		{BottomLeft, image.Pt(8, 4)},
		{BottomRight, image.Pt(14, 4)},
	}
	for _, tt := range tests {
		r := New()
		r.Render(30, 10, Overlay(
			Column(Fixed(3, nil), Fixed(1, Row(Fixed(10, nil), Fixed(4, Text("anch")).ID("a")))),
			Layer(Text("LL")).Size(2, 1).ID("l").AnchorTo("a", tt.side),
		))
		want := image.Rectangle{Min: tt.want, Max: tt.want.Add(image.Pt(2, 1))}
		if got, ok := r.Frame().Rect("l"); !ok || got != want {
			t.Errorf("side %d: layer at %v (%v), want %v", tt.side, got, ok, tt.want)
		}
	}
}

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
}

func TestBoxEdgeAt(t *testing.T) {
	t.Parallel()

	// Junctions at exact cells, corners included, counted from either end,
	// beside a title placed with Edge, which keeps its own place.
	box := Box(rounded, Text("")).
		EdgeAt(Top, 4, Text("┬")).
		EdgeAt(Top, -3, Text("┬").ID("back")).
		EdgeAt(Left, 0, Text("├")).
		Edge(Top, Start, Text("ab").ID("title")).
		EdgeAt(Bottom, 20, Text("x").ID("outside")).
		EdgeAt(Bottom, 8, Text("long")). // clipped to the side, corner and all
		EdgeAt(Right, -1, Text("┤"))     // then drawn over the corner
	want := strings.Join([]string{
		"├─ab┬───┬─╮",
		"│         │",
		"╰───────lo┤",
	}, "\n")
	if got := ansi.Strip(Render(box, 11, 3)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	f := Solve(box, 11, 3)
	expectRect(t, f, "back", image.Rect(8, 0, 9, 1))
	expectRect(t, f, "title", image.Rect(2, 0, 4, 1))
	if _, ok := f.Rect("outside"); ok {
		t.Fatal("a child past the end of the side was shown")
	}

	// Drawn in the order given: the title covers a junction under it.
	box = Box(rounded, Text("")).EdgeAt(Top, 2, Text("┬")).Edge(Top, Start, Text("ab"))
	if got := ansi.Strip(Render(box, 8, 3)); !strings.HasPrefix(got, "╭─ab───╮") {
		t.Fatalf("got:\n%s", got)
	}

	// A side without a border shows nothing.
	noTop := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true)
	f = Solve(Box(noTop, Text("x")).EdgeAt(Top, 2, Text("t").ID("t")).EdgeAt(Left, 0, Text("l").ID("l")), 10, 3)
	if _, ok := f.Rect("t"); ok {
		t.Fatal("a child on a side with no border was shown")
	}
	expectRect(t, f, "l", image.Rect(0, 0, 1, 1))
}
