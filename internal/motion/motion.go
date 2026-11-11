// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package motion tracks the layout animations of boxes with IDs from one
// frame to the next: boxes gliding to where layout puts them, and boxes
// moving there with effects.
package motion

import (
	"image"
	"time"

	"bloomfx.dev/bloom/anim"
)

// state is the animation of a box with an ID: its place gliding, as four
// values, or, for a box that moves with effects, where it was and the
// progress from there. Either way, at is where it was last laid out,
// relative to the origin it moves with, for the other to carry on from.
type state struct {
	values   [4]anim.Value
	seen     bool
	moving   bool // moves with effects rather than values.
	at, from image.Rectangle
	progress anim.Value
}

// gliding reports whether a box's place is still gliding.
func (s *state) gliding() bool {
	for i := range s.values {
		if s.values[i].Animating() {
			return true
		}
	}
	return false
}

// Tracker keeps the layout animations of boxes by ID across frames. Call
// [Tracker.Begin] before laying out a frame and [Tracker.End] after it.
type Tracker struct {
	now           func() time.Time
	states        map[any]*state
	at            time.Time
	size          image.Point
	snap, running bool
}

// New returns a Tracker that reads the time from now.
func New(now func() time.Time) *Tracker {
	return &Tracker{now: now, states: make(map[any]*state)}
}

// Begin starts a w×h frame. When the size differs from the last frame's,
// boxes snap to where they go rather than animating there.
func (m *Tracker) Begin(w, h int) {
	size := image.Pt(w, h)
	m.snap, m.size, m.running = size != m.size, size, false
	m.at = time.Time{}
	for _, s := range m.states {
		s.seen = false
	}
}

// End forgets the boxes that were not laid out since [Tracker.Begin].
func (m *Tracker) End() {
	for id, s := range m.states {
		if !s.seen {
			delete(m.states, id)
		}
	}
}

// Running reports whether any animation laid out this frame is still going.
func (m *Tracker) Running() bool { return m.running }

// Len returns how many boxes have animation state.
func (m *Tracker) Len() int { return len(m.states) }

// Values returns the current x, y, width, and height values of the box with
// ID id as it glides, before rounding to cells, for tests that check
// progress finer than a cell.
func (m *Tracker) Values(id any) (v [4]float64) {
	if s := m.states[id]; s != nil {
		for i := range s.values {
			v[i] = s.values[i].Float()
		}
	}
	return v
}

// Rect returns where the box with ID id goes this frame, on its way to
// target. Its position animates relative to origin, where the nearest
// animated box it is in is on screen, so that it moves with that box rather
// than trailing behind it. opts configure its values when it starts gliding.
func (m *Tracker) Rect(id any, opts []anim.Option, target image.Rectangle, origin image.Point) image.Rectangle {
	m.clock()
	rel := target.Min.Sub(origin)
	coords := [4]int{rel.X, rel.Y, max(target.Dx(), 0), max(target.Dy(), 0)}
	s := m.states[id]
	if s == nil || s.moving {
		// It glides from where it was last laid out, if it was moving with
		// effects, which leave it there.
		from := coords
		if s != nil {
			from = [4]int{s.at.Min.X, s.at.Min.Y, s.at.Dx(), s.at.Dy()}
		}
		s = &state{}
		for i, c := range from {
			s.values[i] = anim.NewValue(float64(c), opts...)
		}
		m.states[id] = s
	}
	s.seen = true
	s.at = image.Rectangle{Min: rel, Max: rel.Add(target.Size())}
	for i, c := range coords {
		v := &s.values[i]
		if m.snap {
			v.Set(float64(c))
		} else {
			// Moving parents can change a child's target every frame. Sample its
			// existing motion before retargeting so it keeps making progress.
			if v.Target() != float64(c) {
				v.Advance(m.at)
			}
			v.SetTarget(float64(c), m.at)
		}
		if v.Advance(m.at) {
			m.running = true
		}
		coords[i] = v.Int()
	}
	at := image.Pt(coords[0], coords[1]).Add(origin)
	return image.Rectangle{Min: at, Max: at.Add(image.Pt(max(coords[2], 0), max(coords[3], 0)))}
}

// Move returns where the box with ID id was, and how far it has moved from
// there to target, from 0 to 1, for a box that moves with effects: it is
// drawn at target, and effects draw it on its way. It reports false when it
// is not moving. Like [Tracker.Rect], it works relative to origin, so a box
// moving with a gliding ancestor has not moved. A box still gliding, as Rect
// left it, is not moving with effects; the caller glides it on with Rect.
// opts configure its progress when it starts moving with effects.
func (m *Tracker) Move(id any, opts []anim.Option, target image.Rectangle, origin image.Point) (from image.Rectangle, t float64, ok bool) {
	m.clock()
	rel := target.Sub(origin)
	s := m.states[id]
	if s == nil || !s.moving {
		// It moves from where it was last laid out, if it glided there.
		at := rel
		if s != nil {
			at = s.at
		}
		s = &state{moving: true, at: at, progress: anim.NewValue(1, opts...)}
		m.states[id] = s
	}
	s.seen = true
	if rel != s.at && !m.snap {
		s.from = s.at
		s.progress.Set(0)
		s.progress.SetTarget(1, m.at)
	}
	s.at = rel
	if m.snap {
		s.progress.Set(1)
	}
	if s.progress.Advance(m.at) {
		m.running = true
	}
	if !s.progress.Animating() {
		return image.Rectangle{}, 1, false
	}
	// A spring may overshoot, which an effect's progress does not.
	return s.from.Add(origin), min(max(s.progress.Float(), 0), 1), true
}

// clock reads the time of the frame once, when the first animated box of
// the frame is laid out, so frames without one do not read it.
func (m *Tracker) clock() {
	if m.at.IsZero() {
		m.at = m.now()
	}
}

// Gliding reports whether the box with ID id is gliding, as [Tracker.Rect]
// moves it.
func (m *Tracker) Gliding(id any) bool {
	s := m.states[id]
	return s != nil && !s.moving && s.gliding()
}
