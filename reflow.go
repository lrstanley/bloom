// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"time"

	"github.com/lrstanley/bloom/anim"
)

type layoutAnimation struct{ opts []anim.Option }

type layoutFrameMsg struct{ root *Root }

// rectAnimation is the animation of a box with an ID: its place gliding, as
// four values, or, for a box that moves with effects, where it was and the
// progress from there. Either way, at is where it was last laid out,
// relative to the origin it moves with, for the other to carry on from.
type rectAnimation struct {
	values   [4]anim.Value
	seen     bool
	moving   bool // moves with effects rather than values.
	at, from image.Rectangle
	progress anim.Value
}

// gliding reports whether a box's place is still gliding.
func (s *rectAnimation) gliding() bool {
	for i := range s.values {
		if s.values[i].Animating() {
			return true
		}
	}
	return false
}

// layoutMotion belongs only to a retained Root; pooled renderers stay stateless.
type layoutMotion struct {
	now           func() time.Time
	states        map[any]*rectAnimation
	at            time.Time
	size          image.Point
	snap, running bool
}

func (m *layoutMotion) begin(w, h int) {
	size := image.Pt(w, h)
	m.snap, m.size, m.running = size != m.size, size, false
	m.at = time.Time{}
	for _, s := range m.states {
		s.seen = false
	}
}

// rect returns where the box with ID id goes this frame, on its way to
// target. Its position animates relative to origin, where the nearest
// animated box it is in is on screen, so that it moves with that box rather
// than trailing behind it.
func (m *layoutMotion) rect(id any, cfg *layoutAnimation, target image.Rectangle, origin image.Point) image.Rectangle {
	if m.at.IsZero() {
		m.at = m.now()
	}
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
		s = &rectAnimation{}
		for i, c := range from {
			s.values[i] = anim.NewValue(float64(c), cfg.opts...)
		}
		if m.states == nil {
			m.states = make(map[any]*rectAnimation)
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

// move returns where the box with ID id was, and how far it has moved from
// there to target, from 0 to 1, for a box that moves with effects: it is
// drawn at target, and effects draw it on its way. It reports false when it
// is not moving. Like rect, it works relative to origin, so a box moving
// with a gliding ancestor has not moved. A box still gliding, as rect left
// it, is not moving with effects; the caller glides it on with rect.
func (m *layoutMotion) move(id any, cfg *layoutAnimation, target image.Rectangle, origin image.Point) (from image.Rectangle, t float64, ok bool) {
	if m.at.IsZero() {
		m.at = m.now()
	}
	rel := target.Sub(origin)
	s := m.states[id]
	if s == nil || !s.moving {
		// It moves from where it was last laid out, if it glided there.
		at := rel
		if s != nil {
			at = s.at
		}
		s = &rectAnimation{moving: true, at: at, progress: anim.NewValue(1, cfg.opts...)}
		if m.states == nil {
			m.states = make(map[any]*rectAnimation)
		}
		m.states[id] = s
	}
	s.seen = true
	if rel != s.at && !m.snap {
		s.from, s.at = s.at, rel
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

// gliding reports whether the box with ID id is gliding, as rect moves it.
func (m *layoutMotion) gliding(id any) bool {
	s := m.states[id]
	return s != nil && !s.moving && s.gliding()
}

func (m *layoutMotion) end() {
	for id, s := range m.states {
		if !s.seen {
			delete(m.states, id)
		}
	}
}
