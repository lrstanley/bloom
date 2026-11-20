// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"cmp"
	"image"

	"bloomfx.dev/bloom/internal/anchor"
)

// arrangeLayer places a layer of an overlay laid out in r.
func (e *engine) arrangeLayer(i int, r image.Rectangle) {
	n, _ := e.boxes[i].child.(LayerNode)
	if n.float {
		r = e.area
	}
	area := r.Inset(n.margin)
	kid := e.firstKid(i)

	// The components in the layer see the side it goes on, which is the one
	// asked for until it is known whether the layer fits there.
	defer func(side Direction) { e.side = side }(e.side)
	e.side = 0
	var target image.Rectangle
	if n.anchor != nil {
		var ok bool
		if target, ok = e.rectOf(n.anchor); !ok {
			e.boxes[i].hidden = true
			return
		}
		e.side = cmp.Or(n.side, Bottom)
	}

	built := len(e.boxes)
	w, h := e.layerSize(n, kid, area)
	at := area.Min.Add(image.Pt(n.x.offset(w, area.Dx()), n.y.offset(h, area.Dy())))
	if n.anchor != nil {
		var side Direction
		off := image.Pt(n.offsetX, n.offsetY)
		at, side = anchor.Place(target, area, image.Pt(w, h), e.side, off)
		if side != e.side && len(e.boxes) > built {
			// Measuring built components, which may have built themselves
			// for the side asked for, so the layer's child is built again
			// for the side it goes on, and placed there at its new size.
			// What was built before stays in the arena, unreachable and
			// never arranged, as boxes a Layouter built for other space do.
			kid = e.build(n.child, i, 0)
			b := &e.boxes[i]
			b.kidsOff, b.kidsLen = len(e.kids), 1
			e.kids = append(e.kids, kid)
			asked := e.side
			e.side = side
			w, h = e.layerSize(n, kid, area)
			at = anchor.On(target, area, image.Pt(w, h), asked, side, off)
		}
		e.side = side
	} else {
		at = at.Add(image.Pt(n.offsetX, n.offsetY))
	}

	lr := image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}
	if e.motion != nil && n.id != nil && n.animation != nil {
		lr = e.animate(i, n.id, n.animation, n.moves, lr)
		defer func(at image.Point) { e.moved = at }(e.moved)
		e.moved = lr.Min
	}
	e.boxes[i].rect = lr
	e.arrange(kid, lr)
}

// layerSize returns the size of layer n, whose child is box kid, within
// area. A side the layer fits to its child is measured with the other side
// at the layer's size, as the child is drawn, so content that wraps in a
// layer of fixed width gets the height it wraps to there.
func (e *engine) layerSize(n LayerNode, kid int, area image.Rectangle) (w, h int) {
	w, h = min(max(n.w, 0), area.Dx()), min(max(n.h, 0), area.Dy())
	if w == 0 || h == 0 {
		s := e.measure(kid, Size{W: cmp.Or(w, area.Dx()), H: cmp.Or(h, area.Dy())})
		w, h = cmp.Or(w, s.W), cmp.Or(h, s.H)
	}
	return w, h
}

// rectOf returns the rectangle of the box with ID id, if some of it is on
// screen: laid out, and not clipped away by the boxes it is in.
func (e *engine) rectOf(id any) (image.Rectangle, bool) {
	if i := e.shown(id); i >= 0 {
		return e.boxes[i].rect, true
	}
	return image.Rectangle{}, false
}

// shown returns the index of the box with ID id, if some of it is on screen,
// or -1.
func (e *engine) shown(id any) int {
	for i := range e.boxes {
		b := &e.boxes[i]
		if b.meta.id != id || b.hidden {
			continue
		}
		// A layer that floats is clipped to the screen, not to what it is in.
		vis := b.rect.Intersect(e.area)
		for p := i; !vis.Empty() && !e.boxes[p].float && e.boxes[p].parent >= 0; {
			p = e.boxes[p].parent
			vis = vis.Intersect(e.boxes[p].rect)
		}
		if !vis.Empty() {
			return i
		}
	}
	return -1
}
