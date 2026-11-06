// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"github.com/lrstanley/bloom/_examples/internal/demo"
	"github.com/lrstanley/steep"
	"github.com/lrstanley/steep/reel"
)

func TestBorders(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("borders", 96, 26)...)

	// The effects loop while the list scrolls under its scrollbar.
	h.Wait("◆ secrets").Wait("1-16 of 36").Sleep(1200 * time.Millisecond)
	for range 4 {
		h.Key("down").Sleep(120 * time.Millisecond)
	}
	h.Wait("5-20 of 36").Sleep(400 * time.Millisecond)

	// The list's scrollbar is on its right border, x=60, from y=1. Its thumb
	// can be grabbed and dragged, and a click on the rest of it jumps there.
	h.LeftDrag(60, 4, 60, 10, reel.Delay(60*time.Millisecond)).Wait("18-33 of 36").Sleep(700 * time.Millisecond)
	h.LeftClick(60, 2).Wait("1-16 of 36").Sleep(700 * time.Millisecond)

	// The log scrolls sideways, with its scrollbar on its bottom border.
	h.LeftDrag(12, 24, 44, 24, reel.Delay(25*time.Millisecond)).Wait("lease renewed").Sleep(900 * time.Millisecond)

	// Icons down the list's left side are buttons.
	h.LeftClick(0, 3).Wait("clicked ✎: edit").Sleep(700 * time.Millisecond)

	// The tabs on the border can be clicked: "pki", whose list fits, with no
	// scrollbar, then "all".
	h.LeftClick(54, 0).Wait("1-12 of 12").Sleep(900 * time.Millisecond)
	h.LeftClick(47, 0).Wait("1-16 of 36").Sleep(500 * time.Millisecond)

	h.Key("space").Sleep(300 * time.Millisecond)
	h.Key("q")
	h.WaitFinished()
}
