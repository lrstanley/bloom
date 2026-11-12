// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image/color"
	"math/bits"
	"unicode"
	"unicode/utf8"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

// Braille redraws everything as braille dots in grays, so what is beneath a
// dialog reads as a texture rather than as text. It suits backdrops, where t
// is the strength.
//
// Each glyph becomes a pattern made from it, so the dots change as what is
// beneath them does. Lines and blocks keep their shape: box drawing becomes
// dotted lines that still meet, half blocks and quadrants fill the matching
// dots, and shades become dithers. Text keeps the outline of its words,
// with ascenders and descenders, and braille is kept as it is.
//
// Backgrounds become the terminal's. A cell whose background stood out from
// it is filled with an even dither instead, denser the more it stood out,
// with its glyph, if any, cut out of the fill. Blank cells on the terminal's
// background stay blank, so the dots keep the shape of what was drawn.
//
// The dots take the gray of their color, faded toward the gray of the
// terminal's background by Amount; follow Braille with [Recolor] to tint them. As t
// rises, colors fade to gray and cells turn to dots in a scattered order,
// all of them by the time t reaches 0.85.
type Braille struct {
	// Amount is how far the dots' grays move toward the terminal's
	// background, from 0 to 1; zero is 0.65.
	Amount float64
	// Seed picks the order cells turn to dots in.
	Seed uint32
}

// brailleSpread is the strength by which every cell has turned to dots.
const brailleSpread = 0.85

// Apply implements [Effect].
func (e Braille) Apply(s *canvas.Surface, t float64) {
	if t <= 0 {
		return
	}
	amount := e.Amount
	if amount == 0 {
		amount = 0.65
	}
	// Dots fade toward the gray of the terminal's background, so they stay
	// gray on a tinted one.
	f, env := min(amount*t, 1), s.Env.Bg
	fade := s.Gray(env, 1)
	ink := func(c color.Color) color.Color { return s.Mix(s.Gray(c, t), fade, f) }

	w, h := s.Width(), s.Height()
	for y := range h {
		for x := 0; x < w; x++ {
			c := s.Cell(x, y)
			if c.Width == 0 || !s.Visible(x, y) {
				// The tail of a wide glyph is drawn with its lead.
				continue
			}
			fg, bg := s.Fg(c), c.Style.Bg
			if c.Style.Attrs&uv.AttrReverse != 0 {
				fg, bg = s.Bg(c), fg
			}

			if t < 1 && hash(x, y, e.Seed)*brailleSpread >= t {
				// Not turned yet: fading, keeping its glyph.
				c.Style.Attrs &^= uv.AttrReverse
				c.Style.Fg = ink(fg)
				if c.Style.Bg = bg; bg != nil {
					c.Style.Bg = s.Mix(s.Gray(bg, t), env, t)
				}
				x += c.Width - 1
				continue
			}

			var style uv.Style
			var fill uint8
			if bg != nil {
				style.Bg, fill = env, fillDots(bg, env)
			}
			if c.Width == 1 {
				glyph := braillePattern(c.Content, 0, false)
				dots, filled := overFill(glyph, fill)
				c.Content, c.Style = brailleCell(dots), style
				if filled {
					c.Style.Fg = ink(bg)
				} else if dots != 0 {
					c.Style.Fg = ink(fg)
				}
				continue
			}

			// A wide glyph becomes a pattern in each of its cells.
			cell := uv.Cell{Width: 1, Link: c.Link}
			content, wide := c.Content, c.Width
			for i := range wide {
				glyph := braillePattern(content, i, true)
				dots, filled := overFill(glyph, fill)
				cell.Content, cell.Style = brailleCell(dots), style
				if filled {
					cell.Style.Fg = ink(bg)
				} else {
					cell.Style.Fg = ink(fg)
				}
				s.Set(x+i, y, &cell)
			}
			x += wide - 1
		}
	}
}

// brailleCell returns the content of a cell with dots: a blank for none.
func brailleCell(dots uint8) string {
	if dots == 0 {
		return " "
	}
	return brailleGlyphs[dots]
}

// The dithers a background is filled with, by how far it stands out from
// the terminal's: two, four, six, and all eight dots of a cell, laid out to
// tile evenly across cells.
var fills = [...]uint8{
	dot(0, 0) | dot(1, 2),
	dot(0, 0) | dot(1, 1) | dot(0, 2) | dot(1, 3),
	0xff &^ (dot(1, 0) | dot(0, 2)),
	0xff,
}

// fillDots returns the dither for a cell with background bg over the
// terminal's background env, or none when bg is close to it. How far it
// stands out is the largest difference in a channel.
func fillDots(bg, env color.Color) uint8 {
	r1, g1, b1, _ := bg.RGBA()
	r2, g2, b2, _ := env.RGBA()
	diff := func(a, b uint32) uint32 { return max(a, b) - min(a, b) }
	switch d := max(diff(r1, r2), diff(g1, g2), diff(b1, b2)); {
	case d < 0x0a00: // about 4%
		return 0
	case d < 0x2600: // 15%
		return fills[0]
	case d < 0x5a00: // 35%
		return fills[1]
	case d < 0x9900: // 60%
		return fills[2]
	}
	return fills[3]
}

// overFill returns the dots of a glyph over a fill, and whether they take
// the fill's color: a sparse fill is drawn with the glyph, and the glyph is
// cut out of a denser one.
func overFill(glyph, fill uint8) (dots uint8, filled bool) {
	switch {
	case fill == 0:
		return glyph, false
	case glyph == 0:
		return fill, true
	case fill == fills[0]:
		return glyph | fill, false
	}
	return fill &^ glyph, true
}

// brailleGlyphs holds the braille glyph of each pattern of dots.
var brailleGlyphs = func() (g [256]string) {
	for i := range g {
		g[i] = string(rune(0x2800 + i))
	}
	return g
}()

// dot returns the bit of the braille dot in column col, 0 or 1, and row
// row, 0 to 3 from the top.
func dot(col, row int) uint8 {
	if row == 3 {
		return 0x40 << col
	}
	return 1 << (row + 3*col)
}

// dots returns the bits of the dots in cols, 0 for the left column, 1 for
// the right, and 2 for both, of rows from to to, inclusive.
func dots(cols, from, to int) uint8 {
	var d uint8
	for row := from; row <= to; row++ {
		if cols != 1 {
			d |= dot(0, row)
		}
		if cols != 0 {
			d |= dot(1, row)
		}
	}
	return d
}

// braillePattern returns the pattern of dots for cell i of a glyph, wide
// when it covers more than one: none for a blank.
func braillePattern(content string, i int, wide bool) uint8 {
	r, size := utf8.DecodeRuneInString(content)
	switch {
	case size == 0, r == ' ', r == utf8.RuneError && size <= 1:
		return 0
	case size == len(content) && r < utf8.RuneSelf:
		return asciiDots[r]
	case r >= 0x2800 && r <= 0x28ff:
		return uint8(r - 0x2800) //nolint:gosec // in range.
	case r >= 0x2500 && r <= 0x257f:
		return boxDots[r-0x2500]
	case r >= 0x2580 && r <= 0x259f:
		return blockDots[r-0x2580]
	case unicode.IsSpace(r):
		return 0
	}

	// Anything else: a pattern picked by the glyph, as tall as a letter of
	// its kind, or denser for a wide glyph.
	seed := uint32(i) //nolint:gosec // a small part index.
	for j := range len(content) {
		seed = seed*16777619 ^ uint32(content[j])
	}
	switch {
	case wide:
		return pick(wideShapes, r, seed)
	case unicode.IsLower(r):
		return pick(lowerShapes, r, seed)
	case unicode.IsLetter(r), unicode.IsDigit(r):
		return pick(tallShapes, r, seed)
	}
	return pick(symbolShapes, r, seed)
}

// pick returns one of shapes for a glyph.
func pick(shapes []uint8, r rune, seed uint32) uint8 {
	return shapes[int(hash(int(r), 0, seed)*float64(len(shapes)))]
}

// The shapes a glyph's pattern is picked from: the four dots between the
// baseline and x-height, for lowercase letters; dots reaching from the cap
// height to the baseline, for capitals, digits, and letters without case;
// a few dots anywhere, for symbols; and more of them, for wide glyphs.
var lowerShapes, tallShapes, symbolShapes, wideShapes = func() (lower, tall, symbol, wide []uint8) {
	top, mid, base := dots(2, 0, 0), dots(2, 1, 1), dots(2, 2, 2)
	for m := 1; m < 256; m++ {
		d := uint8(m) //nolint:gosec // m is below 256.
		n := bits.OnesCount8(d)
		if d&^(mid|base) == 0 && n >= 2 {
			lower = append(lower, d)
		}
		if d&^(top|mid|base) == 0 && d&top != 0 && d&base != 0 && n >= 3 && n <= 5 {
			tall = append(tall, d)
		}
		if n >= 2 && n <= 4 {
			symbol = append(symbol, d)
		}
		if n >= 4 && n <= 6 {
			wide = append(wide, d)
		}
	}
	return lower, tall, symbol, wide
}()

// asciiDots holds the pattern of each ASCII glyph. Punctuation is drawn
// after its shape, and letters and digits are picked from the shapes for
// their kind, with ascenders and descenders.
var asciiDots = func() (a [128]uint8) {
	d := dot
	shapes := map[byte]uint8{
		'.': d(0, 2), ',': d(1, 2) | d(0, 3), ':': d(0, 0) | d(0, 2), ';': d(1, 0) | d(1, 2) | d(0, 3),
		'\'': d(0, 0), '`': d(0, 0), '"': d(0, 0) | d(1, 0), '^': d(0, 1) | d(1, 0),
		'-': dots(2, 1, 1), '_': dots(2, 3, 3), '=': dots(2, 0, 0) | dots(2, 2, 2), '~': d(0, 2) | d(1, 1),
		'|': dots(0, 0, 3), '!': dots(0, 0, 1) | d(0, 3),
		'/': d(1, 0) | d(1, 1) | d(0, 2) | d(0, 3), '\\': d(0, 0) | d(0, 1) | d(1, 2) | d(1, 3),
		'(': d(1, 0) | d(0, 1) | d(0, 2) | d(1, 3), ')': d(0, 0) | d(1, 1) | d(1, 2) | d(0, 3),
		'[': dots(0, 0, 3) | d(1, 0) | d(1, 3), ']': dots(1, 0, 3) | d(0, 0) | d(0, 3),
		'{': d(1, 0) | d(0, 1) | d(1, 1) | d(0, 2) | d(1, 3), '}': d(0, 0) | d(0, 1) | d(1, 1) | d(1, 2) | d(0, 3),
		'<': d(1, 0) | d(0, 1) | d(1, 2), '>': d(0, 0) | d(1, 1) | d(0, 2),
		'+': dots(0, 0, 2) | d(1, 1), '*': d(0, 0) | d(1, 0) | d(0, 1) | d(1, 1),
		'#': dots(2, 0, 2), '%': d(0, 0) | d(1, 1) | d(0, 2) | d(1, 2) | d(1, 0),
		'?': d(0, 0) | d(1, 0) | d(1, 1) | d(0, 3), '@': dots(2, 0, 2) | d(1, 3),
		'$': d(1, 0) | d(0, 1) | d(1, 2) | d(0, 3) | dots(0, 1, 2), '&': d(0, 0) | d(0, 1) | d(1, 1) | dots(2, 2, 2),
		// The stems and their dots.
		'i': d(0, 0) | d(0, 2) | d(1, 2), 'j': d(1, 0) | d(1, 2) | dots(2, 3, 3),
	}
	for c := byte('!'); c <= '~'; c++ {
		r, seed := rune(c), uint32(c)
		switch {
		case shapes[c] != 0:
			a[c] = shapes[c]
		case c >= 'a' && c <= 'z':
			a[c] = pick(lowerShapes, r, seed)
			switch c {
			case 'l':
				a[c] = dots(0, 0, 2) | a[c]&d(1, 2)
			case 'd':
				a[c] |= d(1, 0)
			case 'b', 'f', 'h', 'k', 't':
				a[c] |= d(0, 0)
			}
			switch c {
			case 'p':
				a[c] |= d(0, 3)
			case 'g', 'q', 'y':
				a[c] |= d(1, 3)
			}
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			a[c] = pick(tallShapes, r, seed)
		default:
			a[c] = pick(symbolShapes, r, seed)
		}
	}
	return a
}()

// boxDots holds the pattern of each box-drawing glyph, from U+2500: lines
// along the second row and the left column, so that they meet across cells
// as the glyphs do.
var boxDots = func() (b [128]uint8) {
	const (
		left = 1 << iota
		right
		up
		down
	)
	arms := map[int]string{
		left | right:             "─━┄┅┈┉╌╍═╼╾",
		up | down:                "│┃┆┇┊┋╎╏║╽╿",
		right | down:             "┌┍┎┏╒╓╔╭",
		left | down:              "┐┑┒┓╕╖╗╮",
		right | up:               "└┕┖┗╘╙╚╰",
		left | up:                "┘┙┚┛╛╜╝╯",
		up | down | right:        "├┝┞┟┠┡┢┣╞╟╠",
		up | down | left:         "┤┥┦┧┨┩┪┫╡╢╣",
		left | right | down:      "┬┭┮┯┰┱┲┳╤╥╦",
		left | right | up:        "┴┵┶┷┸┹┺┻╧╨╩",
		left | right | up | down: "┼┽┾┿╀╁╂╃╄╅╆╇╈╉╊╋╪╫╬",
		left:                     "╴╸",
		up:                       "╵╹",
		right:                    "╶╺",
		down:                     "╷╻",
	}
	for a, glyphs := range arms {
		d := dot(0, 1)
		if a&right != 0 {
			d |= dot(1, 1)
		}
		if a&up != 0 {
			d |= dot(0, 0)
		}
		if a&down != 0 {
			d |= dots(0, 2, 3)
		}
		for _, r := range glyphs {
			b[r-0x2500] = d
		}
	}
	slash := dot(1, 0) | dot(1, 1) | dot(0, 2) | dot(0, 3)
	backslash := dot(0, 0) | dot(0, 1) | dot(1, 2) | dot(1, 3)
	b['╱'-0x2500], b['╲'-0x2500], b['╳'-0x2500] = slash, backslash, slash|backslash
	return b
}()

// blockDots holds the pattern of each block element, from U+2580: the dots
// each covers, the nearest when it covers part of one, and dithers for the
// shades.
var blockDots = [32]uint8{
	dots(2, 0, 1), dots(2, 3, 3), dots(2, 3, 3), dots(2, 2, 3), // ▀ ▁ ▂ ▃
	dots(2, 2, 3), dots(2, 1, 3), dots(2, 1, 3), dots(2, 1, 3), // ▄ ▅ ▆ ▇
	0xff, 0xff, 0xff, dots(0, 0, 3), // █ ▉ ▊ ▋
	dots(0, 0, 3), dots(0, 0, 3), dots(0, 0, 3), dots(0, 0, 3), // ▌ ▍ ▎ ▏
	dots(1, 0, 3), dot(0, 0) | dot(1, 2), // ▐ ░
	dot(0, 0) | dot(1, 1) | dot(0, 2) | dot(1, 3), 0xff &^ (dot(1, 0) | dot(0, 2)), // ▒ ▓
	dots(2, 0, 0), dots(1, 0, 3), dots(0, 2, 3), dots(1, 2, 3), // ▔ ▕ ▖ ▗
	dots(0, 0, 1), dots(0, 0, 3) | dots(1, 2, 3), dots(0, 0, 1) | dots(1, 2, 3), dots(2, 0, 1) | dots(0, 2, 3), // ▘ ▙ ▚ ▛
	dots(2, 0, 1) | dots(1, 2, 3), dots(1, 0, 1), dots(1, 0, 1) | dots(0, 2, 3), dots(1, 0, 3) | dots(0, 2, 3), // ▜ ▝ ▞ ▟
}
