// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"math"
	"strconv"
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

// drag holds button down at from, moves it to to over d, a frame at a
// time, and lets go there.
func drag(h *steep.Harness, button vt.MouseButton, from, to [2]int, d time.Duration) {
	const frames = 30
	h.Mouse(vt.MousePress, button, from[0], from[1])
	for i := 1; i <= frames; i++ {
		x := from[0] + (to[0]-from[0])*i/frames
		y := from[1] + (to[1]-from[1])*i/frames
		h.Mouse(vt.MouseMotion, button, x, y).Sleep(d / frames)
	}
	h.Mouse(vt.MouseRelease, button, to[0], to[1])
}

// wave sweeps the left button held down along a wave from x0 to x1 at row
// y, over d.
func wave(h *steep.Harness, x0, x1, y int, d time.Duration) {
	const frames = 40
	h.Mouse(vt.MousePress, vt.MouseLeft, x0, y)
	for i := 1; i <= frames; i++ {
		x := x0 + (x1-x0)*i/frames
		h.Mouse(vt.MouseMotion, vt.MouseLeft, x, y+int(math.Round(2*math.Sin(float64(i)/4)))).Sleep(d / frames)
	}
	h.Mouse(vt.MouseRelease, vt.MouseLeft, x1, y)
}

// hover moves the pointer, with no button down, from one cell to another.
func hover(h *steep.Harness, from, to [2]int, d time.Duration) {
	const frames = 20
	for i := 0; i <= frames; i++ {
		x := from[0] + (to[0]-from[0])*i/frames
		y := from[1] + (to[1]-from[1])*i/frames
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, x, y).Sleep(demo.Hold(d / frames))
	}
}

// pick clicks a material in the sidebar.
func pick(h *steep.Harness, name string) {
	h.Locate(reel.Contains(name)).LeftClick().Sleep(demo.Hold(300 * time.Millisecond))
}

// wheel turns the wheel over the world at x, y, a notch at a time, from a
// radius of from to one of to. Each notch waits for the last to show, and
// a frame more, as the program drops notches closer together than that.
func wheel(h *steep.Harness, x, y, from, to int) {
	button, step := vt.MouseWheelUp, 1
	if to < from {
		button, step = vt.MouseWheelDown, -1
	}
	for r := from + step; r != to+step; r += step {
		h.Wheel(button, x, y).Wait(" " + strconv.Itoa(r) + " px").
			Sleep(max(demo.Hold(120*time.Millisecond), time.Second/fps))
	}
}

func TestSandbox(t *testing.T) {
	t.Parallel()

	// The example draws at its own rate, so the meters read as they do in a
	// terminal, and is recorded at 50 frames a second, the most browsers
	// play: they slow frames of 10ms, which 100 would need, to 100ms.
	opts := append(demo.Options("sandbox", 120, 34, effects.Mouse()),
		steep.WithProgramOptions(options()...),
		steep.WithRecording(reel.Profile{FPS: 50}), steep.WithTimeout(45*time.Second))
	h := steep.NewHarness(t, newModel(), opts...)
	h.Wait("bloom · sandbox").Wait("frame times")

	// The brush follows the pointer over the world.
	hover(h, [2]int{50, 16}, [2]int{12, 3}, 600*time.Millisecond)

	// Rainbow sand poured along the top, which settles in bands over the
	// ledges.
	wave(h, 10, 70, 3, 2500*time.Millisecond)
	h.Sleep(1500 * time.Millisecond)

	// A dam of stone down to the floor, drawn with a bigger brush.
	pick(h, "3 Stone")
	hover(h, [2]int{90, 6}, [2]int{56, 8}, 400*time.Millisecond)
	wheel(h, 56, 8, 3, 4)
	drag(h, vt.MouseLeft, [2]int{56, 8}, [2]int{58, 32}, 800*time.Millisecond)

	// Water behind it.
	pick(h, "2 Water")
	hover(h, [2]int{90, 6}, [2]int{68, 3}, 300*time.Millisecond)
	wheel(h, 68, 3, 4, 6)
	wave(h, 68, 86, 3, 1800*time.Millisecond)
	h.Sleep(1200 * time.Millisecond)

	// Lava over the water, which hardens to stone in a cloud of steam.
	pick(h, "4 Lava")
	drag(h, vt.MouseLeft, [2]int{74, 3}, [2]int{82, 3}, 1000*time.Millisecond)
	h.Sleep(1500 * time.Millisecond)

	// A hole erased in the dam with the right button, with lava still
	// picked, which the water floods out of.
	hover(h, [2]int{90, 10}, [2]int{50, 27}, 400*time.Millisecond)
	wheel(h, 50, 27, 6, 3)
	drag(h, vt.MouseRight, [2]int{50, 27}, [2]int{66, 27}, 700*time.Millisecond)
	h.Sleep(2500 * time.Millisecond)

	// A rain of everything, for the meters.
	h.Key("r").Wait("rain (on)").Sleep(demo.Hold(3 * time.Second))
	h.Key("r").Wait("rain (off)")

	h.Key("space").Wait("(paused)").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()
}
