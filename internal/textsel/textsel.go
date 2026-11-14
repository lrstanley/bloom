// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package textsel works out the text of cells selected with the mouse: the
// words of a row, the text of rows of cells, and how the rows of wrapped
// text join again once copied.
package textsel

import (
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/words"
)

// Words finds the words of rows of cells, for selecting a word with a
// double click. Its scratch is kept from one call to the next.
type Words struct {
	// Chars are the characters, besides letters, digits, and symbols, that
	// are part of a word, so that a path is one.
	Chars string

	row  []byte
	offs []int
	segs []wordSeg
}

// At returns the columns of the word at column p of a row w cells wide,
// whose cells cell returns, inclusive, and whether there is one there.
func (ws *Words) At(w, p int, cell func(x int) *uv.Cell) (x0, x1 int, ok bool) {
	if w <= 0 || p >= w {
		return 0, 0, false
	}
	row, offs := ws.row[:0], ws.offs[:0]
	for x := range w {
		c := cell(x)
		offs = append(offs, len(row))
		switch {
		case c == nil || c.Content == "" && c.Width != 0:
			row = append(row, ' ')
		case c.Width == 0:
			// The second half of a wide character: part of the first.
			offs[x] = offs[max(x-1, 0)]
		default:
			row = append(row, c.Content...)
		}
	}
	ws.row, ws.offs = row, offs

	segs := ws.segs[:0]
	at, hit := offs[p], -1
	for it := words.FromBytes(row); it.Next(); {
		if at >= it.Start() && at < it.End() {
			hit = len(segs)
		}
		segs = append(segs, wordSeg{start: it.Start(), end: it.End(), word: ws.wordish(it.Value())})
	}
	ws.segs = segs
	if hit < 0 || !segs[hit].word {
		return 0, 0, false
	}
	lo, hi := hit, hit
	for lo > 0 && segs[lo-1].word {
		lo--
	}
	for hi < len(segs)-1 && segs[hi+1].word {
		hi++
	}
	x0 = -1
	for x, off := range offs {
		if off >= segs[lo].start && off < segs[hi].end {
			if x0 < 0 {
				x0 = x
			}
			x1 = x
		}
	}
	return x0, x1, x0 >= 0
}

// wordSeg is a segment of a row that Unicode text segmentation found, and
// whether it is part of a word.
type wordSeg struct {
	start, end int
	word       bool
}

// wordish reports whether seg, a segment Unicode text segmentation found, is
// part of a word: one with a letter, digit, or symbol such as an emoji, or
// one of only the word characters, which join the words around them.
func (ws *Words) wordish(seg []byte) bool {
	s := string(seg)
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsMark(c) || unicode.Is(unicode.So, c) {
			return true
		}
	}
	for _, c := range s {
		if !strings.ContainsRune(ws.Chars, c) {
			return false
		}
	}
	return true
}

// RowText returns the text of the cells from x0 to x1 of row y, inclusive,
// with a blank where there is no cell, taking whole a wide character that
// x0 is the second half of.
func RowText(cell func(x, y int) *uv.Cell, x0, x1, y int) string {
	for x0 > 0 {
		if c := cell(x0, y); c == nil || c.Width != 0 {
			break
		}
		x0--
	}
	var b strings.Builder
	for x := x0; x <= x1; x++ {
		switch c := cell(x, y); {
		case c == nil || c.Content == "" && c.Width != 0:
			b.WriteByte(' ')
		case c.Width == 0:
			// The second half of a wide character.
		default:
			b.WriteString(c.Content)
		}
	}
	return b.String()
}

// CutText returns the text of full, a line drawn cut to fit with "…" in row
// y of src, that the cells from x0 to x1 of the row select, and whether the
// row was cut. A selection that takes the "…" takes the rest of the line,
// and one that ends just before it, the rest of the word it cut.
func CutText(src *canvas.Canvas, full string, y, x0, x1 int) (string, bool) {
	if y < 0 || y >= src.Height() {
		return "", false
	}
	cut := -1
	for x := src.Width() - 1; x >= 0; x-- {
		c := src.CellAt(x, y)
		if c.Content == "…" {
			cut = x
			break
		}
		if c.Content != "" && c.Content != " " {
			break
		}
	}
	if cut < 0 {
		return "", false
	}
	// What is shown before the cut is where it starts in full, after any
	// padding of its style.
	start := 0
	for start < cut {
		if c := src.CellAt(start, y); c.Content != "" && c.Content != " " {
			break
		}
		start++
	}
	shown := RowText(src.CellAt, start, cut-1, y)
	at := strings.Index(full, shown)
	if at < 0 {
		return "", false
	}
	offset := func(x int) int {
		if x <= start {
			return at
		}
		return at + len(RowText(src.CellAt, start, min(x, cut)-1, y))
	}
	from, to := offset(x0), len(full)
	if x1 < cut {
		to = offset(x1 + 1)
		if x1 == cut-1 {
			for to < len(full) && full[to] != ' ' {
				to++
			}
		}
	}
	return strings.TrimRight(full[from:max(to, from)], " "), true
}

// Join is what comes between two rows of a text node in the text copied.
type Join uint8

const (
	JoinBreak Join = iota // a line break of its own
	JoinSpace             // wrapped where it broke at a space
	JoinNone              // wrapped in the middle of a word
)

// WrapJoins returns what ends each row of src, the cells of text s as
// drawn, in the text copied: the rows s wrapped to fit are joined, as the
// lines they are, by finding each row's text in s in turn. Where a row is
// not found in s, such as a border its style drew, it ends with a break.
func WrapJoins(src *canvas.Canvas, s string) []Join {
	plain := strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	joins := make([]Join, src.Height())
	at, prev := 0, -1
	for y := range joins {
		row := strings.TrimSpace(RowText(src.CellAt, 0, src.Width()-1, y))
		if row == "" {
			prev = -1
			continue
		}
		gap := at
		for gap < len(plain) && (plain[gap] == ' ' || plain[gap] == '\n') {
			gap++
		}
		if !strings.HasPrefix(plain[gap:], row) {
			prev = -1
			continue
		}
		if prev >= 0 && prev == y-1 {
			switch skipped := plain[at:gap]; {
			case strings.Contains(skipped, "\n"):
			case skipped != "":
				joins[prev] = JoinSpace
			default:
				joins[prev] = JoinNone
			}
		}
		at, prev = gap+len(row), y
	}
	return joins
}

// RowSpan returns the columns of row y that a run from from to to takes,
// in an area w cells wide.
func RowSpan(from, to image.Point, y, w int) (x0, x1 int) {
	x0, x1 = 0, w-1
	if y == from.Y {
		x0 = from.X
	}
	if y == to.Y {
		x1 = to.X
	}
	return x0, x1
}

// EdgeStep returns -1 when p is before lo, where what is shown of content
// from start to end starts, and some of it is out of sight there, 1 when p
// is at or past hi, where what is shown ends, and some of it is out of
// sight there, or 0.
func EdgeStep(p, lo, hi, start, end int) int {
	switch {
	case p < lo && start < lo:
		return -1
	case p >= hi && end > hi:
		return 1
	}
	return 0
}

// Contrast returns the Contrast ratio of a and b, as WCAG measures it, from
// 1 for the same luminance to 21 for black and white.
func Contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// luminance returns the relative luminance of c, as WCAG defines it.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
