// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"unicode/utf8"

	"bloomfx.dev/bloom/internal/same"
	"charm.land/lipgloss/v2"
)

// frameOf returns the frame of style, the style of box i. It is worked out
// once a frame, and kept from the last frame while the box at i has the same
// style, as boxes usually do.
func (e *engine) frameOf(i int, style lipgloss.Style) frame {
	b := &e.boxes[i]
	if b.frameOK {
		return b.frame
	}
	m := e.frameMemos.At(i)
	if e.scratch = style; !m.OK || !same.Style(&m.V.style, &e.scratch) {
		m.OK, m.V = true, frameMemo{style: style, frame: frameOf(style)}
	}
	b.frame, b.frameOK = m.V.frame, true
	return m.V.frame
}

// frameMemo is the frame of the style a box had in the last frame.
type frameMemo struct {
	style lipgloss.Style
	frame frame
}

// boxContent returns the box of what is inside box node n at index i, or -1
// when it has none. The children on its edges come after it.
func (e *engine) boxContent(i int, n BoxNode) int {
	if kids := e.kidsOf(i); len(kids) > len(n.edges) {
		return kids[0]
	}
	return -1
}

// boxEdges returns the boxes of the children on box node n's edges, in the
// order of n.edges.
func (e *engine) boxEdges(i int, n BoxNode) []int {
	kids := e.kidsOf(i)
	return kids[len(kids)-len(n.edges):]
}

// arrangeEdges places the children on the edges of box node n at index i,
// laid out in r. See [BoxNode.Edge].
func (e *engine) arrangeEdges(i int, n BoxNode, r image.Rectangle) {
	if len(n.edges) == 0 {
		return
	}
	kids, ring, f := e.boxEdges(i, n), n.bordered(r), e.frameOf(i, n.style)
	for _, side := range [...]Direction{Top, Bottom, Left, Right} {
		if l, ok := f.edgeLine(side, ring); ok && !e.boxes[i].cramped {
			e.arrangeEdge(n, kids, side, l)
			continue
		}
		for j, edge := range n.edges {
			if edge.side == side {
				e.boxes[kids[j]].hidden = true
			}
		}
	}
}

// edgeLine is the row or column of a side of a box's border.
type edgeLine struct {
	vertical   bool
	at         int // the row, or column, of the side
	lo, hi     int // the span of it that children share
	mid        int // the middle of the side
	start, end int // the whole side, corners and all, for [BoxNode.EdgeAt]
}

// edgeLine returns the line of side of the border of a box with frame f,
// the outer cells of ring, reporting false if that side has no border.
func (f frame) edgeLine(side Direction, ring image.Rectangle) (edgeLine, bool) {
	// Children on the top and bottom are kept a cell from the corners.
	horizontal := edgeLine{
		lo: ring.Min.X + f.bl + 1, hi: ring.Max.X - f.br - 1, mid: (ring.Min.X + ring.Max.X) / 2,
		start: ring.Min.X, end: ring.Max.X,
	}
	vertical := edgeLine{
		vertical: true, lo: ring.Min.Y + f.bt, hi: ring.Max.Y - f.bb, mid: (ring.Min.Y + ring.Max.Y) / 2,
		start: ring.Min.Y, end: ring.Max.Y,
	}
	switch side { //nolint:exhaustive // A corner, or the zero value, is not a side.
	case Top:
		horizontal.at = ring.Min.Y
		return horizontal, f.bt > 0
	case Bottom:
		horizontal.at = ring.Max.Y - 1
		return horizontal, f.bb > 0
	case Left:
		vertical.at = ring.Min.X
		return vertical, f.bl > 0
	case Right:
		vertical.at = ring.Max.X - 1
		return vertical, f.br > 0
	}
	return edgeLine{}, false
}

// arrangeEdge places the children of box node n on side, whose line is l,
// given their boxes, kids.
func (e *engine) arrangeEdge(n BoxNode, kids []int, side Direction, l edgeLine) {
	// The ends first, in order, from the corners inward. Children at a cell
	// of their own take no room from the rest.
	for j, edge := range n.edges {
		if edge.side != side {
			continue
		}
		if edge.exact {
			at, length := edge.at, l.end-l.start
			if at < 0 {
				at += length
			}
			if at < 0 || at >= length {
				e.boxes[kids[j]].hidden = true
				continue
			}
			e.placeEdge(kids[j], l, l.start+at, e.edgeSize(kids[j], l, length-at))
			continue
		}
		if edge.align == Center {
			continue
		}
		s := l.hi - l.lo
		if edge.align != Stretch {
			s = e.edgeSize(kids[j], l, s)
		}
		at := l.lo
		if edge.align == End {
			at = l.hi - s
		}
		if !e.placeEdge(kids[j], l, at, s) {
			continue
		}
		if edge.align == End {
			l.hi -= s + 1
		} else {
			l.lo += s + 1
		}
	}

	// Then the middle, sized in order from what is left, and centered on the
	// side as far as the ends allow. Measuring again in the same space is
	// free.
	total := -1
	for j, edge := range n.edges {
		if edge.side == side && !edge.exact && edge.align == Center {
			if s := e.edgeSize(kids[j], l, l.hi-l.lo-total-1); s > 0 {
				total += s + 1
			}
		}
	}
	at, used := min(max(l.mid-total/2, l.lo), l.hi-total), -1
	for j, edge := range n.edges {
		if edge.side == side && !edge.exact && edge.align == Center {
			s := e.edgeSize(kids[j], l, l.hi-l.lo-used-1)
			if e.placeEdge(kids[j], l, at, s) {
				at, used = at+s+1, used+s+1
			}
		}
	}
}

// edgeSize returns how much of room along line l box k takes.
func (e *engine) edgeSize(k int, l edgeLine, room int) int {
	main, _ := axes(e.measure(k, size(max(room, 0), 1, l.vertical)), l.vertical)
	return main
}

// placeEdge arranges box k at at along line l, size cells long, or hides it
// for a size of zero, reporting whether it was placed.
func (e *engine) placeEdge(k int, l edgeLine, at, size int) bool {
	switch {
	case size <= 0:
		e.boxes[k].hidden = true
		return false
	case l.vertical:
		e.arrange(k, image.Rect(l.at, at, l.at+1, at+size))
	default:
		e.arrange(k, image.Rect(at, l.at, at+size, l.at+1))
	}
	return true
}

// paintFrame adds the op that draws the frame of box i, a [BoxNode] drawn
// with style, which is not its own while hovered, at clip.
func (r *renderer) paintFrame(i int, style lipgloss.Style, hovered bool, clip image.Rectangle) {
	rect := r.boxes[i].rect
	if !r.parse {
		r.blit("", rect, clip)
		return
	}
	if !hovered {
		m := r.stretch.At(i)
		if r.style = style; !m.OK || !same.Style(&m.V.style, &r.style) {
			m.OK, m.V = true, r.sampleFrame(i, style)
		}
		if v := &m.V; v.ok && rect.Dx() >= v.size.X && rect.Dy() >= v.size.Y {
			r.ops = append(r.ops, op{kind: opStretch, src: r.cache.Get(v.s), mid: v.mid, rect: rect, clip: clip})
			return
		}
	}
	frame, _ := r.textAt(i, TextNode{style: style, styled: true}, rect.Dx(), rect.Dy())
	r.blit(frame, rect, clip)
}

// stretchMemo is the frame of a box drawn with style around a middle two
// cells wide and tall, as s, size cells in all, with the middle's top-left
// cell at mid, and whether drawing it stretched to a larger size is the
// same as drawing it at that size.
type stretchMemo struct {
	style lipgloss.Style
	s     string
	size  image.Point
	mid   image.Point
	ok    bool
}

// sampleFrame returns the frame of box i drawn small with style, its own,
// to stretch to the box's size, as it can be for a frame of one-cell glyphs
// in one color on each side. A box that changes size, as it does while a
// layout animates or the terminal is resized, then draws its frame without
// Lip Gloss drawing it again, and without parsing a new string of its size.
// It cannot be for a style that sets a size of its own, draws text of its
// own or changes what it is given, blends its border across its length, or
// has an edge of other than one glyph one cell wide: one that repeats more,
// which a middle of two cells can show as one, such as "aab" as "aa", or a
// mark that combines with the corner before it.
func (r *renderer) sampleFrame(i int, style lipgloss.Style) stretchMemo {
	v := stretchMemo{style: style}
	cell := func(edge string) bool {
		return edge == "" || utf8.RuneCountInString(edge) == 1 && r.method.StringWidth(edge) == 1
	}
	b := style.GetBorderStyle()
	if style.GetWidth() != 0 || style.GetHeight() != 0 || style.GetMaxWidth() != 0 || style.GetMaxHeight() != 0 ||
		style.Value() != "" || style.GetTransform() != nil || len(style.GetBorderForegroundBlend()) > 0 ||
		!cell(b.Top) || !cell(b.Bottom) || !cell(b.Left) || !cell(b.Right) {
		return v
	}
	f := r.frameOf(i, style)
	v.size, v.mid = image.Pt(f.l+f.r+2, f.t+f.b+2), image.Pt(f.l, f.t)
	v.s, _ = textOf(TextNode{style: style, styled: true}, v.size.X, v.size.Y, r.method)
	src := r.cache.Get(v.s)
	v.ok = src.Width() == v.size.X && src.Height() == v.size.Y && src.Stretchable(v.mid)
	return v
}
