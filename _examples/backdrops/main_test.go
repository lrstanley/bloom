// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"math"
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

// sweep moves the pointer along an arc around the middle of a w×h screen,
// leaving drops behind it.
func sweep(h *steep.Harness, w, ht int, from, to float64) {
	for a := from; a <= to; a += 0.12 {
		x := w/2 + int(float64(w)*0.38*math.Cos(a))
		y := ht/2 + int(float64(ht)*0.36*math.Sin(a))
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, x, y).Sleep(demo.Hold(30 * time.Millisecond))
	}
}

func TestBackdrops(t *testing.T) {
	t.Parallel()

	const w, ht = 120, 32
	h := steep.NewHarness(t, newModel(), demo.Options("backdrops", w, ht, effects.Mouse())...)
	h.Wait("bloom · backdrops").Wait(" Ripple ·").Sleep(demo.Hold(1500 * time.Millisecond))

	// Clicks drop rings, and the pointer leaves smaller ones as it moves.
	h.LeftClick(20, 8).Sleep(demo.Hold(500 * time.Millisecond))
	h.LeftClick(96, 22).Sleep(demo.Hold(900 * time.Millisecond))
	sweep(h, w, ht, 0, math.Pi)
	h.Sleep(demo.Hold(1200 * time.Millisecond))

	for _, name := range []string{"Drift", "Rain"} {
		h.Key("tab").Wait(" " + name + " ·").Sleep(demo.Hold(1200 * time.Millisecond))
		sweep(h, w, ht, math.Pi, 2*math.Pi)
		h.LeftClick(w/4, ht/3).Sleep(demo.Hold(1600 * time.Millisecond))
	}

	// Each of the dialog's backdrops in turn over the screen, then none.
	for _, d := range dialogs {
		h.Key("b").Wait("Backdrop: " + d.name).Sleep(demo.Hold(1400 * time.Millisecond))
	}
	h.Key("esc").WaitNot("Backdrop:").Sleep(demo.Hold(600 * time.Millisecond))

	h.Key("space").Wait("(paused)").Sleep(demo.Hold(400 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()
}
