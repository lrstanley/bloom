// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anim

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (k *Ticker) frameMsg() FrameMsg { return FrameMsg{id: k.id, gen: k.gen} }

func TestTicker(t *testing.T) {
	t.Parallel()

	c := &clock{now: time.Unix(0, 0)}
	k := NewTicker(Clock(c.Now))
	if k.Elapsed() != 0 || k.Animating() {
		t.Fatalf("new ticker at %v, animating %v", k.Elapsed(), k.Animating())
	}
	if k.Start() == nil || !k.Animating() {
		t.Fatal("Start did not ask for a frame")
	}
	if k.Start() != nil {
		t.Fatal("Start of a running ticker asked for another frame")
	}

	c.advance(16 * time.Millisecond)
	if k.Update(k.frameMsg()) == nil {
		t.Fatal("a running ticker stopped asking for frames")
	}
	c.advance(16 * time.Millisecond)
	k.Update(k.frameMsg())
	if k.Elapsed() != 32*time.Millisecond {
		t.Fatalf("elapsed %v, want 32ms", k.Elapsed())
	}

	// Frames not its own, or from before a stop, are ignored.
	stale := k.frameMsg()
	k.Stop()
	c.advance(time.Second)
	if k.Update(stale) != nil || k.Elapsed() != 32*time.Millisecond {
		t.Fatalf("a stopped ticker moved to %v", k.Elapsed())
	}

	// It picks up where it stopped, not counting the time it was stopped.
	k.Start()
	c.advance(10 * time.Millisecond)
	k.Update(k.frameMsg())
	if k.Elapsed() != 42*time.Millisecond {
		t.Fatalf("elapsed %v after restarting, want 42ms", k.Elapsed())
	}

	// A frame that arrives very late moves it on by a bounded step.
	c.advance(time.Hour)
	k.Update(k.frameMsg())
	if got := k.Elapsed() - 42*time.Millisecond; got != maxTickerStep {
		t.Fatalf("a late frame moved it %v, want %v", got, maxTickerStep)
	}

	k.Reset()
	if k.Elapsed() != 0 || !k.Animating() {
		t.Fatalf("reset left it at %v, animating %v", k.Elapsed(), k.Animating())
	}
	k.Toggle()
	if k.Animating() {
		t.Fatal("Toggle did not stop it")
	}
	if k.Toggle() == nil || !k.Animating() {
		t.Fatal("Toggle did not start it")
	}
}

func TestTickerUnfocused(t *testing.T) {
	t.Parallel()

	c := &clock{now: time.Unix(0, 0)}
	k := NewTicker(Clock(c.Now), FPS(60), UnfocusedFPS(4))
	k.Start()
	k.Update(tea.BlurMsg{})
	// While blurred, frames come far apart, and each moves it on in full.
	c.advance(250 * time.Millisecond)
	k.Update(k.frameMsg())
	if k.Elapsed() != 250*time.Millisecond {
		t.Fatalf("elapsed %v, want 250ms", k.Elapsed())
	}

	// Frames further apart than the bound on a step still move it on in full.
	k = NewTicker(Clock(c.Now), FPS(1))
	k.Start()
	c.advance(time.Second)
	k.Update(k.frameMsg())
	if k.Elapsed() != time.Second {
		t.Fatalf("elapsed %v at 1 FPS, want 1s", k.Elapsed())
	}

	// A frame asked for while blurred, arriving once focused again.
	k = NewTicker(Clock(c.Now), UnfocusedFPS(1))
	k.Update(tea.BlurMsg{})
	k.Start()
	if k.delay() != time.Second {
		t.Fatalf("blurred frames come %v apart, want 1s", k.delay())
	}
	c.advance(time.Second)
	k.Update(tea.FocusMsg{})
	k.Update(k.frameMsg())
	if k.Elapsed() != time.Second {
		t.Fatalf("elapsed %v after refocusing, want 1s", k.Elapsed())
	}
}

func TestTickerMotionOff(t *testing.T) {
	setMotion(t, false)

	k := NewTicker()
	if k.Start() != nil || k.Animating() {
		t.Fatal("a ticker started with motion off")
	}
	if k.Elapsed() != 0 {
		t.Fatalf("elapsed %v", k.Elapsed())
	}
}
