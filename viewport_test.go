// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func wantRender(t *testing.T, w, h int, c Child, want ...string) {
	t.Helper()
	got := ansi.Strip(Render(w, h, c))
	if w := strings.Join(want, "\n"); got != w {
		t.Fatalf("got:\n%s\nwant:\n%s", got, w)
	}
}

func TestBufferLines(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"a\n\n", []string{"a", ""}},
		{"a\r\nb", []string{"a", "b"}},
		{"\n", []string{""}},
	} {
		b := NewBuffer(tc.in)
		var got []string
		for i := range b.Len() {
			got = append(got, b.Line(i))
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("NewBuffer(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}

	var b Buffer
	b.Append("one", "two\nthree\n", "four")
	b.Append()
	if b.Len() != 4 || b.Line(2) != "three" || b.Line(9) != "" || b.String() != "one\ntwo\nthree\nfour" {
		t.Fatalf("Append: %q", b.String())
	}
	b.SetLines("x", "y")
	if b.String() != "x\ny" {
		t.Fatalf("SetLines: %q", b.String())
	}
	b.Clear()
	if b.Len() != 0 {
		t.Fatalf("Clear left %d lines", b.Len())
	}
}

func TestBufferCarriesStyles(t *testing.T) {
	t.Parallel()

	b := NewBuffer("\x1b[31mred\nstill red\x1b[0m\nplain\n\x1b]8;;https://x.dev\x1b\\link\nstill link\x1b]8;;\x1b\\ done")
	if p := b.penOf(0); !p.zero() {
		t.Fatal("the first line starts styled")
	}
	if p := b.penOf(1); p.style.Fg == nil {
		t.Fatal("the red was not carried to the second line")
	}
	if p := b.penOf(2); !p.zero() {
		t.Fatal("the reset was not carried to the third line")
	}
	if p := b.penOf(4); p.link.URL != "https://x.dev" {
		t.Fatalf("the link was not carried: %+v", p.link)
	}
	// Each line is drawn on its own, from the top, in the style it is in.
	c := canvas.Parse(Render(10, 2, Viewport(b).At(ViewportPos{Line: 1})), ansi.GraphemeWidth)
	if fg := c.CellAt(0, 0).Style.Fg; fg == nil {
		t.Fatal("a line scrolled to the top lost the style it started in")
	}
	if fg := c.CellAt(0, 1).Style.Fg; fg != nil {
		t.Fatal("the line after the reset is styled")
	}
	c = canvas.Parse(Render(12, 1, Viewport(b).At(ViewportPos{Line: 4})), ansi.GraphemeWidth)
	if l := c.CellAt(0, 0).Link; l.URL != "https://x.dev" {
		t.Fatalf("a line scrolled to the top lost the link it started in: %+v", l)
	}
	if l := c.CellAt(11, 0).Link; l.URL != "" {
		t.Fatalf("the link ran past where it ended: %+v", l)
	}
	if b.Text(1) != "still red" {
		t.Fatalf("Text: %q", b.Text(1))
	}
}

func TestWrapLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		s    string
		w    int
		mode WrapMode
		want []int
	}{
		{"two is a longer line", 14, WrapWords, []int{0, 9}},
		{"two is a longer line", 14, WrapChars, []int{0, 14}},
		{"abcdefghij", 4, WrapWords, []int{0, 4, 8}},
		{"ab abcdefghij", 4, WrapWords, []int{0, 3, 7, 11}},
		// Spaces hang past the width rather than starting a row.
		{"abcd     efgh", 4, WrapWords, []int{0, 9}},
		{"  indented words", 10, WrapWords, []int{0, 11}},
		// A wide glyph that would cross the width starts the next row.
		{"日本語", 5, WrapChars, []int{0, 4}},
		{"日本語", 1, WrapChars, []int{0, 2, 4}},
		{"a\tb", 3, WrapChars, []int{0, 3}},
		{"\x1b[1mbold\x1b[m words", 5, WrapWords, []int{0, 5}},
		// After a hyphen, as Lip Gloss wraps text.
		{"hello-world", 8, WrapWords, []int{0, 6}},
		{"a-b-c-d-e-f", 4, WrapWords, []int{0, 4, 8}},
		{"self-contained text here", 10, WrapWords, []int{0, 5, 15}},
		{"--flag-name value", 6, WrapWords, []int{0, 2, 7, 12}},
		{"a-\x1b[1mbcdef", 4, WrapWords, []int{0, 2, 6}},
		{"hello-world", 8, WrapChars, []int{0, 8}},
	} {
		_, got := wrapLine(tc.s, ansi.GraphemeWidth, tc.w, tc.mode, nil)
		if !slices.Equal(got, tc.want) {
			t.Errorf("wrapLine(%q, %d, %d): got %v, want %v", tc.s, tc.w, tc.mode, got, tc.want)
		}
	}
}

// TestWrapIndexGrows checks that the rows a buffer works out as lines are
// appended are those of a buffer given the same lines at once, at each
// width and mode, and after its content is replaced.
func TestWrapIndexGrows(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 2))
	words := []string{"a", "word", "longerword", "\x1b[32mgreen\x1b[m", "日本", "tab\there", "", "x"}
	line := func() string {
		var parts []string
		for range rng.IntN(12) {
			parts = append(parts, words[rng.IntN(len(words))])
		}
		return strings.Join(parts, strings.Repeat(" ", 1+rng.IntN(3)))
	}
	var grown Buffer
	var all []string
	for round := range 30 {
		var add []string
		for range rng.IntN(20) {
			add = append(add, line())
		}
		grown.Append(add...)
		all = append(all, add...)
		if round == 15 {
			grown.SetLines(all...)
		}
		for _, mode := range []WrapMode{WrapNone, WrapWords, WrapChars} {
			for _, w := range []int{1, 3, 7, 16, 200} {
				fresh := NewBuffer(strings.Join(all, "\n"))
				if len(all) > 0 && all[len(all)-1] == "" {
					// NewBuffer would end the text before its last empty line.
					fresh.Append("")
				}
				a := grown.index(ansi.GraphemeWidth, mode, w)
				b := fresh.index(ansi.GraphemeWidth, mode, w)
				if a.n != b.n || a.total() != b.total() {
					t.Fatalf("round %d mode %d width %d: grown %d lines %d rows, fresh %d lines %d rows",
						round, mode, w, a.n, a.total(), b.n, b.total())
				}
				for i := range a.n {
					if a.rowsOf(i) != b.rowsOf(i) {
						t.Fatalf("round %d mode %d width %d line %d: grown %d rows, fresh %d", round, mode, w, i, a.rowsOf(i), b.rowsOf(i))
					}
					want, _ := wrapLine(all[i], ansi.GraphemeWidth, w, mode, nil)
					if mode == WrapNone {
						want = 1
					}
					if a.rowsOf(i) != want {
						t.Fatalf("round %d mode %d width %d line %d %q: %d rows, wrapLine says %d", round, mode, w, i, all[i], a.rowsOf(i), want)
					}
				}
			}
		}
	}
}

func TestViewportDraws(t *testing.T) {
	t.Parallel()

	b := NewBuffer("one\ntwo is a longer line of words here\nthree\nfive\tTab\n日本語のテキスト")
	wantRender(t, 14, 3, Viewport(b),
		"one           ",
		"two is a longe",
		"three         ")
	wantRender(t, 14, 3, Viewport(b).Wrap(WrapWords),
		"one           ",
		"two is a      ",
		"longer line of")
	wantRender(t, 14, 3, Viewport(b).Wrap(WrapChars).At(ViewportPos{Line: 1, Row: 1}),
		"r line of word",
		"s here        ",
		"three         ")
	// Sideways, a wide glyph cut at the left edge is drawn as a space.
	wantRender(t, 6, 2, Viewport(b).At(ViewportPos{Line: 3, X: 5}),
		"   Tab",
		" のテ ")
	// Past the end, the last lines fill the viewport.
	wantRender(t, 14, 2, Viewport(b).At(ViewportEnd),
		"five    Tab   ",
		"日本語のテキス")
	// Fewer lines than rows leave the rest blank.
	wantRender(t, 5, 3, Viewport(NewBuffer("a\nb")), "a    ", "b    ", "     ")
	// Nothing to show.
	wantRender(t, 3, 1, Viewport(nil), "   ")
	wantRender(t, 3, 1, Viewport(&Buffer{}), "   ")
}

func TestViewportDrawsInBox(t *testing.T) {
	t.Parallel()

	b := NewBuffer(strings.TrimSuffix(strings.Repeat("line\n", 20), "\n"))
	tree := Box(rounded, Viewport(b).ID("log").At(ViewportPos{Line: 10})).
		Edge(Right, Stretch, ScrollbarOf("log").Thumb("#", lipgloss.NewStyle()))
	// 4 of 20 rows in view, from row 10: as a scroll would show them.
	wantRender(t, 8, 6, tree,
		"╭──────╮",
		"│line  │",
		"│line  │",
		"│line  #",
		"│line  │",
		"╰──────╯")
}

func TestViewportPosition(t *testing.T) {
	t.Parallel()

	b := NewBuffer("a\nbb bb bb\nc\nd\ne\nf")
	view := func(n ViewportNode) ViewportView {
		t.Helper()
		v, ok := Solve(5, 3, n.ID("v")).Viewport("v")
		if !ok {
			t.Fatal("no viewport in the frame")
		}
		return v
	}
	// bb bb bb wraps to two rows at 5: the lines take 7 rows.
	v := view(Viewport(b).Wrap(WrapWords))
	if v.TotalRows != 7 || v.Rows != 3 || v.Lines != 6 || v.Width != 5 || v.Widest != 8 {
		t.Fatalf("view: %+v", v)
	}
	for _, tc := range []struct {
		at   ViewportPos
		want ViewportPos
		row  int
	}{
		{ViewportPos{Line: 1, Row: 1}, ViewportPos{Line: 1, Row: 1}, 2},
		{ViewportPos{Line: 1, Row: 9}, ViewportPos{Line: 1, Row: 1}, 2},
		{ViewportPos{Line: -4, Row: 2}, ViewportPos{}, 0},
		{ViewportPos{Line: 5}, ViewportPos{Line: 3}, 4},
		{ViewportEnd, ViewportPos{Line: 3}, 4},
		// Lines that wrap do not scroll sideways.
		{ViewportPos{Line: 2, X: 3}, ViewportPos{Line: 2}, 3},
	} {
		got := view(Viewport(b).Wrap(WrapWords).At(tc.at))
		if got.Pos != tc.want || got.Row != tc.row {
			t.Errorf("At(%+v): got %+v at row %d, want %+v at row %d", tc.at, got.Pos, got.Row, tc.want, tc.row)
		}
	}
	// Without wrapping, X scrolls as far as the widest line needs.
	if wide := view(Viewport(b).At(ViewportPos{X: 99})); wide.Pos.X != 3 {
		t.Fatalf("X clamped to %d, want 3", wide.Pos.X)
	}

	v = view(Viewport(b).Wrap(WrapWords))
	for name, tc := range map[string]struct {
		got, want ViewportPos
	}{
		"Down":         {v.Down(2), ViewportPos{Line: 1, Row: 1}},
		"Up":           {v.Up(2), ViewportPos{}},
		"PageDown":     {v.PageDown(), ViewportPos{Line: 2}},
		"HalfPageDown": {v.HalfPageDown(), ViewportPos{Line: 1}},
		"Bottom":       {v.Bottom(), ViewportPos{Line: 3}},
		"Top":          {v.Top(), ViewportPos{}},
		"AtRow":        {v.AtRow(3), ViewportPos{Line: 2}},
		"Reveal near":  {v.Reveal(1), ViewportPos{}},
		"Reveal far":   {v.Reveal(4), ViewportPos{Line: 2}},
		"Right":        {v.Right(2), ViewportPos{}},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, tc.got, tc.want)
		}
	}
	if !v.AtTop() || v.AtBottom() || v.Percent() != 0 {
		t.Fatalf("at the top: AtTop %v, AtBottom %v, Percent %v", v.AtTop(), v.AtBottom(), v.Percent())
	}
	end := view(Viewport(b).Wrap(WrapWords).At(ViewportEnd))
	if end.AtTop() || !end.AtBottom() || end.Percent() != 1 || end.Up(1) != (ViewportPos{Line: 2}) {
		t.Fatalf("at the end: %+v, Up %+v", end, end.Up(1))
	}
	if p := view(Viewport(b)).Right(2); p.X != 2 {
		t.Fatalf("Right: %+v", p)
	}
}

func TestViewportFollow(t *testing.T) {
	t.Parallel()

	b := NewBuffer("1\n2\n3")
	r := New()
	var pos ViewportPos
	tree := func() Child {
		return Viewport(b).ID("log").At(pos).Follow().
			OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })
	}
	frame := func() ViewportView {
		t.Helper()
		r.Render(4, 2, tree())
		v, ok := r.Frame().Viewport("log")
		if !ok {
			t.Fatal("no viewport in the frame")
		}
		return v
	}
	if v := frame(); v.Pos.AtEnd() || v.Row != 0 {
		t.Fatalf("followed the end from the top: %+v", v)
	}
	// Scrolling down to the end follows it from there.
	r.Update(tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelDown})
	if !pos.AtEnd() {
		t.Fatalf("the wheel scrolled to %+v, not the end", pos)
	}
	b.Append("4", "5")
	wantRender(t, 4, 2, tree(), "4   ", "5   ")
	if v := frame(); !v.Pos.AtEnd() || v.Row != 3 {
		t.Fatalf("not following: %+v", v)
	}
	// Scrolled away from the end, it stays where it is.
	r.Update(tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelUp})
	if pos != (ViewportPos{Line: 0}) {
		t.Fatalf("the wheel scrolled up to %+v", pos)
	}
	b.Append("6")
	if v := frame(); v.Pos != (ViewportPos{}) {
		t.Fatalf("scrolled away, then moved to %+v", v.Pos)
	}
}

func TestViewportWheel(t *testing.T) {
	t.Parallel()

	b := NewBuffer(strings.Repeat("0123456789\n", 10))
	r := New()
	var pos ViewportPos
	tree := func() Child {
		return Viewport(b).At(pos).OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })
	}
	r.Render(4, 3, tree())
	r.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	if pos != (ViewportPos{Line: 3}) {
		t.Fatalf("wheel down: %+v", pos)
	}
	r.Render(4, 3, tree())
	r.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelRight})
	if pos != (ViewportPos{Line: 3, X: 3}) {
		t.Fatalf("wheel right: %+v", pos)
	}
	r.Render(4, 3, tree())
	// At the top, the wheel up has nowhere to go: no call.
	pos = ViewportPos{}
	r.Render(4, 3, tree())
	called := false
	tree2 := Viewport(b).OnScroll(func(ViewportPos) tea.Cmd { called = true; return nil })
	r.Render(4, 3, tree2)
	r.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelUp})
	if called {
		t.Fatal("the wheel scrolled up from the top")
	}
}

func TestViewportScrollbar(t *testing.T) {
	t.Parallel()

	b := NewBuffer(strings.TrimSuffix(strings.Repeat("line\n", 20), "\n"))
	var pos ViewportPos
	tree := func() Child {
		return Row(
			Fill(Viewport(b).ID("log").At(pos).OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })),
			Fixed(1, ScrollbarOf("log").Thumb("#", lipgloss.NewStyle())),
		)
	}
	r := New()
	r.Render(6, 4, tree())
	// Pressing at the bottom of the track jumps there, with the viewport's
	// OnScroll, as the scrollbar has none of its own.
	r.Update(down(5, 3))
	if pos != (ViewportPos{Line: 16}) {
		t.Fatalf("the scrollbar scrolled to %+v", pos)
	}
	r.Update(release(5, 3))
	// Sideways, in columns.
	wide := NewBuffer("0123456789abcdef")
	f := Solve(4, 2, Column(Viewport(wide).ID("w").At(ViewportPos{X: 6}), ScrollbarOf("w").Horizontal()))
	if v, _ := f.Viewport("w"); v.Pos.X != 6 || v.Widest != 16 || v.Width != 4 {
		t.Fatalf("horizontal: %+v", v)
	}
	got := ansi.Strip(Render(8, 2, Column(Viewport(wide).ID("w").At(ViewportPos{X: 6}), ScrollbarOf("w").Horizontal())))
	if !strings.Contains(got, "━") {
		t.Fatalf("no horizontal scrollbar:\n%s", got)
	}
}

func TestViewportGutters(t *testing.T) {
	t.Parallel()

	lines := make([]string, 12)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	lines[9] = "a much longer line"
	b := NewBuffer(strings.Join(lines, "\n"))
	mark := func(row GutterRow) string {
		if row.Line%2 == 0 {
			return "●"
		}
		return ""
	}
	wantRender(t, 13, 4, Viewport(b).Wrap(WrapWords).At(ViewportPos{Line: 8}).LineNumbers(lipgloss.NewStyle()).Gutter(2, mark),
		" 9 ● line 9  ",
		"10   a much  ",
		"     longer  ",
		"     line    ")
	// The number gutter grows with the lines.
	if v, _ := Solve(10, 2, Viewport(b).ID("v").LineNumbers(lipgloss.NewStyle())).Viewport("v"); v.Width != 7 {
		t.Fatalf("lines shown %d wide beside the numbers, want 7", v.Width)
	}
	// The gutter's style is drawn in its cells.
	c := canvas.Parse(Render(10, 1, Viewport(b).LineNumbers(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")))), ansi.GraphemeWidth)
	if c.CellAt(1, 0).Style.Fg == nil || c.CellAt(4, 0).Style.Fg != nil {
		t.Fatal("line numbers drawn without their style, or the line with it")
	}
}

func TestViewportMarks(t *testing.T) {
	t.Parallel()

	hl := lipgloss.NewStyle().Background(lipgloss.Color("#ffff00"))
	cur := lipgloss.NewStyle().Background(lipgloss.Color("#0000ff"))
	b := NewBuffer("an error, then\n\x1b[31mred error\x1b[0m here\nno\tmatch error")
	re := regexp.MustCompile("error")
	var texts []string
	n := Viewport(b).
		Marks(func(line int, text string) []Mark {
			texts = append(texts, text)
			return MarkMatches(re, hl)(line, text)
		}).
		LineStyle(func(line int) lipgloss.Style {
			if line == 1 {
				return cur
			}
			return lipgloss.Style{}
		})
	c := canvas.Parse(Render(20, 3, n), ansi.GraphemeWidth)
	if want := []string{"an error, then", "red error here", "no\tmatch error"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("Marks given %q, want %q", texts, want)
	}
	bg := func(x, y int) color.Color { return c.CellAt(x, y).Style.Bg }
	yellow, blue := colorOf(hl.GetBackground()), colorOf(cur.GetBackground())
	sameColor := func(a, b color.Color) bool {
		if a == nil || b == nil {
			return a == b
		}
		r1, g1, b1, _ := a.RGBA()
		r2, g2, b2, _ := b.RGBA()
		return r1 == r2 && g1 == g2 && b1 == b2
	}
	for _, tc := range []struct {
		x, y int
		want color.Color
	}{
		{2, 0, nil},
		{3, 0, yellow},
		{7, 0, yellow},
		{8, 0, nil},
		// The line style is under the mark, across the whole row, past the
		// red text's reset.
		{0, 1, blue},
		{4, 1, yellow},
		{8, 1, yellow},
		{9, 1, blue},
		{19, 1, blue},
		// A tab is drawn as spaces, and a mark after it lands after them.
		{9, 2, nil},
		{10, 2, yellow},
		{14, 2, yellow},
		{15, 2, nil},
	} {
		if got := bg(tc.x, tc.y); !sameColor(got, tc.want) {
			t.Errorf("cell %d,%d: background %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	if fg := c.CellAt(4, 1).Style.Fg; fg == nil {
		t.Fatal("the mark took the text's own color")
	}
}

func TestViewportMeasures(t *testing.T) {
	t.Parallel()

	b := NewBuffer("short\na longer line here")
	if got := Render(0, 0, Viewport(b)); ansi.Strip(got) != "short             \na longer line here" {
		t.Fatalf("natural size: %q", got)
	}
	f := Solve(30, 10, Column(Viewport(b).ID("v").LineNumbers(lipgloss.NewStyle()), Text("after").ID("after")))
	if r, _ := f.Rect("v"); r != image.Rect(0, 0, 30, 2) {
		t.Fatalf("viewport laid out at %v", r)
	}
	// Wrapped, it is as tall as its rows at the width it is given.
	f = Solve(8, 10, Column(Auto(Viewport(b).ID("v").Wrap(WrapWords)), Text("after").ID("after")))
	if r, _ := f.Rect("after"); r.Min.Y != 4 {
		t.Fatalf("text after the viewport at %v, want it after 4 rows", r)
	}
	// Empty lines are a row each.
	f = Solve(8, 10, Column(Auto(Viewport(NewBuffer("\n\n\n")).Wrap(WrapWords)), Text("after").ID("after")))
	if r, _ := f.Rect("after"); r.Min.Y != 3 {
		t.Fatalf("text after three empty lines at %v", r)
	}
	if got := Debug(8, 2, Viewport(b).Wrap(WrapWords).At(ViewportEnd).Follow()); !strings.Contains(got, "Viewport 0,0 8x2 lines=2 at=end wrap=words follow") {
		t.Fatalf("Debug:\n%s", got)
	}
}

func TestViewportSelection(t *testing.T) {
	t.Parallel()

	b := NewBuffer("first line\nsecond is a wrapped line\nthird\tline\nfourth")
	r := New()
	tree := func() Child {
		return Viewport(b).ID("v").Wrap(WrapWords).LineNumbers(lipgloss.NewStyle()).Selectable()
	}
	// The second line wraps to "second is a" and "wrapped line", beside
	// the numbers: dragging from "is" to "wrapped" joins its rows again.
	msg, ok := selectDrag(r, 14, 3, tree, 0, [2]int{9, 1}, [2]int{8, 2})
	if !ok || msg.Text != "is a wrapped" {
		t.Fatalf("selected %q, %v", msg.Text, ok)
	}
	// All of it, with what is out of sight, and the tab as it was.
	if !r.SelectAll("v") {
		t.Fatal("SelectAll found nothing")
	}
	wantSelection(t, r, "first line\nsecond is a wrapped line\nthird\tline\nfourth")

	// A word, by double clicking it.
	r2 := New()
	r2.after = immediate
	r2.Render(14, 3, tree())
	for range 2 {
		r2.Update(down(4, 1))
		r2.Render(14, 3, tree())
		r2.Update(release(4, 1))
		r2.Render(14, 3, tree())
	}
	wantSelection(t, r2, "second")
}

func TestViewportSelectionScrolled(t *testing.T) {
	t.Parallel()

	b := NewBuffer("a\nb\nc\nd\ne\nf")
	r := New()
	var pos ViewportPos
	tree := func() Child {
		return Viewport(b).At(pos).ID("v").Selectable().OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })
	}
	r.after = immediate
	r.Render(3, 2, tree())
	r.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	r.Render(3, 2, tree())
	// The text scrolls while the pointer is held past the bottom, and the
	// selection keeps its start, scrolled out of sight.
	cmd := r.Update(dragTo(0, 3))
	r.Render(3, 2, tree())
	for _, msg := range drain(cmd) {
		r.Update(msg)
		r.Render(3, 2, tree())
	}
	if pos.Line == 0 {
		t.Fatal("the selection did not scroll the viewport")
	}
	r.Update(release(0, 3))
	r.Render(3, 2, tree())
	got, _ := r.Selection()
	if !strings.HasPrefix(got, "a\nb\nc") {
		t.Fatalf("selected %q", got)
	}
}

func TestBufferFind(t *testing.T) {
	t.Parallel()

	b := NewBuffer("no\n\x1b[1mfoo\x1b[m bar foo\nnone\nfoo")
	re := regexp.MustCompile("foo")
	var got []Match
	for m, ok := b.Find(re, 0, 0); ok; m, ok = b.Find(re, m.Line, m.End) {
		got = append(got, m)
	}
	want := []Match{{1, 0, 3}, {1, 8, 11}, {3, 0, 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("Find: got %v, want %v", got, want)
	}
	got = got[:0]
	for m, ok := b.FindBefore(re, 3, 3); ok; m, ok = b.FindBefore(re, m.Line, m.Start) {
		got = append(got, m)
	}
	want = []Match{{3, 0, 3}, {1, 8, 11}, {1, 0, 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("FindBefore: got %v, want %v", got, want)
	}
	if _, ok := b.Find(nil, 0, 0); ok {
		t.Fatal("found a nil regexp")
	}
	if MarkMatches(nil, lipgloss.Style{}) != nil {
		t.Fatal("MarkMatches of nil marks")
	}
}

func TestViewportFrameUnchanged(t *testing.T) {
	t.Parallel()

	// The same rows draw the same ops, so the frame is not composited
	// again; appending a line out of sight does not change them either.
	b := NewBuffer(strings.Repeat("\x1b[2mdim\x1b[m line\tcol\n", 50))
	r := New()
	tree := func() Child { return Viewport(b).Wrap(WrapWords).LineNumbers(lipgloss.NewStyle()) }
	r.Render(10, 5, tree())
	b.Append("more")
	r.r.paintTree(10, 5, tree())
	if !r.r.sameOps() {
		t.Fatal("an unchanged viewport drew different ops")
	}
}

func TestViewportCellsMatchParse(t *testing.T) {
	t.Parallel()

	// The widths the buffer works out are those of the cells the canvas
	// parses for the same lines.
	for _, s := range []string{
		"plain", "日本語", "é combining", "\x1b[31mred\x1b[m", "a\x1b[31ḿb", "emoji 👨‍👩‍👧 family", "tab\tstop\t!",
	} {
		w, _ := lineWidth32(s, ansi.GraphemeWidth)
		drawn := cutRow(s, checkpoint{}, measureLine(s, ansi.GraphemeWidth), ansi.GraphemeWidth, 0, 1<<20, nil)
		c := canvas.Parse(drawn, ansi.GraphemeWidth)
		if c.Width() != w {
			t.Errorf("%q: measured %d cells, parsed %d", s, w, c.Width())
		}
	}
}

// TestASCIIFastPaths checks that the paths that measure and wrap a line of
// ASCII a byte at a time agree with the scanner, which goes a glyph at a
// time, and that the canvas parses the cells they count.
func TestASCIIFastPaths(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(3, 4))
	parts := []string{"a", "bc", "def", " ", "  ", "\t", "\x1b[1m", "\x1b[m", "\x1b[38;5;196m", "\x1b]8;;https://x\x1b\\", "\x1b]8;;\x07", "longword", "-", "x-y", "--"}
	for range 3000 {
		var b strings.Builder
		for range rng.IntN(16) {
			b.WriteString(parts[rng.IntN(len(parts))])
		}
		s := b.String()
		fast, ok := asciiWidth(s)
		if !ok {
			t.Fatalf("asciiWidth(%q) took the slow path", s)
		}
		if slow, _ := scanWidth(s, ansi.GraphemeWidth); fast != slow {
			t.Fatalf("width of %q: fast %d, scanned %d", s, fast, slow)
		}
		if c := canvas.Parse(cutRow(s, checkpoint{}, 0, ansi.GraphemeWidth, 0, 1<<20, nil), ansi.GraphemeWidth); c.Width() != fast {
			t.Fatalf("width of %q: %d, parsed %d", s, fast, c.Width())
		}
		// Cutting a row from the line draws the cells the scanner does.
		start := pen{}
		if rng.IntN(2) == 0 {
			start.style.Attrs = uv.AttrItalic
		}
		for range 4 {
			a := rng.IntN(fast + 1)
			b := a + rng.IntN(fast-a+2)
			line := strings.ReplaceAll(s, "\t", " ")
			if measureLine(line, ansi.GraphemeWidth)&asciiLine == 0 {
				t.Fatalf("%q is not marked as ASCII", line)
			}
			got := asciiCut(line, checkpoint{pen: start}, a, b)
			// Flags of 0 and a mark out of the way take the scanner.
			want := cutRow(line, checkpoint{pen: start}, 0, ansi.GraphemeWidth, a, b, &rowStyle{marks: []overlay{{start: 1 << 20, end: 1 << 21, sgr: "\x1b[0m"}}})
			gc, wc := canvas.Parse(got, ansi.GraphemeWidth), canvas.Parse(want, ansi.GraphemeWidth)
			if gc.Width() != wc.Width() {
				t.Fatalf("cut %q from %d to %d: %d cells, scanned %d", s, a, b, gc.Width(), wc.Width())
			}
			for x := range gc.Width() {
				g, w := gc.CellAt(x, 0), wc.CellAt(x, 0)
				if g.Content != w.Content || !g.Style.Equal(&w.Style) || g.Link != w.Link {
					t.Fatalf("cut %q from %d to %d: cell %d is %+v, scanned %+v", s, a, b, x, g, w)
				}
			}
		}
		for _, mode := range []WrapMode{WrapWords, WrapChars} {
			for w := 1; w <= 9; w++ {
				got, fast := asciiWrap(s, w, mode, []int{0})
				want := scanWrap(s, ansi.GraphemeWidth, w, mode, []int{0})
				if !fast || !slices.Equal(got, want) {
					t.Fatalf("wrap %q at %d mode %d: fast %v (%v), scanned %v", s, w, mode, got, fast, want)
				}
			}
		}
	}
	for _, s := range []string{"\x1bc", "\x1b[", "a\rb", "\x1b]8;;x", "日本"} {
		if _, ok := asciiWidth(s); ok {
			t.Errorf("asciiWidth(%q) took the fast path", s)
		}
		if _, ok := asciiWrap(s, 4, WrapWords, nil); ok {
			t.Errorf("asciiWrap(%q) took the fast path", s)
		}
	}
}

// TestCountRowsInParallel checks that a buffer too big to measure and wrap
// on one processor is measured and wrapped as it would be on one.
func TestCountRowsInParallel(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(5, 6))
	words := []string{"a", "word", "longerword", "\x1b[32mgreen\x1b[m", "日本", "tab\there"}
	lines := make([]string, 3*parallelLines+17)
	for i := range lines {
		var parts []string
		for range rng.IntN(24) {
			parts = append(parts, words[rng.IntN(len(words))])
		}
		lines[i] = strings.Join(parts, " ")
	}
	var b Buffer
	b.Append(lines...)
	m := b.measure(ansi.GraphemeWidth)
	widths := make([]int32, len(lines))
	measureLines(lines, ansi.GraphemeWidth, widths)
	if !slices.Equal(m.widths, widths) {
		t.Fatal("widths measured in parallel differ")
	}
	for _, mode := range []WrapMode{WrapWords, WrapChars} {
		x := b.index(ansi.GraphemeWidth, mode, 13)
		rows := make([]int32, len(lines))
		countRowsOf(lines, widths, ansi.GraphemeWidth, 13, mode, rows)
		for i, r := range rows {
			if x.rowsOf(i) != int(r) {
				t.Fatalf("mode %d line %d: %d rows in parallel, %d on one", mode, i, x.rowsOf(i), r)
			}
		}
	}
}

// sameCells fails t when strings a and b do not draw the same cells.
func sameCells(t *testing.T, what, a, b string) {
	t.Helper()
	ac, bc := canvas.Parse(a, ansi.GraphemeWidth), canvas.Parse(b, ansi.GraphemeWidth)
	if ac.Width() != bc.Width() {
		t.Fatalf("%s: %d cells, want %d", what, ac.Width(), bc.Width())
	}
	for x := range ac.Width() {
		g, w := ac.CellAt(x, 0), bc.CellAt(x, 0)
		if g.Content != w.Content || !g.Style.Equal(&w.Style) || g.Link != w.Link {
			t.Fatalf("%s: cell %d is %+v, want %+v", what, x, g, w)
		}
	}
}

// TestLongLines checks that rows cut out of a long line from its
// checkpoints, and its rows wrapped as kept, are those of the line scanned
// from its start.
func TestLongLines(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(7, 8))
	parts := []string{"word ", "a ", "\x1b[31m", "\x1b[1;4m", "\x1b[m", "\t", "日本 ", "\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\ ", "longerword "}
	var sb strings.Builder
	for sb.Len() < 6*longLineBytes {
		sb.WriteString(parts[rng.IntN(len(parts))])
	}
	// The long line starts in the style the line before it left open.
	b := NewBuffer("\x1b[32mopen\n" + sb.String() + "\nafter")
	s, start := b.Line(1), b.penOf(1)
	m := b.measure(ansi.GraphemeWidth)
	width, flags := m.width(1), m.widths[1]
	// Checkpoints are made only as far along the line as they are wanted.
	l := b.longLine(1, ansi.GraphemeWidth)
	b.cutFrom(1, ansi.GraphemeWidth, 3000)
	if len(l.cps) > 4 || l.done {
		t.Fatalf("%d checkpoints to reach cell 3000", len(l.cps))
	}
	b.cutFrom(1, ansi.GraphemeWidth, width)
	if len(l.cps) < width/checkpointEvery {
		t.Fatalf("%d checkpoints for %d cells", len(l.cps), width)
	}
	for range 200 {
		a := rng.IntN(width)
		e := a + 1 + rng.IntN(150)
		got := cutRow(s, b.cutFrom(1, ansi.GraphemeWidth, a), flags, ansi.GraphemeWidth, a, e, nil)
		want := cutRow(s, checkpoint{pen: start}, 0, ansi.GraphemeWidth, a, e, nil)
		sameCells(t, fmt.Sprintf("cells %d to %d", a, e), got, want)
	}
	var scratch []int
	for _, w := range []int{40, 77, 40} {
		got := b.rowStarts(1, ansi.GraphemeWidth, w, WrapWords, &scratch)
		_, want := wrapLine(s, ansi.GraphemeWidth, w, WrapWords, nil)
		if !slices.Equal(got, want) {
			t.Fatalf("rows at %d kept as %d starts, want %d", w, len(got), len(want))
		}
		// A short line worked out after does not change what is kept.
		b.rowStarts(2, ansi.GraphemeWidth, w, WrapWords, &scratch)
		if !slices.Equal(got, want) {
			t.Fatal("working out a short line changed the long line's rows")
		}
	}
	// Drawn, as wrapped or scrolled sideways, it is the line from its start.
	wrapped := Render(60, 8, Viewport(b).Wrap(WrapWords).At(ViewportPos{Line: 1, Row: 120}))
	if strings.Count(ansi.Strip(wrapped), "\n") != 7 {
		t.Fatalf("wrapped:\n%s", wrapped)
	}
	b.Clear()
	if b.long != [4]*longLine{} {
		t.Fatal("Clear kept long lines")
	}
}

// TestViewportDropsSequences checks that escape sequences other than styles
// and hyperlinks in a buffer's lines do not reach the terminal, including
// one hidden inside an operating system command.
func TestViewportDropsSequences(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"a\x1b]0;\x1b_Ga=d,d=A\x1b\\b",
		"a\x1b_Ga=d\x1b\\b",
		"a\x1bPq#0\x1b\\b",
		"a\x1b[2Jb",
	} {
		out := Render(10, 1, Viewport(NewBuffer(line)))
		for _, intro := range []string{"\x1b_", "\x1bP", "\x1b[2J", "\x1b]0"} {
			if strings.Contains(out, intro) {
				t.Errorf("%q drawn as %q, with %q", line, out, intro)
			}
		}
	}
}

// TestCutRowToLineEnd checks that a row cut to the end of a line, as
// selection asks for the cells of a row, does not overflow the size it
// guesses for the row.
func TestCutRowToLineEnd(t *testing.T) {
	t.Parallel()

	s := "日本語日本語日本語"
	if got := cutRow(s, checkpoint{}, measureLine(s, ansi.GraphemeWidth), ansi.GraphemeWidth, 8, math.MaxInt, nil); ansi.Strip(got) != "本語日本語" {
		t.Fatalf("got %q", got)
	}
}

// TestLineStyleUnderText checks that a line's style is drawn under the
// colors of its text, as LineStyle says, and a mark over both.
func TestLineStyleUnderText(t *testing.T) {
	t.Parallel()

	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Background(lipgloss.Color("#000080"))
	n := Viewport(NewBuffer("\x1b[31mred\x1b[0m plain")).LineStyle(func(int) lipgloss.Style { return green })
	c := canvas.Parse(Render(12, 1, n), ansi.GraphemeWidth)
	red := ansi.Red
	if fg := c.CellAt(0, 0).Style.Fg; fg != red {
		t.Fatalf("text's own red drawn as %v", fg)
	}
	if bg := c.CellAt(0, 0).Style.Bg; bg == nil {
		t.Fatal("the line's background is not under the red text")
	}
	if fg := c.CellAt(5, 0).Style.Fg; fg == nil || fg == red {
		t.Fatalf("plain text after the reset drawn in %v, not the line's color", fg)
	}
}

// TestViewportStyle checks that a viewport's style is drawn under its
// lines' own colors, across its rows and the blank rows below them.
func TestViewportStyle(t *testing.T) {
	t.Parallel()

	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Background(lipgloss.Color("#000080"))
	c := canvas.Parse(Render(8, 3, Viewport(NewBuffer("\x1b[31mred\x1b[m x\nplain")).Style(base)), ansi.GraphemeWidth)
	if fg := c.CellAt(0, 0).Style.Fg; fg != ansi.Red {
		t.Fatalf("the text's own red drawn as %v", fg)
	}
	for _, p := range []image.Point{{0, 0}, {7, 0}, {0, 1}, {7, 2}} {
		if bg := c.CellAt(p.X, p.Y).Style.Bg; bg == nil {
			t.Errorf("cell %v has no background", p)
		}
	}
	if fg := c.CellAt(4, 0).Style.Fg; fg == nil || fg == ansi.Red {
		t.Fatalf("text after the reset drawn in %v, not the viewport's color", fg)
	}
	// With no buffer at all, its rows are filled too.
	c = canvas.Parse(Render(4, 2, Viewport(nil).Style(base)), ansi.GraphemeWidth)
	if bg := c.CellAt(3, 1).Style.Bg; bg == nil {
		t.Fatal("a viewport without a buffer is not filled")
	}
}

// TestPenSGRsKeepsColorKinds checks that the escape sequences kept for
// styles tell a color of the terminal's palette from the RGB it stands for.
func TestPenSGRsKeepsColorKinds(t *testing.T) {
	t.Parallel()

	var c penSGRs
	palette := uv.Style{Fg: ansi.Red}
	r, g, b, _ := ansi.Red.RGBA()
	rgb := uv.Style{Fg: color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}}
	if !palette.Equal(&rgb) {
		t.Skip("the colors are told apart by ultraviolet already")
	}
	if a, b := c.of(&palette), c.of(&rgb); a == b {
		t.Fatalf("both drawn with %q", a)
	}
}

// wrappedColor is a color that can be compared, unless it holds one that
// cannot, such as a [sliceColor].
type wrappedColor struct{ color.Color }

// TestPenSGRsIncomparableColors checks that styles with colors that cannot
// be compared are drawn, rather than panicking when looked up.
func TestPenSGRsIncomparableColors(t *testing.T) {
	t.Parallel()

	var c penSGRs
	s := uv.Style{Fg: wrappedColor{sliceColor{0xff}}}
	o := uv.Style{Fg: wrappedColor{sliceColor{0xff}}}
	if c.of(&s) == "" || c.of(&o) == "" {
		t.Fatal("no escape sequence for a color that cannot be compared")
	}
}
