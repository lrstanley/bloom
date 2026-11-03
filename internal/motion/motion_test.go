// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package motion

import (
	"image"
	"testing"
	"time"

	"github.com/lrstanley/bloom/anim"
)

var linear = []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}

// frame lays out one frame of a w×h screen, running fn between Begin and End.
func frame(m *Tracker, w, h int, fn func()) {
	m.Begin(w, h)
	fn()
	m.End()
}

func TestRectGlides(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	var got image.Rectangle
	place := func(r image.Rectangle) func() {
		return func() { got = m.Rect("a", linear, r, image.Point{}) }
	}

	frame(m, 100, 10, place(image.Rect(0, 0, 20, 10)))
	if got != image.Rect(0, 0, 20, 10) || m.Running() {
		t.Fatalf("first frame = %v, running %v", got, m.Running())
	}
	frame(m, 100, 10, place(image.Rect(20, 0, 60, 10)))
	if got != image.Rect(0, 0, 20, 10) || !m.Running() {
		t.Fatalf("retarget = %v, running %v", got, m.Running())
	}
	now = now.Add(500 * time.Millisecond)
	frame(m, 100, 10, place(image.Rect(20, 0, 60, 10)))
	if got != image.Rect(10, 0, 40, 10) {
		t.Fatalf("halfway = %v", got)
	}
	if v := m.Values("a"); v != [4]float64{10, 0, 30, 10} {
		t.Fatalf("values = %v", v)
	}
	if !m.Gliding("a") {
		t.Fatal("not gliding halfway")
	}
	now = now.Add(500 * time.Millisecond)
	frame(m, 100, 10, place(image.Rect(20, 0, 60, 10)))
	if got != image.Rect(20, 0, 60, 10) || m.Running() || m.Gliding("a") {
		t.Fatalf("settled = %v, running %v, gliding %v", got, m.Running(), m.Gliding("a"))
	}
}

func TestRectRelativeToOrigin(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	frame(m, 100, 10, func() { m.Rect("a", linear, image.Rect(5, 0, 10, 1), image.Pt(5, 0)) })
	// Its origin moved with it, so it has not moved.
	var got image.Rectangle
	frame(m, 100, 10, func() { got = m.Rect("a", linear, image.Rect(25, 0, 30, 1), image.Pt(25, 0)) })
	if got != image.Rect(25, 0, 30, 1) || m.Running() {
		t.Fatalf("moved with origin = %v, running %v", got, m.Running())
	}
}

func TestResizeSnaps(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	frame(m, 100, 10, func() { m.Rect("a", linear, image.Rect(0, 0, 20, 10), image.Point{}) })
	var got image.Rectangle
	frame(m, 80, 10, func() { got = m.Rect("a", linear, image.Rect(40, 0, 60, 10), image.Point{}) })
	if got != image.Rect(40, 0, 60, 10) || m.Running() {
		t.Fatalf("resized = %v, running %v", got, m.Running())
	}
}

func TestEndForgetsUnseen(t *testing.T) {
	m := New(time.Now)
	frame(m, 10, 10, func() {
		m.Rect("a", linear, image.Rect(0, 0, 1, 1), image.Point{})
		m.Rect("b", linear, image.Rect(0, 0, 1, 1), image.Point{})
	})
	if m.Len() != 2 {
		t.Fatalf("len = %d", m.Len())
	}
	frame(m, 10, 10, func() { m.Rect("a", linear, image.Rect(0, 0, 1, 1), image.Point{}) })
	if m.Len() != 1 {
		t.Fatalf("len after b left = %d", m.Len())
	}
	if v := m.Values("b"); v != [4]float64{} {
		t.Fatalf("values of a forgotten box = %v", v)
	}
}

func TestMove(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	var (
		from  image.Rectangle
		prog  float64
		moved bool
	)
	place := func(r image.Rectangle) func() {
		return func() { from, prog, moved = m.Move("a", linear, r, image.Point{}) }
	}

	frame(m, 100, 10, place(image.Rect(0, 0, 10, 1)))
	if moved {
		t.Fatal("moving on its first frame")
	}
	frame(m, 100, 10, place(image.Rect(50, 0, 60, 1)))
	if !moved || from != image.Rect(0, 0, 10, 1) || prog != 0 {
		t.Fatalf("started = %v %v %v", from, prog, moved)
	}
	if m.Gliding("a") {
		t.Fatal("a box moving with effects is gliding")
	}
	now = now.Add(500 * time.Millisecond)
	frame(m, 100, 10, place(image.Rect(50, 0, 60, 1)))
	if !moved || prog != 0.5 {
		t.Fatalf("halfway = %v %v", prog, moved)
	}
	now = now.Add(500 * time.Millisecond)
	frame(m, 100, 10, place(image.Rect(50, 0, 60, 1)))
	if moved || m.Running() {
		t.Fatalf("finished = %v, running %v", moved, m.Running())
	}
}

func TestMoveThenGlide(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	frame(m, 100, 10, func() { m.Move("a", linear, image.Rect(30, 0, 40, 1), image.Point{}) })
	// Switching to gliding starts from where it was last laid out.
	var got image.Rectangle
	frame(m, 100, 10, func() { got = m.Rect("a", linear, image.Rect(0, 0, 10, 1), image.Point{}) })
	if got != image.Rect(30, 0, 40, 1) || !m.Running() {
		t.Fatalf("glide start = %v, running %v", got, m.Running())
	}
}

func TestGlideThenMove(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	frame(m, 100, 10, func() { m.Rect("a", linear, image.Rect(30, 0, 40, 1), image.Point{}) })
	// Switching to moving with effects starts from where it was last laid out.
	var (
		from  image.Rectangle
		moved bool
	)
	frame(m, 100, 10, func() { from, _, moved = m.Move("a", linear, image.Rect(0, 0, 10, 1), image.Point{}) })
	if !moved || from != image.Rect(30, 0, 40, 1) {
		t.Fatalf("move start = %v, moved %v", from, moved)
	}
}

func TestMoveResizeSnaps(t *testing.T) {
	now := time.Unix(1, 0)
	m := New(func() time.Time { return now })
	frame(m, 100, 10, func() { m.Move("a", linear, image.Rect(0, 0, 10, 1), image.Point{}) })
	var moved bool
	frame(m, 80, 10, func() { _, _, moved = m.Move("a", linear, image.Rect(50, 0, 60, 1), image.Point{}) })
	if moved || m.Running() {
		t.Fatalf("resized = moved %v, running %v", moved, m.Running())
	}
}
