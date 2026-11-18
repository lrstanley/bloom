// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package textsel works out the text of cells selected with the mouse: the
// words of a row, the text of rows of cells, and how the rows of wrapped
// text join again once copied; where the ends of a selection are among the
// leaves of text it runs through and the lists between them, how many
// clicks a press is, and how selected cells are drawn.
package textsel

import (
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/words"
)

// Words finds the words of rows of cells, for selecting a word with a
// double click, as [Word] finds them in the text of the row. Its scratch is
// kept from one call to the next.
type Words struct {
	// Chars are the characters, besides letters, digits, and symbols, that
	// are part of a word, so that a path is one.
	Chars string

	row  []byte
	offs []int
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

	from, to, ok := Word(string(row), offs[p], ws.Chars)
	if !ok {
		return 0, 0, false
	}
	x0 = -1
	for x, off := range offs {
		if off >= from && off < to {
			if x0 < 0 {
				x0 = x
			}
			x1 = x
		}
	}
	return x0, x1, x0 >= 0
}

// Word returns the word of s that byte i is in, from byte from up to to,
// and whether i is in one. Words are found as Unicode text segmentation
// (UAX #29) finds them, and a segment is part of a word when it holds a
// letter, digit, mark, or symbol, such as an emoji, or is made only of
// chars, such as "_" or "/"; segments that are part of a word next to each
// other are one word, so that chars join the words either side of them.
//
// Since a word never runs across whitespace, only the text from the
// nearest whitespace before i to the nearest after it is segmented, the
// whitespace included so that it is segmented as it is in s, which keeps
// finding a word in a long line cheap.
func Word(s string, i int, chars string) (from, to int, ok bool) {
	if i < 0 || i >= len(s) {
		return 0, 0, false
	}
	from, to, ok = wordAfter(s, i, chars, false)
	return from, to, ok && from <= i
}

// WordEnd returns the end of the word of s that byte i is in, or of the
// first word after it, as [Word] finds words, and whether there is one.
// Only the text from the nearest whitespace before i to the whitespace
// after that word is segmented.
func WordEnd(s string, i int, chars string) (int, bool) {
	_, to, ok := wordAfter(s, min(max(i, 0), len(s)), chars, true)
	return to, ok
}

// WordStart returns the start of the last word of s that starts before
// byte i, as [Word] finds words, and whether there is one. Only the text
// from the whitespace before that word to the nearest whitespace after i
// is segmented.
func WordStart(s string, i int, chars string) (int, bool) {
	limit := min(max(i, 0), len(s))
	lo, hi := spaceBefore(s, limit), spaceAfter(s, limit)
	for {
		at := -1
		eachWord(s[:hi], lo, chars, func(from, _ int) bool {
			if from >= limit {
				return false
			}
			at = from
			return true
		})
		switch {
		case at >= 0 && (at > lo || lo == 0):
			return at, true
		case at >= 0:
			// The word starts the text segmented, as one joined to a space
			// by a mark on it does, and may start before it.
			lo = spaceBefore(s, lo)
			continue
		case lo == 0:
			return 0, false
		}
		// No word starts between the whitespace and limit: on to the text
		// before the whitespace.
		j := strings.LastIndexFunc(s[:lo], func(r rune) bool { return !unicode.IsSpace(r) })
		if j < 0 {
			return 0, false
		}
		limit, lo, hi = lo, spaceBefore(s, j+1), spaceAfter(s, j)
	}
}

// wordAfter returns the first word of s that ends after byte i, from byte
// from up to to, and whether there is one: among the text around i, up to
// the whitespace either side, or with further, the text after it too.
func wordAfter(s string, i int, chars string, further bool) (from, to int, ok bool) {
	lo, hi := spaceBefore(s, i), spaceAfter(s, i)
	for {
		from, to, ok = 0, 0, false
		eachWord(s[:hi], lo, chars, func(f, t int) bool {
			if t > i {
				from, to, ok = f, t, true
				return false
			}
			return true
		})
		switch {
		case ok && from == lo && lo > 0:
			// The word starts the text segmented, as one joined to a space by
			// a mark on it does, and may start before it.
			lo = spaceBefore(s, lo)
			continue
		case ok || !further || hi >= len(s):
			return from, to, ok
		}
		// No word ends after i before the whitespace: on to the text after
		// it.
		j := strings.IndexFunc(s[hi:], func(r rune) bool { return !unicode.IsSpace(r) })
		if j < 0 {
			return 0, 0, false
		}
		lo, hi = spaceBefore(s, hi+j), spaceAfter(s, hi+j)
	}
}

// spaceBefore returns the byte of s that the last whitespace before byte i
// starts at, or 0 when there is none, where segmenting s for a word at i
// can start.
func spaceBefore(s string, i int) int {
	return max(strings.LastIndexFunc(s[:min(i, len(s))], unicode.IsSpace), 0)
}

// spaceAfter returns the byte of s just past the first whitespace at or
// after byte i, or the end of s when there is none, where segmenting s for
// a word at i can stop: a word never runs on past it. A space that a mark
// or a joiner follows is part of a segment with them, which may be part of
// a word, and is passed over.
func spaceAfter(s string, i int) int {
	for j := max(i, 0); j < len(s); {
		r, n := utf8.DecodeRuneInString(s[j:])
		j += n
		if !unicode.IsSpace(r) {
			continue
		}
		if next, _ := utf8.DecodeRuneInString(s[j:]); j >= len(s) || !unicode.IsMark(next) && !unicode.Is(unicode.Cf, next) {
			return j
		}
	}
	return len(s)
}

// eachWord calls fn with the start and the end of each word of s from byte
// lo on, in turn, until it returns false. Segments are found only as far
// as they are wanted.
func eachWord(s string, lo int, chars string, fn func(from, to int) bool) {
	from, to := -1, 0
	for it := words.FromString(s[lo:]); it.Next(); {
		if wordish(it.Value(), chars) {
			if from < 0 {
				from = lo + it.Start()
			}
			to = lo + it.End()
			continue
		}
		if from >= 0 && !fn(from, to) {
			return
		}
		from = -1
	}
	if from >= 0 {
		fn(from, to)
	}
}

// wordish reports whether seg, a segment Unicode text segmentation found, is
// part of a word: one with a letter, digit, mark, or symbol such as an
// emoji, or one of only chars, which join the words around them.
func wordish(seg, chars string) bool {
	if strings.TrimSpace(seg) == "" {
		return false
	}
	for _, c := range seg {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsMark(c) || unicode.Is(unicode.So, c) {
			return true
		}
	}
	for _, c := range seg {
		if !strings.ContainsRune(chars, c) {
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
