// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image"
	"math"
	"slices"
	"sync"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

// Fly flies a moving node's cells over from where it was, [canvas.Surface.From],
// to where it is, for a slot or layer that moves with effects, such as with
// [bloom.SlotNode.MoveWith]. The node starts out whole where it was, squeezed
// or stretched to the size it was there. Its columns, or its rows for a move mostly up or down, take off
// one after another, from the edge facing where they go, each cell a little
// apart from its neighbors, arc over, and land.
// A glyph in flight shows a bright random glyph in its own color, and blank
// cells wait where they were until they land, so the node is whole at both
// ends. Flights are the same for the same Seed. On a node that is not
// moving, it does nothing.
//
// [bloom.SlotNode.MoveWith]: https://pkg.go.dev/bloomfx.dev/bloom#SlotNode.MoveWith
type Fly struct {
	Seed uint32
}

const (
	flySpread = 0.45 // how much of t is spent starting the columns
	flyJitter = 0.12 // how much later than its column a cell may start
	flyTime   = 1 - flySpread - flyJitter
	flySteps  = 40 // how many times over a flight the glyphs change
)

// flyCell is a cell of a node that has not landed: where it lands, its
// progress, and what it shows.
type flyCell struct {
	x, y int
	p    float64
	c    uv.Cell
}

// flyBuf is what a flight needs between drawing its cells: those not landed,
// and, for a node that changed size, which of them each of its cells is, or
// was, for the rest of a wide glyph, counting from 1.
type flyBuf struct {
	cells []flyCell
	at    []int32
}

var flyBufs = sync.Pool{New: func() any { return new(flyBuf) }}

// Apply implements [Effect].
func (e Fly) Apply(s *canvas.Surface, t float64) {
	from, to := s.From, s.Rect()
	if t >= 1 || from.Empty() || to.Empty() || from == to {
		return
	}
	buf, _ := flyBufs.Get().(*flyBuf)
	defer func() {
		clear(buf.cells) // so no cell is kept past the frame
		buf.cells = buf.cells[:0]
		flyBufs.Put(buf)
	}()

	shift := from.Min.Sub(to.Min) // from a node's cells to where it was
	resized := from.Size() != to.Size()
	waiting := e.lift(s, buf, t, resized)

	// Cells waiting to leave are drawn where they were, and those in flight
	// over them.
	if resized && waiting {
		e.waitResized(s, buf, from, shift)
	} else if !resized {
		for i := range buf.cells {
			if c := &buf.cells[i]; c.p <= 0 {
				s.Set(c.x+shift.X, c.y+shift.Y, &c.c)
			}
		}
	}
	step := uint32(max(t, 0) * flySteps)
	for i := range buf.cells {
		c := &buf.cells[i]
		if c.p <= 0 || c.c.Content == " " || c.c.Content == "" {
			continue
		}
		// A cell starts as far into where the node was as it is into where
		// it is.
		start := shift
		if resized {
			start = start.Add(image.Pt(c.x*from.Dx()/s.Width()-c.x, c.y*from.Dy()/s.Height()-c.y))
		}
		k := smoothstep(c.p)
		c.c.Content, c.c.Width = flicker(decodeGlyphs, c.x, c.y, e.Seed, step), 1
		c.c.Style.Attrs |= uv.AttrBold
		if c.c.Style.Fg == nil {
			c.c.Style.Fg = s.Env.Fg
		}
		// An arc up and over, a row or two high.
		lift := int(math.Round(math.Sin(math.Pi*k) * (1 + hash(c.y, c.x, e.Seed))))
		s.Set(
			c.x+start.X-int(math.Round(float64(start.X)*k)),
			c.y+start.Y-int(math.Round(float64(start.Y)*k))-lift,
			&c.c,
		)
	}
}

// lift hides the cells of the node on s that have not landed at t, keeping
// them in buf, with which of them each is if the node was resized, and
// reports whether any are waiting to leave. Cells that have landed stay where
// they are drawn.
func (e Fly) lift(s *canvas.Surface, buf *flyBuf, t float64, resized bool) (waiting bool) {
	w, h := s.Width(), s.Height()
	if resized {
		buf.at = slices.Grow(buf.at[:0], w*h)[:w*h]
		clear(buf.at)
	}
	// Parts leave in order from the edge facing where they go, by how the
	// node's middle moves: columns for a move mostly across, rows for one
	// mostly up or down.
	from, to := s.From, s.Rect()
	d := from.Min.Add(from.Max).Sub(to.Min.Add(to.Max))
	across := abs(d.X) >= abs(d.Y)
	parts, toward := w, d.X
	if !across {
		parts, toward = h, d.Y
	}
	for y := range h {
		lifted := false // whether the last glyph was, for the rest of a wide one
		for x := range w {
			if !s.Visible(x, y) {
				continue
			}
			if s.Cell(x, y).Width == 0 {
				if lifted {
					s.Hide(x, y) // drawn with its glyph, which Set makes whole
					if resized {
						buf.at[y*w+x] = int32(len(buf.cells)) //nolint:gosec // a node has fewer cells.
					}
				}
				continue
			}
			part := x
			if !across {
				part = y
			}
			lead := float64(part) / float64(max(parts-1, 1))
			if toward < 0 {
				lead = 1 - lead // going right or down, the far edge leaves first
			}
			start := lead*flySpread + hash(x, y, e.Seed)*flyJitter
			p := min(max((t-start)/flyTime, 0), 1)
			if lifted = p < 1; !lifted {
				continue
			}
			buf.cells = append(buf.cells, flyCell{x: x, y: y, p: p, c: *s.Cell(x, y)})
			waiting = waiting || p <= 0
			if resized {
				buf.at[y*w+x] = int32(len(buf.cells)) //nolint:gosec // a node has fewer cells.
			}
			s.Hide(x, y)
		}
	}
	return waiting
}

// waitResized draws the cells of a node that changed size waiting to leave,
// squeezed or stretched to fill where it was, from, each cell there showing
// the one as far into the node. A wide glyph waits as blanks in its style.
func (e Fly) waitResized(s *canvas.Surface, buf *flyBuf, from image.Rectangle, shift image.Point) {
	w, h := s.Width(), s.Height()
	for oy := range from.Dy() {
		row := oy * h / from.Dy() * w
		for ox := range from.Dx() {
			i := buf.at[row+ox*w/from.Dx()] - 1
			if i < 0 || buf.cells[i].p > 0 {
				continue
			}
			c := buf.cells[i].c
			if c.Width != 1 {
				c.Empty()
			}
			s.Set(shift.X+ox, shift.Y+oy, &c)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
