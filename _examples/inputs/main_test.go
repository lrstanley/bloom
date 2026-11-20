// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"getsteep.dev/steep"
)

// TestInputs checks that the cursor is in the focused input, after typing,
// a click, and tab, though both inputs report a cursor all along.
func TestInputs(t *testing.T) {
	t.Parallel()

	const w = 60
	h := steep.NewHarness(t, newModel(), steep.WithWindowSize(w, 6), steep.WithTimeout(5*time.Second))
	cursorIn := func(want pane) {
		t.Helper()
		c := h.CursorPosition()
		if !h.CursorVisible() || c.Y != 1 || (c.X < w/2) != (want == paneLeft) {
			t.Fatalf("cursor at %v (visible %v), want in %s on row 1", c, h.CursorVisible(), want)
		}
	}

	h.Wait("focus: left")
	h.Type("hello").Wait("left> hello")
	cursorIn(paneLeft)

	// A click on the right box focuses it; keys go there now.
	h.LeftClick(w-5, 1).Wait("focus: right")
	h.Type("world").Wait("right> world").Wait("left> hello")
	cursorIn(paneRight)

	h.Key("tab").Wait("focus: left")
	cursorIn(paneLeft)
	h.Type("!").Wait("left> hello!").WaitNot("world!")

	h.AssertSnapshot()
	h.Key("esc")
	h.WaitFinished()
}
