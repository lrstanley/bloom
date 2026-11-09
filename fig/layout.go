// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"math"
	"strings"
	"unicode/utf8"
)

// The builder below follows C figlet 2.2.5's getletter, smushamt, addchar,
// splitline, and main loop closely, including how it treats rows of
// different lengths, so that its output matches figlet's byte for byte. It
// also records which glyph drew each cell, and every cell where two glyphs'
// characters met.

// cellp is a cell of a row being built: a sub-character, and the glyph that
// drew it, or -1.
type cellp struct {
	s sub
	g int32
}

// placed is a glyph laid out on a line.
type placed struct {
	r   rune // as written
	src int  // byte offset in the text
	gl  *glyph
	// cols is the column of each row's first sub-character. Rows of a glyph
	// can move apart: figlet joins rows of different lengths, and cuts each
	// row to fit a line on its own.
	cols []int
}

// column returns cols for a glyph h rows tall, every row starting in column
// x.
func column(x, h int) []int {
	cols := make([]int, h)
	for i := range cols {
		cols[i] = x
	}
	return cols
}

// lseam is a cell where two glyphs met, on a line being built.
type lseam struct {
	x, y        int
	under, over int32 // the earlier and later glyph
	us, os, res sub
	merge       Merge
}

// inchar is a character of the text: as written, as mapped by the font's
// control file, and where it is.
type inchar struct {
	r, c rune
	src  int
}

// fline is a finished line of text.
type fline struct {
	rows   [][]cellp
	glyphs []placed
	seams  []lseam
}

type builder struct {
	smusher

	stack   Stacking
	track   int
	limit   int // the widest a line may be, figlet's outlinelenlimit
	inLimit int // the most characters a line may hold, figlet's inchrlinelenlimit

	rows   [][]cellp
	outlen int
	in     []inchar
	glyphs []placed
	seams  []lseam
	lines  []fline
}

// hmode returns the smushmode for o, as figlet's options set it.
func hmode(fc *face, o Options) int {
	font := fc.layout
	if o.Rules&hRules != 0 {
		font = int(o.Rules&hRules) | smSmush
	}
	switch o.Horizontal {
	case Full:
		return 0
	case Fitted:
		return smKern
	case Smushed:
		return smSmush | font
	case Universal:
		return smSmush
	case Default:
	}
	return font
}

// vmode returns the vertical layout bits for o.
func vmode(fc *face, o Options) int {
	font := fc.layout & (vRules | svFit | svSmush)
	if o.Rules&vRules != 0 {
		font = int(o.Rules&vRules) | svSmush
	}
	switch o.Vertical {
	case Full:
		return 0
	case Fitted:
		return svFit
	case Smushed:
		return svSmush | font&vRules
	case Universal:
		return svSmush
	case Default:
	}
	if !o.Strict {
		return 0
	}
	return font
}

// layout lays text out, as figlet does.
func (f *Font) layout(text string, o Options) *Figure {
	fc := f.load()
	b := &builder{
		fc: fc, mode: hmode(fc, o), strict: o.Strict, rtl: fc.rtl, stack: o.Stacking, track: o.Tracking,
		limit: math.MaxInt32, inLimit: math.MaxInt32,
	}
	switch o.Direction {
	case LeftToRight:
		b.rtl = false
	case RightToLeft:
		b.rtl = true
	case FontDirection:
	}
	if o.Width > 0 {
		b.limit, b.inLimit = o.Width, (o.Width+1)*4+100
	}
	b.clearline()
	b.run(input(text, o.Paragraph, o.Control))
	if b.outlen != 0 {
		b.printline()
	}
	j := o.Justify
	if j == Auto {
		j = Left
		if b.rtl {
			j = Right
		}
	}
	width := max(o.Width, 0)
	return b.figure(width, j, vmode(fc, o))
}

// input decodes text into the characters figlet's main loop sees: after
// paragraph joining and the control file's mapping, with tabs as spaces,
// other whitespace as line breaks, and other control characters dropped.
func input(text string, paragraph bool, ctl *Control) []inchar {
	out := make([]inchar, 0, len(text))
	lastEOL := false
	for i, r := range text {
		if r == '\r' && strings.HasPrefix(text[i+1:], "\n") {
			continue // a CRLF is one line break, as figlet reads text
		}
		c := r
		if c == '\n' && paragraph && !lastEOL {
			next, _ := utf8.DecodeRuneInString(text[i+1:])
			if i+1 < len(text) && isSpace(next) {
				c = '\n'
			} else {
				c = ' '
			}
		}
		lastEOL = isSpace(c) && c != '\t' && c != ' '
		c = ctl.Map(c)
		if isSpace(c) {
			if c == '\t' || c == ' ' {
				c = ' '
			} else {
				c = '\n'
			}
		}
		if c > 0 && c < ' ' && c != '\n' || c == 127 {
			continue
		}
		out = append(out, inchar{r: r, c: c, src: i})
	}
	return out
}

func isSpace(c rune) bool { return c == ' ' || c >= '\t' && c <= '\r' }

// run is figlet's main loop, which breaks lines at spaces when they get too
// wide.
func (b *builder) run(in []inchar) {
	// wordbreak, as figlet's wordbreakmode:
	//	-1: blanks are absorbed, after a line was broken by a blank or by a
	//	    character wider than a line
	//	 0: nothing but blanks yet, which are not absorbed
	//	 1: no word break yet
	//	 2: after a word, and the blanks after it
	//	 3: in a word, after an earlier word break
	wordbreak := 0
	for _, c := range in {
		for {
			if wordbreak == -1 {
				if c.c == ' ' {
					break
				}
				wordbreak = 0
				if c.c == '\n' {
					break
				}
			}
			retry := false
			switch {
			case c.c == '\n':
				b.printline()
				wordbreak = 0
			case b.addchar(c):
				switch {
				case c.c != ' ' && wordbreak >= 2:
					wordbreak = 3
				case c.c != ' ':
					wordbreak = 1
				case wordbreak > 0:
					wordbreak = 2
				}
			case b.outlen == 0:
				b.oversize(c)
				wordbreak = -1
			case c.c == ' ':
				if wordbreak == 2 {
					b.splitline()
				} else {
					b.printline()
				}
				wordbreak = -1
			default:
				if wordbreak >= 2 {
					b.splitline()
				} else {
					b.printline()
				}
				if wordbreak == 3 {
					wordbreak = 1
				} else {
					wordbreak = 0
				}
				retry = true
			}
			if !retry {
				break
			}
		}
	}
}

func (b *builder) clearline() {
	b.rows = make([][]cellp, b.fc.height)
	b.outlen = 0
	b.in, b.glyphs, b.seams = nil, nil, nil
}

func (b *builder) printline() {
	b.lines = append(b.lines, fline{rows: b.rows, glyphs: b.glyphs, seams: b.seams})
	b.clearline()
}

// oversize puts a glyph too wide for a line on a line of its own, cut to the
// line's width: its right end for right-to-left text, as in figlet.
func (b *builder) oversize(c inchar) {
	gl := b.fc.lookup(c.c)
	cols := make([]int, len(gl.rows))
	for i, row := range gl.rows {
		cut := 0
		if b.rtl && len(row) > b.limit {
			// Right to left, each row keeps its own end.
			cut = len(row) - b.limit
			cols[i] = -cut
		}
		b.rows[i] = owned(row[cut:], 0)
	}
	b.glyphs = []placed{{r: c.r, src: c.src, gl: gl, cols: cols}}
	b.printline()
}

// owned returns subs as cells drawn by glyph g, blanks drawn by none.
func owned(subs []sub, g int32) []cellp {
	cells := make([]cellp, len(subs))
	for i, s := range subs {
		cells[i] = cellp{s: s, g: -1}
		if !blank(s) {
			cells[i].g = g
		}
	}
	return cells
}

// splitline breaks the line at its last run of blanks, prints the part
// before it, and starts a new line with the part after it.
func (b *builder) splitline() {
	in := b.in
	gotspace, last, i := false, len(in)-1, len(in)-1
	for ; i >= 0; i-- {
		if !gotspace && in[i].c == ' ' {
			gotspace, last = true, i
		}
		if gotspace && in[i].c != ' ' {
			break
		}
	}
	part1 := append([]inchar(nil), in[:i+1]...)
	part2 := append([]inchar(nil), in[last+1:]...)
	b.clearline()
	for _, c := range part1 {
		b.addchar(c)
	}
	b.printline()
	for _, c := range part2 {
		b.addchar(c)
	}
}

func (fc *face) lookup(c rune) *glyph {
	if g, ok := fc.glyphs[c]; ok {
		return g
	}
	return fc.missing
}

func subAt(row []sub, i int) sub {
	if i < 0 || i >= len(row) {
		return subNone
	}
	return row[i]
}

func cellAt(row []cellp, i int) cellp {
	if i < 0 || i >= len(row) {
		return cellp{s: subNone, g: -1}
	}
	return row[i]
}

// addchar adds c to the line, reporting false if the line has no room for
// it.
func (b *builder) addchar(c inchar) bool {
	gl := b.fc.lookup(c.c)
	b.smusher.prev, b.smusher.cur = b.smusher.cur, gl.width
	amt := b.smushamt(gl)
	if b.track != 0 && b.outlen > 0 {
		amt = min(amt-b.track, gl.width, b.outlen)
	}
	if b.outlen+gl.width-amt > b.limit || len(b.in)+1 > b.inLimit {
		return false
	}
	if amt < 0 {
		b.gap(-amt)
		b.outlen, amt = len(b.rows[0]), 0
	}
	gi := int32(len(b.glyphs)) //nolint:gosec // glyphs are counted from the text, far under 2^31.
	if b.rtl {
		b.addRTL(gl, amt, gi)
		b.glyphs = append(b.glyphs, placed{r: c.r, src: c.src, gl: gl, cols: column(0, b.fc.height)})
	} else {
		b.glyphs = append(b.glyphs, placed{r: c.r, src: c.src, gl: gl, cols: column(b.outlen-amt, b.fc.height)})
		b.addLTR(gl, amt, gi)
	}
	b.outlen = len(b.rows[0])
	b.in = append(b.in, c)
	return true
}

// gap puts n blank columns where the next glyph goes, for a positive
// [Options.Tracking].
func (b *builder) gap(n int) {
	blanks := make([]cellp, n)
	for i := range blanks {
		blanks[i] = cellp{s: subSpace, g: -1}
	}
	for i, row := range b.rows {
		if b.rtl {
			b.rows[i] = append(append([]cellp(nil), blanks...), row...)
		} else {
			b.rows[i] = append(row, blanks...)
		}
	}
	if b.rtl {
		for i := range b.seams {
			b.seams[i].x += n
		}
		for _, p := range b.glyphs {
			for r := range p.cols {
				p.cols[r] += n
			}
		}
	}
}

// smushamt returns how far the glyph can move into the line, as figlet's
// smushamt does.
func (b *builder) smushamt(gl *glyph) int {
	if b.mode&(smSmush|smKern) == 0 {
		return 0
	}
	best := gl.width
	for row := range b.fc.height {
		line, ch := b.rows[row], gl.rows[row]
		var ch1, ch2 sub
		var amt int
		if b.rtl {
			best = min(best, len(line))
			charbd := len(ch)
			for ch1 = subAt(ch, charbd); charbd > 0 && blank(ch1); ch1 = subAt(ch, charbd) {
				charbd--
			}
			linebd := 0
			for ch2 = cellAt(line, 0).s; ch2 == subSpace; ch2 = cellAt(line, linebd).s {
				linebd++
			}
			amt = linebd + gl.width - 1 - charbd
		} else {
			linebd := len(line)
			for ch1 = cellAt(line, linebd).s; linebd > 0 && blank(ch1); ch1 = cellAt(line, linebd).s {
				linebd--
			}
			charbd := 0
			for ch2 = subAt(ch, 0); ch2 == subSpace; ch2 = subAt(ch, charbd) {
				charbd++
			}
			amt = charbd + b.outlen - 1 - linebd
		}
		if blank(ch1) {
			amt++
		} else if ch2 != subNone {
			if s, _ := b.smushem(ch1, ch2); s != subNone {
				amt++
			}
		}
		best = min(best, amt)
	}
	return best
}

// merge merges l and r, the cells on the left and right, where newer is the
// glyph being added, on the right unless rtl. It records a seam when both
// cells hold something.
func (b *builder) merge(l, r cellp, x, y int) cellp {
	res, how := b.smushem(l.s, r.s)
	older, newer := l, r
	if b.rtl {
		older, newer = r, l
	}
	full := func(c cellp) bool { return !blank(c.s) }
	if res == subNone && b.track < 0 && full(l) && full(r) {
		res, how = b.stacked(older.s, newer.s), MergeOverlap
	}
	if res == subNone {
		return cellp{s: subNone, g: -1}
	}
	switch {
	case full(older) && full(newer):
		if older.s != subCont && newer.s != subCont {
			b.seams = append(b.seams, lseam{x: x, y: y, under: older.g, over: newer.g, us: older.s, os: newer.s, res: res, merge: how})
		}
		top := newer.g
		if b.stack == EarlierOnTop {
			top = older.g
		}
		return cellp{s: res, g: top}
	case full(newer):
		return cellp{s: res, g: newer.g}
	}
	return cellp{s: res, g: older.g}
}

// stacked returns which of two overlapping characters is drawn on top: the
// newer or older one, as [Options.Stacking] says, unless it is a hardblank.
func (b *builder) stacked(older, newer sub) sub {
	top, under := newer, older
	if b.stack == EarlierOnTop {
		top, under = older, newer
	}
	if top == subHard {
		return under
	}
	return top
}

// put writes c at i of row as C writes a character into a string: past its
// end it is lost, at its end it extends it, and a NUL ends it.
func put(row []cellp, i int, c cellp) []cellp {
	switch {
	case i < len(row) && c.s == subNone:
		return row[:i]
	case i < len(row):
		row[i] = c
	case i == len(row) && c.s != subNone:
		return append(row, c)
	}
	return row
}

func (b *builder) addLTR(gl *glyph, amt int, gi int32) {
	for y := range b.fc.height {
		line, ch := b.rows[y], owned(gl.rows[y], gi)
		for k := range amt {
			col := max(b.outlen-amt+k, 0)
			line = put(line, col, b.merge(cellAt(line, col), cellAt(ch, k), col, y))
		}
		if amt < len(ch) {
			line = append(line, ch[amt:]...)
		}
		b.rows[y] = line
	}
}

func (b *builder) addRTL(gl *glyph, amt int, gi int32) {
	shifts := make([]int, b.fc.height)
	for y := range b.fc.height {
		line, tmp := b.rows[y], owned(gl.rows[y], gi)
		for k := range amt {
			i := gl.width - amt + k
			tmp = put(tmp, i, b.merge(cellAt(tmp, i), cellAt(line, k), i, y))
		}
		shifts[y] = len(tmp) - amt
		if amt < len(line) {
			tmp = append(tmp, line[amt:]...)
		}
		b.rows[y] = tmp
	}
	// What was on the line moved right; the seams just recorded are at
	// their places already.
	fresh := 0
	for i := len(b.seams) - 1; i >= 0 && b.seams[i].over == gi; i-- {
		fresh++
	}
	old := b.seams[:len(b.seams)-fresh]
	for i := range old {
		s := &old[i]
		if s.x < amt {
			s.x += gl.width - amt
		} else {
			s.x += shifts[s.y]
		}
	}
	for _, p := range b.glyphs {
		for r := range p.cols {
			if p.cols[r] < amt {
				p.cols[r] += gl.width - amt
			} else {
				p.cols[r] += shifts[r]
			}
		}
	}
}
