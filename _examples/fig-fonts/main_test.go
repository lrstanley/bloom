// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"getsteep.dev/steep"
	"github.com/lrstanley/bloom/_examples/internal/demo"
	"github.com/lrstanley/bloom/fig/fonts"
)

func TestFonts(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("fig-fonts", 110, 30)...)
	h.Wait("› standard").Wait("column paint").Sleep(900 * time.Millisecond)

	// The paints, on the standard font.
	for _, p := range paints[1:] {
		h.Key("right").Wait(p.name + " paint").Sleep(450 * time.Millisecond)
	}
	// Back to coloring each glyph on its own.
	for range 4 {
		h.Key("left").Sleep(120 * time.Millisecond)
	}
	h.Wait("glyph paint").Sleep(300 * time.Millisecond)

	// Every font, a glyph at a time, around to the standard font again, each
	// revealed as it is picked.
	for i := range fonts.Names {
		name := fonts.Names[(standard+1+i)%len(fonts.Names)]
		h.Key("down").Wait("› " + name).Sleep(380 * time.Millisecond)
	}

	// Back on the standard font, filled in.
	h.Wait("› standard").Sleep(500 * time.Millisecond)
	h.Key("right").Key("right").Key("right").Key("right").Wait("filled paint").Sleep(700 * time.Millisecond)
	h.AssertSnapshot()

	h.Key("up").Wait("› smslant").Sleep(600 * time.Millisecond)
	h.Key("q")
	h.WaitFinished()
	if m := h.FinalModelAs[*model](); m.font != standard-1 || m.paint != len(paints)-1 {
		t.Fatalf("font %d, paint %d, want %d and %d", m.font, m.paint, standard-1, len(paints)-1)
	}
}
