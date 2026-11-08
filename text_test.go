// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"math"
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
			got := ansi.Strip(Render(n, tt.width, 1))
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if marquee := Render(Marquee(tt.text, 0).Style(tt.style), tt.width, 1); Render(n, tt.width, 1) != marquee {
				t.Fatalf("differs from marquee: %q", marquee)
			}
		})
	}
	if got := Text("abc\ndef").Truncate().View(); got != "abc def" {
		t.Fatalf("natural size: %q", got)
	}
	if got := ansi.Strip(Render(Column(Text("abcdef").Truncate(), Text("end")), 3, 0)); got != "ab…\nend" {
		t.Fatalf("natural height: %q", got)
	}
}

func TestTruncateKeepsStyleLimits(t *testing.T) {
	t.Parallel()
	tall := Text("x").Style(lipgloss.NewStyle().Height(3)).Truncate()
	expectRect(t, Solve(Column(tall, Text("y").ID("y")), 0, 0), "y", image.Rect(0, 3, 1, 4))

	// A transform that lengthens the text or adds lines is cut to one line too.
	for _, tf := range []func(string) string{
		func(s string) string { return strings.Repeat(s, 4) },
		func(s string) string { return s + "\n" + s },
	} {
		grown := Text("abc").Style(lipgloss.NewStyle().Transform(tf)).Truncate()
		if got := Render(Column(grown, Text("end")), 4, 0); got != "abc…\nend " {
			t.Fatalf("transform: %q", got)
		}
	}

	narrow := Text("abcdef").Style(lipgloss.NewStyle().MaxWidth(3)).Truncate()
	if got := Render(narrow, 0, 0); got != "ab…" {
		t.Fatalf("MaxWidth: %q", got)
	}
	if got := Render(Row(narrow, Text("|")), 0, 0); got != "ab…|" {
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
				f := Solve(Row(Auto(n).ID("n")), avail, 0)
				r, _ := f.Rect("n")
				got := textOf(n, r.Dx(), 0, ansi.GraphemeWidth)
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

			got := ansi.Strip(Render(tt.node, tt.width, 1))
			if want := padTo(tt.want, tt.width, ansi.GraphemeWidth); got != want {
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
	if got := Render(n, 0, 0); ansi.Strip(got) != "hello world" {
		t.Fatalf("natural render = %q", got)
	}
	if v := n.View(); v != Render(n, 0, 0) {
		t.Fatalf("View = %q", v)
	}

	// It shrinks in a slot, instead of pushing its neighbours out.
	got := ansi.Strip(Render(Row(Fixed(5, Text("ab")), Fill(Marquee("hello world", 0)), Fixed(1, Text("|"))), 20, 1))
	if want := "ab   hello world   |"; got != want {
		t.Fatalf("in a row: %q, want %q", got, want)
	}
	got = ansi.Strip(Render(Row(Fixed(5, Text("ab")), Fill(Marquee("hello world", 0)), Fixed(1, Text("|"))), 12, 1))
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
			got := ansi.Strip(Render(Marquee(text, phase), width, 1))
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

	if got := ansi.Strip(Render(Marquee(text, 0), 5, 1)); got != "世界…" {
		t.Fatalf("truncated at a wide glyph: %q", got)
	}
	// The cut lands in the middle of the second glyph, at cell 3.
	if got := ansi.Strip(Render(Marquee(text, 3.5/13), 5, 1)); got != " 你好" {
		t.Fatalf("scrolled half a glyph: %q", got)
	}
	if got := ansi.Strip(Render(Marquee(text, 4.5/13), 5, 1)); got != "你好 " {
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
		got := ansi.Strip(newRenderer(ansi.WcWidth).render(Row(Fill(tt.node), Text("|")), 6, 1))
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
		got := Render(Marquee("0123456789", tt.phase).Style(st), 8, 1)
		if ansi.Strip(got) != tt.want {
			t.Errorf("phase %v: %q, want %q", tt.phase, ansi.Strip(got), tt.want)
		}
		if !strings.Contains(got, "\x1b[") {
			t.Errorf("phase %v: %q is not styled", tt.phase, got)
		}
	}

	// Padding is part of the natural width, and counts against a fit.
	if got := ansi.Strip(Render(Marquee("abc", 0).Style(st), 0, 0)); got != " abc " {
		t.Fatalf("natural width with padding = %q", got)
	}
	if got := ansi.Strip(Render(Marquee("abcdef", 0).Style(st), 7, 1)); got != " abcd… " {
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

			if got := ansi.Strip(Render(tt.node, tt.w, tt.h)); got != tt.want {
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
