// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anim

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// maxTickerStep is the most a [Ticker] moves on for one frame, or its frame
// intervals if longer, so that time the program spent suspended, or a frame
// that arrived very late, does not make whatever follows it jump.
const maxTickerStep = 250 * time.Millisecond

// Ticker is a clock that runs for as long as it is started, for things that
// animate without an end, such as a backdrop of fx.Ripple or fx.Drift, whose
// Time field takes [Ticker.Elapsed]. Unlike a looping [Sequence], it never
// wraps, so what follows it need not repeat.
//
// Use it like a [Value]: return the command from [Ticker.Start], and pass
// every message to Update. It asks for frames at [FPS], and at [UnfocusedFPS]
// while the terminal is not focused. While motion is off, see [SetMotion],
// it stands still. The zero Ticker does not tick; make one with [NewTicker].
type Ticker struct {
	frames

	elapsed time.Duration
	last    time.Time
	running bool
}

// NewTicker returns a stopped ticker at zero. Of opts, only [FPS],
// [UnfocusedFPS], and [Clock] apply.
func NewTicker(opts ...Option) Ticker {
	return Ticker{frames: newFrames(opts)}
}

// Elapsed returns how long the ticker has run.
func (k *Ticker) Elapsed() time.Duration { return k.elapsed }

// Seconds returns how long the ticker has run, in seconds.
func (k *Ticker) Seconds() float64 { return k.elapsed.Seconds() }

// Animating reports whether the ticker is running.
func (k *Ticker) Animating() bool { return k.running }

// Start runs the ticker from where it stopped, returning the command for its
// first frame, or nil when it is already running or motion is off.
func (k *Ticker) Start() tea.Cmd {
	if k.now == nil || k.running || !Motion() {
		return nil
	}
	k.running, k.last = true, k.now()
	k.gen++
	return k.frame()
}

// Stop stops the ticker where it is. Its frames are ignored until Start asks
// for new ones.
func (k *Ticker) Stop() { k.running = false }

// Toggle stops a running ticker, and starts a stopped one.
func (k *Ticker) Toggle() tea.Cmd {
	if k.running {
		k.Stop()
		return nil
	}
	return k.Start()
}

// Reset puts the ticker back at zero, leaving it running or stopped.
func (k *Ticker) Reset() {
	k.elapsed = 0
	if k.now != nil {
		k.last = k.now()
	}
}

// Update advances the ticker for its own [FrameMsg], returning the command
// for the next one while it is running. It notes [tea.FocusMsg] and
// [tea.BlurMsg] for [UnfocusedFPS].
func (k *Ticker) Update(msg tea.Msg) tea.Cmd {
	k.noteFocus(msg)
	if !k.own(msg) || !k.running {
		return nil
	}
	if !Motion() {
		k.Stop()
		return nil
	}
	now := k.now()
	if d := now.Sub(k.last); d > 0 {
		// A frame asked for while blurred may arrive once focused: allow
		// the longer of the two intervals.
		k.elapsed += min(d, max(maxTickerStep, k.interval, k.unfocused))
	}
	k.last = now
	return k.frame()
}
