// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/internal/linebuf"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		Edge(Right, Stretch, ScrollbarOf("log").Style(ScrollbarStyle{Thumb: "#"}))
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
			Fixed(1, ScrollbarOf("log").Style(ScrollbarStyle{Thumb: "#"})),
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

// TestViewportLongLine checks that a line long enough for its buffer to keep
// checkpoints of is drawn wrapped from deep inside it.
func TestViewportLongLine(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	for sb.Len() < 6*linebuf.LongLineBytes {
		sb.WriteString("word \x1b[31mred\x1b[m 日本\tlongerword ")
	}
	b := NewBuffer("\x1b[32mopen\n" + sb.String() + "\nafter")
	wrapped := Render(60, 8, Viewport(b).Wrap(WrapWords).At(ViewportPos{Line: 1, Row: 120}))
	if strings.Count(ansi.Strip(wrapped), "\n") != 7 {
		t.Fatalf("wrapped:\n%s", wrapped)
	}
}
