// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
)

// notesLines are the notes, longer than the pane shows.
var notesLines = []string{
	"Refunds that time out upstream are retried three times, 2s apart, with the same idempotency key.",
	"A 502 from payments is not a failed refund: check the retry queue before refunding again.",
	"Order 1042 was refunded by hand at 14:05, and its retries were cancelled.",
	"Latency over 1s on POST /refund pages whoever is on call.",
	"Logs older than 30 days move to cold storage at midnight UTC.",
}

// gutter is how wide the reader's line numbers are.
const gutter = 3

// reader shows lines of text wrapped at words to w cells, beside a gutter
// of line numbers, h rows at a time from row top of all that they wrap to,
// as a component that scrolls its own text does. It implements
// bloom.TextMap, so that a selection in it is kept on its text as it
// scrolls, takes what is scrolled out of sight, and leaves the gutter out.
type reader struct {
	lines    []string
	top      int
	w, h     int
	numStyle lipgloss.Style
}

// row is a row of a reader: the bytes of a line from from up to to.
type row struct {
	line int
	from int
	to   int
}

// rows returns all of the rows the lines wrap to.
func (r *reader) rows() []row {
	var rows []row
	w := max(r.w-gutter, 1)
	for i, l := range r.lines {
		for from := 0; ; {
			to := min(from+w, len(l))
			if sp := strings.LastIndexByte(l[from:to], ' '); to < len(l) && sp > 0 {
				to = from + sp + 1
			}
			rows = append(rows, row{line: i, from: from, to: to})
			if to >= len(l) {
				break
			}
			from = to
		}
	}
	return rows
}

// shown returns the rows in sight.
func (r *reader) shown() []row {
	rows := r.rows()
	top := min(r.top, max(len(rows)-1, 0))
	return rows[top:min(top+r.h, len(rows))]
}

func (r *reader) scroll(dy int) { r.top = min(max(r.top+dy, 0), max(len(r.rows())-r.h, 0)) }

func (r *reader) View() string {
	out := make([]string, 0, r.h)
	for _, s := range r.shown() {
		num := ""
		if s.from == 0 {
			num = strconv.Itoa(s.line + 1)
		}
		out = append(out, r.numStyle.Render(fmt.Sprintf("%*s ", gutter-1, num))+r.lines[s.line][s.from:s.to])
	}
	return strings.Join(out, "\n")
}

func (r *reader) clamp(p bloom.Pos) bloom.Pos {
	last := len(r.lines) - 1
	if p.Line > last {
		return bloom.Pos{Line: last, Col: len(r.lines[last])}
	}
	p.Line = max(p.Line, 0)
	p.Col = min(max(p.Col, 0), len(r.lines[p.Line]))
	return p
}

// PosAt implements bloom.TextMap: a cell in the gutter is the start of its
// row, and one past the text of a row its end.
func (r *reader) PosAt(cell image.Point, _ bloom.Size) bloom.Pos {
	rows := r.shown()
	if cell.Y >= len(rows) {
		return r.clamp(bloom.Pos{Line: len(r.lines)})
	}
	s := rows[max(cell.Y, 0)]
	return bloom.Pos{Line: s.line, Col: s.from + min(max(cell.X-gutter, 0), s.to-s.from)}
}

// Cells implements bloom.TextMap.
func (r *reader) Cells(from, to bloom.Pos, _ bloom.Size, cells func(y, x0, x1 int)) {
	for y, s := range r.shown() {
		if s.line < from.Line || s.line > to.Line {
			continue
		}
		lo, hi := s.from, s.to
		if s.line == from.Line {
			lo = max(lo, from.Col)
		}
		if s.line == to.Line {
			hi = min(hi, to.Col)
		}
		if lo < hi {
			cells(y, gutter+lo-s.from, gutter+hi-s.from)
		}
	}
}

// Text implements bloom.TextMap.
func (r *reader) Text(from, to bloom.Pos) string {
	from, to = r.clamp(from), r.clamp(to)
	var b strings.Builder
	for i := from.Line; i <= to.Line; i++ {
		lo, hi := 0, len(r.lines[i])
		if i == from.Line {
			lo = from.Col
		}
		if i == to.Line {
			hi = to.Col
		}
		if i > from.Line {
			b.WriteByte('\n')
		}
		b.WriteString(r.lines[i][lo:max(hi, lo)])
	}
	return b.String()
}
