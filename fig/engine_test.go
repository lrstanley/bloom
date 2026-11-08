// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig_test

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/lrstanley/bloom/fig"
	"github.com/lrstanley/bloom/fig/fonts"
)

// charFont returns a font one row tall whose glyphs are their own
// characters, with the given Full_Layout.
func charFont(t *testing.T, layout int) *fig.Font {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "flf2a$ 1 1 3 0 0 0 %d\n", layout)
	for r := ' '; r <= '~'; r++ {
		end := '@'
		if r == '@' {
			end = '#'
		}
		fmt.Fprintf(&b, "%c%c%c\n", r, end, end)
	}
	for range 7 {
		b.WriteString("?@@\n")
	}
	f, err := fig.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, data := range []string{"", "hello", "flf2a$ 1", "tlf2a$ x y z"} {
		if _, err := fig.Parse([]byte(data)); !errors.Is(err, fig.ErrNotFont) {
			t.Errorf("Parse(%q) error = %v, want ErrNotFont", data, err)
		}
	}
	// Glyphs far taller or wider than any font's would make every figure
	// that large.
	for _, data := range []string{
		"flf2a$ 100000 1 1 0 0",
		"flf2a$ 1 1 1 0 0\n" + strings.Repeat("#", 1000) + "@\n",
		"flf2a$ 1 1 1 0 0\nA" + strings.Repeat("\u0301", 1000) + "@\n", // one cell of many accents
	} {
		if _, err := fig.Parse([]byte(data)); !errors.Is(err, fig.ErrTooLarge) {
			t.Errorf("Parse(%.20q) error = %v, want ErrTooLarge", data, err)
		}
	}
}

func TestParseTruncated(t *testing.T) {
	t.Parallel()

	// A font that ends early has blank glyphs, as in figlet, however many
	// comment lines its header claims.
	f, err := fig.Parse([]byte("flf2a$ 3 2 4 0 2000000000\ncomment\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Figure("Hi", fig.Options{}).String(); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestCutSeams(t *testing.T) {
	t.Parallel()

	// Right to left, the glyphs of "ABA" merge where the third column is,
	// which a Width of 3 cuts off.
	var b strings.Builder
	b.WriteString("flf2a$ 2 2 5 0 0 1 128\n")
	for r := ' '; r <= '~'; r++ {
		switch r {
		case 'A':
			b.WriteString("XX@\nX@@\n")
		case 'B':
			b.WriteString(" X@\nXX@@\n")
		default:
			b.WriteString("@\n@@\n")
		}
	}
	f, err := fig.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	fg := f.Figure("ABA", fig.Options{Width: 3})
	w, h := fg.Size()
	for _, s := range fg.Seams() {
		if s.X >= w || s.Y >= h {
			t.Errorf("seam %+v outside the %dx%d figure", s, w, h)
		}
	}
}

func TestParseCompressed(t *testing.T) {
	t.Parallel()

	src := fontText(t, "standard")
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(src))
	_ = zw.Close()
	var zipped bytes.Buffer
	z := zip.NewWriter(&zipped)
	w, _ := z.Create("standard.flf")
	_, _ = w.Write([]byte(strings.ReplaceAll(src, "\n", "\r\n")))
	_ = z.Close()

	want := fonts.Standard().Figure("Zip!", fig.Options{}).String()
	for name, data := range map[string][]byte{"gzip": gz.Bytes(), "zip with CRLF": zipped.Bytes()} {
		f, err := fig.Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := f.Figure("Zip!", fig.Options{}).String(); got != want {
			t.Errorf("%s: got\n%s\nwant\n%s", name, got, want)
		}
	}
}

// fontText returns a bundled font's text.
func fontText(t *testing.T, name string) string {
	t.Helper()
	f, _ := fonts.ByName(name)
	return fontSource(t, f)
}

func TestControl(t *testing.T) {
	t.Parallel()

	c := fig.ParseControl([]byte("# swap cases, then map A to Z\nt a-z A-Z\nt A-Z a-z\nf\n0x61 0x7A\nt \\0x31 9\n"))
	tests := map[rune]rune{'a': 'A', 'B': 'b', 'A': 'a', 'b': 'B', '1': '9', '?': '?'}
	// After the freeze, 'a' (from 'A') maps to 'z'.
	tests['A'] = 'z'
	for in, want := range tests {
		if got := c.Map(in); got != want {
			t.Errorf("Map(%q) = %q, want %q", in, got, want)
		}
	}
	f := charFont(t, 0).WithControl(c)
	if got := f.Figure("ab", fig.Options{}).String(); got != "AB" {
		t.Errorf("figure with control = %q, want AB", got)
	}
}

func TestStrict(t *testing.T) {
	t.Parallel()

	f := charFont(t, int(fig.RuleEqual)|128)
	// figlet does not smush glyphs narrower than two columns; the standard
	// does.
	if got := f.Figure("||", fig.Options{}).String(); got != "||" {
		t.Errorf("figlet layout = %q, want ||", got)
	}
	if got := f.Figure("||", fig.Options{Strict: true}).String(); got != "|" {
		t.Errorf("strict layout = %q, want |", got)
	}
}

func TestVertical(t *testing.T) {
	t.Parallel()

	all := int(fig.RuleVEqual | fig.RuleVUnderscore | fig.RuleVHierarchy | fig.RuleVLine | fig.RuleVSuper)
	f := charFont(t, all|16384)
	tests := []struct {
		text string
		opts fig.Options
		want string
	}{
		{"|\n|", fig.Options{}, "|\n|"}, // figlet stacks lines at full height
		{"|\n|", fig.Options{Strict: true}, "|"},
		{"_\n/", fig.Options{Vertical: fig.Smushed}, "/"},
		{"/\n[", fig.Options{Vertical: fig.Smushed}, "["},
		{"-\n_", fig.Options{Vertical: fig.Smushed}, "="},
		{"a\nb", fig.Options{Vertical: fig.Smushed}, "a\nb"},
		{"a\nb", fig.Options{Vertical: fig.Universal}, "b"},
		{"a\nb", fig.Options{Vertical: fig.Fitted}, "a\nb"},
		{"a \n b", fig.Options{Vertical: fig.Fitted}, "ab"},
		{"-\n_", fig.Options{Vertical: fig.Smushed, Rules: fig.RuleVEqual}, "-\n_"},
	}
	for _, tt := range tests {
		if got := f.Figure(tt.text, tt.opts).String(); got != tt.want {
			t.Errorf("%q with %+v: got %q, want %q", tt.text, tt.opts, got, tt.want)
		}
	}
}

func TestTracking(t *testing.T) {
	t.Parallel()

	f := fonts.Standard()
	w0, _ := f.Figure("Bloom", fig.Options{}).Size()
	w1, _ := f.Figure("Bloom", fig.Options{Tracking: 1}).Size()
	if w1 != w0+4 {
		t.Errorf("tracking 1 makes \"Bloom\" %d wide, want %d", w1, w0+4)
	}
	later := f.Figure("Bloom", fig.Options{Tracking: -2})
	earlier := f.Figure("Bloom", fig.Options{Tracking: -2, Stacking: fig.EarlierOnTop})
	if later.String() == earlier.String() {
		t.Error("stacking made no difference")
	}
	overlaps := 0
	for _, s := range later.Seams() {
		if s.Merge == fig.MergeOverlap {
			overlaps++
			if s.Content != s.LaterContent {
				t.Errorf("later on top shows %q at %d,%d, not the later %q", s.Content, s.X, s.Y, s.LaterContent)
			}
		}
	}
	if overlaps == 0 {
		t.Error("no glyphs overlapped")
	}
}

func TestProvenance(t *testing.T) {
	t.Parallel()

	f := fonts.Standard().Figure("Bloom", fig.Options{})
	if got := len(f.Seams()); got != 4 {
		t.Fatalf("%d seams, want 4", got)
	}
	for _, s := range f.Seams() {
		if s.Later != s.Earlier+1 || s.Merge != fig.MergeEqual || s.Winner != fig.Both {
			t.Errorf("seam %+v", s)
		}
		if c := f.At(s.X, s.Y); c.Kind != fig.KindSeam || c.Glyph != s.Later {
			t.Errorf("cell at seam %+v", c)
		}
	}
	// Src is where each character is in the text as given, CRLFs and all.
	if g := fonts.Standard().Figure("A\r\nB", fig.Options{}).Glyphs(); g[len(g)-1].Src != 3 {
		t.Errorf("B is at %d of \"A\\r\\nB\", want 3", g[len(g)-1].Src)
	}
	glyphs := f.Glyphs()
	if len(glyphs) != 5 || glyphs[1].Rune != 'l' || glyphs[1].Src != 1 {
		t.Fatalf("glyphs %+v", glyphs)
	}
	if !glyphs[0].Rect.Overlaps(glyphs[1].Rect) {
		t.Error("smushed glyphs do not overlap")
	}
	inside := 0
	o := fonts.Standard().Figure("O", fig.Options{})
	w, h := o.Size()
	for y := range h {
		for x := range w {
			if o.At(x, y).Kind == fig.KindInside {
				inside++
			}
		}
	}
	if inside == 0 {
		t.Error("no cells inside \"O\"")
	}
}

func TestPaints(t *testing.T) {
	t.Parallel()

	f := fonts.Standard().Figure("Hi", fig.Options{})
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	calls := 0
	count := fig.PaintFunc(func(*fig.Figure, fig.Cell) uv.Style { calls++; return uv.Style{} })
	paints := map[string]fig.Paint{
		"solid":    fig.Solid(red),
		"gradient": fig.Gradient(red, blue).By(fig.ByDiagonal),
		"palette":  fig.Palette(red, blue),
		"rainbow":  fig.Rainbow(),
		"metal":    fig.Metal(),
		"fill":     fig.Fill(blue, fig.Solid(red)),
	}
	for name, p := range paints {
		if out := f.Render(p); !strings.Contains(out, "\x1b[") {
			t.Errorf("%s: no styles in\n%s", name, out)
		}
	}
	f.Render(count)
	first := calls
	f.Render(count)
	if calls != 2*first {
		t.Errorf("a PaintFunc was not rendered again: %d calls, then %d", first, calls)
	}
	if f.Render(nil) != f.String() {
		t.Error("Render(nil) is not String()")
	}
	// Each glyph of a palette is one color: "H" red and "i" blue.
	cell := func(g int) fig.Cell {
		r := f.Glyphs()[g].Rect
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if c := f.At(x, y); c.Kind == fig.KindStroke && c.Glyph == g {
					return c
				}
			}
		}
		t.Fatalf("no stroke of glyph %d", g)
		return fig.Cell{}
	}
	pal := fig.Palette(red, blue)
	if pal.Shade(f, cell(0)).Fg != red || pal.Shade(f, cell(1)).Fg != blue {
		t.Error("palette colors are not by glyph")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(charFontText()))
	f.Add([]byte("flf2a$ 2 1 4 -1 0\n@\n@@\n"))
	f.Add([]byte("tlf2a$ 1 1 4 0 1 1 16384\ncomment\n█@@\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		font, err := fig.Parse(data)
		if err != nil {
			return
		}
		for _, o := range []fig.Options{{}, {Vertical: fig.Smushed, Width: 10, Tracking: -1}, {Direction: fig.RightToLeft, Justify: fig.Center, Width: 12}} {
			font.Figure("Hi\nthere, fuzz! ÄÖ", o).Render(fig.Gradient(color.White, color.Black).Blend())
		}
	})
}

func charFontText() string {
	var b strings.Builder
	b.WriteString("flf2a$ 1 1 3 0 0 0 159\n")
	for r := ' '; r <= '~'; r++ {
		fmt.Fprintf(&b, "%c##\n", r)
	}
	return b.String()
}
