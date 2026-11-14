// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// newStore returns a store holding the lines of s.
func newStore(s string) *Store {
	b := &Store{}
	if s != "" {
		b.AppendText(s)
	}
	return b
}

// appendAll appends each of lines to b.
func appendAll(b *Store, lines ...string) {
	for _, l := range lines {
		b.AppendText(l)
	}
}

func TestStoreCarriesStyles(t *testing.T) {
	t.Parallel()

	b := newStore("\x1b[31mred\nstill red\x1b[0m\nplain\n\x1b]8;;https://x.dev\x1b\\link\nstill link\x1b]8;;\x1b\\ done")
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
}

func TestWrapLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		s    string
		w    int
		mode Mode
		want []int
	}{
		{"two is a longer line", 14, Words, []int{0, 9}},
		{"two is a longer line", 14, Chars, []int{0, 14}},
		{"abcdefghij", 4, Words, []int{0, 4, 8}},
		{"ab abcdefghij", 4, Words, []int{0, 3, 7, 11}},
		// Spaces hang past the width rather than starting a row.
		{"abcd     efgh", 4, Words, []int{0, 9}},
		{"  indented words", 10, Words, []int{0, 11}},
		// A wide glyph that would cross the width starts the next row.
		{"日本語", 5, Chars, []int{0, 4}},
		{"日本語", 1, Chars, []int{0, 2, 4}},
		{"a\tb", 3, Chars, []int{0, 3}},
		{"\x1b[1mbold\x1b[m words", 5, Words, []int{0, 5}},
		// After a hyphen, as Lip Gloss wraps text.
		{"hello-world", 8, Words, []int{0, 6}},
		{"a-b-c-d-e-f", 4, Words, []int{0, 4, 8}},
		{"self-contained text here", 10, Words, []int{0, 5, 15}},
		{"--flag-name value", 6, Words, []int{0, 2, 7, 12}},
		{"a-\x1b[1mbcdef", 4, Words, []int{0, 2, 6}},
		{"hello-world", 8, Chars, []int{0, 8}},
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
	var grown Store
	var all []string
	for round := range 30 {
		var add []string
		for range rng.IntN(20) {
			add = append(add, line())
		}
		appendAll(&grown, add...)
		all = append(all, add...)
		if round == 15 {
			grown.Clear()
			appendAll(&grown, all...)
		}
		for _, mode := range []Mode{None, Words, Chars} {
			for _, w := range []int{1, 3, 7, 16, 200} {
				fresh := newStore(strings.Join(all, "\n"))
				if len(all) > 0 && all[len(all)-1] == "" {
					// AppendText would end the text before its last empty line.
					fresh.AppendText("")
				}
				a := grown.Index(ansi.GraphemeWidth, mode, w)
				b := fresh.Index(ansi.GraphemeWidth, mode, w)
				if a.n != b.n || a.Total() != b.Total() {
					t.Fatalf("round %d mode %d width %d: grown %d lines %d rows, fresh %d lines %d rows",
						round, mode, w, a.n, a.Total(), b.n, b.Total())
				}
				for i := range a.n {
					if a.RowsOf(i) != b.RowsOf(i) {
						t.Fatalf("round %d mode %d width %d line %d: grown %d rows, fresh %d", round, mode, w, i, a.RowsOf(i), b.RowsOf(i))
					}
					want, _ := wrapLine(all[i], ansi.GraphemeWidth, w, mode, nil)
					if mode == None {
						want = 1
					}
					if a.RowsOf(i) != want {
						t.Fatalf("round %d mode %d width %d line %d %q: %d rows, wrapLine says %d", round, mode, w, i, all[i], a.RowsOf(i), want)
					}
				}
			}
		}
	}
}

func TestCellsMatchParse(t *testing.T) {
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
			want := cutRow(line, checkpoint{pen: start}, 0, ansi.GraphemeWidth, a, b, &RowStyle{Marks: []Overlay{{Start: 1 << 20, End: 1 << 21, SGR: "\x1b[0m"}}})
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
		for _, mode := range []Mode{Words, Chars} {
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
		if _, ok := asciiWrap(s, 4, Words, nil); ok {
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
	var b Store
	appendAll(&b, lines...)
	m := b.Measure(ansi.GraphemeWidth)
	widths := make([]int32, len(lines))
	measureLines(lines, ansi.GraphemeWidth, widths)
	if !slices.Equal(m.widths, widths) {
		t.Fatal("widths measured in parallel differ")
	}
	for _, mode := range []Mode{Words, Chars} {
		x := b.Index(ansi.GraphemeWidth, mode, 13)
		rows := make([]int32, len(lines))
		countRowsOf(lines, widths, ansi.GraphemeWidth, 13, mode, rows)
		for i, r := range rows {
			if x.RowsOf(i) != int(r) {
				t.Fatalf("mode %d line %d: %d rows in parallel, %d on one", mode, i, x.RowsOf(i), r)
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
	for sb.Len() < 6*LongLineBytes {
		sb.WriteString(parts[rng.IntN(len(parts))])
	}
	// The long line starts in the style the line before it left open.
	b := newStore("\x1b[32mopen\n" + sb.String() + "\nafter")
	s, start := b.Line(1), b.penOf(1)
	m := b.Measure(ansi.GraphemeWidth)
	width, flags := m.Width(1), m.widths[1]
	// Checkpoints are made only as far along the line as they are wanted.
	l := b.longLine(1, ansi.GraphemeWidth)
	b.cutFrom(1, ansi.GraphemeWidth, 3000)
	if len(l.cps) > 4 || l.done {
		t.Fatalf("%d checkpoints to reach cell 3000", len(l.cps))
	}
	b.cutFrom(1, ansi.GraphemeWidth, width)
	if len(l.cps) < width/CheckpointEvery {
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
		got := b.RowStarts(1, ansi.GraphemeWidth, w, Words, &scratch)
		_, want := wrapLine(s, ansi.GraphemeWidth, w, Words, nil)
		if !slices.Equal(got, want) {
			t.Fatalf("rows at %d kept as %d starts, want %d", w, len(got), len(want))
		}
		// A short line worked out after does not change what is kept.
		b.RowStarts(2, ansi.GraphemeWidth, w, Words, &scratch)
		if !slices.Equal(got, want) {
			t.Fatal("working out a short line changed the long line's rows")
		}
	}
	b.Clear()
	if b.long != [4]*longLine{} {
		t.Fatal("Clear kept long lines")
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

// TestSGRsKeepsColorKinds checks that the escape sequences kept for
// styles tell a color of the terminal's palette from the RGB it stands for.
func TestSGRsKeepsColorKinds(t *testing.T) {
	t.Parallel()

	var c SGRs
	palette := uv.Style{Fg: ansi.Red}
	r, g, b, _ := ansi.Red.RGBA()
	rgb := uv.Style{Fg: color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}}
	if !palette.Equal(&rgb) {
		t.Skip("the colors are told apart by ultraviolet already")
	}
	if a, b := c.Of(&palette), c.Of(&rgb); a == b {
		t.Fatalf("both drawn with %q", a)
	}
}

// TestSGRsIncomparableColors checks that styles with colors that cannot
// be compared are drawn, rather than panicking when looked up.
func TestSGRsIncomparableColors(t *testing.T) {
	t.Parallel()

	var c SGRs
	s := uv.Style{Fg: wrappedColor{sliceColor{0xff}}}
	o := uv.Style{Fg: wrappedColor{sliceColor{0xff}}}
	if c.Of(&s) == "" || c.Of(&o) == "" {
		t.Fatal("no escape sequence for a color that cannot be compared")
	}
}

// wrappedColor is a color that can be compared, unless it holds one that
// cannot, such as a [sliceColor].
type wrappedColor struct{ color.Color }

// sliceColor is a valid [color.Color] that cannot be compared with ==.
type sliceColor []uint8

func (c sliceColor) RGBA() (r, g, b, a uint32) {
	v := uint32(c[0]) * 0x101
	return v, v, v, 0xffff
}
