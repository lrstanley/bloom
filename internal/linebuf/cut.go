// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"image/color"
	"reflect"
	"slices"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Overlay is a style drawn over part of a row, such as a mark or the style
// of its line, as the escape sequence that sets it.
type Overlay struct {
	Start, End int // cells of the line, End exclusive; bytes as given to [MarkCells]
	SGR        string
}

// MarkCells appends to dst marks, given in bytes of the text of line s as
// [PlainText] returns it, as the cells of the line they cover, in order,
// dropping those that cover none or set no style, and those overlapping one
// before them.
func MarkCells(s string, method ansi.Method, marks, dst []Overlay) []Overlay {
	// The marks drawn, in bytes, which do not overlap, so that each glyph
	// is in one at most, found by walking both in order.
	at := len(dst)
	sorted := slices.SortedStableFunc(slices.Values(marks), func(a, b Overlay) int { return a.Start - b.Start })
	for i := range sorted {
		m := &sorted[i]
		if m.End <= m.Start || (len(dst) > at && m.Start < dst[len(dst)-1].End) {
			continue
		}
		if m.SGR != "" {
			dst = append(dst, *m)
		}
	}
	bytes := slices.Clone(dst[at:])
	if len(bytes) == 0 {
		return dst
	}
	marked := dst[at:]
	for k := range marked {
		marked[k].Start, marked[k].End = -1, -1
	}

	// Each glyph is at the byte of the text its first cell comes from.
	sc, plain, k := newScanner(s, method), 0, 0
	for sc.next() != glyphEnd {
		b := plain
		switch {
		case sc.tabFirst:
			plain++
		case sc.tab:
			b = plain - 1
		default:
			plain += len(sc.g)
		}
		for k < len(bytes) && b >= bytes[k].End {
			k++
		}
		if k == len(bytes) {
			break
		}
		if b >= bytes[k].Start {
			o := &marked[k]
			if o.Start < 0 {
				o.Start = sc.x
			}
			o.End = sc.x + sc.w
		}
	}
	out := dst[:at]
	for _, o := range marked {
		if o.Start >= 0 && o.End > o.Start {
			out = append(out, o)
		}
	}
	return out
}

// RowStyle is what is drawn over a row of a line: Line under the text's own
// styles, across all of the row, which it pads with spaces to fill Fill
// cells, and Marks over the cells they cover. Pens keeps the escape
// sequences of the text's styles, which are drawn again over Line after
// each of its own.
type RowStyle struct {
	Line  string
	Marks []Overlay
	Fill  int
	Pens  *SGRs
	Pass  bool // keep the commands that [escape.PassThrough] keeps
}

// SGRs keeps the escape sequences of the styles that text was drawn in,
// as making one allocates, and the same few styles come up row after row,
// such as the colors of a log's levels. A nil SGRs keeps none.
type SGRs struct {
	styles []uv.Style
	sgrs   []string
}

// Of returns the escape sequence of style s.
func (c *SGRs) Of(s *uv.Style) string {
	if c == nil {
		return s.String()
	}
	for i := range c.styles {
		if sameSGR(&c.styles[i], s) {
			return c.sgrs[i]
		}
	}
	if len(c.styles) >= 64 {
		c.styles, c.sgrs = c.styles[:0], c.sgrs[:0]
	}
	sgr := s.String()
	c.styles, c.sgrs = append(c.styles, *s), append(c.sgrs, sgr)
	return sgr
}

// sameSGR reports whether styles a and b are set with the same escape
// sequence: unlike [uv.Style.Equal], a color of the terminal's palette and
// the RGB it stands for differ, as the terminal may show them differently.
func sameSGR(a, b *uv.Style) bool {
	return a.Attrs == b.Attrs && a.Underline == b.Underline &&
		sameColorKind(a.Fg, b.Fg) && sameColorKind(a.Bg, b.Bg) && sameColorKind(a.UnderlineColor, b.UnderlineColor)
}

// sameColorKind reports whether a and b are the same color of the same type.
// The types Lip Gloss and escape sequences make colors of are compared
// directly; others are compared through reflection, which allocates, and
// those that cannot be compared, such as one holding a slice, are never the
// same.
func sameColorKind(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch x := a.(type) {
	case ansi.BasicColor:
		return sameAs(x, b)
	case ansi.IndexedColor:
		return sameAs(x, b)
	case ansi.RGBColor:
		return sameAs(x, b)
	case color.RGBA:
		return sameAs(x, b)
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	return va.Type() == vb.Type() && va.Comparable() && vb.Comparable() && a == b
}

// sameAs reports whether b is x, of the same type.
func sameAs[T comparable](x T, b color.Color) bool {
	y, ok := b.(T)
	return ok && x == y
}

// cutRow returns the cells from a up to b of line s as a string that draws
// them on its own: in the style the line is in at a, with st drawn over
// them, if not nil, and with its styles reset at the end. The line is
// scanned from from, its start or a checkpoint before a, and flags are what
// measuring it told of it. A glyph cut by a is drawn as spaces, and one cut
// by b is left out.
func cutRow(s string, from checkpoint, flags int32, method ansi.Method, a, b int, st *RowStyle) string {
	if st == nil {
		st = &RowStyle{}
	}
	if st.Line == "" && len(st.Marks) == 0 {
		if flags&plainLine != 0 && from.pen.zero() {
			return s[min(a, len(s)):min(max(b, a), len(s))]
		}
		if flags&asciiLine != 0 {
			return asciiCut(s, from, a, b)
		}
	}
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	w := rowWriter{sc: from.scanner(s, method, p), line: st.Line, marks: st.Marks, pens: st.Pens, in: -1}
	w.sc.pass = st.Pass
	w.out.Grow(min(len(s)-from.i, b-a) + 32)

	end := true // the row runs to the end of the line
scan:
	for k := w.sc.next(); k != glyphEnd; k = w.sc.next() {
		if k == glyphSeq {
			w.sequence()
			continue
		}
		if k == glyphPass {
			// Drawn with the glyph after it, if that is in the row.
			w.pass = append(w.pass, w.sc.g...)
			continue
		}
		x, gw := w.sc.x, w.sc.w
		switch {
		case gw == 0:
			// A combining mark goes with the glyph before it.
			if w.started && x <= b {
				w.out.WriteString(w.sc.g)
			}
			continue
		case x+gw <= a:
			w.pass = w.pass[:0]
			continue
		case x+gw > b:
			end = false
			break scan
		}
		w.glyph(x, a)
	}
	if end && w.started && len(w.pass) > 0 {
		// Commands after the line's last glyph are drawn with it, as the
		// canvas draws those after a string's last glyph.
		w.out.Write(w.pass)
	}
	return w.finish(st.Fill)
}

// asciiCut is [cutRow] with nothing drawn over the row, for a line of
// printable ASCII and whole control sequences and commands, without tabs,
// as asciiLine marks it, whose cells it cuts from the line as they are, with
// the style the line is in where they start.
func asciiCut(s string, cp checkpoint, a, b int) string {
	from, to, x, seqs := -1, len(s), cp.x, false
	for i := cp.i; i < len(s); {
		if s[i] == ansi.ESC {
			seqs = true
			if i = skipSeq(s, i); i < 0 {
				break
			}
			continue
		}
		if x == a && from < 0 {
			from = i
		}
		if x == b {
			to = i
			break
		}
		x++
		i++
	}
	if from < 0 {
		return ""
	}
	at := cp.pen
	if seqs {
		at = penAfter(s[cp.i:from], cp.pen)
	}
	if at.zero() && !seqs {
		return s[from:to]
	}
	var out strings.Builder
	out.Grow(to - from + 48)
	if !ZeroStyle(&at.style) {
		out.WriteString(at.style.String())
	}
	if at.link.URL != "" {
		out.WriteString(ansi.SetHyperlink(at.link.URL, at.link.Params))
	}
	out.WriteString(s[from:to])
	out.WriteString(ansi.ResetStyle)
	if at.link.URL != "" || strings.Contains(s[from:to], "\x1b]8;") {
		out.WriteString(ansi.ResetHyperlink())
	}
	return out.String()
}

// rowWriter writes a row of a line as [cutRow] draws it.
type rowWriter struct {
	out     strings.Builder
	sc      scanner
	line    string    // the style of the line, drawn over all of it
	marks   []Overlay // in order
	pens    *SGRs
	mark    int    // the next mark to enter
	pass    []byte // commands to draw with the next glyph
	in      int    // the mark being drawn, or -1
	started bool   // the row's first glyph was written
	styled  bool   // a style was written, to reset at the end
	drawn   int    // cells written
}

// setPen writes the style drawn over the line, and the style the text is in
// over it.
func (w *rowWriter) setPen() {
	if w.line != "" {
		w.out.WriteString(w.line)
		w.styled = true
	}
	if !ZeroStyle(&w.sc.pen.style) {
		w.out.WriteString(w.pens.Of(&w.sc.pen.style))
		w.styled = true
	}
}

// begin starts the row in the style and hyperlink the line is in.
func (w *rowWriter) begin() {
	w.started = true
	w.setPen()
	if l := w.sc.pen.link; l.URL != "" {
		w.out.WriteString(ansi.SetHyperlink(l.URL, l.Params))
	}
}

// leave ends the mark being drawn.
func (w *rowWriter) leave() {
	w.out.WriteString(ansi.ResetStyle)
	w.setPen()
	w.in = -1
}

// moveTo leaves and enters marks for a glyph at cell x.
func (w *rowWriter) moveTo(x int) {
	if w.in >= 0 && w.marks[w.in].End <= x {
		w.leave()
	}
	for w.in < 0 && w.mark < len(w.marks) && w.marks[w.mark].Start <= x {
		if w.marks[w.mark].End > x {
			w.out.WriteString(w.marks[w.mark].SGR)
			w.styled, w.in = true, w.mark
		}
		w.mark++
	}
}

// sequence writes the style or hyperlink sequence the scanner found, once
// the row has started. Under a line's style or a mark, a style the text
// sets is drawn again as the scanner followed it, between the two, as a
// reset in the text would otherwise drop them.
func (w *rowWriter) sequence() {
	if !w.started {
		return
	}
	w.styled = true
	if (w.line == "" && w.in < 0) || !ansi.HasCsiPrefix(w.sc.g) {
		w.out.WriteString(w.sc.g)
		return
	}
	w.out.WriteString(ansi.ResetStyle)
	w.setPen()
	if w.in >= 0 {
		w.out.WriteString(w.marks[w.in].SGR)
	}
}

// glyph writes the glyph the scanner found at cell x, in a row from cell a,
// as spaces when it starts before a.
func (w *rowWriter) glyph(x, a int) {
	if !w.started {
		w.begin()
	}
	w.moveTo(x)
	if len(w.pass) > 0 {
		w.out.Write(w.pass)
		w.pass = w.pass[:0]
	}
	if x < a {
		cut := x + w.sc.w - a
		w.out.WriteString(strings.Repeat(" ", cut))
		w.drawn += cut
		return
	}
	w.out.WriteString(w.sc.g)
	w.drawn += w.sc.w
}

// finish pads the row to fill cells in the line's style, if it has one,
// resets its styles, and returns it.
func (w *rowWriter) finish(fill int) string {
	if w.line != "" && fill > w.drawn {
		if !w.started {
			w.begin()
		}
		if w.in >= 0 {
			w.leave()
		}
		w.out.WriteString(strings.Repeat(" ", fill-w.drawn))
	}
	if w.styled {
		w.out.WriteString(ansi.ResetStyle)
	}
	if w.started && w.sc.pen.link.URL != "" {
		w.out.WriteString(ansi.ResetHyperlink())
	}
	return w.out.String()
}

// cellText returns the text of the glyphs of line s that start in the cells
// from a to b, inclusive, without styles, with a tab once for the spaces of
// a tab, scanning it from from, its start or a checkpoint before a.
func cellText(s string, from checkpoint, method ansi.Method, a, b int) string {
	var out strings.Builder
	sc := from.scanner(s, method, nil)
	taken := false
	for sc.next() != glyphEnd {
		x := sc.x
		if x > b {
			break
		}
		if x < a {
			continue
		}
		switch {
		case sc.w == 0:
			if taken {
				out.WriteString(sc.g)
			}
		case sc.tab:
			if sc.tabFirst || !taken {
				out.WriteByte('\t')
			}
		default:
			out.WriteString(sc.g)
		}
		taken = true
	}
	return out.String()
}
