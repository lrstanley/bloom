// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	Solve(200, 60, nested(16, meteredLeaf{n: &n}))
	if n > 200 {
		t.Fatalf("the leaf was measured %d times at depth 16", n)
	}
}

func TestGrid(t *testing.T) {
	t.Parallel()
	children := []Child{Text("a").ID("a"), Text("bb").ID("b"), Text("c").ID("c")}
	n := Grid(2, children...).Gap(1, 1)
	f := Solve(11, 7, n)
	expectRect(t, f, "a", image.Rect(0, 0, 5, 3))
	expectRect(t, f, "b", image.Rect(6, 0, 11, 3))
	expectRect(t, f, "c", image.Rect(0, 4, 5, 7))
	if got := n.View(); got != "a  bb\n     \nc    " {
		t.Fatalf("natural: %q", got)
	}
	f = Solve(10, 2, Grid(2, Fixed(2, Text("a")).ID("fixed"), Fill(Text("b")).ID("fill")))
	expectRect(t, f, "fixed", image.Rect(0, 0, 2, 2))
	expectRect(t, f, "fill", image.Rect(5, 0, 10, 2))
	expectRect(t, Solve(4, 2, Grid(0, Text("a").ID("a"))), "a", image.Rect(0, 0, 4, 2))
	if got := Grid(2).View(); got != "" {
		t.Fatalf("empty: %q", got)
	}
	n = Grid(2, Text("a\nb"), Text("c"), Text("d\ne\nf")).Gap(0, 1)
	if got := Render(10, 0, Column(n, Text("end").ID("end"))); len(strings.Split(got, "\n")) != 7 {
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
			if got := ansi.Strip(Render(tt.width, 0, tt.node)); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	n := Wrap(Text("aaa").ID("a"), Text("bbb").ID("b")).Gap(1, 1)
	expectRect(t, Solve(5, 10, Column(Auto(n), Text("end").ID("end"))), "end", image.Rect(0, 3, 5, 4))
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
	Solve(40, 60, Column(Auto(Wrap(children...).Gap(1, 1)), Text("footer")))
	if calls != len(children) {
		t.Fatalf("measured %d times for %d children", calls, len(children))
	}
}

func TestGridNaturalRows(t *testing.T) {
	t.Parallel()
	n := Grid(2, Text("a"), Text("b"), Text("c\nd\ne").ID("tall")).Gap(1, 1)
	expectRect(t, Solve(9, 0, n), "tall", image.Rect(0, 2, 4, 5))
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
		f := Solve(10, 10, n)
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
	if got := Render(0, 0, Present(p, Text("hidden"))); got != "" {
		t.Fatalf("absent root: %q", got)
	}
	p.Show()
	f := Solve(10, 1, Row(Present(p, Text("shown").ID("shown")), Text("b").ID("b")).Gap(1))
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
	if got := Render(4, 3, Row(Fixed(1, box), Text("y"))); strings.ContainsAny(got, "╭│╰") {
		t.Fatalf("a box one cell wide drew part of its border:\n%s", got)
	}
	if got := Render(4, 3, Row(Fixed(2, box))); !strings.Contains(got, "╭╮") {
		t.Fatalf("a box just big enough for its border was not drawn:\n%s", got)
	}
}

func TestEmptySlotTakesNoGap(t *testing.T) {
	t.Parallel()
	for _, n := range []Child{Fixed(0, Text("x")), Auto(Text(""))} {
		f := Solve(10, 1, Row(n, Text("b").ID("b"), n, Text("c").ID("c"), n).Gap(2))
		expectRect(t, f, "b", image.Rect(0, 0, 1, 1))
		expectRect(t, f, "c", image.Rect(3, 0, 4, 1))
		if got := Render(0, 0, Row(n, Text("b"), n, Text("c")).Gap(2)); got != "b  c" {
			t.Fatalf("natural size: %q", got)
		}
	}
	// A growing slot with no content still takes its gap, so measuring
	// leaves room for it rather than wrapping the text.
	if got := Render(0, 0, Row(Text("a b"), Space()).Gap(1)); got != "a b " {
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
	f := Solve(0, 0, row)
	expectRect(t, f, "a", image.Rect(0, 0, 5, 1))
	expectRect(t, f, "b", image.Rect(6, 0, 12, 1))
	expectRect(t, f, "c", image.Rect(13, 0, 24, 1))

	// A child held to its Min or Max leaves the others the rest, as
	// sharing out space does, so the row needs no more than their sizes.
	for want, row := range map[int]Child{
		11: Row(Fill(Text("x").Style(wraps)).Max(1), Fill(Text("abcdefghij").Style(wraps))),
		30: Row(Fill(Text("x").Style(wraps)).Min(20), Fill(Text("abcdefghij").Style(wraps))),
	} {
		r, _ := Solve(0, 0, Column(row, Text("end").ID("end"))).Rect("end")
		if r.Dx() != want || r.Min.Y != 1 {
			t.Fatalf("row of %d cells measured %v", want, r)
		}
	}

	// A column measures the row's height at that size, so the row is as tall
	// stretched across the column as it is at its natural size.
	for _, w := range []int{0, 40} {
		if r, _ := Solve(w, 0, Column(row, Text("end").ID("end"))).Rect("end"); r.Min.Y != 1 {
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
		f := Solve(1, h, grid)
		expectRect(t, f, "a", want[0])
		expectRect(t, f, "b", want[1])
	}
	if got := Render(1, 5, grid); !strings.Contains(got, "d") {
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
	f := Solve(20, 5, box)
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
	if got := ansi.Strip(Render(20, 5, box)); got != want {
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
	f := Solve(14, 3, box)
	expectRect(t, f, "a", image.Rect(2, 0, 8, 1))
	expectRect(t, f, "z", image.Rect(9, 0, 12, 1))
	if _, ok := f.Rect("mid"); ok {
		t.Fatal("a middle child with no room left was shown")
	}

	f = Solve(20, 3, box)
	expectRect(t, f, "mid", image.Rect(9, 0, 12, 1))

	// Clipped to the room there is, rather than drawn over the corner.
	if got := ansi.Strip(Render(8, 3, Box(rounded, nil).Edge(Top, Start, Text("abcdefgh")))); !strings.HasPrefix(got, "╭─abcd─╮") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestBoxEdgesNeedABorder(t *testing.T) {
	t.Parallel()

	noTop := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true)
	f := Solve(10, 3, Box(noTop, Text("x")).Edge(Top, Start, Text("t").ID("t")))
	if _, ok := f.Rect("t"); ok {
		t.Fatal("a child on a side with no border was shown")
	}

	// A box too small for its frame shows nothing on its edges either.
	box := Box(rounded, Text("x")).Edge(Top, Start, Text("t").ID("t"))
	if got := Render(4, 3, Row(Fixed(1, box))); strings.Contains(got, "t") {
		t.Fatalf("a cramped box showed its edge:\n%s", got)
	}

	// Margins are outside the border.
	margins := rounded.Margin(1, 2)
	f = Solve(12, 6, Box(margins, Text("x")).Edge(Top, Start, Text("t").ID("t")).Edge(Left, Start, Text("l").ID("l")))
	expectRect(t, f, "t", image.Rect(4, 1, 5, 2))
	expectRect(t, f, "l", image.Rect(2, 2, 3, 3))
}

func TestBoxEdgeTextRunsDownTheSides(t *testing.T) {
	t.Parallel()

	box := Box(rounded, Text("")).
		Edge(Left, Start, Text("AB").Truncate()).
		Edge(Right, End, Text("\x1b[31m✕\x1b[m"))
	got := Render(4, 4, box)
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
	if got := ansi.Strip(Render(11, 3, box)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	f := Solve(11, 3, box)
	expectRect(t, f, "back", image.Rect(8, 0, 9, 1))
	expectRect(t, f, "title", image.Rect(2, 0, 4, 1))
	if _, ok := f.Rect("outside"); ok {
		t.Fatal("a child past the end of the side was shown")
	}

	// Drawn in the order given: the title covers a junction under it.
	box = Box(rounded, Text("")).EdgeAt(Top, 2, Text("┬")).Edge(Top, Start, Text("ab"))
	if got := ansi.Strip(Render(8, 3, box)); !strings.HasPrefix(got, "╭─ab───╮") {
		t.Fatalf("got:\n%s", got)
	}

	// A side without a border shows nothing.
	noTop := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true)
	f = Solve(10, 3, Box(noTop, Text("x")).EdgeAt(Top, 2, Text("t").ID("t")).EdgeAt(Left, 0, Text("l").ID("l")))
	if _, ok := f.Rect("t"); ok {
		t.Fatal("a child on a side with no border was shown")
	}
	expectRect(t, f, "l", image.Rect(0, 0, 1, 1))
}

func TestCenteredIgnoresSlotSizing(t *testing.T) {
	t.Parallel()

	// A slot's sizing applies in a row or column, not in Centered, which
	// places its child at its natural size.
	for _, c := range []Child{
		Centered(Fill(Text("x").ID("x"))),
		Centered(Fixed(0, Text("x").ID("x"))),
		Centered(Auto(Text("x").ID("x")).Align(Stretch)),
	} {
		expectRect(t, Solve(9, 5, c), "x", image.Rect(4, 2, 5, 3))
	}
}

// sided is a menu that draws an arrow toward the node its layer is against,
// and is taller below it than above it, as given by its Context.
type sided struct{}

func (sided) View() string { return "" }

func (sided) Layout(ctx Context) Child {
	side, ok := ctx.Side()
	if !ok {
		return Text("none").ID("arrow")
	}
	if side == Top {
		return Column(Text("item"), Text("item"), Text("vvvv").ID("arrow"))
	}
	return Column(Text("^^^^").ID("arrow"), Text("item"), Text("item"), Text("item"))
}

func TestContextSide(t *testing.T) {
	t.Parallel()

	tree := func(row int) Child {
		return Overlay(
			Column(Fixed(row, nil), Text("field").ID("field")),
			Layer(sided{}).ID("menu").AnchorTo("field", Bottom),
		)
	}

	// Below the field, with room.
	r := New()
	r.Render(10, 10, tree(2))
	f := r.Frame()
	expectRect(t, f, "menu", image.Rect(0, 3, 4, 7))
	expectRect(t, f, "arrow", image.Rect(0, 3, 4, 4))

	// Near the bottom, the menu is built again for the top, at its size
	// there, with the arrow toward the field.
	r.Render(10, 10, tree(8))
	f = r.Frame()
	expectRect(t, f, "menu", image.Rect(0, 5, 4, 8))
	expectRect(t, f, "arrow", image.Rect(0, 7, 4, 8))
	lines := strings.Split(ansi.Strip(r.Render(10, 10, tree(8))), "\n")
	if got := strings.TrimSpace(lines[7]); got != "vvvv" {
		t.Fatalf("row above the field is %q, want the arrow\n%s", got, strings.Join(lines, "\n"))
	}

	// A gap below the field stays a gap above it, once built again.
	r.Render(10, 10, Overlay(
		Column(Fixed(8, nil), Text("field").ID("field")),
		Layer(sided{}).ID("menu").AnchorTo("field", Bottom).Offset(0, 1),
	))
	expectRect(t, r.Frame(), "menu", image.Rect(0, 4, 4, 7))

	// Select all finds what is on screen, not what was built for the
	// bottom before the menu flipped.
	r.Render(10, 10, Overlay(
		Column(Fixed(8, nil), Text("field").ID("field")),
		Layer(Column(sided{}).ID("body").Selectable()).AnchorTo("field", Bottom),
	))
	if !r.SelectAll("body") {
		t.Fatal("select all found nothing in the menu")
	}
	if got, _ := r.Selection(); got != "item\nitem\nvvvv" {
		t.Fatalf("selected %q", got)
	}

	// Outside an anchored layer, there is no side.
	if got := ansi.Strip(Render(10, 1, sided{})); strings.TrimSpace(got) != "none" {
		t.Fatalf("outside a layer drew %q", got)
	}
	if got := ansi.Strip(Render(10, 1, Overlay(Text(""), Layer(sided{})))); !strings.Contains(got, "none") {
		t.Fatalf("in a layer that is not anchored drew %q", got)
	}
}

// field is a 1-row overlay with a 3-row menu anchored below its input,
// floating or not.
func field(float bool) Child {
	menu := Layer(Text("m1\nm2\nm3")).ID("menu").AnchorTo("input", Bottom)
	if float {
		menu = menu.Float()
	}
	return Overlay(Text("input").ID("input"), menu)
}

func TestFloatPastItsOverlay(t *testing.T) {
	t.Parallel()

	tree := func(float bool) Child {
		return Column(Fixed(1, field(float)), Text("one"), Text("two"), Text("three"), Text("four"))
	}

	// Kept to its overlay, the menu has one row, over the input.
	f := Solve(10, 5, tree(false))
	expectRect(t, f, "menu", image.Rect(0, 0, 2, 1))

	f = Solve(10, 5, tree(true))
	expectRect(t, f, "menu", image.Rect(0, 1, 2, 4))
	// The menu covers what it is over, as a layer does.
	want := strings.Join([]string{"input", "m1e", "m2o", "m3ree", "four"}, "\n")
	if got := trimLines(ansi.Strip(Render(10, 5, tree(true)))); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// It flips against the screen, not its overlay.
	f = Solve(10, 5, Column(Fixed(4, nil), Fixed(1, field(true))))
	expectRect(t, f, "menu", image.Rect(0, 1, 2, 4))

	if got := Debug(10, 5, tree(true)); !strings.Contains(got, `Layer id="menu" 0,1 2x3 anchorTo="input"/bottom float`) {
		t.Fatalf("debug tree does not show the menu floating:\n%s", got)
	}
}

// trimLines trims the spaces at the end of each line of s.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// dialogTree is a modal dialog with a field whose menu floats past its
// bottom edge, and a toast declared after the dialog, over the menu.
func dialogTree(l *log, effects bool) Child {
	menu := Layer(Column(
		Text("opt1").ID("opt1").Focusable().On(on.Click(l.add("opt1"))),
		Text("opt2").ID("opt2").Focusable(),
		Text("opt3").ID("opt3").Focusable(),
	)).ID("menu").AnchorTo("input", Bottom).Float().On(on.ClickOutside(l.add("close menu")))
	body := Overlay(Column(Text("title"), Text("input").ID("input").Focusable()), menu)
	dialog := Layer(Box(rounded, body)).ID("dialog").Modal().Anchor(Start, Start).
		On(on.ClickOutside(l.add("close dialog")))
	if effects {
		dialog = dialog.Transition(1, fx.Dim{})
	}
	toast := Layer(Text("TOAST")).Anchor(Start, Start).Offset(3, 5)
	return Overlay(Text(strings.Repeat("base\n", 8)), dialog, toast)
}

func TestFloatInDialog(t *testing.T) {
	t.Parallel()

	for _, effects := range []bool{false, true} {
		var l log
		r := New()
		got := trimLines(ansi.Strip(r.Render(10, 8, dialogTree(&l, effects))))
		// The dialog's box is rows 0 to 3. The menu goes over its bottom edge
		// and past it, over the base, and under the toast declared after the
		// dialog.
		want := strings.Join([]string{
			"╭─────╮",
			"│title│",
			"│input│",
			"╰opt1─╯",
			"bopt2",
			"bopTOAST",
			"base",
			"base",
		}, "\n")
		if got != want {
			t.Fatalf("effects %v: got:\n%s\nwant:\n%s", effects, got, want)
		}

		f := r.Frame()
		expectRect(t, f, "menu", image.Rect(1, 3, 5, 6))
		if hit := f.Hit(2, 5); len(hit) < 3 || hit[0] != "dialog" || hit[len(hit)-1] != "opt3" {
			t.Fatalf("effects %v: hit %v under the menu, outside the dialog", effects, hit)
		}

		// A click on the menu, outside the dialog's box, is inside the dialog.
		r.ClickAt(2, 3)
		expect(t, &l, "opt1")
		// A click on the base closes both.
		r.ClickAt(8, 6)
		expect(t, &l, "close dialog", "close menu")

		// Focus inside the dialog's scope reaches the menu.
		var focus Focus[string]
		focus.Push("dialog")
		var order []string
		for range 4 {
			focus.Next(f)
			id, _ := focus.Current()
			order = append(order, id)
		}
		if want := []string{"input", "opt1", "opt2", "opt3"}; strings.Join(order, ",") != strings.Join(want, ",") {
			t.Fatalf("effects %v: focus went %v, want %v", effects, order, want)
		}
	}
}

func TestAnchorInsideFloat(t *testing.T) {
	t.Parallel()

	// A submenu anchored to an item of a menu that floats past its overlay
	// finds the item, though it is outside that overlay.
	menu := Overlay(
		Text("sub").ID("item"),
		Layer(Text("submenu")).ID("submenu").AnchorTo("item", Right).Float(),
	)
	tree := Column(Fixed(1, Overlay(
		Text("input").ID("input"),
		Layer(Column(Text("first"), menu)).ID("menu").AnchorTo("input", Bottom).Float(),
	)))
	f := Solve(20, 5, tree)
	expectRect(t, f, "item", image.Rect(0, 2, 5, 3))
	expectRect(t, f, "submenu", image.Rect(5, 2, 12, 3))

	// A node in a layer that floats off screen is no anchor.
	f = Solve(20, 5, Overlay(
		Overlay(Text(""), Layer(Text("item").ID("item")).Anchor(Start, Start).Offset(100, 0).Float()),
		Layer(Text("tip")).ID("tip").AnchorTo("item", Right),
	))
	if r, ok := f.Rect("tip"); ok {
		t.Fatalf("tip at %v, though its anchor is off screen", r)
	}
}
