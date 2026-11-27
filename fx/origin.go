// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import "image"

// Origin is a spot an effect works out of, such as where a dialog opens
// from: a cell on screen, such as where a click was, or a side, corner, or
// the middle of the node. Build one with [At] or [Side], or use [Middle].
//
// The zero value is no spot, for which each effect given one documents what
// it does.
type Origin struct {
	at   image.Point
	side Direction
	kind originKind
}

type originKind uint8

const (
	originNone originKind = iota
	originSide
	originAt
)

// Middle is the middle of a node.
var Middle = Origin{kind: originSide}

// At is the cell p, in frame coordinates, such as where a click was, or the
// middle of a button's rectangle from the previous frame.
func At(p image.Point) Origin { return Origin{at: p, kind: originAt} }

// Side is side or corner d of a node, such as its top for a menu dropping
// down from its top edge. The zero value is the middle, as [Middle].
func Side(d Direction) Origin { return Origin{side: d, kind: originSide} }

// IsZero reports whether o is no spot.
func (o Origin) IsZero() bool { return o.kind == originNone }

// Cell returns the cell of node r nearest to o, relative to r's top-left
// cell: the cell itself for a point on r, the middle cell of a side, the
// corner cell for a corner, and the middle cell for [Middle] or no spot. An
// empty r gives the zero point.
func (o Origin) Cell(r image.Rectangle) image.Point {
	if r.Empty() {
		return image.Point{}
	}
	x, y := o.point(r)
	return image.Pt(
		min(max(int(x), 0), r.Dx()-1),
		min(max(int(y), 0), r.Dy()-1),
	)
}

// point returns where o is relative to r's top-left corner, each cell
// spanning one unit: the middle of the cell for a point, which may be off r,
// and r's own edges for a side or corner, so the middle of a 10×4 node is
// (5, 2) and its bottom right corner (10, 4).
func (o Origin) point(r image.Rectangle) (x, y float64) {
	if o.kind == originAt {
		p := o.at.Sub(r.Min)
		return float64(p.X) + 0.5, float64(p.Y) + 0.5
	}
	dx, dy := o.side.Vector()
	return edge(dx, r.Dx()), edge(dy, r.Dy())
}

// edge is where along n cells a side is that faces the way d: the start
// going forward, the end going back, or the middle.
func edge(d, n int) float64 {
	switch {
	case d > 0:
		return 0
	case d < 0:
		return float64(n)
	}
	return float64(n) / 2
}
