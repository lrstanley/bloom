// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"math"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRows(t *testing.T) {
	t.Parallel()

	// "hello world" wrapped at words six wide: "hello " and "world".
	r := newStore("hello world").Index(ansi.GraphemeWidth, Words, 6)
	var scratch []int
	if line, a, end := r.RowAt(0, &scratch); line != 0 || a != 0 || end != 6 {
		t.Fatalf("row 0 is %d from %d to %d", line, a, end)
	}
	if _, a, end := r.RowAt(1, &scratch); a != 6 || end != math.MaxInt {
		t.Fatalf("row 1 is from %d to %d", a, end)
	}
	for _, tc := range []struct {
		row, cell int
		at, after Pos
	}{
		{0, 2, Pos{0, 2}, Pos{0, 3}},
		{0, 9, Pos{0, 6}, Pos{0, 6}}, // past the width: where the next row starts
		{1, 0, Pos{0, 6}, Pos{0, 7}},
		{1, 4, Pos{0, 10}, Pos{0, 11}},
		{1, 7, Pos{0, 11}, Pos{0, 11}}, // past the text
		{-1, 3, Pos{}, Pos{}},
		{5, 0, Pos{0, 11}, Pos{0, 11}},
	} {
		if got := r.PosAt(tc.row, tc.cell, &scratch); got != tc.at {
			t.Errorf("PosAt(%d, %d) = %v, want %v", tc.row, tc.cell, got, tc.at)
		}
		if got := r.After(tc.row, tc.cell, &scratch); got != tc.after {
			t.Errorf("After(%d, %d) = %v, want %v", tc.row, tc.cell, got, tc.after)
		}
	}
	// The place the line wraps is at the start of the row after, or at the
	// end of the row before for a caret there.
	for _, tc := range []struct {
		p         Pos
		end       bool
		row, cell int
	}{
		{Pos{0, 6}, false, 1, 0},
		{Pos{0, 6}, true, 0, 6},
		{Pos{0, 3}, true, 0, 3},
		{Pos{0, 99}, false, 1, 5},
	} {
		if row, cell := r.CellOf(tc.p, tc.end, &scratch); row != tc.row || cell != tc.cell {
			t.Errorf("CellOf(%v, %v) = %d, %d, want %d, %d", tc.p, tc.end, row, cell, tc.row, tc.cell)
		}
	}
	if p, wraps := r.RowEnd(0, &scratch); p != (Pos{0, 6}) || !wraps {
		t.Errorf("row 0 ends at %v, wraps %v", p, wraps)
	}
	if p, wraps := r.RowEnd(1, &scratch); p != (Pos{0, 11}) || wraps {
		t.Errorf("row 1 ends at %v, wraps %v", p, wraps)
	}
	if !r.EndsRow(Pos{0, 6}, 0, &scratch) || r.EndsRow(Pos{0, 6}, 1, &scratch) || r.EndsRow(Pos{0, 5}, 0, &scratch) {
		t.Error("EndsRow is wrong about where row 0 ends")
	}
	if p, wraps := r.AtColumn(1, 2, &scratch); p != (Pos{0, 8}) || wraps {
		t.Errorf("column 2 of row 1 is %v, wraps %v", p, wraps)
	}
	if p, wraps := r.AtColumn(0, 9, &scratch); p != (Pos{0, 6}) || !wraps {
		t.Errorf("column 9 of row 0 is %v, wraps %v", p, wraps)
	}
	for _, tc := range []struct {
		row, cell int
		at        Pos
		ok        bool
	}{
		{0, 5, Pos{0, 5}, true},
		{0, 6, Pos{}, false}, // the space hanging off the row is past its text
		{1, 4, Pos{0, 10}, true},
		{1, 5, Pos{}, false},
		{0, -1, Pos{}, false},
	} {
		if at, ok := r.CharAt(tc.row, tc.cell, &scratch); at != tc.at || ok != tc.ok {
			t.Errorf("CharAt(%d, %d) = %v, %v, want %v, %v", tc.row, tc.cell, at, ok, tc.at, tc.ok)
		}
	}
	if got := r.BlockText(1, 0, 3, 1); got != "ell\norl" {
		t.Errorf("BlockText = %q", got)
	}

	// A column inside a wide character goes to the nearer side of it, and
	// a caret past the width of a wrapped row stays in the column past it.
	w := newStore("日本").Index(ansi.GraphemeWidth, Chars, 3)
	if p, _ := w.AtColumn(0, 1, &scratch); p != (Pos{0, 0}) {
		t.Errorf("column 1 of 日 is %v", p)
	}
	if row, cell := w.CaretIn(Pos{0, 3}, true, &scratch); row != 0 || cell != 2 {
		t.Errorf("a caret at the end of row 0 is at %d, %d", row, cell)
	}
}

func TestRowsInSight(t *testing.T) {
	t.Parallel()

	// A short line, one that wraps to three rows four wide, and another.
	s := newStore("ab\nabcdefghij\ncd")
	r := s.Index(ansi.GraphemeWidth, Chars, 4)
	got := r.InSight(1, 1, 3, 0, nil)
	want := []RowSpan{{Line: 1, Sub: 1, A: 4, B: 8}, {Line: 1, Sub: 2, A: 8, B: 10}, {Line: 2, A: 0, B: 4}}
	if len(got) != len(want) {
		t.Fatalf("in sight %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("in sight %+v, want %+v", got, want)
		}
	}
	// Not wrapped, from a column on.
	if flat := s.Index(ansi.GraphemeWidth, Chars, 20).InSight(0, 0, 9, 3, nil); len(flat) != 3 || flat[1] != (RowSpan{Line: 1, A: 3, B: 23}) {
		t.Fatalf("unwrapped in sight %+v", flat)
	}

	if n := s.CountRows(ansi.GraphemeWidth, Chars, 4, 100); n != 5 {
		t.Fatalf("counted %d rows, want 5", n)
	}
	if n := s.CountRows(ansi.GraphemeWidth, Chars, 4, 2); n != 4 {
		t.Fatalf("counted %d rows up to 2, want 4: the line that passes it whole", n)
	}
}

func TestRowsCovered(t *testing.T) {
	t.Parallel()

	// "hello world" wrapped at words six wide, "hello " and "world", then
	// "next": the cells of each row that a span of the text takes.
	s := newStore("hello world\nnext")
	r := s.Index(ansi.GraphemeWidth, Words, 6)
	rows := r.InSight(0, 0, 3, 0, nil)
	for _, tc := range []struct {
		from, to Pos
		want     [3][2]int // lo and hi of each row, or -1 for none
	}{
		{Pos{0, 2}, Pos{0, 4}, [3][2]int{{2, 4}, {-1, -1}, {-1, -1}}},
		// Across the wrap: the first row to its end, the second up to the text's.
		{Pos{0, 2}, Pos{0, 8}, [3][2]int{{2, math.MaxInt}, {6, 8}, {-1, -1}}},
		// From the wrap point: nothing of the first row.
		{Pos{0, 6}, Pos{0, 8}, [3][2]int{{-1, -1}, {6, 8}, {-1, -1}}},
		// Across the line break: the last row of the line to its end.
		{Pos{0, 9}, Pos{1, 2}, [3][2]int{{-1, -1}, {9, math.MaxInt}, {0, 2}}},
		{Pos{1, 1}, Pos{1, 4}, [3][2]int{{-1, -1}, {-1, -1}, {1, 4}}},
	} {
		for k, row := range rows {
			lo, hi, ok := r.Covered(row, tc.from, tc.to)
			got := [2]int{-1, -1}
			if ok {
				got = [2]int{lo, hi}
			}
			if got != tc.want[k] {
				t.Errorf("%v to %v, row %d: %v, want %v", tc.from, tc.to, k, got, tc.want[k])
			}
		}
	}

	// The cells of a row are cut once while it is asked for again, and
	// again when the text changes.
	var m RowMemo
	if got := r.RowCells(1, &m); got != "world" {
		t.Fatalf("row 1 is %q", got)
	}
	if got := r.RowCells(0, &m); got != "hello " {
		t.Fatalf("row 0 is %q", got)
	}
	s.Replace(TextEdit{From: Pos{0, 0}, To: Pos{0, 5}, Text: "HOWDY"})
	if got := s.Index(ansi.GraphemeWidth, Words, 6).RowCells(0, &m); got != "HOWDY " {
		t.Fatalf("row 0 is %q after the edit", got)
	}
}
