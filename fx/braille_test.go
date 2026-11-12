// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image/color"
	"strings"
	"testing"
	"unicode/utf8"

	"bloomfx.dev/bloom/canvas"
	"github.com/charmbracelet/x/ansi"
)

// brailleFrame returns a region over all of text, drawn in sgr, such as
// "38;2;255;255;255" for white glyphs on no background.
func brailleFrame(text, sgr string) *canvas.Surface {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = "\x1b[" + sgr + "m" + l + "\x1b[m"
	}
	frame := canvas.Parse(strings.Join(lines, "\n"), ansi.GraphemeWidth)
	return frame.Region(frame.Bounds(), env, &canvas.Mixer{})
}

const whiteText = "38;2;255;255;255"

func isBraille(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	return size == len(s) && r >= 0x2800 && r <= 0x28ff
}

// row returns the content of row y of s.
func row(s *canvas.Surface, y int) string {
	var b strings.Builder
	for x := range s.Width() {
		b.WriteString(s.Cell(x, y).Content)
	}
	return b.String()
}

func TestBraille(t *testing.T) {
	t.Parallel()

	t.Run("glyphs become dots, blanks stay blank", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("The quick brown fox!\n  jumps   over   ", "38;2;255;0;0")
		Braille{}.Apply(s, 1)
		for y := range s.Height() {
			for x := range s.Width() {
				c := s.Cell(x, y)
				if c.Content == " " {
					continue
				}
				if !isBraille(c.Content) || c.Content == "⠀" {
					t.Fatalf("cell %d,%d is %q, want inked braille", x, y, c.Content)
				}
			}
		}
		if got := row(s, 1); !strings.HasPrefix(got, "  ") || got[2] == ' ' || !strings.HasSuffix(got, "   ") {
			t.Fatalf("blanks moved: %q", got)
		}
		// Dots take the gray of their color, faded toward the terminal's
		// background, black, 0.65 of the way by default; unset backgrounds
		// stay unset.
		c := s.Cell(0, 0)
		if got := rgba(c.Style.Fg); got.R != got.G || got.G != got.B || got.R == 0 || got.R > 40 {
			t.Fatalf("dots are %v, want a dark gray", got)
		}
		if c.Style.Bg != nil {
			t.Fatalf("set an unset background to %v", c.Style.Bg)
		}
	})

	t.Run("no strength, no change", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("abc", whiteText)
		Braille{}.Apply(s, 0)
		if got := row(s, 0); got != "abc" {
			t.Fatalf("t=0 drew %q", got)
		}
	})

	t.Run("dots follow the glyph", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("aaab", whiteText)
		Braille{}.Apply(s, 1)
		a, b := s.Cell(0, 0).Content, s.Cell(3, 0).Content
		if s.Cell(1, 0).Content != a || s.Cell(2, 0).Content != a {
			t.Fatalf("the same glyph drew different dots: %q", row(s, 0))
		}
		if a == b {
			t.Fatalf("a and b both drew %q", a)
		}
	})

	t.Run("lines and blocks keep their shape", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("╭──╮\n│▀▄│\n╰──╯", whiteText)
		Braille{}.Apply(s, 1)
		want := []string{"⡖⠒⠒⡆", "⡇⠛⣤⡇", "⠓⠒⠒⠃"}
		for y, w := range want {
			if got := row(s, y); got != w {
				t.Errorf("row %d is %q, want %q", y, got, w)
			}
		}
	})

	t.Run("braille is kept", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("⣿⡀⢸", whiteText)
		Braille{}.Apply(s, 1)
		if got := row(s, 0); got != "⣿⡀⢸" {
			t.Fatalf("braille became %q", got)
		}
	})

	t.Run("wide glyphs become a pattern in each cell", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame("a日b", whiteText)
		Braille{}.Apply(s, 1)
		for x := 1; x <= 2; x++ {
			if c := s.Cell(x, 0); c.Width != 1 || !isBraille(c.Content) {
				t.Fatalf("cell %d of the wide glyph is %q, width %d", x, c.Content, c.Width)
			}
		}
		if c := s.Cell(3, 0); !isBraille(c.Content) {
			t.Fatalf("the cell after the wide glyph is %q", c.Content)
		}
	})

	t.Run("cells turn in a scattered order", func(t *testing.T) {
		t.Parallel()
		s := brailleFrame(strings.Repeat("x", 200), whiteText)
		Braille{Seed: 3}.Apply(s, 0.4)
		turned := 0
		for x := range s.Width() {
			if isBraille(s.Cell(x, 0).Content) {
				turned++
			} else if rgba(s.Cell(x, 0).Style.Fg) == white {
				t.Fatalf("cell %d kept its glyph without fading", x)
			}
		}
		if turned < 60 || turned > 140 {
			t.Fatalf("t=0.4 turned %d of 200 cells", turned)
		}
		s = brailleFrame(strings.Repeat("x", 200), whiteText)
		Braille{Seed: 3}.Apply(s, brailleSpread)
		if got := row(s, 0); strings.Contains(got, "x") {
			t.Fatalf("t=%v left glyphs: %q", brailleSpread, got)
		}
	})

	t.Run("backgrounds", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name, sgr string
			want      string // the dots of "a " in braille, or blanks.
			filled    bool   // whether the dots take the background's gray.
		}{
			{"standing out", "38;2;255;255;255;48;2;0;0;255", brailleGlyphs[0xff&^asciiDots['a']] + "⣿", true},
			{"sparse", "38;2;255;255;255;48;2;32;32;32", brailleGlyphs[asciiDots['a']|fills[0]] + brailleGlyphs[fills[0]], false},
			{"close to the terminal's", "38;2;255;255;255;48;2;5;5;5", brailleGlyphs[asciiDots['a']] + " ", false},
		}
		for _, tt := range tests {
			s := brailleFrame("a ", tt.sgr)
			Braille{}.Apply(s, 1)
			if got := row(s, 0); got != tt.want {
				t.Errorf("%s: drew %q, want %q", tt.name, got, tt.want)
			}
			for x := range 2 {
				if bg := s.Cell(x, 0).Style.Bg; bg == nil || rgba(bg) != black {
					t.Errorf("%s: cell %d has background %v, want the terminal's", tt.name, x, bg)
				}
			}
			// White glyphs fade to a brighter gray than any of these
			// backgrounds.
			if fg := rgba(s.Cell(0, 0).Style.Fg); (fg.R < 60) != tt.filled {
				t.Errorf("%s: dots are %v, filled %v", tt.name, fg, tt.filled)
			}
		}
	})

	t.Run("gray on a tinted background", func(t *testing.T) {
		t.Parallel()
		frame := canvas.Parse("\x1b[38;2;255;255;255mab\x1b[m", ansi.GraphemeWidth)
		navy := canvas.Env{Fg: white, Bg: color.RGBA{B: 100, A: 0xff}}
		s := frame.Region(frame.Bounds(), navy, &canvas.Mixer{})
		Braille{}.Apply(s, 1)
		if got := rgba(s.Cell(0, 0).Style.Fg); got.R != got.G || got.G != got.B {
			t.Fatalf("dots are %v, want a gray", got)
		}
	})

	t.Run("comparable", func(t *testing.T) {
		t.Parallel()
		var e Effect = Braille{Amount: 0.5, Seed: 2}
		if e != (Braille{Amount: 0.5, Seed: 2}) {
			t.Fatal("equal effects compare unequal")
		}
	})
}

func TestBraillePatterns(t *testing.T) {
	t.Parallel()

	for _, r := range " \t\u00a0\u3000\u2800" {
		if d := braillePattern(string(r), 0, false); d != 0 {
			t.Errorf("%U drew dots %08b", r, d)
		}
	}
	for i, d := range boxDots {
		if d == 0 {
			t.Errorf("%U has no dots", rune(0x2500+i))
		}
	}
	for c := byte('!'); c <= '~'; c++ {
		if asciiDots[c] == 0 {
			t.Errorf("%q has no dots", c)
		}
	}
	// Ascenders reach the top row, descenders the bottom one, and the rest
	// of the lowercase letters neither.
	top, bottom := dots(2, 0, 0), dots(2, 3, 3)
	for c := byte('a'); c <= 'z'; c++ {
		d := asciiDots[c]
		asc := strings.IndexByte("bdfhiklt", c) >= 0
		desc := strings.IndexByte("gjpqy", c) >= 0
		if asc != (d&top != 0) && c != 'j' {
			t.Errorf("%q: ascender %v, top row %08b", c, asc, d&top)
		}
		if desc != (d&bottom != 0) {
			t.Errorf("%q: descender %v, bottom row %08b", c, desc, d&bottom)
		}
	}
}

func BenchmarkBraille(b *testing.B) {
	// Half the rows on a background that stands out, to be filled.
	text := strings.Repeat("│ The quick brown fox jumps over the lazy dog ▀▄ 0123 ─┼─ │\n"+
		"\x1b[48;2;60;40;90m│ The quick brown fox jumps over the lazy dog ▀▄ 0123 ─┼─ │\x1b[m\n", 20)
	src := canvas.Parse(strings.TrimSuffix(text, "\n"), ansi.GraphemeWidth)
	frame := canvas.New(src.Width(), src.Height(), ansi.GraphemeWidth)
	mixer := &canvas.Mixer{}
	b.ReportAllocs()
	for b.Loop() {
		frame.Blit(src, src.Bounds(), frame.Bounds())
		s := frame.Region(frame.Bounds(), env, mixer)
		Braille{Seed: 1}.Apply(s, 1)
	}
}
