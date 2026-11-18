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

func TestTooltips(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("tooltips", 80, 16, effects.Mouse())...)
	move := func(x, y int) *steep.Harness { return h.Mouse(vt.MouseMotion, vt.MouseUnknown, x, y) }
	h.Wait("Deployments").Sleep(demo.Hold(800 * time.Millisecond))

	// The toolbar is on row 1, with its buttons' icons at 17, 21, 25, 29,
	// and 33. The pointer rests on the first, whose tooltip shows after the
	// delay.
	for _, x := range []int{40, 28, 17} {
		move(x, 5).Sleep(demo.Hold(120 * time.Millisecond))
	}
	move(17, 1).Wait("Refresh").Sleep(demo.Hold(900 * time.Millisecond))

	// Moving along the toolbar, each next tooltip shows at once. Settings
	// asks for its right side.
	for _, b := range []struct {
		x   int
		tip string
	}{{21, "Deploy"}, {25, "Roll back"}, {29, "Stop"}, {33, "Settings"}} {
		move(b.x-2, 1).Sleep(demo.Hold(60 * time.Millisecond))
		move(b.x, 1).Wait(b.tip).Sleep(demo.Hold(550 * time.Millisecond))
	}

	// The payments row is row 6. Its image, from 15 to 50, is cut, and
	// unfolds in place, over the status from 52.
	move(33, 3).Sleep(demo.Hold(150 * time.Millisecond))
	move(32, 6).Wait("hotfix.2").Sleep(demo.Hold(1200 * time.Millisecond))

	// Moving on along the row, the unfold stays while the pointer is over
	// the image, and closes once it moves onto the status, whose own tooltip
	// shows at once.
	for _, x := range []int{38, 44, 49} {
		move(x, 6).Sleep(demo.Hold(120 * time.Millisecond))
	}
	move(53, 6).WaitNot("hotfix.2").Wait("2 of 3 replicas").Sleep(demo.Hold(1400 * time.Millisecond))

	// Its secret is cut too, but has NoTooltip, so it shows nothing.
	move(66, 6).WaitNot("2 of 3 replicas").Sleep(demo.Hold(900 * time.Millisecond))

	// The failed deploy on the last row explains itself below, in the empty
	// space, rather than over the row above.
	move(60, 9).Wait("CrashLoopBackOff").Sleep(demo.Hold(1400 * time.Millisecond))

	// t sends tooltips to the status line.
	h.Key("t").Wait("sink (t)").Sleep(demo.Hold(400 * time.Millisecond))
	move(25, 1).Wait("Roll back").Sleep(demo.Hold(900 * time.Millisecond))
	move(58, 8).Wait("4 of 6 pods").Sleep(demo.Hold(1100 * time.Millisecond))
	h.Key("t").Wait("float (t)")
	move(60, 13).Sleep(demo.Hold(500 * time.Millisecond))

	// The selected row's image is pinned open, and follows the selection.
	h.Key("j").Wait("build.8812").Sleep(demo.Hold(700 * time.Millisecond))
	h.Key("j").WaitNot("build.8812").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("j").Wait("hotfix.2").Sleep(demo.Hold(900 * time.Millisecond))

	// Tab moves the focus along the toolbar, which shows each tooltip.
	h.Key("tab").Wait("Refresh").Sleep(demo.Hold(700 * time.Millisecond))
	h.Key("tab").WaitNot("Refresh").Sleep(demo.Hold(700 * time.Millisecond))
	h.Key("tab").Wait("Roll back")
	h.AssertSnapshot()
	h.Sleep(demo.Hold(1200 * time.Millisecond))

	// esc clears the selection and the focus, which closes both.
	h.Key("esc").WaitNot("hotfix.2").WaitNot("Roll back").Sleep(demo.Hold(1000 * time.Millisecond))

	h.Key("q")
	h.WaitFinished()
}
