// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/internal/escape"
)

// Sanitize returns s with only its text, tabs, line breaks, styles, and
// hyperlinks, for text from outside the program, such as a log or a file,
// that could hold any escape sequence. Parsing a string drops most
// sequences other than styles and hyperlinks already, but keeps APC, DCS,
// SOS, and PM commands in its cells, to be written to the terminal as they
// are, as images drawn with the Kitty graphics protocol or sixel are, which
// is safe only for what the program made itself. Sanitize takes those out
// too, along with C1 control characters and bytes that are not UTF-8,
// which a terminal may read as the start of one. A string with nothing to
// take out is returned as it is.
func Sanitize(s string) string {
	for !clean(s) {
		t := strip(s)
		if t == s {
			break
		}
		// Taking a sequence out can join what was around it into another,
		// so the result is looked at again.
		s = t
	}
	return s
}

// clean reports whether s has nothing for [strip] to take out, looking at
// each byte once without decoding sequences. Only text, tabs, line breaks,
// styles in their plainest form, and hyperlinks without escapes or bytes
// past ASCII in them are taken as clean; anything else is left for strip.
func clean(s string) bool {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c >= 0x20 && c < 0x7f, c == '\t', c == '\n', c == '\r':
			i++
		case c == ansi.ESC:
			if i = cleanSeq(s, i); i < 0 {
				return false
			}
		case c >= utf8.RuneSelf:
			r, n := utf8.DecodeRuneInString(s[i:])
			if (r == utf8.RuneError && n == 1) || (r >= 0x80 && r < 0xa0) {
				return false
			}
			i += n
		default:
			return false
		}
	}
	return true
}

// cleanSeq returns the index past the sequence at s[i], an ESC, if it is a
// style (SGR) of only numbers and separators or a hyperlink (OSC 8) of only
// printable ASCII, and -1 otherwise.
func cleanSeq(s string, i int) int {
	switch {
	case strings.HasPrefix(s[i:], "\x1b[") && !escape.Overflows(s[i:]):
		j := i + 2
		for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';' || s[j] == ':') {
			j++
		}
		if j < len(s) && s[j] == 'm' {
			return j + 1
		}
	case strings.HasPrefix(s[i:], "\x1b]8;"):
		return escape.OSCEnd(s, i+4)
	}
	return -1
}

// strip returns s with only its text, tabs, line breaks, styles, and
// hyperlinks, decoded as the canvas decodes s when it parses it. It decodes
// without a parser, which a sequence with too many parameters makes panic.
func strip(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	var state byte
	for i := 0; i < len(s); {
		seq, width, n, next := ansi.DecodeSequence(s[i:], state, nil)
		if n <= 0 {
			n = 1
		}
		state = next
		i += n
		switch {
		case escape.IsC1(seq):
		case isSGR(seq), isLink(seq), seq == "\t", seq == "\n", seq == "\r":
			out.WriteString(seq)
		case width > 0, seq != "" && seq[0] >= 0xc0:
			// A glyph, or one that takes no cells, such as a combining mark
			// parted from the glyph before it by a sequence. Bytes of it that
			// are not UTF-8, which a terminal could read as one-byte
			// introducers, are left out, and a glyph of only those is drawn
			// as U+FFFD, in the cells it takes.
			if !utf8.ValidString(seq) {
				if seq = strings.ToValidUTF8(seq, ""); seq == "" && width > 0 {
					seq = strings.Repeat(string(utf8.RuneError), width)
				}
			}
			out.WriteString(seq)
		}
	}
	return out.String()
}

// isSGR reports whether seq, a whole escape sequence, sets a style, with
// no more parameters than a parser holds.
func isSGR(seq string) bool {
	var params string
	switch {
	case strings.HasPrefix(seq, "\x1b["):
		params = seq[2:]
	case seq != "" && seq[0] == ansi.CSI:
		params = seq[1:]
	default:
		return false
	}
	if params == "" || params[len(params)-1] != 'm' || escape.Overflows(seq) {
		return false
	}
	for i := range len(params) - 1 {
		if c := params[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// isLink reports whether seq, a whole escape sequence, is a hyperlink
// (OSC 8) that holds only printable characters.
func isLink(seq string) bool {
	return (strings.HasPrefix(seq, "\x1b]8;") || (seq != "" && seq[0] == ansi.OSC && strings.HasPrefix(seq[1:], "8;"))) &&
		escape.CleanLink(seq)
}
