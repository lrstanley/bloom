// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"image"
	"strings"
	"time"

	"bloomfx.dev/bloom/internal/linebuf"
	uv "github.com/charmbracelet/ultraviolet"
)

// Joiner joins the text of the leaves of a selection, read leaf by leaf in
// order: a leaf beside the last on the same row with a space, as a cell of
// a table is beside its label, and any other on a line of its own.
type Joiner struct {
	b       strings.Builder
	started bool
	row     int // the last row of the leaf added last
	// Apart is set for the next leaf to go on a line of its own, such as
	// one in an item of a list, or after one, whose rows are its own.
	Apart bool
}

// Add adds the text of a leaf whose first row is top and last is bottom,
// after the leaf before it.
func (j *Joiner) Add(text string, top, bottom int) {
	if j.started {
		if top == j.row && !j.Apart {
			j.b.WriteByte(' ')
		} else {
			j.b.WriteByte('\n')
		}
	}
	j.b.WriteString(text)
	j.started, j.Apart, j.row = true, false, bottom
}

// String returns the text joined, without the line breaks at its end.
func (j *Joiner) String() string { return strings.TrimRight(j.b.String(), "\n") }

// SpanText returns the text of the cells w wide, from from to to in
// reading order, with the spaces at the end of each row left out, and the
// rows joined as joins tells, or with line breaks past them.
func SpanText(cell func(x, y int) *uv.Cell, w int, from, to image.Point, joins []Join) string {
	var b strings.Builder
	for y := from.Y; y <= to.Y; y++ {
		x0, x1 := RowSpan(from, to, y, w)
		b.WriteString(strings.TrimRight(RowText(cell, x0, x1, y), " "))
		if y < to.Y {
			j := JoinBreak
			if y < len(joins) {
				j = joins[y]
			}
			switch j {
			case JoinBreak:
				b.WriteByte('\n')
			case JoinSpace:
				b.WriteByte(' ')
			case JoinNone:
			}
		}
	}
	return b.String()
}

// RectText returns the text of the cells in area, a line to a row, with
// the spaces at the end of each, and the empty rows at the end, left out.
func RectText(cell func(x, y int) *uv.Cell, area image.Rectangle) string {
	var b strings.Builder
	for y := area.Min.Y; y < area.Max.Y; y++ {
		if y > area.Min.Y {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(RowText(cell, area.Min.X, area.Max.X-1, y), " "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// Corners returns the rectangle of cells with p and q at opposite corners.
func Corners(p, q image.Point) image.Rectangle {
	return image.Rect(min(p.X, q.X), min(p.Y, q.Y), max(p.X, q.X)+1, max(p.Y, q.Y)+1)
}

// Unit is what a selection takes whole: a double click selects words, and
// a triple click lines.
type Unit uint8

const (
	UnitCell Unit = iota
	UnitWord
	UnitLine
)

// Clicks counts presses quick enough after each other, and near enough, to
// be a double or triple click.
type Clicks struct {
	unit Unit
	ok   bool
	at   image.Point
	time time.Time
}

// Next reports whether a press at p at now is the next click after the
// last: on the same row, within slop columns of it, within d after it, and
// no more than the third.
func (c *Clicks) Next(p image.Point, now time.Time, slop int, d time.Duration) bool {
	return c.ok && c.at.Y == p.Y && abs(c.at.X-p.X) <= slop && c.unit < UnitLine && now.Sub(c.time) <= d
}

// Press notes a press at p at now, the next click after the last when
// next, and returns what it selects.
func (c *Clicks) Press(next bool, p image.Point, now time.Time) Unit {
	unit := UnitCell
	if next {
		unit = c.unit + 1
	}
	*c = Clicks{unit: unit, ok: true, at: p, time: now}
	return unit
}

// Reset forgets the presses, for the next to count from one.
func (c *Clicks) Reset() { c.ok = false }

// Time returns when the last press was.
func (c *Clicks) Time() time.Time { return c.time }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Memo is the text of a selection read last, kept to read it again: for
// the selection with key, while the stores the text was read from are at
// the revisions they were. Its storage is kept from one read to the next.
type Memo[K comparable] struct {
	ok   bool
	key  K
	text string
	revs []StoreRev
}

// StoreRev is a store text was read from, at the revision it was read at.
type StoreRev struct {
	S   *linebuf.Store
	Rev uint64
}

// Get returns the text kept for key, and whether it is still the text a
// read would return.
func (m *Memo[K]) Get(key K) (string, bool) {
	if !m.ok || m.key != key {
		return "", false
	}
	for _, r := range m.revs {
		if r.S.Rev() != r.Rev {
			return "", false
		}
	}
	return m.text, true
}

// Reset forgets the text kept.
func (m *Memo[K]) Reset() {
	clear(m.revs)
	*m = Memo[K]{revs: m.revs[:0]}
}

// Read forgets the text kept, and returns storage to note the stores the
// next read reads from in, with [NoteStore], for [Memo.Done].
func (m *Memo[K]) Read() []StoreRev {
	m.Reset()
	return m.revs
}

// Done ends a read, of text for key from the stores in revs, and keeps the
// text if keep.
func (m *Memo[K]) Done(key K, text string, revs []StoreRev, keep bool) {
	if !keep {
		clear(revs)
		*m = Memo[K]{revs: revs[:0]}
		return
	}
	*m = Memo[K]{ok: true, key: key, text: text, revs: revs}
}

// NoteStore adds s, at its revision now, to the stores in revs, unless it
// is the one added last.
func NoteStore(revs []StoreRev, s *linebuf.Store) []StoreRev {
	if len(revs) > 0 && revs[len(revs)-1].S == s {
		return revs
	}
	return append(revs, StoreRev{S: s, Rev: s.Rev()})
}
