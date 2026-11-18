// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"fmt"
	"image"
	"slices"
	"testing"
)

// listOutline returns the outline of a leaf, a list of ten items that built
// items 3 and 4, with a leaf in item 3 and two in item 4, the first inner,
// and a leaf after the list.
func listOutline() *Outline {
	var o Outline
	o.Reset()
	o.AddLeaf(Leaf{}) // 0
	k := o.OpenList(List{Path: 1, Box: 7, Count: 10, First: 3, Last: 4})
	o.InItem(k, 3)
	o.AddLeaf(Leaf{}) // 1
	o.InItem(k, 4)
	o.AddLeaf(Leaf{Inner: true}) // 2
	o.AddLeaf(Leaf{})            // 3
	o.CloseList(k)
	o.AddLeaf(Leaf{}) // 4
	return &o
}

// inList returns the place of an end in item i of the list at path 1.
func inList(i int) *Place { return &Place{Items: [MaxNest]Item{{List: 1, Index: i}}, Nest: 1} }

func TestOutlineNotes(t *testing.T) {
	t.Parallel()

	o := listOutline()
	wantMarks := []Mark{{List: 0, At: 1}, {List: 0, At: 4, Close: true}}
	if !slices.Equal(o.Marks, wantMarks) {
		t.Fatalf("marks %+v", o.Marks)
	}
	if l := o.Lists[0]; l.Open != 0 || l.Close != 1 || l.Parent != -1 || l.Item != -1 {
		t.Fatalf("list %+v", l)
	}
	if l := o.Leaves[4]; l.List != -1 {
		t.Fatalf("the leaf after the list is in list %d", l.List)
	}
	var p Place
	o.PlaceOf(3, &p)
	if p.Nest != 1 || p.Items[0] != (Item{List: 1, Index: 4}) {
		t.Fatalf("leaf 3 is at %+v", p)
	}

	// A list in an item of another keeps the items of both, outermost first.
	o.Reset()
	outer := o.OpenList(List{Path: 1, Count: 5, First: 0, Last: 0})
	o.InItem(outer, 0)
	inner := o.OpenList(List{Path: 2, Count: 3, First: 1, Last: 1})
	o.InItem(inner, 1)
	o.AddLeaf(Leaf{})
	o.CloseList(inner)
	o.AddLeaf(Leaf{})
	o.CloseList(outer)
	o.PlaceOf(0, &p)
	if p.Nest != 2 || p.Items[0] != (Item{List: 1, Index: 0}) || p.Items[1] != (Item{List: 2, Index: 1}) {
		t.Fatalf("the nested leaf is at %+v", p)
	}
	o.PlaceOf(1, &p)
	if p.Nest != 1 || p.Items[0] != (Item{List: 1, Index: 0}) {
		t.Fatalf("the leaf after the nested list is at %+v", p)
	}
}

func TestOutlineStops(t *testing.T) {
	t.Parallel()

	o := listOutline()
	for _, tc := range []struct {
		name string
		p    *Place
		leaf int
		want Stop
	}{
		{"its leaf", inList(4), 3, Stop{Kind: StopLeaf, I: 3}},
		{"an item before", inList(1), -1, Stop{Kind: StopMark, I: 0}},
		{"an item after", inList(8), -1, Stop{Kind: StopMark, I: 1}},
		{"an item past the count", inList(12), -1, Stop{Kind: StopLost}},
		{"a list that left", &Place{Items: [MaxNest]Item{{List: 9}}, Nest: 1}, -1, Stop{Kind: StopLost}},
		{"an item built, without its leaf", inList(3), -1, Stop{}},
		{"the start", &Place{Edge: -1}, 2, Stop{Kind: StopBegin}},
		{"the end", &Place{Edge: 1}, -1, Stop{Kind: StopEnd}},
	} {
		if got := o.StopOf(tc.p, tc.leaf, 0); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}

	leaf := func(i int) Stop { return Stop{Kind: StopLeaf, I: i} }
	open, closed := Stop{Kind: StopMark, I: 0}, Stop{Kind: StopMark, I: 1}
	none := &Place{}
	for _, tc := range []struct {
		name          string
		p             *Place
		a             Stop
		q             *Place
		b             Stop
		before, known bool
	}{
		{"leaves in order", none, leaf(1), none, leaf(4), true, true},
		{"the same leaf", none, leaf(1), none, leaf(1), false, false},
		{"items of the list", inList(1), open, inList(8), closed, true, true},
		{"items in order, either side", inList(8), closed, inList(1), open, false, true},
		{"the same item", inList(1), open, inList(1), open, false, false},
		{"a mark and the leaf before it", inList(1), open, none, leaf(0), false, true},
		{"an item and the leaf of a later one", inList(1), open, inList(4), leaf(3), true, true},
		{"the start", &Place{Edge: -1}, Stop{Kind: StopBegin}, none, leaf(0), true, true},
	} {
		if before, known := o.Before(tc.p, tc.a, tc.q, tc.b); before != tc.before || known != tc.known {
			t.Errorf("%s: %v, %v, want %v, %v", tc.name, before, known, tc.before, tc.known)
		}
	}

	if got := o.FirstAfter(open); got != 1 {
		t.Errorf("the first leaf after the open mark is %d", got)
	}
	if got := o.LastBefore(closed); got != 3 {
		t.Errorf("the last leaf before the close mark is %d", got)
	}
	inner := Outline{Leaves: []Leaf{{Inner: true}, {}, {Inner: true}}}
	if first, last := inner.FirstAfter(Stop{Kind: StopBegin}), inner.LastBefore(Stop{Kind: StopEnd}); first != 1 || last != 1 {
		t.Errorf("past the inner leaves: %d and %d", first, last)
	}
}

func TestOutlineWalk(t *testing.T) {
	t.Parallel()

	o := listOutline()
	walk := func(a, b Stop, stopAt string) (string, bool) {
		var got string
		ok := o.Walk(a, b,
			func(i int) { got += fmt.Sprintf("L%d ", i) },
			func(m int) bool { got += fmt.Sprintf("M%d ", m); return got != stopAt })
		return got, ok
	}
	for _, tc := range []struct {
		a, b   Stop
		stopAt string
		want   string
		ok     bool
	}{
		{Stop{Kind: StopBegin}, Stop{Kind: StopEnd}, "", "L0 M0 L1 L2 L3 M1 L4 ", true},
		{Stop{Kind: StopMark, I: 0}, Stop{Kind: StopLeaf, I: 3}, "", "M0 L1 L2 L3 ", true},
		{Stop{Kind: StopLeaf, I: 1}, Stop{Kind: StopMark, I: 1}, "", "L1 L2 L3 M1 ", true},
		// A mark stops it; a leaf cannot.
		{Stop{Kind: StopBegin}, Stop{Kind: StopEnd}, "L0 M0 ", "L0 M0 ", false},
	} {
		if got, ok := walk(tc.a, tc.b, tc.stopAt); got != tc.want || ok != tc.ok {
			t.Errorf("from %+v to %+v: %q, %v, want %q, %v", tc.a, tc.b, got, ok, tc.want, tc.ok)
		}
	}

	// The items not built that a selection takes at each mark.
	open, closed := Stop{Kind: StopMark, I: 0}, Stop{Kind: StopMark, I: 1}
	begin, end := Stop{Kind: StopBegin}, Stop{Kind: StopEnd}
	for _, tc := range []struct {
		m        int
		a        Stop
		pa       *Place
		b        Stop
		pb       *Place
		from, to int
		in, out  bool
	}{
		{0, open, inList(1), end, &Place{}, 1, 2, true, false},
		{1, begin, &Place{}, closed, inList(8), 5, 8, false, true},
		{1, begin, &Place{}, end, &Place{}, 5, 9, false, false},
		{0, begin, &Place{}, end, &Place{}, 0, 2, false, false},
	} {
		from, to, in, out := o.Unbuilt(tc.m, tc.a, tc.pa, tc.b, tc.pb)
		if from != tc.from || to != tc.to || in != tc.in || out != tc.out {
			t.Errorf("mark %d: items %d to %d, %v %v, want %d to %d, %v %v", tc.m, from, to, in, out, tc.from, tc.to, tc.in, tc.out)
		}
	}
}

func TestOutlineGeometry(t *testing.T) {
	t.Parallel()

	// Two leaves side by side on rows 0 and 1, a third below them scrolled
	// half out of sight, and an inner leaf over the first.
	var o Outline
	o.Reset()
	o.AddLeaf(Leaf{Rect: image.Rect(0, 0, 5, 2), Clip: image.Rect(0, 0, 5, 2)})
	o.AddLeaf(Leaf{Rect: image.Rect(6, 0, 10, 2), Clip: image.Rect(6, 0, 10, 2)})
	o.AddLeaf(Leaf{Rect: image.Rect(0, 3, 10, 9), Clip: image.Rect(0, 3, 10, 5)})
	o.AddLeaf(Leaf{Rect: image.Rect(0, 0, 5, 2), Clip: image.Rect(0, 0, 5, 2), Inner: true})
	for _, tc := range []struct {
		p    image.Point
		leaf int
		at   image.Point
	}{
		{image.Pt(2, 1), 0, image.Pt(2, 1)},  // under it, not the inner leaf
		{image.Pt(7, 0), 1, image.Pt(1, 0)},  // under it
		{image.Pt(3, 7), 2, image.Pt(3, 4)},  // laid out there, out of sight
		{image.Pt(5, 1), 0, image.Pt(5, 1)},  // between two on the row: the one before
		{image.Pt(3, 2), 0, image.Pt(3, 2)},  // below two: the one above in its column
		{image.Pt(12, 2), 1, image.Pt(6, 2)}, // past them: the nearest before
		{image.Pt(3, 12), 2, image.Pt(3, 9)},
	} {
		if leaf, at := o.LeafAt(tc.p); leaf != tc.leaf || at != tc.at {
			t.Errorf("at %v: leaf %d at %v, want %d at %v", tc.p, leaf, at, tc.leaf, tc.at)
		}
	}
	// Above them all, the first, at the row above it.
	var below Outline
	below.AddLeaf(Leaf{Rect: image.Rect(0, 4, 5, 6)})
	if leaf, at := below.LeafAt(image.Pt(2, 1)); leaf != 0 || at != image.Pt(0, -1) {
		t.Errorf("above: leaf %d at %v", leaf, at)
	}
	var inner Outline
	inner.AddLeaf(Leaf{Rect: image.Rect(0, 0, 5, 5), Inner: true})
	if leaf, _ := inner.LeafAt(image.Pt(1, 1)); leaf != -1 {
		t.Errorf("found inner leaf %d", leaf)
	}

	shown, all := o.Extent()
	if shown != image.Rect(0, 0, 10, 5) || all != image.Rect(0, 0, 10, 9) {
		t.Fatalf("extent %v of %v", shown, all)
	}
	// A list at the top of what is in sight that did not build its first
	// items goes on above it, and an open leaf on every way.
	k := o.OpenList(List{Rect: image.Rect(0, 0, 10, 9), Count: 10, First: 2, Last: 9})
	o.CloseList(k)
	if _, all = o.Extent(); all.Min.Y != Endless.Min.Y || all.Max.Y != 9 {
		t.Fatalf("with the list, all of it is %v", all)
	}
	o.AddLeaf(Leaf{Rect: image.Rect(0, 9, 10, 10), Open: true})
	if _, all = o.Extent(); all != Endless {
		t.Fatalf("with an open leaf, all of it is %v", all)
	}
}
