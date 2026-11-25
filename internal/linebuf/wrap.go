// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Mode is how lines wider than a viewport are fitted to it.
type Mode uint8

const (
	// None shows lines whole, a row each.
	None Mode = iota
	// Words wraps lines at the spaces between words, and after hyphens,
	// breaking a word only when it is wider than the viewport.
	Words
	// Chars wraps lines at the viewport's width, wherever it falls.
	Chars
)

// wrapLine appends to starts the cells line s's rows start at, wrapped to w
// cells with mode, the first at 0, and returns how many rows it takes.
//
// A row with words mode ends at the start of the last word that fits, or
// just after the last hyphen that does, and the spaces before that word are
// its own, hanging past its width, unseen.
// A glyph wider than w takes a row of its own.
func wrapLine(s string, method ansi.Method, w int, mode Mode, starts []int) (rows int, out []int) {
	starts = append(starts, 0)
	if w <= 0 {
		return 1, starts
	}
	if fast, ok := asciiWrap(s, w, mode, starts); ok {
		return len(fast), fast
	}
	starts = scanWrap(s, method, w, mode, starts)
	return len(starts), starts
}

// breakable reports whether glyph g is a space a row wrapped at words may
// start after, which hangs off the row before it: a space, or another
// Unicode space but those meant not to break a line, as x/ansi wraps text.
func breakable(g string) bool {
	if g == " " {
		return true
	}
	if g == "" || g[0] < utf8.RuneSelf {
		return false
	}
	// A space with a mark on it is a glyph to keep.
	r, n := utf8.DecodeRuneInString(g)
	return n == len(g) && unicode.IsSpace(r) && r != '\u00a0' && r != '\u2007' && r != '\u202f'
}

// scanWrap is [wrapLine] for any line, a glyph at a time.
func scanWrap(s string, method ansi.Method, w int, mode Mode, starts []int) []int {
	words := mode == Words
	// open is set where a row may start at the next glyph: after spaces
	// (see [breakable]), or a hyphen.
	start, brk, open := 0, -1, false
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		x, gw := sc.x, sc.w
		if gw == 0 {
			continue
		}
		if words {
			if breakable(sc.g) {
				open = true
				continue
			}
			if open {
				brk = x
			}
			open = sc.g == "-"
		}
		if x+gw-start <= w || x == start {
			continue
		}
		if words && brk > start {
			start = brk
			starts = append(starts, start)
		}
		if x+gw-start > w && x > start {
			start = x
			starts = append(starts, start)
		}
	}
	return starts
}

// asciiWrap is [wrapLine] for a line that [asciiWidth] can measure, a cell
// a byte, which it wraps a word at a time rather than a glyph at a time. It
// returns false for any other line, with starts as it was given.
func asciiWrap(s string, w int, mode Mode, starts []int) ([]int, bool) {
	given := len(starts)
	if mode == Chars {
		// Every cell is as wide as the next: a row every w cells.
		width, ok := asciiWidth(s)
		if !ok {
			return starts, false
		}
		for at := w; at < width; at += w {
			starts = append(starts, at)
		}
		return starts, true
	}
	start := 0
	// fit breaks the rows of the run of cells from ws up to we, which a
	// space came before when spaced.
	fit := func(ws, we int, spaced bool) {
		if we-start <= w {
			return
		}
		if spaced && ws > start {
			start = ws
			starts = append(starts, start)
		}
		for we-start > w {
			start += w
			starts = append(starts, start)
		}
	}
	// ws is where the run of a word's cells being read starts, spaced is
	// set once a row may start at a run, and hyphen when the last run
	// ended with one, after which a row may start.
	x, ws, spaced, hyphen := 0, -1, false, false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c > ' ' && c < 0x7f:
			if hyphen && ws >= 0 {
				fit(ws, x, spaced)
				ws, spaced = -1, true
			}
			// A run of a word's characters, a cell each, up to a hyphen.
			j := i + 1
			for j < len(s) && s[j] > ' ' && s[j] < 0x7f && s[j-1] != '-' {
				j++
			}
			if ws < 0 {
				ws = x
			}
			x += j - i
			i = j
			hyphen = s[j-1] == '-'
			continue
		case c == ' ' || c == '\t':
			if ws >= 0 {
				fit(ws, x, spaced)
				ws = -1
			}
			hyphen = false
			if c == '\t' {
				x += tabStop - x%tabStop
			} else {
				x++
			}
			spaced = true
			i++
			continue
		case c == ansi.ESC:
			if i = skipSeq(s, i); i >= 0 {
				continue
			}
		}
		return starts[:given], false
	}
	if ws >= 0 {
		fit(ws, x, spaced)
	}
	return starts, true
}

// Wrap returns text s with its lines wrapped at words to w cells, measured
// with method, into the rows a viewport wrapping with [Words] shows them
// in: a newline starts each row past a line's first, in place of the spaces
// that would hang off the row before it, and the spaces that end a line
// past w are dropped. A tab counts as a space a cell wide rather than up to
// the next stop, as the canvas draws none for it in a text, where a viewport
// draws spaces. A w of zero or less leaves s as it is, as does a text with
// no line longer than w in bytes, which is returned without a copy.
func Wrap(s string, method ansi.Method, w int) string {
	if w <= 0 || len(s) <= w {
		return s // a cell takes at least a byte, so every line fits
	}
	var (
		b       strings.Builder
		starts  []int
		blank   [][2]int // scratch for breakLine
		done    int      // the bytes of s written to b
		wrapped bool
	)
	for at := 0; at <= len(s); {
		end := strings.IndexByte(s[at:], '\n')
		if end < 0 {
			end = len(s)
		} else {
			end += at
		}
		if line := s[at:end]; len(line) > w {
			if !wrapped {
				b.Grow(len(s) + len(s)/16)
				wrapped = true
			}
			b.WriteString(s[done:at])
			// Measured with its tabs as spaces, which are a byte each too.
			probe := line
			if strings.IndexByte(line, '\t') >= 0 {
				probe = strings.ReplaceAll(line, "\t", " ")
			}
			_, starts = wrapLine(probe, method, w, Words, starts[:0])
			blank = breakLine(&b, line, probe, method, w, starts, blank[:0])
			done = end
		}
		at = end + 1
	}
	if !wrapped {
		return s
	}
	b.WriteString(s[done:])
	return b.String()
}

// breakLine writes line to b as [Wrap] wraps it, its rows starting at the
// cells starts gives, the first at 0, measured from probe, which is line
// with its tabs as spaces. It returns blank, the scratch it was given.
func breakLine(b *strings.Builder, line, probe string, method ansi.Method, w int, starts []int, blank [][2]int) [][2]int {
	// blank holds the bytes of the spaces since the last glyph that is not
	// one, which are dropped before a row that starts past them, or at the
	// end of a line too wide for them. A row starts past the last of them,
	// or the last glyph, so sequences after them start it as well.
	from, k, after := 0, 1, 0 // the bytes of line written, the next row, and where it may start
	cut := func(to int) {
		for _, sp := range blank {
			b.WriteString(line[from:sp[0]])
			from = sp[1]
		}
		b.WriteString(line[from:to])
		from, blank = to, blank[:0]
	}
	sc := newScanner(probe, method)
	for kind := sc.next(); kind != glyphEnd; kind = sc.next() {
		switch {
		case kind != glyphText || sc.w == 0:
			continue
		case breakable(sc.g):
			blank = append(blank, [2]int{sc.at, sc.at + len(sc.g)})
			after = sc.at + len(sc.g)
			continue
		}
		// Rows start at glyphs that are not spaces, each at its own.
		if k < len(starts) && sc.x == starts[k] {
			cut(after)
			b.WriteByte('\n')
			k++
		}
		blank, after = blank[:0], sc.i
	}
	// Past the last glyph, the scanner is at the end of the line.
	if len(blank) > 0 && sc.x-starts[len(starts)-1] > w {
		cut(len(line))
		return blank
	}
	b.WriteString(line[from:])
	return blank
}
