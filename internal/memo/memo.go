// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package memo keeps a value for each box of a tree, by its index, from one
// frame to the next, for boxes that tend to stay the same.
package memo

// Table keeps a value for each index. Values a frame did not use are
// dropped once it is done, as they would keep what they refer to alive.
type Table[T any] struct {
	slots []Slot[T]
	gen   uint64 // the frame being drawn
}

// Slot is a value kept for an index, V, once OK.
type Slot[T any] struct {
	OK   bool
	V    T
	used uint64 // the frame it was last used in
}

// At returns the slot at index i, marked used by this frame.
func (t *Table[T]) At(i int) *Slot[T] {
	if i >= len(t.slots) {
		t.slots = append(t.slots, make([]Slot[T], i+1-len(t.slots))...)
	}
	s := &t.slots[i]
	s.used = t.gen
	return s
}

// Retire drops the values this frame did not use, and ends it.
func (t *Table[T]) Retire() {
	slots, gen := t.slots, t.gen
	for i := range slots {
		if slots[i].used != gen && slots[i].OK {
			slots[i] = Slot[T]{}
		}
	}
	t.gen++
}
