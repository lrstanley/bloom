// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

func init() { clip.Native = false }

func TestLogs(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("logs", 110, 24, effects.Mouse())...)

	// It starts at the end, following the lines as they are added.
	h.Wait("server.log").Wait("· following").Wait("250001 lines").Wait("250003 lines").Sleep(demo.Hold(600 * time.Millisecond))

	// Scrolled up, it stays where it is as lines are added.
	h.Key("pgup").Key("pgup").WaitNot("· following").Sleep(demo.Hold(800 * time.Millisecond))

	// Cut off rather than wrapped, it scrolls sideways; the top line stays
	// at the top.
	h.Key("w").Wait("cut off").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("right").Key("right").Key("right").Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("w").Wait("wrapping at words").Sleep(demo.Hold(500 * time.Millisecond))

	// A search marks its matches, and moves between them through the whole
	// log, wrapping around.
	h.Key("/").Type("timeout", reel.Delay(60*time.Millisecond)).Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("enter").Wait(`"timeout" on line`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("n").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("n").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("N").Sleep(demo.Hold(500 * time.Millisecond))

	// To the start of a quarter of a million lines, and back to the end,
	// following again.
	h.Key("g").Wait("line 1 of").Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("#").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("#").Sleep(demo.Hold(300 * time.Millisecond))

	// The wheel scrolls it.
	at := h.Locate(reel.Contains("14:00:00.137"))
	for range 3 {
		h.Mouse(vt.MousePress, vt.MouseWheelDown, at.X, at.Y).Sleep(demo.Hold(150 * time.Millisecond))
	}
	h.WaitNot("14:00:00.137").Sleep(demo.Hold(500 * time.Millisecond))

	// A selection across the rows of a wrapped line copies the one line.
	at = h.Locate(reel.Contains("payments: dial tcp"))
	h.LeftDrag(at.X, at.Y, at.X+30, at.Y+1, reel.Delay(30*time.Millisecond)).Wait(`selected "payments: dial tcp`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "payments: dial tcp`).Sleep(demo.Hold(600 * time.Millisecond))

	h.Key("esc").Key("G").Wait("· following").Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()

	if m := h.FinalModelAs[*model](); !m.pos.AtEnd() || m.log.Len() <= preloaded {
		t.Fatalf("finished at %+v with %d lines", m.pos, m.log.Len())
	}
}
