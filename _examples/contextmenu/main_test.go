// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"github.com/lrstanley/bloom/_examples/internal/demo"
	"github.com/lrstanley/steep"
	"github.com/lrstanley/steep/reel/effects"
	"github.com/lrstanley/steep/reel/vt"
)

func TestContextMenu(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("contextmenu", 64, 16, effects.Mouse())...)
	h.Wait("right click a file").Sleep(600 * time.Millisecond)

	// The list's border is row 0, so view.go is row 3. Its menu opens at
	// the pointer, with Open on row 3 and Duplicate on row 4.
	h.RightClick(8, 3).Wait("Duplicate").Sleep(500 * time.Millisecond)
	for y := 3; y <= 4; y++ {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, 12, y).Sleep(250 * time.Millisecond)
	}
	h.LeftClick(12, 4).Wait("duplicated view.go").Wait("view copy.go").WaitNot("Copy path")
	h.Sleep(500 * time.Millisecond)

	// Near the right edge, the empty space's menu opens to the left of the
	// pointer, with New file on row 13 and Sort on row 14.
	h.RightClick(58, 13).Wait("New file").Sleep(500 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseUnknown, 47, 13).Sleep(250 * time.Millisecond)
	h.LeftClick(47, 13).Wait("created untitled-1.txt").Sleep(500 * time.Millisecond)
	h.RightClick(58, 13).Wait("Sort by name").Sleep(300 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseUnknown, 47, 14).Sleep(250 * time.Millisecond)
	h.LeftClick(47, 14).Wait("sorted by name").Sleep(500 * time.Millisecond)

	// go.mod sorts to row 1. A right click on another file while a menu is
	// open moves the menu there.
	h.RightClick(10, 4).Wait("selected styles.go").Wait("Copy path").Sleep(400 * time.Millisecond)
	h.RightClick(50, 1).Wait("selected go.mod").Sleep(300 * time.Millisecond)
	for y := 1; y <= 5; y++ {
		h.Mouse(vt.MouseMotion, vt.MouseUnknown, 38, y).Sleep(200 * time.Millisecond)
	}
	h.AssertSnapshot()
	h.LeftClick(38, 5).Wait("deleted go.mod").WaitNot("Copy path").Sleep(500 * time.Millisecond)

	// Escape closes a menu without running anything.
	h.RightClick(8, 2).Wait("Copy path").Sleep(400 * time.Millisecond)
	h.Key("esc").WaitNot("Copy path").Sleep(600 * time.Millisecond)

	// A click outside a menu closes it and still selects the file it is on,
	// although the menu is gone before the button comes up.
	h.RightClick(8, 2).Wait("Copy path").Sleep(400 * time.Millisecond)
	h.Mouse(vt.MousePress, vt.MouseLeft, 30, 6).WaitNot("Copy path").Sleep(300 * time.Millisecond)
	h.Mouse(vt.MouseRelease, vt.MouseLeft, 30, 6).Wait("selected view copy.go").Sleep(600 * time.Millisecond)

	h.Key("q")
	h.WaitFinished()
}
