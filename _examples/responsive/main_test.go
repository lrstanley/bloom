// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel/vt"
	"getsteep.dev/steep/snapshot"
)

func TestResponsive(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("responsive", 120, 26)...)
	demo.ResizeInBand(t, h)
	h.Wait("showing nav, reader, details").Sleep(demo.Hold(600 * time.Millisecond))

	// Paging moves by the height of the reader, which Update gets from Solve.
	h.Key("pgdown").Wait("row 23 of").Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("pgup").Wait("row 1 of").Sleep(demo.Hold(300 * time.Millisecond))

	// The wheel scrolls the reader it is over.
	h.Mouse(vt.MousePress, vt.MouseWheelDown, 60, 10).Wait("row 4 of").Sleep(demo.Hold(300 * time.Millisecond))

	// Clicking an article opens it at the top; the tag grid reflows to fit.
	h.LeftClick(5, 4).Wait("Parents place").Wait("row 1 of").Sleep(600 * time.Millisecond)
	h.AssertSnapshot(snapshot.WithSuffix("wide"))

	// The toolbar drops its labels, then the details, then the list, go.
	h.Resize(96, 26).Wait("showing nav, reader, details").Sleep(demo.Hold(500 * time.Millisecond))
	h.Resize(68, 26).Wait("showing nav, reader ").Sleep(500 * time.Millisecond)
	h.AssertSnapshot(snapshot.WithSuffix("medium"))
	h.Resize(44, 26).Wait("showing reader ").Sleep(700 * time.Millisecond)
	h.AssertSnapshot(snapshot.WithSuffix("narrow"))

	// Below the size the reader needs, a note takes the layout's place.
	h.Resize(34, 26).Wait("window too small, min 40x10").Sleep(demo.Hold(700 * time.Millisecond))
	h.AssertSnapshot(snapshot.WithSuffix("too-small"))
	// Narrower than the note, it says only that the window is too small.
	h.Resize(20, 8).Wait("  window too small  ").Sleep(demo.Hold(700 * time.Millisecond))
	h.AssertSnapshot(snapshot.WithSuffix("too-small-narrow"))
	h.Resize(44, 26).Wait("showing reader ").Wait("row 1 of").Sleep(demo.Hold(500 * time.Millisecond))

	h.Resize(120, 26).Wait("showing nav, reader, details").Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()
}
