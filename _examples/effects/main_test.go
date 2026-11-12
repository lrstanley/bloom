// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
)

func TestEffects(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("effects", 120, 32)...)

	// One full loop: in, held, and out again.
	h.Wait("Custom: sweep").Wait("t = 1.00").Sleep(demo.Hold(900 * time.Millisecond))
	h.Wait("t = 0.00").Sleep(demo.Hold(400 * time.Millisecond))

	// Each backdrop in turn over the gallery, then none.
	for _, name := range []string{"Dim", "Desaturate", "Recolor: tint", "Recolor: two colors"} {
		h.Key("b").Wait("Backdrop: " + name).Sleep(demo.Hold(900 * time.Millisecond))
	}
	// The braille ones for a whole loop, as their dots follow the glyphs
	// beneath.
	for _, name := range []string{"Braille", "Braille: tinted"} {
		h.Key("b").Wait("Backdrop: " + name).Sleep(demo.Hold(2800 * time.Millisecond))
	}
	h.Key("b").Wait("backdrop (none)").WaitNot("Backdrop:").Sleep(demo.Hold(400 * time.Millisecond))

	h.Key("space").Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()
}
