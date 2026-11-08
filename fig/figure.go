// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"image"
	"slices"
	"strings"
	"sync"
)

// Figure is text laid out in a font: a grid of cells, each holding a
// sub-character and the glyph that drew it, and a record of every cell where
// two glyphs met. It is immutable and safe for concurrent use.
type Figure struct {
	fc     *face
	w, h   int
	cells  []fcell
	glyphs []Glyph
	marks  []mark // where each glyph's own sub-characters are, by glyph
	seams  []Seam
	ends   []int // where each row's content ends; cells past it were cut off
	stack  Stacking

	visible []int       // each glyph's place among the glyphs that show something
	shown   int         // how many glyphs show something
	seamIdx map[int]int // the last seam at each cell, by cell index

	mu      sync.Mutex
	renders map[renderKey]string
	// fixed is the last paint found to be [cacheable], or nil. Comparing a
	// paint with it cannot panic, so a paint equal to it is not checked again.
	fixed Paint
}

type fcell struct {
	s    sub
	g    int32
	kind CellKind
}

// mark is where a glyph's own sub-characters are drawn: its rows, from row
// y, each starting in its column of cols.
type mark struct {
	gl   *glyph
	y    int
	cols []int
}

// at reports whether m and o draw the same glyph in the same place.
func (m mark) at(o mark) bool {
	return m.gl == o.gl && m.y == o.y && slices.Equal(m.cols, o.cols)
}

// Glyph is a character of the text, as laid out in a [Figure].
type Glyph struct {
	// Rune is the character, as written.
	Rune rune
	// Index is the glyph's place among the figure's glyphs, in the order the
	// text was written.
	Index int
	// Src is the byte offset of the character in the text.
	Src int
	// Line is the line of the figure the glyph is on, after wrapping.
	Line int
	// Rect is the cells the glyph's sub-characters cover, which overlap its
	// neighbors' where they were smushed. It is empty for a space.
	Rect image.Rectangle
}

// CellKind is what a cell of a [Figure] holds.
type CellKind uint8

const (
	// KindBlank is an empty cell.
	KindBlank CellKind = iota
	// KindStroke is a sub-character of one glyph.
	KindStroke
	// KindHardblank is a glyph's hardblank, a blank that took part in
	// smushing as a character.
	KindHardblank
	// KindSeam is a cell where two or more glyphs' sub-characters met.
	KindSeam
	// KindInside is a blank enclosed by strokes, such as the middle of an
	// "O", which cannot be reached from the figure's edge through blanks.
	KindInside
)

// Cell is a cell of a [Figure].
type Cell struct {
	X, Y int
	// Content is what the cell shows: its sub-character, or a space.
	Content string
	Kind    CellKind
	// Glyph is the index of the glyph drawn on top, or -1. For a blank
	// inside letters, it is the top glyph whose bounds hold it.
	Glyph int
}

// Seam is a cell where two glyphs' sub-characters met, as glyphs were put
// side by side, or as lines were stacked. A cell where three met has two.
type Seam struct {
	X, Y int
	// Earlier and Later are the glyphs, in the order of the text.
	Earlier, Later int
	// EarlierContent and LaterContent are what each glyph had there.
	EarlierContent, LaterContent string
	// Content is what the cell shows.
	Content string
	// Merge is how the two were merged.
	Merge Merge
	// Winner is whose character the cell shows.
	Winner Side
}

// Size returns the figure's width and height, in cells.
func (f *Figure) Size() (w, h int) { return f.w, f.h }

// Glyphs returns the figure's glyphs, in the order of the text. It must not
// be changed.
func (f *Figure) Glyphs() []Glyph { return f.glyphs }

// Seams returns the cells where glyphs met. It must not be changed.
func (f *Figure) Seams() []Seam { return f.seams }

// At returns the cell at x, y, which is blank outside the figure.
func (f *Figure) At(x, y int) Cell {
	if x < 0 || y < 0 || x >= f.w || y >= f.h {
		return Cell{X: x, Y: y, Content: " ", Glyph: -1}
	}
	c := f.cells[y*f.w+x]
	return Cell{X: x, Y: y, Content: f.content(c.s), Kind: c.kind, Glyph: int(c.g)}
}

func (f *Figure) content(s sub) string {
	if s == subNone || s == subCont {
		return " "
	}
	return f.fc.subs[s]
}

// String returns the figure as plain text, with hardblanks as spaces, and
// each row's trailing spaces removed.
func (f *Figure) String() string {
	var b strings.Builder
	b.Grow(f.w*f.h + f.h)
	for y := range f.h {
		if y > 0 {
			b.WriteByte('\n')
		}
		var row strings.Builder
		for _, c := range f.cells[y*f.w : (y+1)*f.w] {
			if c.s != subCont {
				row.WriteString(f.content(c.s))
			}
		}
		b.WriteString(strings.TrimRight(row.String(), " "))
	}
	return b.String()
}

// figure lays the finished lines out into a Figure: each row cut to width
// and justified within it, as figlet's putstring does, and the lines stacked
// as vm says.
func (b *builder) figure(width int, j Justify, vm int) *Figure {
	f := &Figure{fc: b.fc, stack: b.stack}
	var grid [][]cellp
	for li, ln := range b.lines {
		base := int32(len(f.glyphs)) //nolint:gosec // glyphs are counted from the text, far under 2^31.
		rows := make([][]cellp, len(ln.rows))
		pads, lens := make([]int, len(ln.rows)), make([]int, len(ln.rows))
		for y, row := range ln.rows {
			if width > 0 && len(row) > width {
				row = row[:width]
			}
			pads[y], lens[y] = pad(len(row), width, j), len(row)
			rows[y] = make([]cellp, pads[y], pads[y]+len(row))
			for x := range pads[y] {
				rows[y][x] = cellp{s: subSpace, g: -1}
			}
			for _, c := range row {
				if c.g >= 0 {
					c.g += base
				}
				rows[y] = append(rows[y], c)
			}
		}
		y0 := len(grid)
		if li > 0 && vm&(svFit|svSmush) != 0 {
			k := b.voverlap(grid, rows, vm)
			y0 = len(grid) - k
			for i := range k {
				grid[y0+i] = f.vmerge(grid[y0+i], rows[i], y0+i, vm)
			}
			rows = rows[k:]
		}
		grid = append(grid, rows...)
		for _, p := range ln.glyphs {
			cols := make([]int, len(p.cols))
			for r, x := range p.cols {
				cols[r] = x + pads[r]
			}
			f.marks = append(f.marks, mark{gl: p.gl, y: y0, cols: cols})
			f.glyphs = append(f.glyphs, Glyph{Rune: p.r, Index: len(f.glyphs), Src: p.src, Line: li})
		}
		for _, s := range ln.seams {
			if s.x >= lens[s.y] {
				continue // cut off with the end of its row
			}
			f.addSeam(s.x+pads[s.y], s.y+y0, s.under+base, s.over+base, s.us, s.os, s.res, s.merge)
		}
	}
	f.fill(grid)
	return f
}

// pad returns how many spaces figlet's putstring puts before a row of n
// characters, justified j within width.
func pad(n, width int, j Justify) int {
	var jv int
	switch j { //nolint:exhaustive // Auto is resolved before this, and Left pads nothing.
	case Center:
		jv = 1
	case Right:
		jv = 2
	}
	if width == 0 || jv == 0 {
		return 0
	}
	count := 0
	for i := 1; (3-jv)*i+n+jv-2 < width+1; i++ {
		count++
	}
	return count
}

func (f *Figure) addSeam(x, y int, under, over int32, us, os, res sub, how Merge) {
	s := Seam{
		X: x, Y: y, Earlier: int(under), Later: int(over),
		EarlierContent: f.content(us), LaterContent: f.content(os), Content: f.content(res), Merge: how,
	}
	switch {
	case us == os && res == us:
		s.Winner = Both
	case res == us:
		s.Winner = Earlier
	case res == os:
		s.Winner = Later
	default:
		s.Winner = Neither
	}
	f.seams = append(f.seams, s)
}

// voverlap returns how many rows the next line's rows can move up into the
// grid, as the vertical layout vm says.
func (b *builder) voverlap(grid, rows [][]cellp, vm int) int {
	best := min(len(grid), len(rows))
	w := 0
	for _, r := range grid[len(grid)-best:] {
		w = max(w, len(r))
	}
	for _, r := range rows[:best] {
		w = max(w, len(r))
	}
	for x := 0; x < w && best > 0; x++ {
		k := 0
		for k < best && b.vfits(grid, rows, x, k+1, vm) {
			k++
		}
		best = k
	}
	return best
}

// vfits reports whether column x of the next line can overlap the grid by k
// rows: no characters meet when fitting, and when smushing, one pair meets
// and can be merged, or any number of "|" over "|" with [RuleVSuper].
func (b *builder) vfits(grid, rows [][]cellp, x, k, vm int) bool {
	meets, super := 0, true
	for i := range k {
		u, l := cellAt(grid[len(grid)-k+i], x).s, cellAt(rows[i], x).s
		if blank(u) || blank(l) {
			continue
		}
		meets++
		if vm&svSmush == 0 {
			return false
		}
		res, how := b.fc.vsmush(u, l, vm)
		if res == subNone {
			return false
		}
		super = super && how == MergeSuper
		if meets > 1 && !super {
			return false
		}
	}
	return true
}

// vmerge merges row, of the line below, into up, the grid's row y.
func (f *Figure) vmerge(up, row []cellp, y, vm int) []cellp {
	out := make([]cellp, max(len(up), len(row)))
	for x := range out {
		u, l := cellAt(up, x), cellAt(row, x)
		res, how := f.fc.vsmush(u.s, l.s, vm)
		switch {
		case blank(l.s) || res == subNone:
			out[x] = u
		case blank(u.s):
			out[x] = l
		default:
			f.addSeam(x, y, u.g, l.g, u.s, l.s, res, how)
			top := l.g
			if f.stack == EarlierOnTop {
				top = u.g
			}
			out[x] = cellp{s: res, g: top}
		}
		if out[x].s == subNone {
			out[x] = cellp{s: subSpace, g: -1}
		}
	}
	return out
}

// fill sizes the figure to the grid, and works out what each cell is.
func (f *Figure) fill(grid [][]cellp) {
	f.h = len(grid)
	f.ends = make([]int, f.h)
	for y, r := range grid {
		f.w = max(f.w, len(r))
		f.ends[y] = len(r)
	}
	f.cells = make([]fcell, f.w*f.h)
	for y, r := range grid {
		for x := range f.w {
			c := cellAt(r, x)
			fc := fcell{s: c.s, g: c.g}
			switch {
			case blank(c.s):
				fc = fcell{s: subSpace, g: -1, kind: KindBlank}
			case c.s == subHard:
				fc.kind = KindHardblank
			default:
				fc.kind = KindStroke
			}
			f.cells[y*f.w+x] = fc
		}
	}
	for _, s := range f.seams {
		if c := &f.cells[s.Y*f.w+s.X]; c.kind == KindStroke {
			c.kind = KindSeam
		}
	}
	f.inside()
	f.visible = make([]int, len(f.glyphs))
	for i, m := range f.marks {
		f.glyphs[i].Rect = f.strokes(m).bounds
		f.visible[i] = f.shown
		if !f.glyphs[i].Rect.Empty() {
			f.shown++
		}
	}
	for i := range f.cells {
		if f.cells[i].kind == KindInside {
			f.cells[i].g = int32(f.over(i%f.w, i/f.w)) //nolint:gosec // a glyph index.
		}
	}
	f.seamIdx = make(map[int]int, len(f.seams))
	for i, s := range f.seams {
		f.seamIdx[s.Y*f.w+s.X] = i
	}
}

// seamAt returns the last seam at x, y, or nil.
func (f *Figure) seamAt(x, y int) *Seam {
	if i, ok := f.seamIdx[y*f.w+x]; ok {
		return &f.seams[i]
	}
	return nil
}

// inside marks the blanks that cannot be reached from the figure's edge
// through blanks and hardblanks.
func (f *Figure) inside() {
	open := func(i int) bool { k := f.cells[i].kind; return k == KindBlank || k == KindHardblank }
	seen := make([]bool, len(f.cells))
	var stack []int
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= f.w || y >= f.h {
			return
		}
		if i := y*f.w + x; !seen[i] && open(i) {
			seen[i] = true
			stack = append(stack, i)
		}
	}
	for x := range f.w {
		push(x, 0)
		push(x, f.h-1)
	}
	for y := range f.h {
		push(0, y)
		push(f.w-1, y)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%f.w, i/f.w
		push(x+1, y)
		push(x-1, y)
		push(x, y+1)
		push(x, y-1)
	}
	for i := range f.cells {
		if !seen[i] && open(i) {
			f.cells[i].kind = KindInside
		}
	}
}

// stroked is a glyph's own sub-characters, where they are in the figure.
type stroked struct {
	cells  []strokeCell
	bounds image.Rectangle
}

type strokeCell struct {
	x, y int
	s    sub
}

// strokes returns a glyph's own sub-characters, cut off where the figure's
// rows were.
func (f *Figure) strokes(m mark) stroked {
	var st stroked
	for r, row := range m.gl.rows {
		y := m.y + r
		if y < 0 || y >= f.h {
			continue
		}
		for c, s := range row {
			x := m.cols[r] + c
			if blank(s) || x < 0 || x >= f.ends[y] {
				continue
			}
			st.cells = append(st.cells, strokeCell{x: x, y: y, s: s})
			st.bounds = st.bounds.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return st
}
