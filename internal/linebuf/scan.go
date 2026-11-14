// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package linebuf keeps the lines of text a viewport shows, and works out
// what it draws of them: how wide each line is, the rows it wraps to, and
// the cells of a row as a string that draws them, as fast as a line of a
// log a million lines long needs.
package linebuf

import (
	"strings"

	"bloomfx.dev/bloom/internal/escape"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// tabStop is how many columns apart the stops a tab in a [Store] moves to
// are, as Lip Gloss draws a tab four spaces wide.
const tabStop = 4

// glyphKind is what [scanner.next] found.
type glyphKind uint8

const (
	glyphEnd  glyphKind = iota
	glyphText           // a glyph, or a space a tab is drawn as
	glyphSeq            // a style or hyperlink sequence
	glyphPass           // a command passed to the terminal, when kept
)

// scanner walks a line of a buffer as a viewport draws it: glyph by glyph,
// measured as the canvas measures them when it parses a string, with tabs
// drawn as spaces to the next stop, and control characters and escape
// sequences other than styles and hyperlinks dropped. With a parser, it
// follows the style text is drawn in, from the style the line starts in,
// and when pass is set, it finds the commands that [escape.PassThrough]
// keeps too.
type scanner struct {
	s      string
	i      int
	state  byte
	method ansi.Method
	p      *ansi.Parser
	pass   bool
	pen    pen
	adv    int // cells the last glyph took, to move past before the next
	tabs   int // spaces left of a tab

	// What next found: a glyph's text and width, starting at cell x and
	// byte at of s, or a sequence's text.
	g        string
	w, x, at int
	tab      bool // g is a space of a tab
	tabFirst bool // and the first of them
}

func newScanner(s string, method ansi.Method) scanner {
	return scanner{s: s, method: method}
}

// next finds the next glyph or sequence of the line.
func (sc *scanner) next() glyphKind {
	sc.x += sc.adv
	sc.adv = 0
	if sc.tabs > 0 {
		sc.tabs--
		sc.g, sc.w, sc.adv, sc.tab, sc.tabFirst = " ", 1, 1, true, false
		return glyphText
	}
	sc.tab, sc.tabFirst = false, false
	for sc.i < len(sc.s) {
		at := sc.i
		if c := sc.s[at]; sc.state == 0 && c >= 0x20 && c < 0x7f && (at+1 == len(sc.s) || sc.s[at+1] < 0x80) {
			sc.i++
			sc.g, sc.w, sc.adv, sc.at = sc.s[at:at+1], 1, 1, at
			return glyphText
		}
		// Decoding a sequence with too many parameters with a parser
		// panics, so such a one is decoded without, and dropped.
		p := sc.p
		if c := sc.s[at]; (c == ansi.ESC || c == ansi.CSI || c == ansi.DCS) && escape.Overflows(sc.s[at:]) {
			p = nil
		}
		seq, width, n, state := sc.method.DecodeSequenceInString(sc.s[at:], sc.state, p)
		// The decoder's ASCII fast path does not look for combining marks
		// after the character, as the canvas does when it parses.
		if n == 1 && seq[0] > 0x1f && seq[0] < 0x7f && len(sc.s) > at+1 && sc.s[at+1] >= 0xc0 {
			if cluster, cw := ansi.FirstGraphemeCluster(sc.s[at:], sc.method); len(cluster) > 1 {
				seq, width, n = cluster, cw, len(cluster)
			}
		}
		if n <= 0 {
			n = 1
		}
		sc.i, sc.state = at+n, state
		switch {
		case width > 0:
			sc.g, sc.w, sc.adv, sc.at = seq, width, width, at
			return glyphText
		case seq == "\t":
			sc.tabs = tabStop - sc.x%tabStop - 1
			sc.g, sc.w, sc.adv, sc.at, sc.tab, sc.tabFirst = " ", 1, 1, at, true, true
			return glyphText
		case sc.pass && escape.PassThrough(seq):
			sc.g, sc.at = seq, at
			return glyphPass
		case p != nil && ansi.HasCsiPrefix(seq) && p.Command() == 'm':
			uv.ReadStyle(p.Params(), &sc.pen.style)
			sc.g, sc.at = seq, at
			return glyphSeq
		case p != nil && ansi.HasOscPrefix(seq) && p.Command() == 8 && escape.CleanLink(seq):
			uv.ReadLink(p.Data(), &sc.pen.link)
			sc.g, sc.at = seq, at
			return glyphSeq
		case seq != "" && seq[0] >= 0xc0 && !escape.IsC1(seq):
			// A combining mark parted from its glyph by a sequence, which
			// takes no cells of its own. A C1 control is dropped, as a
			// terminal may read it as the start of a sequence.
			sc.g, sc.w, sc.at = seq, 0, at
			return glyphText
		}
	}
	return glyphEnd
}

// simpleLine reports whether s is only printable ASCII, so that each byte
// is a cell.
func simpleLine(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

// lineWidth32 returns how many cells line s takes, and whether a glyph of
// it is wider than one cell.
func lineWidth32(s string, method ansi.Method) (w int, wide bool) {
	if n, ok := asciiWidth(s); ok {
		return n, false
	}
	return scanWidth(s, method)
}

// scanWidth is [lineWidth32] for any line, a glyph at a time.
func scanWidth(s string, method ansi.Method) (w int, wide bool) {
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		wide = wide || sc.w > 1
	}
	return sc.x + sc.adv, wide
}

// What measuring a line tells of it, kept in the bits of its width above
// the cells it takes.
const (
	// wideLine marks a line with a glyph wider than one cell, which cannot
	// be wrapped by counting its cells alone.
	wideLine = 1 << 30
	// plainLine marks a line of only printable ASCII, a cell a byte.
	plainLine = 1 << 29
	// asciiLine marks a line of printable ASCII and whole control
	// sequences and commands, without tabs, which [asciiCut] cuts.
	asciiLine = 1 << 28
	// widthMask is the cells a line takes.
	widthMask = asciiLine - 1
)

// measureLine returns how many cells line s takes, measured with method,
// with what measuring it told of it in the bits above.
func measureLine(s string, method ansi.Method) int32 {
	w, wide := lineWidth32(s, method)
	flags := 0
	switch {
	case wide:
		flags = wideLine
	case simpleLine(s):
		flags = plainLine | asciiLine
	case strings.IndexByte(s, '\t') < 0:
		if _, ok := asciiWidth(s); ok {
			flags = asciiLine
		}
	}
	return int32(min(w, widthMask) | flags) //nolint:gosec // Bounded by the mask.
}

// LongLineBytes is how long a line must be for a viewport to keep
// checkpoints of it, and the rows it wraps to, rather than scan it from
// its start for each row it shows of it, every frame.
const LongLineBytes = 8 << 10

// CheckpointEvery is how many cells apart the checkpoints of a long line
// are, so that cutting a row out of a line megabytes long scans little more
// than the row.
const CheckpointEvery = 1024

// checkpoint is where a glyph of a line starts, and the style it is drawn
// in there, to start scanning the line from.
type checkpoint struct {
	x, i int // its cell, and its byte
	pen  pen
}

// scanner returns a scanner of line s from cp, following its style.
func (cp checkpoint) scanner(s string, method ansi.Method, p *ansi.Parser) scanner {
	return scanner{s: s, i: cp.i, x: cp.x, method: method, p: p, pen: cp.pen}
}

// skipSeq returns the index just past the escape sequence at s[i], an ESC,
// for a control sequence (CSI) or an operating system command (OSC) that is
// whole, or -1 for any other, which only [scanner] knows how to take.
func skipSeq(s string, i int) int {
	if i+1 >= len(s) {
		return -1
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			return j + 1
		}
	case ']':
		// One with anything but printable ASCII in it, such as an escape
		// that starts a sequence of its own, is left for the scanner.
		return escape.OSCEnd(s, i+2)
	}
	return -1
}

// asciiWidth returns how many cells line s takes, and true, when it is only
// printable ASCII, tabs, and whole control sequences and commands, which
// take no cells, as most logs are; otherwise it returns false.
func asciiWidth(s string) (w int, ok bool) {
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c >= 0x20 && c < 0x7f:
			w++
			i++
		case c == '\t':
			w += tabStop - w%tabStop
			i++
		case c == ansi.ESC:
			if i = skipSeq(s, i); i < 0 {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	return w, true
}

// penAfter returns the style text is drawn in at the end of line s, which
// starts in start. Only its escape sequences are read.
func penAfter(s string, start pen) pen {
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	at := start
	for i := strings.IndexByte(s, ansi.ESC); i >= 0 && i < len(s); {
		// Decoding a sequence with too many parameters with a parser
		// panics, and such a one is not a style.
		q := p
		if escape.Overflows(s[i:]) {
			q = nil
		}
		seq, _, n, _ := ansi.GraphemeWidth.DecodeSequenceInString(s[i:], 0, q)
		switch {
		case q == nil:
		case ansi.HasCsiPrefix(seq) && p.Command() == 'm':
			uv.ReadStyle(p.Params(), &at.style)
		case ansi.HasOscPrefix(seq) && p.Command() == 8:
			uv.ReadLink(p.Data(), &at.link)
		}
		i += max(n, 1)
		next := strings.IndexByte(s[i:], ansi.ESC)
		if next < 0 {
			break
		}
		i += next
	}
	return at
}

// PlainText returns the text of line s without its styles, with its tabs.
func PlainText(s string, method ansi.Method) string {
	if simpleLine(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	sc := newScanner(s, method)
	for sc.next() != glyphEnd {
		switch {
		case sc.tabFirst:
			b.WriteByte('\t')
		case !sc.tab:
			b.WriteString(sc.g)
		}
	}
	return b.String()
}

// pen is the style and hyperlink that text is drawn in at some point of a
// line.
type pen struct {
	style uv.Style
	link  uv.Link
}

func (p *pen) zero() bool { return ZeroStyle(&p.style) && p.link.URL == "" }

func (p *pen) equal(o *pen) bool { return p.style.Equal(&o.style) && p.link == o.link }

func ZeroStyle(s *uv.Style) bool {
	return s.Fg == nil && s.Bg == nil && s.UnderlineColor == nil && s.Underline == 0 && s.Attrs == 0
}
