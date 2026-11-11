// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

func TestLetters(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("fig-letters", 112, 20, effects.Mouse())...)

	// The title is revealed from the middle out, as the clock ticks.
	h.Wait(" 1/5 ").Sleep(2500 * time.Millisecond)

	// The letters fly to their places in the next word.
	for _, n := range []string{"2/5", "3/5"} {
		h.Key("space").Wait(" " + n + " ").Sleep(1400 * time.Millisecond)
	}

	// Letters light up under the pointer, and count clicks.
	for x := 70; x <= 96; x += 2 {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, x, 11).Sleep(70 * time.Millisecond)
	}
	h.LeftClick(86, 11).Wait(" 1 clicks ").Sleep(300 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseUnknown, 60, 18).Sleep(200 * time.Millisecond)

	// The clock has rolled over to the hour; pausing it stops it.
	h.Key("p").Wait("❚❚ paused")
	h.Key("space").Wait(" 4/5 ").Sleep(1400 * time.Millisecond)

	// The title again.
	h.Key("i").Sleep(2200 * time.Millisecond)

	h.Key("q")
	h.WaitFinished()
	m := h.FinalModelAs[*model]()
	if m.word != 3 || m.clicks != 1 || !m.paused {
		t.Fatalf("word %d, clicks %d, paused %v, want 3, 1, and paused", m.word, m.clicks, m.paused)
	}
	if m.now.Hour() != 10 {
		t.Fatalf("the clock is at %s, not past the hour", m.now.Format("15:04:05"))
	}
}
