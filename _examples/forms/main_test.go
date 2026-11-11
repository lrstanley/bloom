// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/snapshot"
)

func TestForms(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("forms", 100, 26)...)
	h.Wait("Profile").Wait("focus: name").Sleep(demo.Hold(500 * time.Millisecond))

	h.Type("Ada Lovelace", reel.Delay(40*time.Millisecond)).Wait("Ada Lovelace").Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("tab").Type("ada@example.com", reel.Delay(40*time.Millisecond)).Wait("ada@example.com")
	if c := h.CursorPosition(); c.X == 0 {
		t.Fatalf("the cursor is not in the email input: %v", c)
	}

	// The dropdown is anchored below the language input; enter picks.
	h.Key("tab").Type("l").Wait("Haskell").Wait("Gleam").Sleep(demo.Hold(500 * time.Millisecond))
	h.Type("e").Wait("Gleam").WaitNot("Haskell").Sleep(demo.Hold(400 * time.Millisecond))
	h.Key("enter").WaitNot("start typing").Sleep(demo.Hold(300 * time.Millisecond))

	// Saving asks once; the first save goes through.
	h.Key("tab").Key("tab").Wait("focus: save").Key("enter").Wait("Save profile?").Wait("in the form: false")
	h.Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("tab").Wait("focus: cancel").Key("tab").Wait("focus: confirm").Key("enter")
	h.Wait("Profile saved").Wait("focus: save").Sleep(demo.Hold(800 * time.Millisecond))
	h.WaitNot("Profile saved") // the toast hides itself, which keeps the snapshot below steady

	// The second save asks again, on top.
	h.Key("enter").Wait("Save profile?").Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("enter").Wait("Overwrite the saved profile?").Wait("dialogs open: 2").Sleep(1 * time.Second)
	h.AssertSnapshot(snapshot.WithSuffix("stacked"))

	// esc closes the top dialog, a click outside the next.
	h.Key("esc").WaitNot("Overwrite the saved profile?").Wait("focus: confirm").Sleep(demo.Hold(1 * time.Second))
	h.LeftClick(1, 1).WaitNot("Save profile?").Wait("focus: save").Sleep(demo.Hold(600 * time.Millisecond))

	h.Key("ctrl+c")
	h.WaitFinished()
}
