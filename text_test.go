// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTextTruncate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, text string
		width      int
		style      lipgloss.Style
		want       string
	}{
		{"plain", "abcdef", 4, lipgloss.NewStyle(), "abc…"},
		{"wide", "世界你好", 4, lipgloss.NewStyle(), "世… "},
		{"grapheme", "e\u0301e\u0301e\u0301", 2, lipgloss.NewStyle(), "e\u0301…"},
		{"newlines", "a\r\nb\nc", 5, lipgloss.NewStyle(), "a b c"},
		{"padding", "abcdef", 6, lipgloss.NewStyle().Padding(0, 1), " abc… "},
		{"border", "abcdef", 6, lipgloss.NewStyle().BorderLeft(true).BorderRight(true).BorderStyle(lipgloss.NormalBorder()), "│abc…│"},
		{"style width", "abcdef", 6, lipgloss.NewStyle().Width(3), "ab…   "},
		{"one cell", "世界", 1, lipgloss.NewStyle(), "…"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := Text(tt.text).Style(tt.style).Truncate()
			got := ansi.Strip(Render(tt.width, 1, n))
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if marquee := Render(tt.width, 1, Marquee(tt.text, 0).Style(tt.style)); Render(tt.width, 1, n) != marquee {
				t.Fatalf("differs from marquee: %q", marquee)
			}
		})
	}
	if got := Render(0, 0, Text("abc\ndef").Truncate()); got != "abc def" {
		t.Fatalf("natural size: %q", got)
	}
	if got := ansi.Strip(Render(3, 0, Column(Text("abcdef").Truncate(), Text("end")))); got != "ab…\nend" {
		t.Fatalf("natural height: %q", got)
	}
}

func TestTruncateKeepsStyleLimits(t *testing.T) {
	t.Parallel()
	tall := Text("x").Style(lipgloss.NewStyle().Height(3)).Truncate()
	expectRect(t, Solve(0, 0, Column(tall, Text("y").ID("y"))), "y", image.Rect(0, 3, 1, 4))

	// A transform that lengthens the text or adds lines is cut to one line too.
	for _, tf := range []func(string) string{
		func(s string) string { return strings.Repeat(s, 4) },
		func(s string) string { return s + "\n" + s },
	} {
		grown := Text("abc").Style(lipgloss.NewStyle().Transform(tf)).Truncate()
		if got := Render(4, 0, Column(grown, Text("end"))); got != "abc…\nend " {
			t.Fatalf("transform: %q", got)
		}
	}

	narrow := Text("abcdef").Style(lipgloss.NewStyle().MaxWidth(3)).Truncate()
	if got := Render(0, 0, narrow); got != "ab…" {
		t.Fatalf("MaxWidth: %q", got)
	}
	if got := Render(0, 0, Row(narrow, Text("|"))); got != "ab…|" {
		t.Fatalf("MaxWidth measured: %q", got)
	}
}

// TestTruncateMeasuresAsDrawn checks that a truncated line's measured size,
// worked out from its style, is the size it is drawn at.
func TestTruncateMeasuresAsDrawn(t *testing.T) {
	t.Parallel()
	base := lipgloss.NewStyle()
	styles := map[string]lipgloss.Style{
		"plain":     base.Bold(true),
		"padding":   base.Padding(1, 2),
		"border":    base.Border(lipgloss.RoundedBorder()),
		"margins":   base.Margin(1, 3),
		"width":     base.Width(6).Padding(0, 1),
		"height":    base.Height(3).Border(lipgloss.NormalBorder()),
		"max width": base.MaxWidth(5).Padding(0, 1),
		"max both":  base.MaxWidth(7).MaxHeight(2).Border(lipgloss.NormalBorder()),
	}
	for name, st := range styles {
		for _, s := range []string{"ab", "a much longer line of text", "wide 字字字 glyphs"} {
			// Widths too narrow for a style's own frame are clipped instead.
			for _, avail := range []int{0, 9, 14, 80} {
				n := Text(s).Style(st).Truncate()
				f := Solve(avail, 0, Row(Auto(n).ID("n")))
				r, _ := f.Rect("n")
				got, _ := textOf(n, r.Dx(), 0, ansi.GraphemeWidth)
				if w, h := lipgloss.Size(got); w != r.Dx() || h != r.Dy() {
					t.Errorf("%s, %q in %d: measured %dx%d, drawn %dx%d:\n%s", name, s, avail, r.Dx(), r.Dy(), w, h, got)
				}
			}
		}
	}
}

func TestMarquee(t *testing.T) {
	t.Parallel()

	// A loop of the text is 13 cells with the default gap, 10 without. at
	// returns the phase for a cell of it, in the middle of the cell so that
	// rounding cannot move it.
	const text = "0123456789"
	at := func(cell, period float64) float64 { return (cell + 0.5) / period }
	tests := []struct {
		name  string
		node  MarqueeNode
		width int
		want  string
	}{
		{"fits", Marquee("abc", 0), 10, "abc"},
		{"fits scrolling", Marquee("abc", 0.5), 10, "abc"},
		{"fits exactly", Marquee(text, 0.5), 10, text},
		{"truncated at phase 0", Marquee(text, 0), 6, "01234…"},
		{"newlines are spaces", Marquee("ab\ncd\r\nef", 0), 20, "ab cd ef"},
		{"start of the loop", Marquee(text, 1), 6, "012345"},
		{"wraps to the start", Marquee(text, 2), 6, "012345"},
		{"a few cells in", Marquee(text, at(3, 13)), 6, "345678"},
		{"into the gap", Marquee(text, at(8, 13)), 6, "89   0"},
		{"in the gap", Marquee(text, at(11, 13)), 6, "  0123"},
		{"back around", Marquee(text, at(12, 13)), 6, " 01234"},
		{"negative phase wraps", Marquee(text, at(12, 13)-1), 6, " 01234"},
		{"custom gap", Marquee(text, at(10, 13)).Gap(" | "), 6, " | 012"},
		{"no gap", Marquee(text, at(7, 10)).Gap(""), 6, "789012"},
		{"nan", Marquee(text, math.NaN()), 6, "01234…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(Render(tt.width, 1, tt.node))
			if want := tt.want + strings.Repeat(" ", tt.width-ansi.StringWidth(tt.want)); got != want {
				t.Fatalf("rendered %q, want %q", got, want)
			}
		})
	}

	if got := Marquee(text, 0.5).Layout(Context{}); got != nil {
		t.Fatalf("laid out %v with no room", got)
	}
}

func TestMarqueeNaturalSize(t *testing.T) {
	t.Parallel()

	n := Marquee("hello world", 0.3)
	if got := Render(0, 0, n); ansi.Strip(got) != "hello world" {
		t.Fatalf("natural render = %q", got)
	}
	if v := Render(0, 0, n); v != Render(0, 0, n) {
		t.Fatalf("View = %q", v)
	}

	// It shrinks in a slot, instead of pushing its neighbours out.
	got := ansi.Strip(Render(20, 1, Row(Fixed(5, Text("ab")), Fill(Marquee("hello world", 0)), Fixed(1, Text("|")))))
	if want := "ab   hello world   |"; got != want {
		t.Fatalf("in a row: %q, want %q", got, want)
	}
	got = ansi.Strip(Render(12, 1, Row(Fixed(5, Text("ab")), Fill(Marquee("hello world", 0)), Fixed(1, Text("|")))))
	if got != "ab   hello…|" {
		t.Fatalf("shrunk in a row: %q", got)
	}
}

func TestMarqueeWide(t *testing.T) {
	t.Parallel()

	// Each glyph is two cells wide: a loop is 10+3 cells. Whichever cell the
	// cut lands on, the line is as wide as asked, and a glyph is never half
	// shown.
	const text = "世界你好啊"
	n := 13
	for i := range n {
		for _, width := range []int{4, 5, 6, 7} {
			phase := (float64(i) + 0.5) / float64(n)
			got := ansi.Strip(Render(width, 1, Marquee(text, phase)))
			if w := ansi.StringWidth(got); w != width {
				t.Fatalf("phase %d/%d width %d: %q is %d cells", i, n, width, got, w)
			}
			for _, r := range got {
				if r != ' ' && !strings.ContainsRune(text, r) {
					t.Fatalf("phase %d/%d width %d: %q holds %q", i, n, width, got, r)
				}
			}
		}
	}

	if got := ansi.Strip(Render(5, 1, Marquee(text, 0))); got != "世界…" {
		t.Fatalf("truncated at a wide glyph: %q", got)
	}
	// The cut lands in the middle of the second glyph, at cell 3.
	if got := ansi.Strip(Render(5, 1, Marquee(text, 3.5/13))); got != " 你好" {
		t.Fatalf("scrolled half a glyph: %q", got)
	}
	if got := ansi.Strip(Render(5, 1, Marquee(text, 4.5/13))); got != "你好 " {
		t.Fatalf("scrolled whole glyphs: %q", got)
	}
}

func TestTextWidthMethod(t *testing.T) {
	t.Parallel()

	// "❤️" is one cell to WcWidth and two to GraphemeWidth, so measuring
	// with the wrong one cuts the text early and pads it with blanks.
	const text = "❤️❤️❤️abc"
	for _, tt := range []struct {
		node Child
		want string
	}{
		{Text(text).Truncate(), "❤️❤️❤️a…|"},
		{Marquee(text, 0), "❤️❤️❤️a…|"},
		{Marquee(text, 2.5/9), "❤️abc |"},
	} {
		got := ansi.Strip(newRenderer(ansi.WcWidth).render(6, 1, Row(Fill(tt.node), Text("|"))))
		if got != tt.want {
			t.Errorf("%T: %q, want %q", tt.node, got, tt.want)
		}
	}
}

func TestMarqueeStyle(t *testing.T) {
	t.Parallel()

	st := lipgloss.NewStyle().Padding(0, 1).Foreground(color.RGBA{R: 255, A: 255})
	tests := []struct {
		phase float64
		want  string
	}{
		{0, " 01234… "},
		{3.5 / 13, " 345678 "},
		{0.5, " 6789   "},
	}
	for _, tt := range tests {
		got := Render(8, 1, Marquee("0123456789", tt.phase).Style(st))
		if ansi.Strip(got) != tt.want {
			t.Errorf("phase %v: %q, want %q", tt.phase, ansi.Strip(got), tt.want)
		}
		if !strings.Contains(got, "\x1b[") {
			t.Errorf("phase %v: %q is not styled", tt.phase, got)
		}
	}

	// Padding is part of the natural width, and counts against a fit.
	if got := ansi.Strip(Render(0, 0, Marquee("abc", 0).Style(st))); got != " abc " {
		t.Fatalf("natural width with padding = %q", got)
	}
	if got := ansi.Strip(Render(7, 1, Marquee("abcdef", 0).Style(st))); got != " abcd… " {
		t.Fatalf("padding against the width = %q", got)
	}
}

// BenchmarkMarqueeFrame lays out and draws a frame of 30 scrolling marquees
// and a selection of truncated ones, as in a list, on a 200×60 terminal.
func BenchmarkMarqueeFrame(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	st := benchItem
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		items := make([]Child, 0, benchH)
		for j := range benchH {
			phase := 0.0
			if j == i%benchH {
				phase = float64(i%240+1) / 240
			}
			items = append(items, Marquee("item with a very long name that has to be cut off or scrolled across the row", phase).Style(st))
		}
		r.View(benchW, benchH, Row(Fixed(40, Column(items...))))
	}
}

func TestSkeleton(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node SkeletonNode
		w, h int
		want string
	}{
		{"natural size", Skeleton(3, 0, 5), 0, 0, "███  \n     \n█████"},
		{"cut to fit", Skeleton(8, 2), 4, 0, "████\n██  "},
		{"negative is blank", Skeleton(-1, 1), 0, 0, " \n█"},
		{"shimmering", Skeleton(2, 2).Shimmer(0.5), 0, 0, "██\n██"},
		{"styled", Skeleton(4).Style(lipgloss.NewStyle().Padding(0, 1)), 4, 1, " ██ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(Render(tt.w, tt.h, tt.node)); got != tt.want {
				t.Fatalf("rendered %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSkeletonColors checks that an unstyled skeleton is drawn in a color
// faded from the terminal's, and a styled one in its style's.
func TestSkeletonColors(t *testing.T) {
	t.Parallel()

	r := New(WithColors(lipgloss.Color("#ffffff"), lipgloss.Color("#000000")))
	if got := r.Render(4, 1, Skeleton(4)); !strings.Contains(got, "38;2;51;51;51m") {
		t.Fatalf("unstyled skeleton was not faded: %q", got)
	}
	styled := Skeleton(4).Style(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")))
	if got := r.Render(4, 1, styled); !strings.Contains(got, "38;2;255;0;0m") {
		t.Fatalf("styled skeleton lost its color: %q", got)
	}
}

func TestStack(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"✕":                  "✕", // one glyph, returned as it is
		"a\x1b[1mb\nc\x1b[m": "a\x1b[1m\nb\nc\x1b[m",
	} {
		if got := stack(in); got != want {
			t.Fatalf("stack(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTextSizeKept checks that the size of a wrapped text, styled or not,
// kept from frame to frame rather than wrapped again, follows its text,
// style, and width as they change.
func TestTextSizeKept(t *testing.T) {
	t.Parallel()

	r := New()
	after := func(w int, n TextNode) int {
		t.Helper()
		r.Render(w, 20, Column(n, Text("after").ID("after")))
		rect, ok := r.Frame().Rect("after")
		if !ok {
			t.Fatal("no text after")
		}
		return rect.Min.Y
	}
	plain := lipgloss.NewStyle()
	words := "one two three four five six"
	more := words + " seven eight nine"
	for _, tc := range []struct {
		w    int
		n    TextNode
		want int
	}{
		{10, Text(words).Style(plain), 3},
		{10, Text(words).Style(plain), 3},
		{20, Text(words).Style(plain), 2},
		{20, Text(more).Style(plain), 3},
		{20, Text(more).Style(plain.PaddingTop(2)), 5},
		{20, Text(more).Style(plain.Width(10)), 5},
		{40, Text(words).Style(plain), 1},
		{10, Text(words), 3},
		{10, Text(words), 3},
		{20, Text(words), 2},
		{20, Text(more), 3},
		{20, Text(more).Style(plain.PaddingTop(2)), 5},
		{20, Text(more), 3},
		{20, Text(more).Truncate(), 1},
		{20, Text(more), 3},
		{40, Text(words), 1},
	} {
		if got := after(tc.w, tc.n); got != tc.want {
			t.Errorf("%q at %d: after it at row %d, want %d", tc.n.s, tc.w, got, tc.want)
		}
	}
}

// TestTextWrapsHyphenRuns checks that text wrapped where a run of hyphens
// meets the end of a row drops none of them, styled or not, plain or not:
// the run stays whole, or breaks after one of its hyphens.
func TestTextWrapsHyphenRuns(t *testing.T) {
	t.Parallel()

	got := strings.Split(ansi.Strip(Render(12, 0, Text("a line of --- between"))), "\n")
	for i := range got {
		got[i] = strings.TrimRight(got[i], " ")
	}
	if want := []string{"a line of --", "- between"}; !slices.Equal(got, want) {
		t.Fatalf("drawn %q, want %q", got, want)
	}
	padded := lipgloss.NewStyle().Padding(0, 1)
	for _, dashes := range []string{"-", "--", "---"} {
		for _, s := range []string{
			"a line of %s between",
			"edit them in one editor, with a line of %s between each, since a line can't tell them apart",
			"x %s y",
		} {
			s = strings.ReplaceAll(s, "%s", dashes)
			for _, s := range []string{s, strings.Replace(s, "i", "í", 1)} {
				want := strings.Join(strings.Fields(s), "")
				for w := 3; w <= len(s); w++ {
					for name, n := range map[string]TextNode{
						"unstyled": Text(s),
						"styled":   Text(s).Style(lipgloss.NewStyle().Bold(true)),
						"padded":   Text(s).Style(padded),
						"sized":    Text(s).Style(lipgloss.NewStyle().Width(w)),
					} {
						got := ansi.Strip(Render(w, 0, n))
						if kept := strings.Join(strings.Fields(got), ""); kept != want {
							t.Fatalf("%s %q at %d: drew\n%s\nkeeping %q", name, s, w, got, kept)
						}
					}
				}
			}
		}
	}
}

// TestTextWraps checks that text too wide for the space it is given wraps
// to it and is measured that many lines tall, styled or not, and that
// truncated text is cut to one line instead.
func TestTextWraps(t *testing.T) {
	t.Parallel()

	const long = "the quick brown fox jumps over the lazy dog, then keeps on running"
	wrapped := []string{"the quick brown fox jumps over", "the lazy dog, then keeps on", "running"}
	for _, tt := range []struct {
		name string
		node TextNode
		want []string
	}{
		{"unstyled", Text(long), wrapped},
		{"styled", Text(long).Style(lipgloss.NewStyle().Bold(true)), wrapped},
		{"padded", Text(long).Style(lipgloss.NewStyle().Padding(0, 2)), []string{
			"  the quick brown fox jumps", "  over the lazy dog, then", "  keeps on running",
		}},
		{"truncated", Text(long).Truncate(), []string{"the quick brown fox jumps ove…"}},
		{"unstyled, a word too long", Text(strings.Repeat("abcdefghij", 4)), []string{
			strings.Repeat("abcdefghij", 3), "abcdefghij",
		}},
		{"unstyled, not wrapped", Text(long + "\nshort").NoWrap(), []string{"the quick brown fox jumps over", "short"}},
		{"styled, not wrapped", Text(long + "\nshort").Style(lipgloss.NewStyle().Padding(0, 2)).NoWrap(), []string{
			"  the quick brown fox jumps", "  short",
		}},
		{"truncated, not wrapped", Text(long).Truncate().NoWrap(), []string{"the quick brown fox jumps ove…"}},
		// Lip Gloss draws a tab as spaces, by default 4, and wraps to a
		// style's own width even at the node's natural size.
		{"tabs, not wrapped", Text("a\tbcde\nx").Style(lipgloss.NewStyle().Width(6)).NoWrap(), []string{"a    b", "x"}},
		{"no tabs, not wrapped", Text("a\tbcdefg").Style(lipgloss.NewStyle().Width(6).TabWidth(0)).NoWrap(), []string{"abcdef"}},
		{"transformed, not wrapped", Text("x").Style(lipgloss.NewStyle().Width(4).Transform(func(string) string {
			return "abcdefgh"
		})).NoWrap(), []string{"abcd"}},
		{"a string of the style's, not wrapped", Text("tail").Style(lipgloss.NewStyle().Width(4).SetString("head")).NoWrap(), []string{
			"head",
		}},
		{"inline, not wrapped", Text("abcd").Style(lipgloss.NewStyle().Inline(true).Width(4).Padding(0, 1)).NoWrap(), []string{"abcd"}},
		{"a string of the style's, truncated", Text("x").Style(lipgloss.NewStyle().Width(6).SetString("abcdefgh")).Truncate(), []string{
			"abcde…",
		}},
		{"tabs, truncated", Text("a\tbcdefgh").Style(lipgloss.NewStyle().Width(6)).Truncate(), []string{"a    …"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tree := Column(tt.node.ID("text"), Text("end").ID("end"))
			// The column is as tall as all of the lines, and the text below
			// them follows them.
			f := Solve(30, 0, tree)
			if r, _ := f.Rect("end"); r.Min.Y != len(tt.want) {
				t.Fatalf("end at row %d, want %d", r.Min.Y, len(tt.want))
			}
			got := strings.Split(ansi.Strip(Render(30, 0, tree)), "\n")
			for i := range got {
				got[i] = strings.TrimRight(got[i], " ")
			}
			if want := append(tt.want, "end"); !slices.Equal(got, want) {
				t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}

	// At its natural size, nothing wraps.
	if got := Render(0, 0, Text(long)); got != long {
		t.Fatalf("natural size: %q", got)
	}

	// Its measured size is the size it is drawn at.
	for _, s := range []string{"ab", long, "a\nmuch longer line with\nlines of its own", "wide 字字字 glyphs, wrapped"} {
		for _, avail := range []int{0, 2, 5, 9, 14, 30, 80} {
			n := Text(s)
			f := Solve(avail, 0, Row(Auto(n).ID("n")))
			r, _ := f.Rect("n")
			got, _ := textOf(n, r.Dx(), 0, ansi.GraphemeWidth)
			if w, h := lipgloss.Size(got); w != r.Dx() || h != r.Dy() {
				t.Errorf("%q in %d: measured %dx%d, drawn %dx%d:\n%s", s, avail, r.Dx(), r.Dy(), w, h, got)
			}
		}
	}

	// Text that does not wrap is measured as it is drawn too, its lines cut
	// inside its style's frame, at any width with room for the frame.
	framed := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	for _, s := range []string{"ab", long, "a\nmuch longer line with\nlines of its own", "wide 字字字 glyphs, cut"} {
		for _, avail := range []int{0, 4, 5, 9, 14, 30, 80} {
			n := Text(s).Style(framed).NoWrap()
			f := Solve(avail, 0, Row(Auto(n).ID("n")))
			r, _ := f.Rect("n")
			got, _ := textOf(n, r.Dx(), 0, ansi.GraphemeWidth)
			if w, h := lipgloss.Size(got); w != r.Dx() || h != r.Dy() || h != strings.Count(s, "\n")+3 {
				t.Errorf("%q in %d: measured %dx%d, drawn %dx%d:\n%s", s, avail, r.Dx(), r.Dy(), w, h, got)
			}
		}
	}

	// Styles and links go on across the rows they wrap to, as they do for
	// styled text.
	styled := "\x1b[31mred words that wrap around\x1b[m and " + ansi.SetHyperlink("https://example.com") + "a link that wraps" + ansi.ResetHyperlink()
	if got, want := Render(12, 0, Text(styled)), Render(12, 0, Text(styled).Style(lipgloss.NewStyle())); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// Truncated text that fits is not cut where an inline style ignores its
	// padding, nor laid out on more than a line where tabs are kept.
	inlinePad := Row(Auto(Text("abc").Style(lipgloss.NewStyle().Inline(true).PaddingLeft(1)).Truncate()), Text("|"))
	if got := ansi.Strip(Render(10, 0, inlinePad)); !strings.HasPrefix(got, "abc") || strings.Contains(got, "…") {
		t.Fatalf("inline, truncated: %q", got)
	}
	kept := Column(Text("a\tbcde").Style(lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion)).Truncate(), Text("end"))
	if got := ansi.Strip(Render(4, 0, kept)); strings.Count(got, "\n") != 1 {
		t.Fatalf("kept tabs, truncated: %q", got)
	}

	// An inline style joins the lines into one.
	inline := Column(Text("abc\ndef").Style(lipgloss.NewStyle().Inline(true).Width(10)).NoWrap(), Text("end"))
	if got := ansi.Strip(Render(10, 0, inline)); !strings.HasPrefix(got, "abc def") || strings.Count(got, "\n") != 1 {
		t.Fatalf("inline: %q", got)
	}

	// At its natural width, tabs count as the spaces they are drawn as.
	if got := ansi.Strip(Render(0, 0, Text("a\tbcde\nx").Style(lipgloss.NewStyle().MaxWidth(20)).NoWrap())); got != "a    bcde\nx        " {
		t.Fatalf("tabs at its natural width: %q", got)
	}

	// Styled text that does not wrap is cut as Lip Gloss measures it, so it
	// has nothing to wrap, whatever the renderer measures with.
	hearts := Text("❤️❤️❤️\nabc").Style(lipgloss.NewStyle()).NoWrap()
	if got := ansi.Strip(newRenderer(ansi.WcWidth).render(4, 0, Column(hearts, Text("end")))); !strings.HasSuffix(got, "\nabc\nend") || strings.Count(got, "\n") != 2 {
		t.Fatalf("WcWidth, not wrapped: %q", got)
	}

	// It wraps at the width the renderer measures with: "❤️" is one cell to
	// WcWidth and two to GraphemeWidth.
	if got := ansi.Strip(newRenderer(ansi.WcWidth).render(4, 0, Text("❤️❤️❤️❤️❤️❤️abc"))); got != "❤️❤️❤️❤️\n❤️❤️ab\nc   " {
		t.Fatalf("WcWidth: %q", got)
	}
}
