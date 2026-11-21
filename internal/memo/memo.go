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

// Map keeps a value for each key, as a [Table] does for each index, for
// values that a box can have more than one of in a frame.
type Map[K comparable, T any] struct {
	slots map[K]*Slot[T]
	peak  int    // the most slots it has held since it was last made
	gen   uint64 // the frame being drawn
}

// At returns the slot for key k, marked used by this frame.
func (m *Map[K, T]) At(k K) *Slot[T] {
	s := m.slots[k]
	if s == nil {
		if m.slots == nil {
			m.slots = make(map[K]*Slot[T])
		}
		s = new(Slot[T])
		m.slots[k] = s
	}
	s.used = m.gen
	return s
}

// Retire drops the values this frame did not use, and ends it. A map does
// not shrink as keys are deleted, and ranging over it costs as much as at
// its largest, so it is made again once it holds far fewer than it did.
func (m *Map[K, T]) Retire() {
	for k, s := range m.slots {
		if s.used != m.gen {
			delete(m.slots, k)
		}
	}
	m.gen++
	if n := len(m.slots); n < m.peak/4 {
		slots := make(map[K]*Slot[T], n)
		for k, s := range m.slots {
			slots[k] = s
		}
		m.slots, m.peak = slots, n
	} else {
		m.peak = max(m.peak, n)
	}
}
