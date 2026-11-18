// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"math"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RowAt returns the line that row y shows, and the cells of it the row
// takes, from a up to end, which for its last row is past the end of the
// line. Wrapped rows are worked out in scratch.
func (r Index) RowAt(y int, scratch *[]int) (line, a, end int) {
	line, sub := r.LineAt(y)
	if r.Uniform() || r.RowsOf(line) == 1 {
		return line, 0, math.MaxInt
	}
	starts := r.s.RowStarts(line, r.key.method, r.key.width, r.key.mode, scratch)
	a, end = starts[sub], math.MaxInt
	if sub+1 < len(starts) {
		end = starts[sub+1]
	}
	return line, a, end
}

// PosAt returns the position in the text of the character drawn at cell
// of row: the end of the row for a cell past its text, which for a row a
// line wraps at is where the next row starts, the start of the text for a
// row before the first, and its end for one past the last.
func (r Index) PosAt(row, cell int, scratch *[]int) Pos {
	switch {
	case r.s.Len() == 0 || row < 0:
		return Pos{}
	case row >= r.Total():
		return r.s.End()
	}
	line, a, end := r.RowAt(row, scratch)
	return r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, r.rowCell(a, end, cell))})
}

// rowCell returns the cell of a line that cell of its row, from cell a of
// the line up to end, is at: end, where the next row starts, for a cell
// past the row's text on a row the line wraps after, either at or past end
// or past the width, among the spaces that hang off a row wrapped at words.
func (r Index) rowCell(a, end, cell int) int {
	c := a + max(cell, 0)
	if end != math.MaxInt && (c >= end || cell >= r.key.width) {
		return end
	}
	return c
}

// After returns the position after the character drawn at cell of row, as
// PosAt finds it: where the next character starts, or the end of the row,
// or PosAt's own for a cell past the text of the row.
func (r Index) After(row, cell int, scratch *[]int) Pos {
	p := r.PosAt(row, cell, scratch)
	if row < 0 || row >= r.Total() {
		return p
	}
	line, a, end := r.RowAt(row, scratch)
	last := r.s.Measure(r.key.method).Width(line)
	if end != math.MaxInt {
		last = min(last, end)
	}
	for c := r.rowCell(a, end, cell) + 1; c <= last; c++ {
		if q := r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, c)}); q != p {
			return q
		}
	}
	return p
}

// CharAt returns the position of the character drawn at cell of row, and
// whether there is one: none for a cell past the text of its row.
func (r Index) CharAt(row, cell int, scratch *[]int) (Pos, bool) {
	if r.s.Len() == 0 || cell < 0 || row < 0 || row >= r.Total() {
		return Pos{}, false
	}
	line, a, end := r.RowAt(row, scratch)
	last := r.s.Measure(r.key.method).Width(line)
	if end != math.MaxInt {
		last = min(end, a+r.key.width)
	}
	if a+cell >= last {
		return Pos{}, false
	}
	return r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, a+cell)}), true
}

// CellOf returns the row and the cell of it that position p, clamped to
// the text, is drawn at. A position where a line wraps is at the start of
// the row after, or with end, as a caret at the end of a row is, past the
// cells of the row before.
func (r Index) CellOf(p Pos, end bool, scratch *[]int) (row, cell int) {
	if r.s.Len() == 0 {
		return 0, 0
	}
	p = r.s.Clamp(p)
	c := r.s.CellOf(p.Line, r.key.method, p.Col)
	row = r.RowsBefore(p.Line)
	if r.Uniform() || r.RowsOf(p.Line) == 1 {
		return row, c
	}
	starts := r.s.RowStarts(p.Line, r.key.method, r.key.width, r.key.mode, scratch)
	k, found := slices.BinarySearch(starts, c)
	if !found {
		k--
	}
	k = max(k, 0)
	if end && k > 0 && c == starts[k] {
		return row + k - 1, c - starts[k-1]
	}
	return row + k, c - starts[k]
}

// CaretIn is CellOf with a cell past the width of a wrapped row, among the
// spaces that hang off it or past them, at the column past the width, as
// a caret is drawn past the row's text.
func (r Index) CaretIn(p Pos, end bool, scratch *[]int) (row, cell int) {
	row, cell = r.CellOf(p, end, scratch)
	if !r.Uniform() {
		cell = min(cell, r.key.width)
	}
	return row, cell
}

// RowEnd returns the end of row, as the end key moves a caret to it, and
// whether it is the place its line wraps after it: the end of the line on
// its last row, and on a row the line wraps after, the place it wraps,
// where the next row starts, after any spaces that hang off a row wrapped
// at words. A tab the line wraps inside has no place there, and the row
// ends before it.
func (r Index) RowEnd(row int, scratch *[]int) (p Pos, wraps bool) {
	line, _, end := r.RowAt(row, scratch)
	if end == math.MaxInt {
		return r.s.Clamp(Pos{Line: line, Col: math.MaxInt}), false
	}
	p = r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, end)})
	return p, r.s.CellOf(line, r.key.method, p.Col) == end
}

// EndsRow reports whether p, clamped, is the end of row, a row its line
// wraps after: the place it wraps, which a caret at the end of the row is
// at.
func (r Index) EndsRow(p Pos, row int, scratch *[]int) bool {
	if row < 0 || row >= r.Total() {
		return false
	}
	line, _, end := r.RowAt(row, scratch)
	return end != math.MaxInt && p.Line == line && r.s.CellOf(line, r.key.method, p.Col) == end
}

// AtColumn returns the position on row at the character boundary nearest
// column of the row, never inside a wide character or a tab, or the end of
// the row, as RowEnd has it, for a column at or past the row's cells, and
// whether that is the place the row's line wraps after it.
func (r Index) AtColumn(row, column int, scratch *[]int) (Pos, bool) {
	line, a, end := r.RowAt(row, scratch)
	last := r.s.Measure(r.key.method).Width(line)
	if end != math.MaxInt {
		// The cells the row shows, without the spaces that hang off it.
		last = min(end, a+r.key.width)
	}
	c := a + max(column, 0)
	if c >= last {
		return r.RowEnd(row, scratch)
	}
	// The character the column falls in, and the one after it.
	p := r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, c)})
	pc := r.s.CellOf(line, r.key.method, p.Col)
	if pc >= c {
		return p, false
	}
	for k := c + 1; k <= last; k++ {
		q := r.s.Clamp(Pos{Line: line, Col: r.s.ByteAt(line, r.key.method, k)})
		if q == p {
			continue
		}
		qc := r.s.CellOf(line, r.key.method, q.Col)
		if qc-c >= c-pc {
			break
		}
		if qc >= last {
			return r.RowEnd(row, scratch)
		}
		return q, false
	}
	return p, false
}

// BlockText returns the text of the cells from x0 up to x1, inclusive, of
// each row from y0 up to y1, inclusive, as laid out, a line to a row, with
// the spaces at the end of each left out. The rows up to y1 must be rows
// of r.
func (r Index) BlockText(x0, y0, x1, y1 int) string {
	var b strings.Builder
	var scratch []int
	for y := max(y0, 0); y <= y1; y++ {
		if y > y0 {
			b.WriteByte('\n')
		}
		line, a, end := r.RowAt(y, &scratch)
		lo, hi := a+x0, min(a+x1, end-1)
		b.WriteString(strings.TrimRight(r.s.CellText(line, r.key.method, lo, hi), " "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// RowMemo is the row whose cells [Rows.RowCells] last cut.
type RowMemo struct {
	s     *Store
	rev   uint64
	key   rowKey
	y     int
	cells string
}

// RowCells returns the cells of row y, all of it from its first cell, kept
// in m for the next call, as reading the cells of a selection asks for
// each cell of a row in turn.
func (r Index) RowCells(y int, m *RowMemo) string {
	if m.s == r.s && m.rev == r.s.Rev() && m.key == r.key && m.y == y {
		return m.cells
	}
	var scratch []int
	line, a, b := r.RowAt(y, &scratch)
	*m = RowMemo{s: r.s, rev: r.s.Rev(), key: r.key, y: y, cells: r.s.CutRow(line, r.key.method, a, b, nil)}
	return m.cells
}

// RowSpan is a row of the lines in sight: the cells from A up to B of
// line Line, and which of the line's rows, Sub, it is.
type RowSpan struct{ Line, Sub, A, B int }

// Covered returns the cells of row, one of those in sight, that the text
// from from up to to takes, from lo up to hi, cells of its line, and
// whether it takes any. A row the text runs on past has [math.MaxInt] for
// hi, for the line break or the wrap after it.
func (r Index) Covered(row RowSpan, from, to Pos) (lo, hi int, ok bool) {
	if row.Line < from.Line || row.Line > to.Line {
		return 0, 0, false
	}
	last := r.Uniform() || row.Sub == r.RowsOf(row.Line)-1
	lo, hi = row.A, math.MaxInt
	if row.Line == from.Line {
		lo = max(lo, r.s.CellOf(row.Line, r.key.method, from.Col))
		if lo >= row.B && !last {
			return 0, 0, false // it starts on a later row of the line
		}
	}
	if row.Line == to.Line {
		if c := r.s.CellOf(row.Line, r.key.method, to.Col); c < row.B || last {
			hi = c
		}
	}
	return lo, hi, lo < hi
}

// InSight appends to out the rows in sight of a viewport h rows tall whose
// top row is row sub of line, and returns it: of a line the rows leave
// whole, the cells from column col on, and of one they wrap, each of its
// rows, as many cells of each as the rows are wide at most.
func (r Index) InSight(line, sub, h, col int, out []RowSpan) []RowSpan {
	// The widths of lines, without the widest, which the caller has.
	m := Measure{s: r.s, method: r.key.method}
	var scratch []int
	for y := 0; y < h && line < r.s.Len(); line++ {
		if r.Uniform() || r.RowsOf(line) == 1 {
			out = append(out, RowSpan{Line: line, A: col, B: col + r.key.width})
			y++
			sub = 0
			continue
		}
		starts := r.s.RowStarts(line, r.key.method, r.key.width, r.key.mode, &scratch)
		end := m.Width(line)
		for k := sub; k < len(starts) && y < h; k++ {
			rowEnd := end
			if k+1 < len(starts) {
				rowEnd = starts[k+1]
			}
			out = append(out, RowSpan{Line: line, Sub: k, A: starts[k], B: min(rowEnd, starts[k]+r.key.width)})
			y++
		}
		sub = 0
	}
	return out
}

// CountRows returns how many rows the lines of the store take wrapped w
// cells wide with mode, measured with method, counting no further than
// most, for whether they fill a height without counting all of them.
func (b *Store) CountRows(method ansi.Method, mode Mode, w, most int) int {
	m := b.Measure(method)
	h := 0
	var starts []int
	for i := 0; i < b.Len() && h < most; i++ {
		if m.Width(i) <= w {
			h++
			continue
		}
		h += len(b.RowStarts(i, method, w, mode, &starts))
	}
	return h
}
