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

func TestDropdowns(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("dropdowns", 64, 24, effects.Mouse())...)
	h.Wait("press s").Sleep(demo.Hold(600 * time.Millisecond))

	// The profile's field is on the last row, so its list opens above it,
	// with the options on rows 19 to 22.
	h.LeftClick(50, 23).Wait("presentation").Sleep(demo.Hold(500 * time.Millisecond))
	for y := 19; y <= 21; y++ {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, 50, y).Sleep(demo.Hold(250 * time.Millisecond))
	}
	h.LeftClick(50, 21).Wait("Profile: presentation").Sleep(demo.Hold(600 * time.Millisecond))

	// The dialog's fields are on rows 9, 11, and 13. The language's list
	// hangs past the dialog's bottom edge, with Français on row 17.
	h.Key("s").Wait("Settings").Sleep(300 * time.Millisecond)
	h.LeftClick(35, 13).Wait("Svenska").Sleep(demo.Hold(500 * time.Millisecond))
	for y := 14; y <= 17; y++ {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, 35, y).Sleep(demo.Hold(250 * time.Millisecond))
	}
	h.AssertSnapshot()
	h.LeftClick(35, 17).Wait("Language: Français").Sleep(demo.Hold(600 * time.Millisecond))

	// The theme, with the keyboard.
	h.Key("shift+tab").Key("shift+tab").Key("enter").Wait("Frappé").Sleep(demo.Hold(400 * time.Millisecond))
	h.Key("down").Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("down").Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("enter").Wait("Theme: Frappé").Sleep(demo.Hold(600 * time.Millisecond))

	// A click in the dialog, outside the font's list, closes the list and
	// leaves the dialog open.
	h.LeftClick(35, 11).Wait("16").Sleep(demo.Hold(600 * time.Millisecond))
	h.LeftClick(16, 15).WaitNot("16").Wait("Settings").Sleep(demo.Hold(600 * time.Millisecond))

	h.Key("tab").Key("tab").Key("tab").Sleep(demo.Hold(300 * time.Millisecond))
	h.Key("enter").Wait("saved: Frappé, 14pt, Français").WaitNot("Settings").Sleep(demo.Hold(time.Second))

	h.Key("q")
	h.WaitFinished()
}
