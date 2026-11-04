// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anim

import (
	"testing"
	"time"
)

// setMotion sets the motion until the test ends. Motion is global, so tests
// using this do not call t.Parallel, and run while the ones that do wait.
func setMotion(t *testing.T, on bool) {
	t.Helper()
	prev := Motion()
	SetMotion(on)
	t.Cleanup(func() { SetMotion(prev) })
}

func TestMotionOffValue(t *testing.T) {
	setMotion(t, false)

	for name, opt := range map[string]Option{"ease": Duration(time.Hour), "spring": Spring(9, 0.3)} {
		v := NewValue(0, opt)
		if cmd := v.To(10); cmd != nil {
			t.Fatalf("%s: To asked for a frame", name)
		}
		if v.Float() != 10 || v.Animating() {
			t.Fatalf("%s: at %v, animating %v", name, v.Float(), v.Animating())
		}
		if cmd := v.Play(0, 5); cmd != nil || v.Float() != 5 {
			t.Fatalf("%s: Play left it at %v, with command %v", name, v.Float(), cmd != nil)
		}

		v.SetTarget(7, time.Now())
		if v.Advance(time.Now()) || v.Float() != 7 {
			t.Fatalf("%s: Advance left it at %v, animating %v", name, v.Float(), v.Animating())
		}
	}
}

func TestMotionOffPresence(t *testing.T) {
	setMotion(t, false)

	p := NewPresence(Spring(9, 0.3))
	if cmd := p.Show(); cmd != nil || !p.Shown() || p.Progress() != 1 {
		t.Fatalf("shown at %v with command %v", p.Progress(), cmd != nil)
	}
	if cmd := p.Hide(); cmd != nil || p.Visible() {
		t.Fatalf("hidden, still visible %v with command %v", p.Visible(), cmd != nil)
	}
}

func TestMotionOffFinishesRunning(t *testing.T) {
	setMotion(t, true)

	c := &clock{now: time.Unix(0, 0)}
	v := NewValue(0, Duration(time.Hour), Clock(c.Now))
	v.To(10)
	SetMotion(false)
	if cmd := v.Update(v.frameMsg()); cmd != nil {
		t.Fatal("asked for a frame after motion was turned off")
	}
	if v.Float() != 10 || v.Animating() {
		t.Fatalf("at %v, animating %v", v.Float(), v.Animating())
	}
}

func TestMotionOffSequence(t *testing.T) {
	setMotion(t, false)

	c := &clock{now: time.Unix(0, 0)}

	// A sequence with nothing to wait for is over at once.
	s := NewSequence(0, To(1, Spring(9, 0.3)), To(4, Duration(time.Hour))).With(Clock(c.Now))
	if cmd := s.Play(); cmd != nil || s.Animating() || s.Float() != 4 {
		t.Fatalf("played to %v, animating %v, command %v", s.Float(), s.Animating(), cmd != nil)
	}

	// Holds still keep time, so a step after one happens when it was due.
	s = NewSequence(0, To(1), Hold(time.Second), To(0)).With(Clock(c.Now))
	if cmd := s.Play(); cmd == nil || s.Step() != 1 || s.Float() != 1 {
		t.Fatalf("hold: step %d at %v, command %v", s.Step(), s.Float(), cmd != nil)
	}
	c.advance(time.Second)
	if cmd := s.Update(s.frameMsg()); cmd != nil || s.Animating() || s.Float() != 0 {
		t.Fatalf("after hold: at %v, animating %v, command %v", s.Float(), s.Animating(), cmd != nil)
	}

	// Loops, holding or not, settle where their steps end.
	for name, tt := range map[string]struct {
		loop Sequence
		want float64
	}{
		"phase": {NewLoop(time.Second, Clock(c.Now)), 1},
		"blink": {NewSequence(0, To(1), Hold(time.Second), To(0), Hold(time.Second)).With(Clock(c.Now)), 0},
	} {
		if cmd := tt.loop.Loop(); cmd != nil || tt.loop.Animating() || tt.loop.Float() != tt.want {
			t.Fatalf("%s: looped to %v, want %v, animating %v, command %v", name, tt.loop.Float(), tt.want, tt.loop.Animating(), cmd != nil)
		}
	}
}

func TestMotionOffSequenceDoesNotDrift(t *testing.T) {
	setMotion(t, false)

	c := &clock{now: time.Unix(0, 0)}
	s := NewSequence(0, To(1), Hold(time.Second), To(2), Hold(time.Second), To(3)).With(Clock(c.Now))
	s.Play()

	// A late frame catches up with every step that was due by then.
	c.advance(3 * time.Second)
	if cmd := s.Update(s.frameMsg()); cmd != nil || s.Animating() || s.Float() != 3 {
		t.Fatalf("at %v, animating %v, command %v", s.Float(), s.Animating(), cmd != nil)
	}
}

func TestMotionOffKeepsHoldAfterInterruptedStep(t *testing.T) {
	setMotion(t, true)

	c := &clock{now: time.Unix(0, 0)}
	s := NewSequence(0, To(1, Duration(10*time.Second)), Hold(time.Second), To(0)).With(Clock(c.Now))
	s.Play()
	c.advance(2 * time.Second)

	SetMotion(false)
	if cmd := s.Update(s.frameMsg()); cmd == nil || s.Step() != 1 || s.Float() != 1 {
		t.Fatalf("step %d at %v, command %v", s.Step(), s.Float(), cmd != nil)
	}
	c.advance(time.Second)
	if cmd := s.Update(s.frameMsg()); cmd != nil || s.Animating() || s.Float() != 0 {
		t.Fatalf("after hold: at %v, animating %v, command %v", s.Float(), s.Animating(), cmd != nil)
	}
}

func TestMotionOffEndsRunningLoop(t *testing.T) {
	setMotion(t, true)

	c := &clock{now: time.Unix(0, 0)}
	s := NewSequence(0, To(1, Duration(time.Second), Ease(Linear)), Hold(time.Second), To(0, Duration(time.Second), Ease(Linear))).With(Clock(c.Now))
	if s.Loop() == nil {
		t.Fatal("loop did not start")
	}
	tick(c, &s, 500*time.Millisecond)

	SetMotion(false)
	if cmd := s.Update(s.frameMsg()); cmd != nil || s.Animating() || s.Float() != 0 {
		t.Fatalf("at %v, animating %v, command %v", s.Float(), s.Animating(), cmd != nil)
	}

	// Once motion is back, the program can start it again.
	SetMotion(true)
	if s.Loop() == nil || !s.Animating() {
		t.Fatal("loop did not restart")
	}
}
