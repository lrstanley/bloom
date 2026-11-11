// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
	"github.com/lrstanley/bloom/_examples/internal/demo"
)

func TestMouse(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("mouse", 64, 14, effects.Mouse())...)
	h.Wait("hover, click").Sleep(600 * time.Millisecond)

	// The hover style follows the pointer down the list.
	for y := 1; y <= 5; y++ {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, 5, y).Sleep(250 * time.Millisecond)
	}

	// The list's border is row 0, so view.go is row 3 and update.go row 4.
	h.LeftClick(5, 3).Wait("selected view.go").Sleep(500 * time.Millisecond)
	h.LeftClick(5, 4, reel.Repeat(2)).Wait("opened update.go").Wait("line 1 of update.go")
	h.Sleep(500 * time.Millisecond)

	// The wheel scrolls the file it is over.
	h.Mouse(vt.MousePress, vt.MouseWheelDown, 40, 6).WaitNot(" 1  // line 1").Sleep(500 * time.Millisecond)

	// The one-cell divider keeps the drag as the pointer leaves it.
	h.MouseDrag(vt.MouseLeft, 22, 6, 36, 6, reel.Delay(40*time.Millisecond)).Wait("the sidebar is 36 cells")
	h.Sleep(400 * time.Millisecond)
	h.MouseDrag(vt.MouseLeft, 36, 6, 16, 6, reel.Delay(40*time.Millisecond)).Wait("the sidebar is 16 cells")
	h.Sleep(800 * time.Millisecond)
	h.AssertSnapshot()

	h.Key("q")
	h.WaitFinished()
}
