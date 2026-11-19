// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/internal/linebuf"
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

func TestBufferClamp(t *testing.T) {
	t.Parallel()

	b := NewBuffer("héllo\n日本\néx\n\tab")
	for _, tc := range []struct{ in, want Pos }{
		{Pos{-1, 5}, Pos{0, 0}},
		{Pos{0, -3}, Pos{0, 0}},
		{Pos{9, 0}, Pos{3, 3}},
		{Pos{0, 99}, Pos{0, 6}},
		{Pos{0, 2}, Pos{0, 1}}, // within é
		{Pos{0, 3}, Pos{0, 3}},
		{Pos{1, 4}, Pos{1, 3}}, // within 本
		{Pos{2, 2}, Pos{2, 0}}, // within e and its accent
		{Pos{2, 3}, Pos{2, 3}},
		{Pos{3, 1}, Pos{3, 1}}, // after the tab, a byte
	} {
		if got := b.Clamp(tc.in); got != tc.want {
			t.Errorf("Clamp(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if got := b.End(); got != (Pos{3, 3}) {
		t.Errorf("End() = %v", got)
	}
	// Positions are clamped, and swapped when out of order.
	b.Replace(Pos{0, 3}, Pos{0, 2}, "")
	b.Delete(Pos{1, 99}, Pos{1, 4})
	if got := b.String(); got != "hllo\n日\néx\n\tab" {
		t.Fatalf("got %q", got)
	}
	var empty Buffer
	if empty.Clamp(Pos{3, 3}) != (Pos{}) || empty.End() != (Pos{}) || empty.Offset(Pos{1, 1}) != 0 || empty.PosOf(7) != (Pos{}) {
		t.Fatal("an empty buffer has a position other than its start")
	}
}

// cellStyles returns the text of the cells s draws, line by line, and the
// foreground color of each, as a terminal draws it.
func cellStyles(s string) (text []string, fg [][]color.Color) {
	lines := strings.Split(s, "\n")
	scr := uv.NewScreenBuffer(200, len(lines))
	uv.NewStyledString(s).Draw(scr, scr.Bounds())
	for y := range lines {
		var row strings.Builder
		var fgs []color.Color
		for x := range 200 {
			c := scr.CellAt(x, y)
			if c == nil || c.Content == "" {
				continue
			}
			row.WriteString(c.Content)
			fgs = append(fgs, c.Style.Fg)
		}
		text = append(text, strings.TrimRight(row.String(), " "))
		fg = append(fg, fgs[:len(strings.TrimRight(row.String(), " "))])
	}
	return text, fg
}

func TestBufferDeleteKeepsStyles(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in       string
		from, to Pos
		text     string
		want     string // the text left, a line a letter, with each letter's color
		colors   string // r for red, g for green, . for none
	}{
		// The red of the text after survives the sequence that set it.
		{"plain \x1b[31mred\x1b[0m more", Pos{0, 4}, Pos{0, 8}, "", "plaid more", "....r....."},
		// And so does what lines removed between left open.
		{"aa\n\x1b[32mgreen\nbb", Pos{0, 1}, Pos{2, 1}, "", "ab", ".g"},
		{"\x1b[31mone\ntwo\nthr\x1b[0mee", Pos{0, 1}, Pos{2, 1}, "", "ohree", "rrr.."},
		// Text put in is drawn in the style of the text before it.
		{"\x1b[31mred\x1b[0m plain", Pos{0, 3}, Pos{0, 3}, "X", "redX plain", "rrrr......"},
		{"\x1b[31mred\x1b[0m plain", Pos{0, 1}, Pos{0, 6}, "X", "rXain", "rr..."},
	} {
		b := NewBuffer(tc.in)
		b.Replace(tc.from, tc.to, tc.text)
		text, fg := cellStyles(b.String())
		got, colors := strings.Join(text, ""), ""
		for _, row := range fg {
			for _, c := range row {
				switch c {
				case nil:
					colors += "."
				case ansi.Red:
					colors += "r"
				case ansi.Green:
					colors += "g"
				default:
					colors += "?"
				}
			}
		}
		if got != tc.want || colors != tc.colors {
			t.Errorf("%q, replacing %v to %v with %q: got %q colored %q (%q), want %q colored %q",
				tc.in, tc.from, tc.to, tc.text, got, colors, b.String(), tc.want, tc.colors)
		}
	}

	// Removing all of a styled buffer leaves an empty line, without the
	// sequences of the text removed.
	b := NewBuffer("\x1b[31mred\nx\x1b[0m")
	b.Delete(Pos{}, b.End())
	if b.Len() != 1 || b.String() != "" {
		t.Fatalf("got %d lines, %q", b.Len(), b.String())
	}
	// Text put in is taken as given, a style it leaves open carrying on.
	b = NewBuffer("a")
	b.Insert(Pos{0, 1}, "\x1b[31mb")
	b.Append("c")
	if _, fg := cellStyles(b.String()); fg[1][0] != ansi.Red {
		t.Fatalf("the style put in did not carry on: %q", b.String())
	}
}

func TestBufferEditNewlines(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in       string
		from, to Pos
		text     string
		want     []string
		end      Pos
	}{
		{"ab", Pos{0, 1}, Pos{0, 1}, "x\n", []string{"ax", "b"}, Pos{1, 0}},
		{"ab", Pos{0, 2}, Pos{0, 2}, "\n", []string{"ab", ""}, Pos{1, 0}},
		{"ab", Pos{0, 0}, Pos{0, 0}, "\n", []string{"", "ab"}, Pos{1, 0}},
		{"ab", Pos{0, 1}, Pos{0, 1}, "1\r\n2\r\n", []string{"a1", "2", "b"}, Pos{2, 0}},
		{"ab", Pos{0, 1}, Pos{0, 1}, "x\ny", []string{"ax", "yb"}, Pos{1, 1}},
		{"ab\ncd", Pos{0, 2}, Pos{1, 0}, "", []string{"abcd"}, Pos{0, 2}},
		{"ab\ncd\nef", Pos{0, 1}, Pos{2, 1}, "-", []string{"a-f"}, Pos{0, 2}},
		{"ab\ncd\nef", Pos{1, 0}, Pos{2, 0}, "", []string{"ab", "ef"}, Pos{1, 0}},
	} {
		b := NewBuffer(tc.in)
		c := b.Replace(tc.from, tc.to, tc.text)
		var got []string
		for i := range b.Len() {
			got = append(got, b.Line(i))
		}
		if !slices.Equal(got, tc.want) || c.End() != tc.end {
			t.Errorf("%q, replacing %v to %v with %q: got %q ending at %v, want %q ending at %v",
				tc.in, tc.from, tc.to, tc.text, got, c.End(), tc.want, tc.end)
		}
	}
}

func TestBufferEditEmpty(t *testing.T) {
	t.Parallel()

	var b Buffer
	c := b.Insert(Pos{5, 5}, "hi")
	if b.Len() != 1 || b.String() != "hi" || c.End() != (Pos{0, 2}) {
		t.Fatalf("got %q ending at %v", b.String(), c.End())
	}
	var e Buffer
	rev := e.Rev()
	e.Delete(Pos{}, Pos{})
	if e.Len() != 1 || e.String() != "" || e.Rev() == rev {
		t.Fatal("an edit to an empty buffer left no line")
	}
	// An edit that changes nothing is not a change.
	rev = e.Rev()
	if c = e.Insert(Pos{}, ""); e.Rev() != rev || c.End() != (Pos{}) {
		t.Fatal("putting in nothing changed the buffer")
	}
	b = *NewBuffer("a\nb")
	b.Delete(Pos{}, b.End())
	if b.Len() != 1 || b.String() != "" {
		t.Fatalf("removing all left %d lines", b.Len())
	}
	b.Clear()
	if b.Len() != 0 {
		t.Fatal("Clear left a line")
	}
	b.Insert(Pos{}, "x")
	b.SetContent("")
	if b.Len() != 0 {
		t.Fatal("SetContent left a line")
	}
	b.Insert(Pos{}, "x")
	b.SetLines()
	if b.Len() != 0 {
		t.Fatal("SetLines left a line")
	}
}

func TestBufferEditOverlapping(t *testing.T) {
	t.Parallel()

	edits := []Edit{
		{Pos{0, 2}, Pos{0, 5}, "X"},
		{Pos{0, 4}, Pos{0, 7}, "Y"}, // overlaps the first, and is dropped
		{Pos{0, 8}, Pos{0, 8}, "Z"},
		{Pos{0, 8}, Pos{0, 8}, "W"}, // after Z, given after it
		{Pos{0, 7}, Pos{0, 8}, "V"}, // touches Z and W, and goes before them
		{Pos{0, 3}, Pos{0, 3}, "U"}, // within the first, and is dropped
	}
	for _, order := range [][]int{{0, 1, 2, 3, 4, 5}, {4, 0, 5, 2, 1, 3}} {
		b := NewBuffer("0123456789")
		var given []Edit
		for _, i := range order {
			given = append(given, edits[i])
		}
		c := b.Edit(given...)
		if got := b.String(); got != "01X56VZW89" {
			t.Fatalf("order %v: got %q", order, got)
		}
		for _, tc := range []struct{ p, want, after Pos }{
			{Pos{0, 1}, Pos{0, 1}, Pos{0, 1}},
			{Pos{0, 2}, Pos{0, 2}, Pos{0, 2}},
			{Pos{0, 3}, Pos{0, 3}, Pos{0, 3}},
			{Pos{0, 5}, Pos{0, 3}, Pos{0, 3}},
			{Pos{0, 7}, Pos{0, 5}, Pos{0, 5}},
			{Pos{0, 8}, Pos{0, 6}, Pos{0, 8}},
			{Pos{0, 9}, Pos{0, 9}, Pos{0, 9}},
		} {
			if got, after := c.Map(tc.p), c.MapAfter(tc.p); got != tc.want || after != tc.after {
				t.Errorf("order %v: %v maps to %v and %v, want %v and %v", order, tc.p, got, after, tc.want, tc.after)
			}
		}
		if c.End() != (Pos{0, 8}) {
			t.Errorf("order %v: ends at %v", order, c.End())
		}
	}
}

func TestChangeMap(t *testing.T) {
	t.Parallel()

	var zero Change
	for _, p := range []Pos{{}, {0, 5}, {3, 1}} {
		if zero.Map(p) != p || zero.MapAfter(p) != p {
			t.Fatalf("the zero change moved %v", p)
		}
	}
	b := NewBuffer("one two\nthree")
	c := b.Replace(Pos{0, 4}, Pos{1, 2}, "2\nTH")
	// "one 2\nTHree"
	for _, tc := range []struct{ p, want Pos }{
		{Pos{0, 3}, Pos{0, 3}},
		{Pos{0, 4}, Pos{0, 4}},
		{Pos{0, 6}, Pos{1, 2}},
		{Pos{1, 1}, Pos{1, 2}},
		{Pos{1, 2}, Pos{1, 2}},
		{Pos{1, 5}, Pos{1, 5}},
	} {
		if got := c.Map(tc.p); got != tc.want {
			t.Errorf("%v maps to %v, want %v", tc.p, got, tc.want)
		}
	}
	// A range keeps text put in at its ends out of it, and a caret stays
	// before text put in where it is.
	b = NewBuffer("abcdef")
	c = b.Insert(Pos{0, 2}, "XY")
	for _, tc := range []struct{ in, want Range }{
		{Range{Anchor: Pos{0, 2}, Head: Pos{0, 4}}, Range{Anchor: Pos{0, 4}, Head: Pos{0, 6}}},
		{Range{Anchor: Pos{0, 4}, Head: Pos{0, 2}}, Range{Anchor: Pos{0, 6}, Head: Pos{0, 4}}},
		{Range{Anchor: Pos{0, 0}, Head: Pos{0, 2}}, Range{Anchor: Pos{0, 0}, Head: Pos{0, 2}}},
		{Range{Anchor: Pos{0, 2}, Head: Pos{0, 2}}, Range{Anchor: Pos{0, 2}, Head: Pos{0, 2}}},
		// The side of the caret where a line wraps is kept.
		{Range{Anchor: Pos{0, 4}, Head: Pos{0, 4}, EndOfRow: true}, Range{Anchor: Pos{0, 6}, Head: Pos{0, 6}, EndOfRow: true}},
		{Range{Anchor: Pos{0, 0}, Head: Pos{0, 4}, EndOfRow: true}, Range{Anchor: Pos{0, 0}, Head: Pos{0, 6}, EndOfRow: true}},
		{Range{Anchor: Pos{0, 5}, Head: Pos{0, 1}, EndOfRow: true}, Range{Anchor: Pos{0, 7}, Head: Pos{0, 1}, EndOfRow: true}},
	} {
		if got := c.MapRange(tc.in); got != tc.want {
			t.Errorf("%v maps to %v, want %v", tc.in, got, tc.want)
		}
	}
	r := Range{Anchor: Pos{2, 1}, Head: Pos{0, 3}}
	if r.From() != (Pos{0, 3}) || r.To() != (Pos{2, 1}) || r.Empty() || !(Range{}).Empty() {
		t.Fatal("a range's ends are out of order")
	}
	// Moving the Head forgets the side of the caret, and collapsing keeps it.
	r.EndOfRow = true
	if got := r.WithHead(Pos{1, 1}); got != (Range{Anchor: Pos{2, 1}, Head: Pos{1, 1}}) {
		t.Errorf("WithHead = %+v", got)
	}
	if got := r.Collapse(); got != (Range{Anchor: Pos{0, 3}, Head: Pos{0, 3}, EndOfRow: true}) || !got.Empty() {
		t.Errorf("Collapse = %+v", got)
	}
	if got := r.WithHead(Pos{1, 1}).Collapse(); got != (Range{Anchor: Pos{1, 1}, Head: Pos{1, 1}}) {
		t.Errorf("WithHead then Collapse = %+v", got)
	}
	if (Pos{1, 2}).Compare(Pos{1, 3}) != -1 || (Pos{2, 0}).Compare(Pos{1, 9}) != 1 || (Pos{1, 1}).Compare(Pos{1, 1}) != 0 {
		t.Fatal("positions compare out of order")
	}
}

func TestBufferChangesSince(t *testing.T) {
	t.Parallel()

	b := NewBuffer("abc")
	r0 := b.Rev()
	c1 := b.Insert(Pos{0, 0}, "x")
	b.Append("more")
	r1 := b.Rev()
	c2 := b.Delete(Pos{0, 1}, Pos{0, 2})
	b.Insert(Pos{}, "") // changes nothing
	check := func(rev uint64, want ...Change) {
		t.Helper()
		got, ok := b.changesSince(rev)
		if !ok || len(got) != len(want) {
			t.Fatalf("changes since %d: %d, %v, want %d", rev, len(got), ok, len(want))
		}
		for i := range got {
			if Pos(got[i].End()) != want[i].End() {
				t.Fatalf("changes since %d: change %d ends at %v, want %v", rev, i, got[i].End(), want[i].End())
			}
			// Both edits end at 0,1, so where they map places tells them apart.
			for _, p := range []Pos{{0, 0}, {0, 2}, {0, 3}} {
				if g := Pos(got[i].Map(linebuf.Pos(p), false)); g != want[i].Map(p) {
					t.Fatalf("changes since %d: change %d maps %v to %v, want %v", rev, i, p, g, want[i].Map(p))
				}
			}
		}
	}
	check(r0, c1, c2)
	check(r1, c2)
	check(b.Rev())
	if _, ok := b.changesSince(b.Rev() + 1); ok {
		t.Fatal("changes since a revision to come")
	}
	// Changes that positions cannot be followed through.
	for name, fn := range map[string]func(b *Buffer){
		"SetContent": func(b *Buffer) { b.SetContent("new") },
		"SetLines":   func(b *Buffer) { b.SetLines("new") },
		"Clear":      func(b *Buffer) { b.Clear() },
		"Restore":    func(b *Buffer) { b.Restore(b.Snapshot()) },
	} {
		b = NewBuffer("abc")
		rev := b.Rev()
		b.Insert(Pos{}, "x")
		fn(b)
		if _, ok := b.changesSince(rev); ok {
			t.Errorf("changes followed through %s", name)
		}
		if got, ok := b.changesSince(b.Rev()); !ok || len(got) != 0 {
			t.Errorf("no changes since %s: %d, %v", name, len(got), ok)
		}
		b.Insert(Pos{}, "y")
		if got, ok := b.changesSince(b.Rev() - 1); !ok || len(got) != 1 {
			t.Errorf("a change after %s: %d, %v", name, len(got), ok)
		}
	}
}

func TestBufferOffsets(t *testing.T) {
	t.Parallel()

	b := NewBuffer("ab\n\x1b[31mcd\x1b[0m\n日本")
	for _, tc := range []struct {
		p   Pos
		off int
	}{
		{Pos{0, 0}, 0},
		{Pos{0, 2}, 2},
		{Pos{1, 0}, 3},
		{Pos{1, 1}, 9},
		{Pos{1, 2}, 10}, // before the reset after it
		{Pos{2, 0}, 15},
		{Pos{2, 3}, 18},
		{Pos{2, 6}, 21},
	} {
		if got := b.Offset(tc.p); got != tc.off {
			t.Errorf("Offset(%v) = %d, want %d", tc.p, got, tc.off)
		}
		if got := b.PosOf(tc.off); got != tc.p {
			t.Errorf("PosOf(%d) = %v, want %v", tc.off, got, tc.p)
		}
	}
	for _, tc := range []struct {
		off int
		p   Pos
	}{
		{-4, Pos{}},
		{4, Pos{1, 0}},  // within the sequence before c
		{14, Pos{1, 2}}, // the newline
		{16, Pos{2, 0}}, // within 日
		{99, Pos{2, 6}},
	} {
		if got := b.PosOf(tc.off); got != tc.p {
			t.Errorf("PosOf(%d) = %v, want %v", tc.off, got, tc.p)
		}
	}
}

func TestBufferCells(t *testing.T) {
	t.Parallel()

	// a at cell 0, 日 at 1 and 2, b at 3, a tab from 4 to 7, c at 8; the
	// bytes of the text are a 0, 日 1, b 4, the tab 5, c 6.
	for _, line := range []string{"a日b\tc", "\x1b[31ma日\x1b[0mb\t\x1b]8;;https://x.dev\x1b\\c\x1b]8;;\x1b\\"} {
		b := NewBuffer(line)
		for _, method := range []ansi.Method{ansi.GraphemeWidth, ansi.WcWidth} {
			for x, want := range []int{0, 1, 1, 4, 5, 5, 5, 5, 6, 7, 7} {
				if got := b.byteAt(0, method, x); got != want {
					t.Errorf("%q: cell %d is byte %d, want %d", line, x, got, want)
				}
			}
			if b.byteAt(0, method, -1) != 0 || b.byteAt(0, method, 100) != 7 {
				t.Errorf("%q: cells outside the line", line)
			}
			for col, want := range []int{0, 1, 1, 1, 3, 4, 8, 9} {
				if got := b.s.CellOf(0, method, col); got != want {
					t.Errorf("%q: byte %d is at cell %d, want %d", line, col, got, want)
				}
			}
		}
	}
	// A combining mark is drawn at the cell of the letter it is on.
	b := NewBuffer("éx")
	for _, method := range []ansi.Method{ansi.GraphemeWidth, ansi.WcWidth} {
		if b.s.CellOf(0, method, 1) != 0 || b.s.CellOf(0, method, 3) != 1 || b.byteAt(0, method, 1) != 3 {
			t.Errorf("a combining mark is misplaced")
		}
	}
}

func TestBufferCellsLongLine(t *testing.T) {
	t.Parallel()

	// Each part is 8 cells and 6 bytes of text: a, b in red, 日, and a tab.
	const parts = 3000
	line := strings.Repeat("a\x1b[31mb\x1b[0m日\t", parts)
	if len(line) < linebuf.LongLineBytes {
		t.Fatal("the line is not long")
	}
	b := NewBuffer("short\n" + line)
	cellByte := func(x int) int {
		k, in := x/8, x%8
		return 6*k + []int{0, 1, 2, 2, 5, 5, 5, 5}[in]
	}
	byteCell := func(col int) int {
		k, in := col/6, col%6
		return 8*k + []int{0, 1, 2, 2, 2, 4}[in]
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 3000 {
		x := rng.IntN(8 * parts)
		if got := b.byteAt(1, ansi.GraphemeWidth, x); got != cellByte(x) {
			t.Fatalf("cell %d is byte %d, want %d", x, got, cellByte(x))
		}
		col := rng.IntN(6 * parts)
		if got := b.s.CellOf(1, ansi.GraphemeWidth, col); got != byteCell(col) {
			t.Fatalf("byte %d is at cell %d, want %d", col, got, byteCell(col))
		}
	}
	if b.byteAt(1, ansi.GraphemeWidth, 8*parts+5) != 6*parts || b.s.CellOf(1, ansi.GraphemeWidth, 6*parts) != 8*parts {
		t.Fatal("the end of the line is misplaced")
	}
}

func TestBufferNextPrev(t *testing.T) {
	t.Parallel()

	family := "👨‍👩‍👧"
	b := NewBuffer("héllo\néx " + family + "🇯🇵🇫🇷\n\tx\n\x1b[31mred\x1b[m b")
	for _, tc := range []struct{ at, next, prev Pos }{
		{Pos{0, 0}, Pos{0, 1}, Pos{0, 0}}, // the start of the buffer
		{Pos{0, 1}, Pos{0, 3}, Pos{0, 0}}, // é, two bytes
		{Pos{0, 3}, Pos{0, 4}, Pos{0, 1}},
		{Pos{0, 2}, Pos{0, 3}, Pos{0, 0}}, // within é: from its start
		{Pos{0, 6}, Pos{1, 0}, Pos{0, 5}}, // the end of a line
		{Pos{1, 0}, Pos{1, 3}, Pos{0, 6}}, // e and its accent, and back over the line end
		{Pos{1, 3}, Pos{1, 4}, Pos{1, 0}},
		{Pos{1, 5}, Pos{1, 5 + len(family)}, Pos{1, 4}},                   // a family, joined
		{Pos{1, 5 + len(family)}, Pos{1, 5 + len(family) + 8}, Pos{1, 5}}, // a flag
		{Pos{1, 5 + len(family) + 8}, Pos{1, 5 + len(family) + 16}, Pos{1, 5 + len(family)}},
		{Pos{2, 0}, Pos{2, 1}, Pos{1, 5 + len(family) + 16}}, // a tab, a byte
		{Pos{3, 0}, Pos{3, 1}, Pos{2, 2}},                    // the styles are not text
		{Pos{3, 3}, Pos{3, 4}, Pos{3, 2}},
		{Pos{3, 5}, Pos{3, 5}, Pos{3, 4}},  // the end of the buffer
		{Pos{9, 9}, Pos{3, 5}, Pos{3, 4}},  // past it, clamped
		{Pos{-1, 0}, Pos{0, 1}, Pos{0, 0}}, // before it
	} {
		wantPos(t, fmt.Sprintf("Next(%v)", tc.at), b.Next(tc.at), tc.next)
		wantPos(t, fmt.Sprintf("Prev(%v)", tc.at), b.Prev(tc.at), tc.prev)
	}
	// Every step forward is one back, through the whole buffer.
	var steps []Pos
	for p := (Pos{}); ; {
		steps = append(steps, p)
		n := b.Next(p)
		if n == p {
			break
		}
		if !p.Before(n) || b.Prev(n) != p {
			t.Fatalf("Next(%v) = %v, and Prev of that %v", p, n, b.Prev(n))
		}
		p = n
	}
	if len(steps) != 22 {
		t.Errorf("%d steps through the buffer: %v", len(steps), steps)
	}
	var empty Buffer
	if empty.Next(Pos{}) != (Pos{}) || empty.Prev(Pos{1, 1}) != (Pos{}) {
		t.Error("an empty buffer has a position other than its start")
	}
}

func TestBufferWords(t *testing.T) {
	t.Parallel()

	const text = "hello, world! foo.bar don't 3.14 snake_case\n\n  -- \x1b[1mПривет\x1b[m мир; γειά σου\nمرحبا بالعالم 日本語 カタカナ..."
	// Words as Unicode finds them, joined across no other characters.
	b := NewBuffer(text)
	b.SetWordChars("")
	for _, tc := range []struct{ at, next, prev Pos }{
		{Pos{0, 0}, Pos{0, 5}, Pos{0, 0}},
		{Pos{0, 2}, Pos{0, 5}, Pos{0, 0}},  // within a word, to its ends
		{Pos{0, 5}, Pos{0, 12}, Pos{0, 0}}, // over the punctuation and spaces between
		{Pos{0, 9}, Pos{0, 12}, Pos{0, 7}},
		{Pos{0, 12}, Pos{0, 21}, Pos{0, 7}}, // foo.bar is a word, as is don't
		{Pos{0, 21}, Pos{0, 27}, Pos{0, 14}},
		{Pos{0, 27}, Pos{0, 32}, Pos{0, 22}}, // and 3.14, and snake_case
		{Pos{0, 32}, Pos{0, 43}, Pos{0, 28}},
		// Across line ends, an empty line, and punctuation.
		{Pos{0, 43}, Pos{2, 17}, Pos{0, 33}},
		{Pos{2, 0}, Pos{2, 17}, Pos{0, 33}},
		{Pos{2, 5}, Pos{2, 17}, Pos{0, 33}},
		// Cyrillic, styled, and Greek.
		{Pos{2, 17}, Pos{2, 24}, Pos{2, 5}},
		{Pos{2, 24}, Pos{2, 34}, Pos{2, 18}},
		{Pos{2, 34}, Pos{2, 41}, Pos{2, 26}},
		{Pos{2, 41}, Pos{3, 10}, Pos{2, 35}},
		// Arabic, ideographs joined into a word, and katakana a word.
		{Pos{3, 10}, Pos{3, 25}, Pos{3, 0}},
		{Pos{3, 25}, Pos{3, 35}, Pos{3, 11}},
		{Pos{3, 29}, Pos{3, 35}, Pos{3, 26}},
		{Pos{3, 35}, Pos{3, 48}, Pos{3, 26}},
		// The ends of the buffer, with nothing but punctuation after.
		{Pos{3, 48}, Pos{3, 51}, Pos{3, 36}},
		{Pos{9, 9}, Pos{3, 51}, Pos{3, 36}},
	} {
		wantPos(t, fmt.Sprintf("NextWord(%v)", tc.at), b.NextWord(tc.at), tc.next)
		wantPos(t, fmt.Sprintf("PrevWord(%v)", tc.at), b.PrevWord(tc.at), tc.prev)
	}

	// By default, the word characters join "--" into a word, and the dots
	// after the katakana into it.
	b = NewBuffer(text)
	for _, tc := range []struct{ at, next, prev Pos }{
		{Pos{0, 12}, Pos{0, 21}, Pos{0, 7}},
		{Pos{0, 43}, Pos{2, 4}, Pos{0, 33}},
		{Pos{2, 5}, Pos{2, 17}, Pos{2, 2}},
		{Pos{3, 35}, Pos{3, 51}, Pos{3, 26}},
		{Pos{9, 9}, Pos{3, 51}, Pos{3, 36}},
	} {
		wantPos(t, fmt.Sprintf("default: NextWord(%v)", tc.at), b.NextWord(tc.at), tc.next)
		wantPos(t, fmt.Sprintf("default: PrevWord(%v)", tc.at), b.PrevWord(tc.at), tc.prev)
	}
	// Others, set for the buffer: a path is not one word, but a range of
	// numbers is.
	b = NewBuffer("open ~/src/main.go lines 10-20")
	b.SetWordChars("-")
	wantPos(t, "custom: NextWord", b.NextWord(Pos{0, 5}), Pos{0, 10})
	wantPos(t, "custom: NextWord past a path", b.NextWord(Pos{0, 10}), Pos{0, 18})
	wantPos(t, "custom: PrevWord", b.PrevWord(Pos{0, 30}), Pos{0, 25})

	var empty Buffer
	if empty.NextWord(Pos{}) != (Pos{}) || empty.PrevWord(Pos{1, 1}) != (Pos{}) {
		t.Error("an empty buffer has a word")
	}
	if _, _, ok := empty.WordAt(Pos{}); ok {
		t.Error("an empty buffer has a word at its start")
	}
}

func TestBufferWordAt(t *testing.T) {
	t.Parallel()

	const text = "see ~/src/main.go, e-mail snake_case 日本語\n\t\x1b[31mred\x1b[m-hot!"
	for _, tc := range []struct {
		name     string
		chars    *string // nil leaves the buffer's unset
		at       Pos
		from, to Pos
		ok       bool
	}{
		// With the default word characters, a path is one word.
		{"path", nil, Pos{0, 12}, Pos{0, 4}, Pos{0, 17}, true},
		{"path start", nil, Pos{0, 4}, Pos{0, 4}, Pos{0, 17}, true},
		{"ends at", nil, Pos{0, 17}, Pos{0, 4}, Pos{0, 17}, true},
		{"between", nil, Pos{0, 18}, Pos{}, Pos{}, false},
		{"hyphen", nil, Pos{0, 20}, Pos{0, 19}, Pos{0, 25}, true},
		{"snake", nil, Pos{0, 30}, Pos{0, 26}, Pos{0, 36}, true},
		{"ideographs", nil, Pos{0, 40}, Pos{0, 37}, Pos{0, 46}, true},
		{"end of line", nil, Pos{0, 99}, Pos{0, 37}, Pos{0, 46}, true},
		{"styled", nil, Pos{1, 2}, Pos{1, 1}, Pos{1, 8}, true},
		{"tab", nil, Pos{1, 0}, Pos{}, Pos{}, false},
		// Set for the buffer, only "-" joins words.
		{"custom path", new("-"), Pos{0, 12}, Pos{0, 10}, Pos{0, 17}, true},
		{"custom hyphen", new("-"), Pos{0, 20}, Pos{0, 19}, Pos{0, 25}, true},
		{"custom styled", new("-"), Pos{1, 2}, Pos{1, 1}, Pos{1, 8}, true},
		// Set to none, words are as Unicode finds them.
		{"no chars, path", new(""), Pos{0, 12}, Pos{0, 10}, Pos{0, 17}, true},
		{"no chars, hyphen", new(""), Pos{0, 20}, Pos{0, 19}, Pos{0, 20}, true}, // the word that ends there
		{"no chars, after hyphen", new(""), Pos{0, 21}, Pos{0, 21}, Pos{0, 25}, true},
		{"no chars, before hyphen", new(""), Pos{0, 19}, Pos{0, 19}, Pos{0, 20}, true},
		{"no chars, snake", new(""), Pos{0, 30}, Pos{0, 26}, Pos{0, 36}, true},
		{"no chars, styled", new(""), Pos{1, 4}, Pos{1, 1}, Pos{1, 4}, true},
	} {
		b := NewBuffer(text)
		if tc.chars != nil {
			b.SetWordChars(*tc.chars)
		}
		from, to, ok := b.WordAt(tc.at)
		if ok != tc.ok || ok && (from != tc.from || to != tc.to) {
			t.Errorf("%s: WordAt(%v) = %v-%v %v, want %v-%v %v", tc.name, tc.at, from, to, ok, tc.from, tc.to, tc.ok)
		}
	}
}

func TestBufferTextIn(t *testing.T) {
	t.Parallel()

	b := NewBuffer("one \x1b[31mred\x1b[m\n\ttabbed\n日本 three")
	for _, tc := range []struct {
		from, to Pos
		want     string
	}{
		{Pos{0, 0}, Pos{0, 3}, "one"},
		{Pos{0, 4}, Pos{1, 1}, "red\n\t"}, // without styles, with tabs
		{Pos{1, 1}, Pos{0, 4}, "red\n\t"}, // swapped
		{Pos{0, 2}, Pos{2, 6}, "e red\n\ttabbed\n日本"},
		{Pos{2, 4}, Pos{9, 9}, "本 three"}, // clamped, from the start of 本
		{Pos{1, 3}, Pos{1, 3}, ""},
	} {
		if got := b.TextIn(tc.from, tc.to); got != tc.want {
			t.Errorf("TextIn(%v, %v) = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}
	if got := b.TextIn(Pos{}, b.End()); got != "one red\n\ttabbed\n日本 three" {
		t.Errorf("all of it: %q", got)
	}
}

// TestViewportDrawsEditedBuffer draws a buffer through random edits, as a
// buffer given its lines from scratch is drawn, wrapped and not.
func TestViewportDrawsEditedBuffer(t *testing.T) {
	t.Parallel()

	parts := []string{
		"word", "a", "日本", "\t", " ", "é", "e\u0301", "👍🏽", "🇯🇵", "\n",
		"\x1b[31m", "\x1b[1;4m", "\x1b[m", "\x1b[38;5;196m", "\x1b[22m", "\x1b]8;;https://x.dev\x1b\\", "\x1b]8;;\x1b\\",
	}
	rng := rand.New(rand.NewPCG(3, 7))
	text := func(n int) string {
		if rng.IntN(40) == 0 {
			// Now and then, a line longer than a long line.
			n = linebuf.LongLineBytes / 3
		}
		var b strings.Builder
		for range n {
			b.WriteString(parts[rng.IntN(len(parts))])
		}
		return b.String()
	}
	b := NewBuffer(text(400))
	pos := func() Pos { return Pos{Line: rng.IntN(b.Len() + 1), Col: rng.IntN(30)} }
	for i := range 120 {
		switch rng.IntN(4) {
		case 0:
			b.Append(text(1 + rng.IntN(20)))
		case 1:
			from := pos()
			b.Edit(Edit{From: from, To: Pos{Line: from.Line + rng.IntN(3), Col: rng.IntN(30)}, Text: text(rng.IntN(6))}, Edit{From: pos(), Text: text(2)})
		default:
			from := pos()
			b.Replace(from, Pos{Line: from.Line + rng.IntN(2), Col: rng.IntN(30)}, text(rng.IntN(10)))
		}
		if i%6 != 0 {
			continue
		}
		var lines []string
		for k := range b.Len() {
			lines = append(lines, b.Line(k))
		}
		var fresh Buffer
		fresh.SetLines(lines...)
		at := ViewportPos{Line: rng.IntN(b.Len())}
		for _, wrap := range []WrapMode{WrapNone, WrapWords} {
			view := func(buf *Buffer) string {
				return New(WithColors(color.White, color.Black)).View(40, 12, Viewport(buf).At(at).Wrap(wrap).LineNumbers(lipgloss.NewStyle())).Content
			}
			if got, want := view(b), view(&fresh); got != want {
				t.Fatalf("step %d: drawn wrapped %v from %v:\n%s\nwant:\n%s", i, wrap, at, got, want)
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

// TestViewportWheelBeforeRedraw checks that wheel events that come before
// the screen is redrawn each scroll a step. Bubble Tea gives mouse events to
// the view last flushed to the terminal, at its frame rate, though View runs
// after every Update.
func TestViewportWheelBeforeRedraw(t *testing.T) {
	t.Parallel()

	b := NewBuffer(strings.Repeat("line\n", 40))
	r := New()
	var pos ViewportPos
	tree := func() Child {
		return Viewport(b).At(pos).OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })
	}
	wheel := tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown}
	flushed := r.View(10, 5, tree())
	for range 4 {
		flushed.OnMouse(wheel)
		r.Update(wheel)
		r.View(10, 5, tree())
	}
	if pos != (ViewportPos{Line: 12}) {
		t.Fatalf("4 steps before a redraw scrolled to %+v, want line 12", pos)
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
	// 16 columns, 8 in view from column 6: a thumb of 4 cells, 6/16 of the
	// way along the 8 cells of the track.
	got := ansi.Strip(Render(8, 2, Column(Viewport(wide).ID("w").At(ViewportPos{X: 6}), ScrollbarOf("w").Horizontal().Style(ScrollbarStyle{Thumb: "#"}))))
	if want := "6789abcd\n   #### "; got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
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
	if pos != (ViewportPos{Line: 3}) {
		t.Fatalf("the selection scrolled the viewport to %+v, want line 3", pos)
	}
	r.Update(release(0, 3))
	r.Render(3, 2, tree())
	wantSelection(t, r, "a\nb\nc\nd\ne\nf")
}

// highlighted returns the rows of frame out, w cells wide, with the cells
// drawn on a background, as a selection draws them, and a dot for each of
// the others.
func highlighted(out string, w int) []string {
	c := canvas.Parse(out, ansi.GraphemeWidth)
	rows := make([]string, c.Height())
	for y := range rows {
		var b strings.Builder
		for x := range w {
			switch cell := c.CellAt(x, y); {
			case cell != nil && cell.Width == 0:
				// The second half of a wide character.
			case cell == nil || cell.Style.Bg == nil:
				b.WriteByte('.')
			case cell.Content == "":
				b.WriteByte(' ')
			default:
				b.WriteString(cell.Content)
			}
		}
		rows[y] = b.String()
	}
	return rows
}

func wantHighlighted(t *testing.T, out string, w int, want ...string) {
	t.Helper()
	if got := highlighted(out, w); !slices.Equal(got, want) {
		t.Fatalf("highlighted:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestViewportSelectionFollowsText(t *testing.T) {
	t.Parallel()

	b := NewBuffer("alpha beta gamma delta epsilon\nsecond line here")
	w, mode := 12, WrapWords
	var pos ViewportPos
	tree := func() Child { return Viewport(b).Wrap(mode).At(pos).Selectable() }
	r := New()
	// From "beta" on the first row to the end of "delta" on the second.
	selectDrag(r, w, 6, tree, 0, [2]int{6, 0}, [2]int{10, 1})
	wantSelection(t, r, "beta gamma delta")
	wantHighlighted(t, r.Render(w, 6, tree()), w,
		"......beta  ", "gamma delta.", "............", "............", "............", "............")

	// Wider, the line wraps elsewhere, and the selection stays on its text.
	w = 20
	wantHighlighted(t, r.Render(w, 6, tree()), w,
		"......beta gamma    ", "delta...............", "....................",
		"....................", "....................", "....................")
	wantSelection(t, r, "beta gamma delta")

	// Unwrapped and scrolled sideways, too.
	mode, w = WrapNone, 12
	wantHighlighted(t, r.Render(w, 2, tree()), w, "......beta g", "............")
	pos.X = 8
	wantHighlighted(t, r.Render(w, 2, tree()), w, "ta gamma del", "............")
	wantSelection(t, r, "beta gamma delta")

	// Lines appended move nothing.
	pos.X = 0
	b.Append("third", "fourth")
	r.Render(w, 2, tree())
	wantSelection(t, r, "beta gamma delta")
}

func TestViewportSelectionFollowsEdits(t *testing.T) {
	t.Parallel()

	b := NewBuffer("zero\none two three\nfour")
	tree := func() Child { return Viewport(b).Selectable() }
	r := New()
	selectDrag(r, 20, 3, tree, 0, [2]int{4, 1}, [2]int{6, 1})
	wantSelection(t, r, "two")

	for _, tt := range []struct {
		name     string
		edit     func()
		want     string
		row1     string
		selected string
	}{
		{"before", func() { b.Insert(Pos{Line: 1}, "my ") }, "two", ".......two..........", ""},
		{"inside", func() { b.Insert(Pos{Line: 1, Col: 9}, "o-tw") }, "two-two", ".......two-two......", ""},
		{"after", func() { b.Replace(Pos{Line: 1, Col: 20}, Pos{Line: 2}, "!\n") }, "two-two", ".......two-two......", ""},
		{"a line before", func() { b.Insert(Pos{}, "new\n") }, "two-two", "....................", ".......two-two......"},
	} {
		tt.edit()
		out := r.Render(20, 3, tree())
		wantSelection(t, r, tt.want)
		rows := highlighted(out, 20)
		if rows[1] != tt.row1 || tt.selected != "" && rows[2] != tt.selected {
			t.Fatalf("after an edit %s, highlighted:\n%s", tt.name, strings.Join(rows, "\n"))
		}
	}
	if got := b.String(); got != "new\nzero\nmy one two-two three!\nfour" {
		t.Fatalf("buffer %q", got)
	}

	// Setting the buffer again loses the selection.
	b.SetContent("other text")
	r.Render(20, 3, tree())
	wantSelection(t, r, "")
	wantHighlighted(t, r.Render(20, 3, tree()), 20, "....................", "....................", "....................")
}

func TestViewportSelectionCappedLog(t *testing.T) {
	t.Parallel()

	b := NewBuffer("")
	for i := range 10 {
		b.Append(fmt.Sprintf("line %d", i))
	}
	tree := func() Child { return Viewport(b).At(ViewportEnd).Selectable() }
	r := New()
	// The last six lines are in sight: from "6" to "7".
	selectDrag(r, 8, 6, tree, 0, [2]int{5, 2}, [2]int{5, 3})
	wantSelection(t, r, "6\nline 7")
	// Lines appended, and as many of the first taken out, as a log capped at
	// ten lines does: the selection stays on its lines as they scroll up.
	for range 2 {
		b.Append(fmt.Sprintf("line %d", b.Len()+3))
		b.Delete(Pos{}, Pos{Line: 1})
	}
	wantHighlighted(t, r.Render(8, 6, tree()), 8, ".....6  ", "line 7..", "........", "........", "........", "........")
	wantSelection(t, r, "6\nline 7")
	// Once its lines are taken out, nothing is selected.
	b.Delete(Pos{}, Pos{Line: 8})
	r.Render(8, 6, tree())
	wantSelection(t, r, "")
}

func TestViewportTripleClickWrappedLine(t *testing.T) {
	t.Parallel()

	b := NewBuffer("short\nthe second line wraps to three rows\nlast")
	tree := func() Child { return Viewport(b).Wrap(WrapWords).Selectable() }
	r := New(WithClock(func() time.Time { return time.Unix(1, 0) }))
	for range 3 {
		selectDrag(r, 12, 5, tree, 0, [2]int{5, 2})
	}
	// All the rows the line wraps to, not the row clicked.
	wantSelection(t, r, "the second line wraps to three rows")
	wantHighlighted(t, r.Render(12, 5, tree()), 12,
		"............", "the second  ", "line wraps  ", "to three    ", "rows........")
}

func TestViewportBlockSelectionResized(t *testing.T) {
	t.Parallel()

	b := NewBuffer("abcdefgh\nijklmnop")
	w := 4
	tree := func() Child { return Viewport(b).Wrap(WrapChars).Selectable() }
	r := New()
	selectDrag(r, w, 4, tree, tea.ModAlt, [2]int{1, 0}, [2]int{2, 2})
	wantSelection(t, r, "bc\nfg\njk")
	// Wider, the rows the corners were on are the lines' first rows, and
	// the rectangle is worked out from where they are drawn now.
	w = 8
	wantHighlighted(t, r.Render(w, 4, tree()), w, ".bc.....", ".jk.....", "........", "........")
	wantSelection(t, r, "bc\njk")
}

func TestViewportSelectAllDrawsInSight(t *testing.T) {
	t.Parallel()

	lines := make([]string, 200_000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d of many", i)
	}
	b := NewBuffer(strings.Join(lines, "\n"))
	pos := ViewportPos{Line: 1000}
	tree := func() Child { return Viewport(b).ID("v").At(pos).Selectable() }
	r := New()
	r.Render(20, 5, tree())
	if !r.SelectAll("v") {
		t.Fatal("SelectAll found nothing")
	}
	for range 3 {
		r.Render(20, 5, tree())
		cells := 0
		for i := range r.r.prevOps {
			if o := &r.r.prevOps[i]; o.kind == opSelect {
				r.r.eachSelected(o, func(image.Point) { cells++ })
			}
		}
		if cells != 20*5 {
			t.Fatalf("drew %d selected cells, want the 100 in sight", cells)
		}
		pos.Line += 50_000
	}
	text, ok := r.Selection()
	if !ok || strings.Count(text, "\n") != len(lines)-1 || !strings.HasSuffix(text, "line 199999 of many") {
		t.Fatalf("selected %d lines", strings.Count(text, "\n")+1)
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
	first := r.Render(10, 5, tree())
	b.Append("more")
	if again := r.Render(10, 5, tree()); unsafe.StringData(first) != unsafe.StringData(again) {
		t.Fatal("a viewport unchanged in sight was drawn again")
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
	if fg := c.CellAt(5, 0).Style.Fg; fg == nil || !sameRGBA(fg, lipgloss.Color("#00ff00")) {
		t.Fatalf("plain text after the reset drawn in %v, not the line's green", fg)
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
	if fg := c.CellAt(4, 0).Style.Fg; fg == nil || !sameRGBA(fg, lipgloss.Color("#00ff00")) {
		t.Fatalf("text after the reset drawn in %v, not the viewport's green", fg)
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
	text := "\x1b[32mopen\n" + sb.String() + "\nafter"
	wrapped := Render(60, 8, Viewport(NewBuffer(text)).Wrap(WrapWords).At(ViewportPos{Line: 1, Row: 120}))
	// The rows drawn from the start of the line, in a buffer of its own,
	// that has no checkpoints to start from.
	whole := strings.Split(Render(60, 128, Viewport(NewBuffer(text)).Wrap(WrapWords).At(ViewportPos{Line: 1})), "\n")
	if want := strings.Join(whole[120:], "\n"); wrapped != want || strings.TrimSpace(ansi.Strip(want)) == "" {
		t.Fatalf("wrapped from row 120:\n%s\nwant:\n%s", wrapped, want)
	}
}

// caretAt returns a caret, an empty range, at line, col.
func caretAt(line, col int) Range {
	return Range{Anchor: Pos{Line: line, Col: col}, Head: Pos{Line: line, Col: col}}
}

func TestViewportCaretMouse(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	b := NewBuffer("hello big world\nsecond line")
	var sel Range
	var told []Range
	tree := func() Child {
		return Column(
			Fixed(2, Viewport(b).ID("v").Caret(&sel).OnCaret(func(rg Range) tea.Cmd { told = append(told, rg); return nil })),
			Text("other text").Selectable(),
		)
	}
	r := New(WithClock(func() time.Time { return now }))
	click := func(x, y int, mod tea.KeyMod) {
		t.Helper()
		now = now.Add(time.Second)
		selectDrag(r, 16, 3, tree, mod, [2]int{x, y})
	}

	// A press puts the caret before the character under it, and tells of it.
	click(3, 0, 0)
	if sel != caretAt(0, 3) || len(told) == 0 || told[len(told)-1] != sel {
		t.Fatalf("after a click, range %+v, told %+v", sel, told)
	}
	wantSelection(t, r, "")
	// Past the end of a line, at its end.
	click(14, 1, 0)
	if sel != caretAt(1, 11) {
		t.Fatalf("after a click past the end, range %+v", sel)
	}
	// Shift moves the caret, keeping the anchor.
	click(6, 0, tea.ModShift)
	if sel != (Range{Anchor: Pos{1, 11}, Head: Pos{0, 6}}) {
		t.Fatalf("after shift+click, range %+v", sel)
	}
	wantSelection(t, r, "big world\nsecond line")
	// A drag runs from the caret at the press to the caret at the pointer,
	// either way, and sends what it selected.
	now = now.Add(time.Second)
	if msg, ok := selectDrag(r, 16, 3, tree, 0, [2]int{10, 0}, [2]int{6, 0}); !ok || msg.Text != "big " || msg.ID != "v" {
		t.Fatalf("SelectionMsg = %+v (%v)", msg, ok)
	}
	if sel != (Range{Anchor: Pos{0, 10}, Head: Pos{0, 6}}) || told[len(told)-1] != sel {
		t.Fatalf("after a drag, range %+v, told %+v", sel, told[len(told)-1])
	}
	wantHighlighted(t, r.Render(16, 3, tree()), 16, "......big ......", "................", "................")
	// Across lines.
	now = now.Add(time.Second)
	selectDrag(r, 16, 3, tree, 0, [2]int{10, 0}, [2]int{3, 1})
	wantSelection(t, r, "world\nsec")

	// A double click selects the word, and a triple click the line.
	click(12, 0, 0)
	now = now.Add(-time.Second + time.Millisecond)
	click(12, 0, 0)
	wantSelection(t, r, "world")
	now = now.Add(-time.Second + time.Millisecond)
	click(12, 0, 0)
	wantSelection(t, r, "hello big world")

	// Selecting elsewhere collapses it to its caret, drawn without a
	// highlight.
	now = now.Add(time.Second)
	selectDrag(r, 16, 3, tree, 0, [2]int{0, 2}, [2]int{4, 2})
	if !sel.Empty() {
		t.Fatalf("after selecting elsewhere, range %+v", sel)
	}
	wantSelection(t, r, "other")
	wantHighlighted(t, r.Render(16, 3, tree()), 16, "................", "................", "other...........")
}

func TestViewportCaretKeyboard(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	b := NewBuffer("hello world\nsecond")
	var sel Range
	tree := func() Child {
		return Column(Fixed(2, Viewport(b).ID("v").Caret(&sel)), Text("other text").Selectable())
	}
	var copied string
	r := New(WithClock(func() time.Time { return now }), WithClipboard(func(s string) error { copied = s; return nil }))
	selectDrag(r, 12, 3, tree, 0, [2]int{0, 2}, [2]int{4, 2})
	wantSelection(t, r, "other")

	// A range the program makes, such as with shift+right, is the selection
	// at once, before a frame is drawn, and clears the other.
	sel = Range{Anchor: Pos{0, 0}, Head: Pos{0, 5}}
	wantSelection(t, r, "hello")
	if out := highlighted(r.Render(12, 3, tree()), 12); out[0] != "hello......." || out[2] != "............" {
		t.Fatalf("highlighted %q", out)
	}
	// Copying it leaves it as it is, and sweeps the band across it.
	cmd := r.CopySelection()
	if cmd == nil || sel != (Range{Head: Pos{0, 5}}) {
		t.Fatalf("copy returned %v, left range %+v", cmd, sel)
	}
	drainSequences(cmd)
	if copied != "hello" {
		t.Fatalf("copied %q", copied)
	}
	steady := highlighted(r.Render(12, 3, tree()), 12)[0]
	now = now.Add(flashFor / 2)
	if band := highlighted(r.Render(12, 3, tree()), 12)[0]; band == steady {
		t.Fatalf("no band half way through the flash: %q", band)
	}
	now = now.Add(flashFor)
	if got := highlighted(r.Render(12, 3, tree()), 12)[0]; got != "hello......." {
		t.Fatalf("the selection is not drawn after the flash: %q", got)
	}

	// A range changed in the same Update as it is copied is the one copied,
	// and one out of the buffer is clamped to it.
	sel = Range{Anchor: Pos{0, 6}, Head: Pos{1, 99}}
	wantCopy(t, r.CopySelection(), "v", "world\nsecond")
	now = now.Add(2 * flashFor)
	if got := highlighted(r.Render(12, 3, tree()), 12); got[0] != "......world " || got[1] != "second......" {
		t.Fatalf("highlighted %q", got)
	}

	// Clearing it collapses it to its caret.
	r.ClearSelection()
	if sel != caretAt(1, 6) {
		t.Fatalf("after ClearSelection, range %+v", sel)
	}
	wantSelection(t, r, "")
	// Selecting all of it, by the viewport's ID.
	if !r.SelectAll("v") || sel != (Range{Head: Pos{1, 6}}) {
		t.Fatalf("after SelectAll, range %+v", sel)
	}
	wantSelection(t, r, "hello world\nsecond")
	// No message is sent for what the program selects.
	sel = Range{Anchor: Pos{0, 6}, Head: Pos{0, 11}}
	for _, m := range drain(r.Update(nil)) {
		if _, ok := m.(SelectionMsg); ok {
			t.Fatal("a SelectionMsg was sent for a range the program made")
		}
	}
	wantSelection(t, r, "world")
	// An edit the program follows the range through.
	c := b.Insert(Pos{0, 0}, "oh ")
	sel = c.MapRange(sel)
	wantSelection(t, r, "world")
}

func TestViewportCaretCursor(t *testing.T) {
	t.Parallel()

	cur := &tea.Cursor{Shape: tea.CursorBar}
	cursorOf := func(w, h int, n ViewportNode) *tea.Cursor {
		t.Helper()
		return New().View(w, h, n).Cursor
	}
	wantCursor := func(name string, w, h int, n ViewportNode, x, y int) {
		t.Helper()
		got := cursorOf(w, h, n)
		if got == nil || got.X != x || got.Y != y || got.Shape != tea.CursorBar {
			t.Errorf("%s: cursor %+v, want at %d, %d", name, got, x, y)
		}
	}
	wantNone := func(name string, w, h int, n ViewportNode) {
		t.Helper()
		if got := cursorOf(w, h, n); got != nil {
			t.Errorf("%s: cursor %+v, want none", name, got)
		}
	}

	b := NewBuffer("abcde\nfgh\n日本\tx")
	sel := caretAt(1, 2)
	wantCursor("a caret", 6, 3, Viewport(b).Caret(&sel).Cursor(cur), 2, 1)
	if got := Debug(6, 3, Viewport(b).Caret(&sel)); !strings.Contains(got, "caret=1:2") || strings.Contains(got, "end-of-row") {
		t.Errorf("Debug:\n%s", got)
	}
	if eor := (Range{Anchor: sel.Head, Head: sel.Head, EndOfRow: true}); !strings.Contains(Debug(6, 3, Viewport(b).Caret(&eor)), "caret=1:2 end-of-row") {
		t.Errorf("Debug with EndOfRow:\n%s", Debug(6, 3, Viewport(b).Caret(&eor)))
	}
	wantCursor("beside line numbers", 8, 3, Viewport(b).Caret(&sel).Cursor(cur).LineNumbers(lipgloss.NewStyle()), 4, 1)
	// At the end of a line as wide as the lines are laid out, in the column
	// kept for it, which the line does not take.
	sel = caretAt(0, 5)
	wantCursor("at the end of a full line", 6, 3, Viewport(b).Caret(&sel).Cursor(cur), 5, 0)
	wantRender(t, 6, 3, Viewport(b).Caret(&sel), "abcde ", "fgh   ", "日本  ")
	// After a wide character and a tab.
	sel = caretAt(2, 7)
	wantCursor("after a tab", 10, 3, Viewport(b).Caret(&sel).Cursor(cur), 8, 2)
	// Out of sight, or without Cursor, there is none.
	wantNone("below", 6, 2, Viewport(b).Caret(&sel).Cursor(cur))
	wantNone("without Cursor", 6, 3, Viewport(b).Caret(&sel))
	wantNone("without Caret", 6, 3, Viewport(b).Cursor(cur))

	// Wrapped rows, at a wrap point on the row after, and on the last row.
	w := NewBuffer("aaaa bbbb cccc")
	for _, tc := range []struct {
		mode WrapMode
		col  int
		x, y int
	}{
		{WrapWords, 4, 4, 0},
		{WrapWords, 5, 0, 1},
		{WrapWords, 14, 4, 2},
		{WrapChars, 5, 0, 1},
		{WrapChars, 4, 4, 0},
		{WrapChars, 14, 4, 2},
	} {
		sel = caretAt(0, tc.col)
		wantCursor(fmt.Sprintf("wrapped %d at %d", tc.mode, tc.col), 6, 3, Viewport(w).Wrap(tc.mode).Caret(&sel).Cursor(cur), tc.x, tc.y)
	}

	// Scrolled sideways, from where it was scrolled to.
	long := NewBuffer("abcdefghij")
	sel = caretAt(0, 5)
	wantCursor("scrolled sideways", 5, 1, Viewport(long).At(ViewportPos{X: 3}).Caret(&sel).Cursor(cur), 2, 0)
	sel = caretAt(0, 10)
	wantCursor("at the end scrolled sideways", 5, 1, Viewport(long).At(ViewportPos{X: 6}).Caret(&sel).Cursor(cur), 4, 0)
	sel = caretAt(0, 1)
	wantNone("scrolled past", 5, 1, Viewport(long).At(ViewportPos{X: 3}).Caret(&sel).Cursor(cur))

	// An empty buffer has the caret at its top-left, and takes edits.
	var empty Buffer
	sel = Range{}
	wantCursor("an empty buffer", 6, 3, Viewport(&empty).Caret(&sel).Cursor(cur), 0, 0)
	if r, _ := Solve(6, 3, Row(Viewport(&empty).ID("v").Caret(&sel))).Rect("v"); r.Dx() != 1 {
		t.Errorf("an empty buffer with a caret is laid out at %v, without a column for it", r)
	}
	c := empty.Insert(sel.Head, "hi")
	sel = Range{Anchor: c.End(), Head: c.End()}
	wantCursor("typed in an empty buffer", 6, 3, Viewport(&empty).Caret(&sel).Cursor(cur), 2, 0)
	// A caret out of the buffer is clamped to it.
	sel = caretAt(9, 9)
	wantCursor("out of the buffer", 6, 3, Viewport(&empty).Caret(&sel).Cursor(cur), 2, 0)
}

func TestViewportCaretDragScrolls(t *testing.T) {
	t.Parallel()

	b := NewBuffer("a\nb\nc\nd\ne\nf")
	var sel Range
	var pos ViewportPos
	tree := func() Child {
		return Viewport(b).At(pos).ID("v").Caret(&sel).OnScroll(func(p ViewportPos) tea.Cmd { pos = p; return nil })
	}
	r := New()
	r.after = immediate
	r.Render(3, 2, tree())
	r.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	r.Render(3, 2, tree())
	cmd := r.Update(dragTo(0, 3))
	r.Render(3, 2, tree())
	for range 10 {
		var next []tea.Cmd
		for _, msg := range drain(cmd) {
			next = append(next, r.Update(msg))
			r.Render(3, 2, tree())
		}
		cmd = tea.Batch(next...)
	}
	if pos.Line == 0 {
		t.Fatal("the drag did not scroll the viewport")
	}
	r.Update(release(0, 3))
	r.Render(3, 2, tree())
	if sel.Anchor != (Pos{}) || sel.Head != b.End() {
		t.Fatalf("range %+v, want to the end", sel)
	}
}

func TestViewportCaretEndOfRow(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	cur := &tea.Cursor{Shape: tea.CursorBar}
	var b *Buffer
	var mode WrapMode
	var sel Range
	tree := func() Child { return Viewport(b).ID("v").Wrap(mode).Caret(&sel).Cursor(cur) }
	r := New(WithClock(func() time.Time { return now }))
	click := func(x, y int, pts ...[2]int) {
		t.Helper()
		now = now.Add(time.Second)
		selectDrag(r, 6, 4, tree, 0, append([][2]int{{x, y}}, pts...)...)
	}
	wantAt := func(what string, head Pos, eor bool, x, y int) {
		t.Helper()
		wantCaret(t, what, sel, head, eor)
		if c := r.View(6, 4, tree()).Cursor; c == nil || c.X != x || c.Y != y {
			t.Errorf("%s: the cursor is at %+v, want %d, %d", what, c, x, y)
		}
		v, _ := r.Frame().Viewport("v")
		if c, ok := v.CaretCell(sel); !ok || c != image.Pt(x, y) {
			t.Errorf("%s: CaretCell = %v (%v), want %d, %d", what, c, ok, x, y)
		}
	}

	for _, mode = range []WrapMode{WrapWords, WrapChars} {
		// Rows of five columns, the sixth the caret's: "aaaa ", "bbbb ",
		// "cccc" either way.
		b = NewBuffer("aaaa bbbb cccc")
		name := fmt.Sprintf("mode %d", mode)
		// Past the end of a row the line wraps after, the caret stays on it.
		click(5, 0)
		wantAt(name+": past the end of the first row", Pos{0, 5}, true, 5, 0)
		// The first cell of the next row is the same place, on that row.
		click(0, 1)
		wantAt(name+": the start of the second row", Pos{0, 5}, false, 0, 1)
		// Past the end of the last row, the end of the line.
		click(5, 2)
		wantAt(name+": past the end of the last row", Pos{0, 14}, false, 4, 2)
		// A drag to past the end of a row ends there, on it.
		click(1, 1, [2]int{5, 1})
		wantAt(name+": a drag past the end of a row", Pos{0, 10}, true, 5, 1)
		wantSelection(t, r, "bbb ")
		// Back to its start, the Head on the next row.
		click(5, 0, [2]int{0, 1})
		wantAt(name+": a drag from past the end of a row", Pos{0, 5}, false, 0, 1)
	}

	// Spaces hanging off a row wrapped at words are taken in.
	mode, b = WrapWords, NewBuffer("aaaa   bbbb")
	click(5, 0)
	wantAt("past spaces hanging off a row", Pos{0, 7}, true, 5, 0)
	click(4, 0)
	wantAt("on a space before the edge", Pos{0, 4}, false, 4, 0)

	// A double click on a word that ends where its line wraps leaves the
	// caret at the end of that row, and with the pointer on the row after,
	// on that row.
	mode, b = WrapChars, NewBuffer("abcde fghi")
	click(1, 0)
	now = now.Add(-time.Second + time.Millisecond)
	click(1, 0)
	wantSelection(t, r, "abcde")
	wantAt("a double click on a word ending a row", Pos{0, 5}, true, 5, 0)
	click(3, 1)
	now = now.Add(-time.Second + time.Millisecond)
	click(3, 1)
	wantSelection(t, r, "fghi")
	wantAt("a double click on the last word", Pos{0, 10}, false, 5, 1)

	// Clearing a selection keeps the side of its caret; selecting all
	// forgets it.
	b = NewBuffer("abcde fghij")
	click(1, 0, [2]int{5, 0})
	wantAt("a drag to the end of a row", Pos{0, 5}, true, 5, 0)
	r.ClearSelection()
	if sel != (Range{Anchor: Pos{0, 5}, Head: Pos{0, 5}, EndOfRow: true}) {
		t.Errorf("after ClearSelection, range %+v", sel)
	}
	r.SelectAll("v")
	if sel != (Range{Head: Pos{0, 11}}) {
		t.Errorf("after SelectAll, range %+v", sel)
	}
}

func TestViewportCaretWords(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	var b *Buffer
	var sel Range
	var mode WrapMode
	var opts []Option
	tree := func() Child { return Viewport(b).ID("v").Wrap(mode).Caret(&sel) }
	doubleClick := func(r *Root, w, h, x, y int) {
		t.Helper()
		now = now.Add(time.Second)
		selectDrag(r, w, h, tree, 0, [2]int{x, y})
		now = now.Add(-time.Second + time.Millisecond)
		selectDrag(r, w, h, tree, 0, [2]int{x, y})
	}
	newRoot := func() *Root {
		return New(append([]Option{WithClock(func() time.Time { return now })}, opts...)...)
	}

	// A word wrapped across two rows is selected whole, from either row.
	mode, b = WrapChars, NewBuffer("see abcdefghijk now")
	r := newRoot()
	for _, at := range [][2]int{{5, 0}, {1, 1}, {6, 1}} {
		doubleClick(r, 8, 4, at[0], at[1])
		wantSelection(t, r, "abcdefghijk")
	}
	// So is one wrapped at words, broken for being wider than a row.
	mode = WrapWords
	doubleClick(r, 8, 4, 2, 2)
	wantSelection(t, r, "abcdefghijk")
	// A cell past the text of a row has no word.
	mode, b = WrapNone, NewBuffer("word")
	doubleClick(r, 8, 4, 6, 0)
	wantSelection(t, r, "")

	// The buffer's word characters, or the Root's when it has none.
	b = NewBuffer("open ~/src/main.go now")
	doubleClick(r, 30, 2, 9, 0)
	wantSelection(t, r, "~/src/main.go")
	wantWord := func(from, to int) {
		t.Helper()
		if f, e, _ := b.WordAt(Pos{Col: 9}); f.Col != from || e.Col != to {
			t.Errorf("WordAt = %d to %d, want %d to %d", f.Col, e.Col, from, to)
		}
	}
	wantWord(5, 18)
	// A buffer with none of its own finds words as the Root that shows it
	// does, so ctrl and the arrows stop where a double click selects.
	opts = []Option{WithWordChars("")}
	fresh := NewBuffer("open ~/src/main.go now")
	if f, e, _ := fresh.WordAt(Pos{Col: 9}); f.Col != 5 || e.Col != 18 {
		t.Errorf("WordAt in a buffer never shown = %d to %d, want 5 to 18", f.Col, e.Col)
	}
	r = newRoot()
	doubleClick(r, 30, 2, 9, 0)
	wantSelection(t, r, "src")
	wantWord(7, 10)
	if got := b.NextWord(Pos{Col: 5}); got.Col != 10 {
		t.Errorf("NextWord = %d, want 10", got.Col)
	}
	b.SetWordChars("/")
	doubleClick(r, 30, 2, 9, 0)
	wantSelection(t, r, "/src/main.go")
	wantWord(6, 18)
	opts = nil

	// A double click selects what WordAt finds, for each cell with a
	// character, wrapped or not, through wide characters, tabs, and
	// accents; and nothing where WordAt has only a word that ends there.
	text := "snake_case, 日本語\tcafé-au-lait don't 3.14 foo.bar\n\tx_y (a) e\u0301t\u00e9"
	for _, mode = range []WrapMode{WrapNone, WrapWords, WrapChars} {
		for _, chars := range []string{"", "_", "-_"} {
			b = NewBuffer(text)
			b.SetWordChars(chars)
			r = newRoot()
			const w, h = 12, 12
			r.Render(w, h, tree())
			v, _ := r.Frame().Viewport("v")
			for y := range v.TotalRows {
				for x := range w - 1 {
					// Past the text of a row, the caret is at the end of the row.
					past := v.CaretAt(image.Pt(x, y)).EndOfRow
					p := v.PosAt(image.Pt(x, y))
					sel = Range{}
					doubleClick(r, w, h, x, y)
					from, to, ok := b.WordAt(p)
					got := Range{Anchor: sel.From(), Head: sel.To()}
					switch {
					case !sel.Empty() && past:
						t.Fatalf("mode %d, chars %q: a double click at %d, %d, past the text of its row, selected %v-%v",
							mode, chars, x, y, got.Anchor, got.Head)
					case !sel.Empty():
						if !ok || to == p || got != (Range{Anchor: from, Head: to}) {
							t.Fatalf("mode %d, chars %q: a double click at %d, %d selected %v-%v, WordAt(%v) has %v-%v (%v)",
								mode, chars, x, y, got.Anchor, got.Head, p, from, to, ok)
						}
					case ok && to != p && !past:
						// WordAt has a word the character under the pointer is in.
						t.Fatalf("mode %d, chars %q: a double click at %d, %d selected nothing, WordAt(%v) has %v-%v",
							mode, chars, x, y, p, from, to)
					}
				}
			}
		}
	}
}

func TestViewportScrollRows(t *testing.T) {
	t.Parallel()

	var sel Range
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	b := NewBuffer(strings.Join(lines, "\n"))
	view := func(at ViewportPos) ViewportView {
		return viewOf(t, 12, 10, Viewport(b).At(at).Caret(&sel))
	}
	for _, tc := range []struct {
		name    string
		at      ViewportPos
		from    Pos
		n       int
		want    Pos
		wantPos ViewportPos
	}{
		// A page down and up, mid-buffer, keeps the caret on the row of the
		// viewport it was on.
		{"down", ViewportPos{Line: 10}, Pos{13, 2}, 9, Pos{22, 2}, ViewportPos{Line: 19}},
		{"up", ViewportPos{Line: 19}, Pos{22, 2}, -9, Pos{13, 2}, ViewportPos{Line: 10}},
		// Near the top it scrolls as far as it can, and the caret moves on.
		{"near the top", ViewportPos{Line: 4}, Pos{6, 2}, -9, Pos{0, 0}, ViewportPos{}},
		{"near the top, not past it", ViewportPos{Line: 4}, Pos{12, 2}, -9, Pos{3, 2}, ViewportPos{}},
		{"at the top", ViewportPos{}, Pos{3, 2}, -9, Pos{0, 0}, ViewportPos{}},
		// Near the bottom, likewise.
		{"near the bottom", ViewportPos{Line: 27}, Pos{30, 2}, 9, Pos{39, 2}, ViewportPos{Line: 30}},
		{"past the bottom", ViewportPos{Line: 30}, Pos{35, 2}, 9, Pos{39, 7}, ViewportPos{Line: 30}},
		// A caret out of sight is brought into it.
		{"out of sight above", ViewportPos{Line: 20}, Pos{2, 2}, 9, Pos{11, 2}, ViewportPos{Line: 11}},
		{"out of sight below", ViewportPos{Line: 5}, Pos{35, 2}, -9, Pos{26, 2}, ViewportPos{Line: 17}},
	} {
		v := view(tc.at)
		from := Range{Anchor: Pos{1, 1}, Head: tc.from}
		got, pos := v.ScrollRows(from, tc.n, v.Column(from))
		wantCaret(t, tc.name, got, tc.want, false)
		if got.Anchor != (Pos{1, 1}) {
			t.Errorf("%s: the anchor moved to %v", tc.name, got.Anchor)
		}
		if pos != tc.wantPos {
			t.Errorf("%s: scrolled to %+v, want %+v", tc.name, pos, tc.wantPos)
		}
		if _, ok := view(pos).CaretCell(got); !ok {
			t.Errorf("%s: the caret is out of sight at %+v", tc.name, pos)
		}
	}

	// By wrapped rows, keeping the caret's row and its side.
	w := NewBuffer(strings.Repeat("aaaa bbbb cccc dddd\n", 10))
	v := viewOf(t, 6, 4, Viewport(w).Wrap(WrapWords).At(ViewportPos{Line: 2, Row: 1}).Caret(&sel))
	end := v.RowEnd(caretAt(2, 6))
	got, pos := v.ScrollRows(end, 3, v.Column(end))
	wantCaret(t, "wrapped", got, Pos{3, 5}, true)
	if pos != (ViewportPos{Line: 3, Row: 0}) {
		t.Errorf("wrapped: scrolled to %+v", pos)
	}
	if c, ok := viewOf(t, 6, 4, Viewport(w).Wrap(WrapWords).At(pos).Caret(&sel)).CaretCell(got); !ok || c != image.Pt(5, 0) {
		t.Errorf("wrapped: the caret is drawn at %v (%v), want on the row it was on", c, ok)
	}

	// Sideways, it is brought into sight as Show brings it.
	long := NewBuffer(strings.Repeat("x", 40) + "\n" + strings.Repeat("y", 40) + "\nz")
	v = viewOf(t, 10, 2, Viewport(long).At(ViewportPos{X: 30}).Caret(&sel))
	got, pos = v.ScrollRows(caretAt(0, 35), 2, 35)
	wantCaret(t, "sideways", got, Pos{2, 1}, false)
	if pos != (ViewportPos{Line: 1, X: 1}) {
		t.Errorf("sideways: scrolled to %+v", pos)
	}

	// An empty buffer stays where it is.
	v = viewOf(t, 10, 3, Viewport(&Buffer{}).Caret(&sel))
	if got, pos = v.ScrollRows(caretAt(2, 2), 5, 0); got.Head != (Pos{}) || pos != v.Pos {
		t.Errorf("an empty buffer: %+v, %+v", got, pos)
	}
}

// wantPos fails t when got is not want.
func wantPos(t *testing.T, what string, got, want Pos) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestViewportPosAtAndCellOf(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	for sb.Len() < 2*linebuf.LongLineBytes {
		sb.WriteString("long words 日本 é \tx ")
	}
	text := "plain line\n日本語 wide and\ttabbed\ncombining é marks à́ here\n" + sb.String() + "\n\x1b[31mstyled\x1b[m text"
	for _, mode := range []WrapMode{WrapNone, WrapWords, WrapChars} {
		for _, gutter := range []bool{false, true} {
			b := NewBuffer(text)
			var sel Range
			pos := ViewportPos{}
			if mode == WrapNone {
				pos.X = 2
			}
			n := Viewport(b).ID("v").Wrap(mode).At(pos).Caret(&sel)
			if gutter {
				n = n.LineNumbers(lipgloss.NewStyle())
			}
			now := time.Unix(1, 0)
			r := New(WithClock(func() time.Time { return now }))
			const w, h = 13, 12
			r.Render(w, h, n)
			v, _ := r.Frame().Viewport("v")
			for y := range h {
				for x := range w {
					p := v.PosAt(image.Pt(x, y))
					// A cell's position is drawn where the mouse puts a caret at it.
					now = now.Add(time.Second)
					selectDrag(r, w, h, func() Child { return n }, 0, [2]int{x, y})
					if sel.Head != p {
						t.Fatalf("mode %d: a click at %d, %d put the caret at %v, PosAt has %v", mode, x, y, sel.Head, p)
					}
					// It is drawn back at a cell that has it.
					c, ok := v.CellOf(p)
					if !ok {
						// Only a wrap point past the last row is out of sight, or in a
						// gutter, a column scrolled out of sight to the left.
						if y < h-1 && c.X >= v.gutter {
							t.Fatalf("mode %d: %v from %d, %d is out of sight at %v", mode, p, x, y, c)
						}
						continue
					}
					if got := v.PosAt(c); got != p {
						t.Fatalf("mode %d: %v from %d, %d is drawn at %v, which is %v", mode, p, x, y, c, got)
					}
				}
			}
		}
	}
}

// viewOf returns how viewport n, with ID "v", was laid out in a w×h frame.
func viewOf(t *testing.T, w, h int, n ViewportNode) ViewportView {
	t.Helper()
	v, ok := Solve(w, h, n.ID("v")).Viewport("v")
	if !ok {
		t.Fatal("no viewport in the frame")
	}
	return v
}

// wantCaret fails t when got is not the caret at want, at the end of its
// row with eor.
func wantCaret(t *testing.T, what string, got Range, want Pos, eor bool) {
	t.Helper()
	if got.Head != want || got.EndOfRow != eor {
		t.Errorf("%s = %v (end of row %v), want %v (%v)", what, got.Head, got.EndOfRow, want, eor)
	}
}

func TestViewportRowEnds(t *testing.T) {
	t.Parallel()

	var sel Range
	for _, tc := range []struct {
		name       string
		text       string
		mode       WrapMode
		at         Pos
		start, end Pos
		eor        bool // the end is where the line wraps, drawn on the row
	}{
		// A row wrapped at words ends after the spaces that hang off it,
		// where the next row starts.
		{"hanging", "aaaa   bbbb", WrapWords, Pos{0, 1}, Pos{0, 0}, Pos{0, 7}, true},
		// One whose space fits, after the space.
		{"spaced", "aaaa bbbb cccc", WrapWords, Pos{0, 2}, Pos{0, 0}, Pos{0, 5}, true},
		// The wrap point is on the row after.
		{"wrap point", "aaaa bbbb cccc", WrapWords, Pos{0, 5}, Pos{0, 5}, Pos{0, 10}, true},
		{"last row", "aaaa bbbb cccc", WrapWords, Pos{0, 12}, Pos{0, 10}, Pos{0, 14}, false},
		// One broken inside a word, after its last character.
		{"broken word", "abcdefghij", WrapWords, Pos{0, 1}, Pos{0, 0}, Pos{0, 5}, true},
		{"chars", "abcdefghijklm", WrapChars, Pos{0, 5}, Pos{0, 5}, Pos{0, 10}, true},
		{"chars wrap point", "abcdefghijklm", WrapChars, Pos{0, 10}, Pos{0, 10}, Pos{0, 13}, false},
		// A wide character that does not fit goes to the next row.
		{"wide", "日本語日本語", WrapChars, Pos{0, 0}, Pos{0, 0}, Pos{0, 6}, true},
		// A tab near the end of a row, and one the row ends inside, which
		// has no place there: the row ends before it.
		{"tab", "ab\tcdefgh", WrapChars, Pos{0, 1}, Pos{0, 0}, Pos{0, 4}, true},
		{"split tab", "abcd\tefgh", WrapChars, Pos{0, 1}, Pos{0, 0}, Pos{0, 4}, false},
		{"unwrapped", "abcdefghij", WrapNone, Pos{0, 5}, Pos{0, 0}, Pos{0, 10}, false},
		{"second line", "ab\ncd", WrapNone, Pos{1, 1}, Pos{1, 0}, Pos{1, 2}, false},
	} {
		v := viewOf(t, 6, 5, Viewport(NewBuffer(tc.text)).Wrap(tc.mode).Caret(&sel))
		at := caretAt(tc.at.Line, tc.at.Col)
		wantCaret(t, tc.name+": RowStart", v.RowStart(at), tc.start, false)
		end := v.RowEnd(at)
		wantCaret(t, tc.name+": RowEnd", end, tc.end, tc.eor)
		// The end of a row is drawn on it, past its text, and in sight.
		start, _ := v.CaretCell(caretAt(tc.start.Line, tc.start.Col))
		if c, ok := v.CaretCell(end); !ok && tc.mode != WrapNone || c.Y != start.Y {
			t.Errorf("%s: RowEnd %+v drawn at %v (%v), off the row of %v", tc.name, end, c, ok, tc.start)
		}
		// Home from there goes back to the start of the same row, and End
		// again stays.
		wantCaret(t, tc.name+": RowStart after RowEnd", v.RowStart(end), tc.start, false)
		wantCaret(t, tc.name+": RowEnd after RowEnd", v.RowEnd(end), tc.end, tc.eor)
		// It keeps the anchor.
		if got := v.RowEnd(Range{Anchor: Pos{0, 1}, Head: tc.at}); got.Anchor != (Pos{0, 1}) {
			t.Errorf("%s: RowEnd moved the anchor to %v", tc.name, got.Anchor)
		}
	}
	// Spaces hanging off a row are drawn at the column past its text, as
	// the end of the row is, and the caret's column is the last.
	v := viewOf(t, 6, 5, Viewport(NewBuffer("aaaa   bbbb")).Wrap(WrapWords).Caret(&sel))
	if c, ok := v.CellOf(Pos{0, 6}); !ok || c != image.Pt(5, 0) || v.Column(caretAt(0, 6)) != 5 {
		t.Errorf("a hanging space is drawn at %v (%v), column %d", c, ok, v.Column(caretAt(0, 6)))
	}
	end := v.RowEnd(caretAt(0, 1))
	if c, ok := v.CaretCell(end); !ok || c != image.Pt(5, 0) || v.Column(end) != 5 {
		t.Errorf("the end of a row with spaces hanging off it is drawn at %v (%v), column %d", c, ok, v.Column(end))
	}
	// The same position without EndOfRow starts the next row.
	if c, ok := v.CaretCell(caretAt(0, 7)); !ok || c != image.Pt(0, 1) || v.Column(caretAt(0, 7)) != 0 {
		t.Errorf("the wrap point is drawn at %v (%v)", c, ok)
	}
	// Where a row is shorter than the viewport, past its last cell.
	v = viewOf(t, 6, 5, Viewport(NewBuffer("日本語日本語")).Wrap(WrapChars).Caret(&sel))
	end = v.RowEnd(caretAt(0, 0))
	if c, ok := v.CaretCell(end); !ok || c != image.Pt(4, 0) || v.Column(end) != 4 {
		t.Errorf("the end of a row of wide characters is drawn at %v (%v), column %d", c, ok, v.Column(end))
	}
	// EndOfRow means nothing where the line does not wrap.
	v = viewOf(t, 6, 5, Viewport(NewBuffer("abc\ndef")).Caret(&sel))
	odd := Range{Anchor: Pos{1, 0}, Head: Pos{1, 0}, EndOfRow: true}
	if c, ok := v.CaretCell(odd); !ok || c != image.Pt(0, 1) || v.Column(odd) != 0 {
		t.Errorf("a caret with EndOfRow at the start of a line is drawn at %v (%v)", c, ok)
	}
	wantCaret(t, "RowEnd with EndOfRow at the start of a line", v.RowEnd(odd), Pos{1, 3}, false)
}

func TestViewportMoveRows(t *testing.T) {
	t.Parallel()

	var sel Range
	for _, tc := range []struct {
		name   string
		text   string
		mode   WrapMode
		w      int
		from   Pos
		n, col int
		want   Pos
		eor    bool
	}{
		// A row too short for the column keeps it for the row after.
		{"short row", "long line here\nab\nanother long", WrapNone, 20, Pos{0, 10}, 1, 10, Pos{1, 2}, false},
		{"past a short row", "long line here\nab\nanother long", WrapNone, 20, Pos{1, 2}, 1, 10, Pos{2, 10}, false},
		// Above the first row is the start of the text, and below the last
		// its end.
		{"first row", "long line here\nab", WrapNone, 20, Pos{0, 10}, -1, 10, Pos{0, 0}, false},
		{"last row", "long line here\nab", WrapNone, 20, Pos{1, 1}, 3, 1, Pos{1, 2}, false},
		{"far down", "a\nb\nc\nd", WrapNone, 20, Pos{0, 0}, 99, 0, Pos{3, 1}, false},
		{"far up", "a\nb\nc\nd", WrapNone, 20, Pos{3, 1}, -99, 1, Pos{0, 0}, false},
		{"first wrapped row", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 2}, -1, 2, Pos{0, 0}, false},
		{"last wrapped row", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 11}, 1, 1, Pos{0, 14}, false},
		// The nearest character boundary, never inside a wide character or a
		// tab, nor a letter and its accent.
		{"wide, left", "x\na日b", WrapNone, 20, Pos{0, 0}, 1, 2, Pos{1, 1}, false},
		{"wide, right", "x\na日bc", WrapNone, 20, Pos{0, 0}, 1, 3, Pos{1, 4}, false},
		{"tab, nearer its end", "x\n\tx", WrapNone, 20, Pos{0, 0}, 1, 3, Pos{1, 1}, false},
		{"tab, nearer its start", "x\n\tx", WrapNone, 20, Pos{0, 0}, 1, 1, Pos{1, 0}, false},
		{"tab, between", "x\n\tx", WrapNone, 20, Pos{0, 0}, 1, 2, Pos{1, 0}, false},
		{"accent", "x\ne\u0301x", WrapNone, 20, Pos{0, 0}, 1, 1, Pos{1, 3}, false},
		// By wrapped rows.
		{"wrapped down", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 1}, 1, 1, Pos{0, 6}, false},
		{"wrapped down two", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 1}, 2, 1, Pos{0, 11}, false},
		{"wrapped up", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 12}, -1, 2, Pos{0, 7}, false},
		{"wrapped short", "aaaa bbbb cc", WrapWords, 6, Pos{0, 8}, 1, 3, Pos{0, 12}, false},
		{"into the next line", "aaaa bbbb\nxy", WrapWords, 6, Pos{0, 6}, 1, 1, Pos{1, 1}, false},
		// At or past the cells of a row the line wraps after, its end, drawn
		// on the row.
		{"onto a wrapped row", "wxyz\n0123456789", WrapChars, 5, Pos{0, 4}, 1, 4, Pos{1, 4}, true},
		{"chars down", "0123456789", WrapChars, 5, Pos{0, 2}, 1, 2, Pos{0, 6}, false},
		{"wrapped, past the row", "aaaa bbbb cccc", WrapWords, 6, Pos{0, 12}, -1, 9, Pos{0, 10}, true},
		{"hanging spaces", "aaaa   bbbb\nxxxxx", WrapWords, 6, Pos{1, 5}, -2, 5, Pos{0, 7}, true},
		// Column 0 is the start of a row, not the end of the one before.
		{"row start", "abcdefghij", WrapChars, 6, Pos{0, 2}, 1, 0, Pos{0, 5}, false},
		// A wide character and a tab near the wrap: never inside either, and
		// a column past the middle of the last character goes to the end.
		{"wide ending a row", "x\nab日cd", WrapChars, 5, Pos{0, 0}, 1, 3, Pos{1, 2}, false},
		{"wide past a row", "x\nabc日d", WrapChars, 5, Pos{0, 0}, 1, 3, Pos{1, 3}, true},
		{"wide past a row, before", "x\nabc日d", WrapChars, 5, Pos{0, 0}, 1, 2, Pos{1, 2}, false},
		{"tab ending a row, nearer its end", "x\nabcd\tefgh", WrapChars, 9, Pos{0, 0}, 1, 7, Pos{1, 5}, true},
		{"tab ending a row, between", "x\nabcd\tefgh", WrapChars, 9, Pos{0, 0}, 1, 6, Pos{1, 4}, false},
		{"tab split by the wrap", "x\nabcd\tefgh", WrapChars, 6, Pos{0, 0}, 1, 5, Pos{1, 4}, false},
	} {
		v := viewOf(t, tc.w, 3, Viewport(NewBuffer(tc.text)).Wrap(tc.mode).Caret(&sel))
		got := v.MoveRows(Range{Anchor: Pos{0, 1}, Head: tc.from}, tc.n, tc.col)
		wantCaret(t, fmt.Sprintf("%s: MoveRows(%v, %d, %d)", tc.name, tc.from, tc.n, tc.col), got, tc.want, tc.eor)
		if got.Anchor != (Pos{0, 1}) {
			t.Errorf("%s: MoveRows moved the anchor to %v", tc.name, got.Anchor)
		}
	}

	// The column of a caret is kept while moving up and down: through a
	// short row and back.
	v := viewOf(t, 20, 3, Viewport(NewBuffer("long line here\nab\nanother long")).Caret(&sel))
	p := caretAt(0, 10)
	col := v.Column(p)
	p = v.MoveRows(v.MoveRows(p, 1, col), 1, col)
	wantCaret(t, "through a short row", p, Pos{2, 10}, false)
	if col != 10 || v.Column(caretAt(2, 3)) != 3 {
		t.Errorf("Column = %d", col)
	}
	v = viewOf(t, 20, 3, Viewport(NewBuffer("日本\tx")).Caret(&sel))
	if got := v.Column(caretAt(0, 7)); got != 8 {
		t.Errorf("Column after a wide character and a tab = %d", got)
	}

	// End on a wrapped row, then down and up again, keeps to the ends of the
	// rows, each drawn on its own row.
	v = viewOf(t, 6, 5, Viewport(NewBuffer("aaaa bbbb cccc dd")).Wrap(WrapWords).Caret(&sel))
	end := v.RowEnd(caretAt(0, 1))
	wantCaret(t, "End", end, Pos{0, 5}, true)
	col = v.Column(end)
	down := v.MoveRows(end, 1, col)
	wantCaret(t, "End, then down", down, Pos{0, 10}, true)
	wantCaret(t, "End, then down, then up", v.MoveRows(down, -1, col), Pos{0, 5}, true)
	// Onto a short last row, its end.
	wantCaret(t, "End, then down to a short row", v.MoveRows(down, 2, col), Pos{0, 17}, false)
	// The row the caret is on is found with its side: up from the end of
	// the first row is the start of the text, not the first row.
	wantCaret(t, "up from the end of the first row", v.MoveRows(end, -1, col), Pos{0, 0}, false)
	if c, _ := v.CaretCell(end); c != image.Pt(5, 0) || col != 5 {
		t.Errorf("End is drawn at %v, column %d", c, col)
	}

	// A long line, by its rows.
	long := strings.Repeat("abcdefghi", 2*linebuf.LongLineBytes/9)
	v = viewOf(t, 10, 3, Viewport(NewBuffer(long+"\nend")).Wrap(WrapChars).Caret(&sel))
	wantCaret(t, "down a long line", v.MoveRows(caretAt(0, 5), 1000, 5), Pos{0, 9005}, false)
	wantCaret(t, "up a long line", v.MoveRows(caretAt(0, 9005), -999, 5), Pos{0, 14}, false)
	wantCaret(t, "row start in a long line", v.RowStart(caretAt(0, 9005)), Pos{0, 9000}, false)
	wantCaret(t, "row end in a long line", v.RowEnd(caretAt(0, 9005)), Pos{0, 9009}, true)
	wantCaret(t, "off the end of a long line", v.MoveRows(caretAt(0, 5), len(long)/9, 2), Pos{1, 2}, false)
	if c, ok := v.CellOf(Pos{0, 9005}); ok || c != image.Pt(5, 1000) {
		t.Errorf("CellOf in a long line = %v (%v)", c, ok)
	}
	wantPos(t, "PosAt in a long line", viewOf(t, 10, 3, Viewport(NewBuffer(long)).Wrap(WrapChars).At(ViewportPos{Row: 1000}).Caret(&sel)).PosAt(image.Pt(5, 0)), Pos{0, 9005})

	// An empty buffer has its start alone.
	v = viewOf(t, 10, 3, Viewport(&Buffer{}).Caret(&sel))
	if v.MoveRows(caretAt(3, 3), 1, 4) != (Range{Anchor: Pos{3, 3}}) || v.RowEnd(Range{}) != (Range{}) || v.RowStart(Range{}) != (Range{}) ||
		v.PosAt(image.Pt(4, 4)) != (Pos{}) || v.CaretAt(image.Pt(4, 4)) != (Range{}) {
		t.Error("an empty buffer has a position other than its start")
	}
}

func TestViewportShow(t *testing.T) {
	t.Parallel()

	var sel Range
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d of the buffer", i)
	}
	b := NewBuffer(strings.Join(lines, "\n"))
	v := viewOf(t, 10, 5, Viewport(b).At(ViewportPos{Line: 10}).Caret(&sel))
	for _, tc := range []struct {
		at   Pos
		want ViewportPos
	}{
		{Pos{12, 0}, ViewportPos{Line: 10}},        // in sight
		{Pos{2, 0}, ViewportPos{Line: 2}},          // above: to the top
		{Pos{17, 0}, ViewportPos{Line: 13}},        // below: to the bottom
		{Pos{14, 0}, ViewportPos{Line: 10}},        // the last row in sight
		{Pos{15, 0}, ViewportPos{Line: 11}},        // just below
		{Pos{12, 12}, ViewportPos{Line: 10, X: 3}}, // to the right, in the caret's column
		{Pos{16, 20}, ViewportPos{Line: 12, X: 11}},
	} {
		if got := v.Show(caretAt(tc.at.Line, tc.at.Col)); got != tc.want {
			t.Errorf("Show(%v) = %+v, want %+v", tc.at, got, tc.want)
		}
	}
	// Back to the left.
	v = viewOf(t, 10, 5, Viewport(b).At(ViewportPos{Line: 10, X: 8}).Caret(&sel))
	if got := v.Show(caretAt(11, 3)); got != (ViewportPos{Line: 10, X: 3}) {
		t.Errorf("Show to the left = %+v", got)
	}
	// What Show gives has the position in sight.
	for _, p := range []Pos{{0, 0}, {19, 21}, {7, 15}, {11, 2}} {
		at := v.Show(caretAt(p.Line, p.Col))
		w := viewOf(t, 10, 5, Viewport(b).At(at).Caret(&sel))
		if _, ok := w.CellOf(p); !ok {
			t.Errorf("%v is out of sight at %+v", p, at)
		}
	}

	// Wrapped, it scrolls by rows and never sideways.
	v = viewOf(t, 8, 3, Viewport(b).Wrap(WrapWords).Caret(&sel))
	if got := v.Show(caretAt(1, 18)); got != (ViewportPos{Line: 1}) {
		t.Errorf("Show wrapped = %+v", got)
	}
	if got := v.Show(caretAt(1, 7)); got != (ViewportPos{Line: 0, Row: 2}) {
		t.Errorf("Show wrapped, to a row inside a line = %+v", got)
	}
	if got := v.Show(caretAt(0, 20)); got != (ViewportPos{}) {
		t.Errorf("Show wrapped, in sight = %+v", got)
	}
	// A view from before an edit shows the buffer as it is.
	v = viewOf(t, 10, 5, Viewport(b).Caret(&sel))
	b.Insert(Pos{0, 0}, "one\ntwo\nthree\n")
	if got := v.Show(caretAt(6, 0)); got != (ViewportPos{Line: 2}) {
		t.Errorf("Show after an edit = %+v", got)
	}
}
