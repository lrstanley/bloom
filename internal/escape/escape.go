// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package escape tells apart the escape sequences in text that are safe to
// decode and to draw from those that are not.
package escape

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

// PassThrough reports whether seq, a whole escape sequence, is one that
// the canvas keeps in its cells when it parses a string, to write to the
// terminal as it is: an APC, DCS, SOS, or PM command ended by ST.
func PassThrough(seq string) bool {
	if !ansi.HasApcPrefix(seq) && !ansi.HasDcsPrefix(seq) &&
		!ansi.HasSosPrefix(seq) && !ansi.HasPmPrefix(seq) {
		return false
	}
	n := len(seq)
	return n >= 2 && seq[n-2] == ansi.ESC && seq[n-1] == '\\'
}

// IsC1 reports whether glyph, as decoded from a string, is a C1 control
// character encoded as UTF-8, which takes no cells, and which a terminal
// may read as the start of a sequence, such as U+009F for APC.
func IsC1(glyph string) bool {
	return len(glyph) >= 2 && glyph[0] == 0xc2 && glyph[1] >= 0x80 && glyph[1] < 0xa0
}

// Overflows reports whether s starts with a control sequence (CSI) or a
// device control string (DCS) with more parameters than an [ansi.Parser]
// holds, which decoding with one panics on. Such a sequence must be decoded
// without a parser, or not at all.
func Overflows(s string) bool {
	i := introducer(s)
	if i == 0 {
		return false
	}
	// Its body as the decoder takes it: markers, then parameters, which a
	// marker or intermediate byte after them ends.
	for i < len(s) && s[i] >= '<' && s[i] <= '?' {
		i++
	}
	seps := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= ';'; i++ {
		if s[i] == ';' || s[i] == ':' {
			seps++
			if seps >= parser.MaxParamsSize-1 {
				return true
			}
		}
	}
	return false
}

// introducer returns the length of the CSI or DCS introducer s starts with,
// in its one-byte form or as an escape, which the decoder takes with
// intermediate bytes between the two, as in "\x1b [", or 0.
func introducer(s string) int {
	if s != "" && (s[0] == ansi.CSI || s[0] == ansi.DCS) {
		return 1
	}
	if s == "" || s[0] != ansi.ESC {
		return 0
	}
	i := 1
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && (s[i] == '[' || s[i] == 'P') {
		return i + 1
	}
	return 0
}

// DropOverflows returns s without the sequences that [Overflows] reports,
// as the decoder takes them, so that it can be parsed, or s itself when it
// has none.
func DropOverflows(s string) string {
	if !mayOverflow(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	// Where the sequences that are cut short at the end of out start, which
	// the decoder drops, but which would join what comes after the one taken
	// out next, as "\x9b" does in "\x9b\x9b;...;A;...;0m".
	var buf [8]int
	cut := buf[:0]
	var state byte
	for i := 0; i < len(s); {
		seq, _, n, next := ansi.DecodeSequence(s[i:], state, nil)
		if n <= 0 {
			seq, n = s[i:i+1], 1
		}
		state = next
		i += n
		switch {
		case Overflows(seq):
			if len(cut) > 0 {
				out = out[:cut[0]]
				cut = cut[:0]
			}
			continue
		case unfinished(seq):
			cut = append(cut, len(out))
		default:
			cut = cut[:0]
		}
		out = append(out, seq...)
	}
	return string(out)
}

// mayOverflow reports whether s has a byte that [Overflows] reports for
// the rest of s from it, even one inside a character, which the decoder
// may find on its own after a string it ends there.
func mayOverflow(s string) bool {
	for i := range len(s) {
		if c := s[i]; (c == ansi.ESC || c == ansi.CSI || c == ansi.DCS) && Overflows(s[i:]) {
			return true
		}
	}
	return false
}

// unfinished reports whether seq, as the decoder takes it, is one it cut
// short.
func unfinished(seq string) bool {
	if seq == "" || (seq[0] != ansi.ESC && seq[0] < 0x80) {
		return false
	}
	_, _, n, state := ansi.DecodeSequence(seq, 0, nil)
	return n >= len(seq) && state != 0
}

// OSCEnd returns the index past the end of the operating system command
// (OSC) whose contents start at s[i], if it ends with BEL or ST and holds
// only printable ASCII before that, and -1 otherwise.
func OSCEnd(s string, i int) int {
	for j := i; j < len(s); j++ {
		switch c := s[j]; {
		case c == 0x07:
			return j + 1
		case c == ansi.ESC:
			if j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			return -1
		case c < 0x20 || c >= 0x7f:
			return -1
		}
	}
	return -1
}

// linkData is how much of a hyperlink a parser from [ansi.GetParser] holds,
// which cuts one longer short, perhaps inside a character.
const linkData = 4 << 10

// CleanLink reports whether seq, a hyperlink (OSC 8), holds only printable
// characters between its introducer and its terminator, if it has one, so
// that writing it to the terminal again cannot end it early or start
// another sequence inside it.
func CleanLink(seq string) bool {
	switch {
	case strings.HasPrefix(seq, "\x1b]"):
		seq = seq[2:]
	case seq != "" && seq[0] == ansi.OSC:
		seq = seq[1:]
	}
	// The decoder ends it at a one-byte ST too, even one inside a
	// character, which leaves the rest of the character for the hyperlink.
	switch {
	case strings.HasSuffix(seq, "\x1b\\"):
		seq = seq[:len(seq)-2]
	case strings.HasSuffix(seq, "\x07"), strings.HasSuffix(seq, "\x9c"):
		seq = seq[:len(seq)-1]
	}
	if len(seq) > linkData {
		return false
	}
	for i := 0; i < len(seq); {
		c := seq[i]
		if c < utf8.RuneSelf {
			if c < 0x20 || c == 0x7f {
				return false
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(seq[i:])
		if r == utf8.RuneError && n == 1 || r < 0xa0 {
			return false
		}
		i += n
	}
	return true
}
