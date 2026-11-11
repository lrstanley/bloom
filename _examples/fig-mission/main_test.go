// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"getsteep.dev/steep"
	"github.com/lrstanley/bloom/_examples/internal/demo"
)

func TestMission(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("fig-mission", 100, 24)...)
	demo.ResizeInBand(t, h)
	h.Wait("header: ansi-compact").Sleep(1500 * time.Millisecond)

	// The toast slides in over the panels, which show through it, and out.
	h.Key("d").Wait("v1.4.42 rolled out").Sleep(2800 * time.Millisecond)

	// Opaque, it hides what is beneath it.
	h.Key("t").Wait("toast: opaque").Key("d").Wait("v1.4.43 rolled out").Sleep(2800 * time.Millisecond)
	h.Key("t").Wait("toast: transparent")

	// The header steps down a font at a time as the window narrows, and the
	// panels collapse.
	for _, step := range []struct {
		w    int
		font string
	}{{76, "doom"}, {62, "small"}, {44, "mini"}, {30, "plain text"}} {
		h.Resize(step.w, 24).Wait("header: " + step.font).Sleep(900 * time.Millisecond)
	}
	h.Resize(100, 24).Wait("header: ansi-compact").Sleep(800 * time.Millisecond)

	h.Key("q")
	h.WaitFinished()
	if m := h.FinalModelAs[*model](); m.version != 43 || m.opaque {
		t.Fatalf("version %d, opaque %v, want 43 and transparent", m.version, m.opaque)
	}
}
