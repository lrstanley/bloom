// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import "github.com/charmbracelet/x/ansi"

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

// scanWrap is [wrapLine] for any line, a glyph at a time.
func scanWrap(s string, method ansi.Method, w int, mode Mode, starts []int) []int {
	words := mode == Words
	// open is set where a row may start at the next glyph: after spaces, or
	// a hyphen.
	start, brk, open := 0, -1, false
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		x, gw := sc.x, sc.w
		if gw == 0 {
			continue
		}
		if words {
			if sc.g == " " {
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
