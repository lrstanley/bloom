// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"image"
	"math"
)

// Item is an item of a list that an end of a selection is in: the list, by
// where it is inside the node selected in, and the item's index.
type Item struct {
	List  uint64
	Index int
}

// MaxNest is how many lists inside items of each other an end of a
// selection keeps the items of. An end in a list nested deeper is kept as
// the items of the outer lists are.
const MaxNest = 4

// Place is what ordering an end of a selection takes of it, besides its
// leaf: Edge, for the start, -1, or the end, 1, of all of the text of the
// node selected in, and the items of the lists the end is in, outermost
// first, of which the first Nest are kept, so that the end is kept while a
// list does not build its item.
type Place struct {
	Edge  int8
	Items [MaxNest]Item
	Nest  int
}

// Leaf is a leaf of text noted in an [Outline]: where all of it is laid
// out, and what of it is in sight, which is empty when it is out of sight;
// whether it is inner, cells drawn for a node that are not text of their
// own; whether it is open, a leaf that does not tell how much of its text
// is out of sight; and the innermost list it is in, by its index in the
// lists noted, or -1, and its item of it.
type Leaf struct {
	Rect, Clip image.Rectangle
	Inner      bool
	Open       bool
	List, Item int
}

// List is a list among the leaves noted: where it is inside the node
// selected in, the caller's node of it, where it is laid out, how many
// items it has, which it
// built, from First to Last, which is First-1 when it built none, its marks
// among the leaves, and the list and item it is in, or -1. The items it did
// not build are not among the leaves; their text is read at its marks.
type List struct {
	Path        uint64
	Box         int
	Rect        image.Rectangle
	Count       int
	First, Last int
	Open, Close int
	Parent      int
	Item        int
}

// Mark is where the items of a list that it did not build go among the
// leaves: those before the first it built, or with Close, after the last,
// before leaf At, and after any mark before it.
type Mark struct {
	List  int
	At    int
	Close bool
}

// StopKind is what a [Stop] is at.
type StopKind uint8

const (
	StopNone  StopKind = iota // nowhere: its leaf is not in the frame
	StopLeaf                  // at a leaf
	StopMark                  // at a mark, in an item of the list that it did not build
	StopBegin                 // before all the leaves
	StopEnd                   // after all the leaves
	StopLost                  // nowhere, for good: its list left, or has no item at its index
)

// Stop is where an end of a selection is in the leaves noted: at leaf or
// mark I, and for a mark, the end's item in the list of it is at Level of
// the end's items.
type Stop struct {
	Kind  StopKind
	I     int
	Level int
}

// Outline is the order of the leaves of text of the node selected in, as
// they were drawn, and of the lists among them, with marks where the items
// they did not build go, to find where the ends of a selection are in it
// and walk the text between them. The caller keeps what the leaves hold,
// at the same indexes.
type Outline struct {
	Leaves []Leaf
	Lists  []List
	Marks  []Mark
	// list and item are the list, and item, the leaves being noted are in,
	// or -1.
	list, item int
}

// Reset empties o, to note the leaves of a node again.
func (o *Outline) Reset() {
	o.Leaves, o.Lists, o.Marks = o.Leaves[:0], o.Lists[:0], o.Marks[:0]
	o.list, o.item = -1, -1
}

// AddLeaf notes leaf l, the next, in the item being noted.
func (o *Outline) AddLeaf(l Leaf) {
	l.List, l.Item = o.list, o.item
	o.Leaves = append(o.Leaves, l)
}

// OpenList notes list l, inside the item being noted, with the mark before
// the items it built, and returns its index, for the leaves of its items to
// be noted with [Outline.InItem], and then [Outline.CloseList].
func (o *Outline) OpenList(l List) int {
	k := len(o.Lists)
	l.Open, l.Parent, l.Item = len(o.Marks), o.list, o.item
	o.Lists = append(o.Lists, l)
	o.Marks = append(o.Marks, Mark{List: k, At: len(o.Leaves)})
	return k
}

// InItem notes the leaves after it as in item of list k.
func (o *Outline) InItem(k, item int) { o.list, o.item = k, item }

// CloseList notes the mark after the items list k built, and the leaves
// after it as in the item the list is in.
func (o *Outline) CloseList(k int) {
	l := &o.Lists[k]
	o.list, o.item = l.Parent, l.Item
	l.Close = len(o.Marks)
	o.Marks = append(o.Marks, Mark{List: k, At: len(o.Leaves), Close: true})
}

// PlaceOf sets the items of p to those of the lists leaf i is in.
func (o *Outline) PlaceOf(i int, p *Place) {
	n := 0
	for k := o.Leaves[i].List; k >= 0; k = o.Lists[k].Parent {
		n++
	}
	p.Nest = min(n, MaxNest)
	for k, j := o.Leaves[i].List, o.Leaves[i].Item; k >= 0; k, j = o.Lists[k].Parent, o.Lists[k].Item {
		n--
		if n < MaxNest {
			p.Items[n] = Item{List: o.Lists[k].Path, Index: j}
		}
	}
}

// findList returns the index of the list at path, or -1.
func (o *Outline) findList(path uint64) int {
	for i := range o.Lists {
		if o.Lists[i].Path == path {
			return i
		}
	}
	return -1
}

// StopOf returns where the end at p, whose leaf is leaf i of those noted,
// or -1 when it is not among them, is: at its leaf, or, when that is not
// built, at the mark of the outermost of its lists, from level on, that
// did not build the item it is in.
func (o *Outline) StopOf(p *Place, leaf, level int) Stop {
	switch p.Edge {
	case -1:
		return Stop{Kind: StopBegin}
	case 1:
		return Stop{Kind: StopEnd}
	}
	if leaf >= 0 {
		return Stop{Kind: StopLeaf, I: leaf}
	}
	for lv := level; lv < p.Nest; lv++ {
		it := p.Items[lv]
		k := o.findList(it.List)
		if k < 0 {
			return Stop{Kind: StopLost}
		}
		l := &o.Lists[k]
		switch {
		case it.Index >= l.Count:
			return Stop{Kind: StopLost}
		case it.Index < l.First:
			return Stop{Kind: StopMark, I: l.Open, Level: lv}
		case it.Index > l.Last:
			return Stop{Kind: StopMark, I: l.Close, Level: lv}
		}
	}
	return Stop{}
}

// stopKey orders the places in the leaves noted: each mark comes before the
// leaf it is at, and after the marks before it.
type stopKey struct{ at, sub int }

func (k stopKey) less(o stopKey) bool { return k.at < o.at || k.at == o.at && k.sub < o.sub }

func (o *Outline) key(s Stop) stopKey {
	switch s.Kind {
	case StopBegin:
		return stopKey{at: -1}
	case StopEnd:
		return stopKey{at: math.MaxInt, sub: math.MaxInt}
	case StopMark:
		return stopKey{at: o.Marks[s.I].At, sub: s.I}
	case StopNone, StopLeaf, StopLost:
	}
	return stopKey{at: s.I, sub: math.MaxInt}
}

// Before reports whether the end at p, at stop a, comes before the end at
// q, at stop b, and whether that can be told: not when both are in the
// same item that is not built. Two items of the same list are in the order
// of their indexes. Two ends at the same leaf are not told apart: the
// caller orders them by where they are in it.
func (o *Outline) Before(p *Place, a Stop, q *Place, b Stop) (before, known bool) {
	if a.Kind == StopLeaf && b.Kind == StopLeaf {
		return a.I < b.I, a.I != b.I
	}
	for k := range min(p.Nest, q.Nest) {
		x, y := p.Items[k], q.Items[k]
		if x.List != y.List {
			break
		}
		if x.Index != y.Index {
			return x.Index < y.Index, true
		}
	}
	ka, kb := o.key(a), o.key(b)
	return ka.less(kb), ka != kb
}

// FirstAfter returns the first leaf from stop s on that is not inner, and
// LastBefore the last up to it, which may be past the leaves.
func (o *Outline) FirstAfter(s Stop) int {
	k := 0
	switch s.Kind {
	case StopLeaf:
		return s.I
	case StopMark:
		k = o.Marks[s.I].At
	case StopNone, StopBegin, StopEnd, StopLost:
	}
	for k < len(o.Leaves) && o.Leaves[k].Inner {
		k++
	}
	return k
}

func (o *Outline) LastBefore(s Stop) int {
	k := len(o.Leaves) - 1
	switch s.Kind {
	case StopLeaf:
		return s.I
	case StopMark:
		k = o.Marks[s.I].At - 1
	case StopNone, StopBegin, StopEnd, StopLost:
	}
	for k >= 0 && o.Leaves[k].Inner {
		k--
	}
	return k
}

// Walk calls leaf with each leaf from stop a up to stop b, and mark with
// each mark between them, in order, and reports whether every mark
// returned true, stopping at the first that does not.
func (o *Outline) Walk(a, b Stop, leaf func(i int), mark func(m int) bool) bool {
	l, m := 0, 0
	switch a.Kind {
	case StopLeaf:
		l = a.I
		for m < len(o.Marks) && o.Marks[m].At <= l {
			m++
		}
	case StopMark:
		l, m = o.Marks[a.I].At, a.I
	case StopNone, StopBegin, StopEnd, StopLost:
	}
	last := o.key(b)
	for {
		if m < len(o.Marks) && o.Marks[m].At <= l {
			if last.less(stopKey{at: o.Marks[m].At, sub: m}) {
				return true
			}
			if !mark(m) {
				return false
			}
			m++
			continue
		}
		if l >= len(o.Leaves) || last.less(stopKey{at: l, sub: math.MaxInt}) {
			return true
		}
		leaf(l)
		l++
	}
}

// Unbuilt returns the items, from from up to to, inclusive, of the list of
// mark m that it did not build and that the selection from the end at pa,
// at stop a, to the end at pb, at stop b, takes: those before the first it
// built, or after the last, from the item the first end is in when it is
// at m, and up to the one the last is in when it is, which in and out
// report.
func (o *Outline) Unbuilt(m int, a Stop, pa *Place, b Stop, pb *Place) (from, to int, in, out bool) {
	mk := o.Marks[m]
	l := &o.Lists[mk.List]
	from, to = 0, l.First-1
	if mk.Close {
		from, to = l.Last+1, l.Count-1
	}
	in = a.Kind == StopMark && a.I == m
	out = b.Kind == StopMark && b.I == m
	if in {
		from = pa.Items[a.Level].Index
	}
	if out {
		to = pb.Items[b.Level].Index
	}
	return from, to, in, out
}

// LeafAt returns the leaf that the pointer at p selects in, and the cell of
// it, relative to its top-left, at p: the leaf under it, in sight or else
// laid out there, the last noted first; or else the nearest before it in
// reading order, preferring one above or below it; or else the first, at
// the row above it. It returns -1 when there are only inner leaves.
func (o *Outline) LeafAt(p image.Point) (int, image.Point) {
	for _, inSight := range [2]bool{true, false} {
		for i := len(o.Leaves) - 1; i >= 0; i-- {
			l := &o.Leaves[i]
			in := l.Rect
			if inSight {
				in = l.Clip
			}
			if !l.Inner && p.In(in) {
				return i, p.Sub(l.Rect.Min)
			}
		}
	}
	best, first := -1, -1
	for i := range o.Leaves {
		l := &o.Leaves[i]
		if l.Inner || l.Rect.Empty() {
			continue
		}
		if first < 0 {
			first = i
		}
		rows := p.Y >= l.Rect.Min.Y && p.Y < l.Rect.Max.Y
		if p.Y < l.Rect.Max.Y && (!rows || p.X < l.Rect.Min.X) {
			continue // after p
		}
		if best < 0 || inColumn(l, p) || !inColumn(&o.Leaves[best], p) {
			best = i
		}
	}
	switch {
	case best >= 0:
		return best, p.Sub(o.Leaves[best].Rect.Min)
	case first >= 0:
		return first, image.Pt(0, -1)
	}
	return -1, image.Point{}
}

func inColumn(l *Leaf, p image.Point) bool { return p.X >= l.Rect.Min.X && p.X < l.Rect.Max.X }

// Endless is the extent of text that goes on without end, for what is out
// of sight of it.
var Endless = image.Rect(-math.MaxInt32, -math.MaxInt32, math.MaxInt32, math.MaxInt32)

// Extent returns what is in sight of the leaves that are not inner, and all
// of them, as laid out: Endless when a leaf is open, and endless above or
// below a list at that edge of what is in sight that did not build its
// items past it.
func (o *Outline) Extent() (shown, all image.Rectangle) {
	for i := range o.Leaves {
		if l := &o.Leaves[i]; !l.Inner {
			shown, all = shown.Union(l.Clip), all.Union(l.Rect)
			if l.Open {
				all = Endless
			}
		}
	}
	for i := range o.Lists {
		l := &o.Lists[i]
		if l.First > 0 && l.Rect.Min.Y <= shown.Min.Y {
			all.Min.Y = Endless.Min.Y
		}
		if l.Last < l.Count-1 && l.Rect.Max.Y >= shown.Max.Y {
			all.Max.Y = Endless.Max.Y
		}
	}
	return shown, all
}
