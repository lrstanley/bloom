// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/_examples/internal/demo"
	"github.com/lrstanley/x/term/reel"
	"github.com/lrstanley/x/term/reel/effects"
	"github.com/lrstanley/x/term/reel/vt"
	"github.com/lrstanley/x/term/steep"
)

// find returns the cell text starts at on screen.
func find(t *testing.T, h *steep.Harness, text string) (x, y int) {
	t.Helper()
	view := h.View()
	for y, line := range strings.Split(view, "\n") {
		if before, _, ok := strings.Cut(line, text); ok {
			return ansi.StringWidth(before), y
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, view)
	return 0, 0
}

// pick clicks the tint named name in the sidebar, and waits for the panes
// to wave back in with its colors.
func pick(t *testing.T, h *steep.Harness, name string) {
	t.Helper()
	x, y := find(t, h, name)
	h.LeftClick(x+2, y).Wait("▸ " + name).Sleep(1800 * time.Millisecond)
}

func TestShowcase(t *testing.T) {
	t.Parallel()

	// Recorded at a higher frame rate than the other examples, for the
	// docs' front page.
	opts := append(demo.Options("showcase", 130, 24, effects.Mouse()), steep.WithRecording(reel.Profile{FPS: 50}))
	h := steep.NewHarness(t, newModel(), opts...)
	demo.ResizeInBand(t, h)
	h.Wait("▸ TokyoNight").Wait("billing").Sleep(1500 * time.Millisecond)

	// A click selects a service.
	x, y := find(t, h, "billing")
	h.LeftClick(x, y).Wait("billing is slow").Sleep(500 * time.Millisecond)
	pick(t, h, "Rose Pine")

	// A pane dragged by its title floats under the pointer, and the others
	// make room as it passes them.
	x1, y1 := find(t, h, "⠿ Activity")
	x2, y2 := find(t, h, "⠿ Services")
	h.MouseDrag(vt.MouseLeft, x1+3, y1, x2+20, y2+4, reel.Delay(20*time.Millisecond))
	h.Sleep(1200 * time.Millisecond)
	a, _ := find(t, h, "⠿ Activity")
	if s, _ := find(t, h, "⠿ Services"); a < s {
		t.Fatalf("Activity did not move past Services:\n%s", h.View())
	}
	pick(t, h, "Catppuccin Mocha")

	// A double click zooms a pane, and another gives the rest their width
	// back.
	x, y = find(t, h, "⠿ Palette")
	h.LeftClick(x+3, y, reel.Repeat(2)).WaitNot("⠿ Services").Sleep(1200 * time.Millisecond)
	x, y = find(t, h, "⠿ Palette")
	h.LeftClick(x+3, y, reel.Repeat(2)).Wait("⠿ Services").Sleep(1000 * time.Millisecond)

	// The tour spotlights each part in turn.
	h.Key("?").Wait("Click one to switch").Sleep(1600 * time.Millisecond)
	h.Key("?").Wait("Double click to zoom").Sleep(1600 * time.Millisecond)
	h.Key("?").Wait("lowest priority first").Sleep(1600 * time.Millisecond)
	h.Key("esc").WaitNot("lowest priority first").Sleep(300 * time.Millisecond)
	pick(t, h, "Kanagawa Wave")

	// Narrowing the terminal collapses the palette, then the sidebar.
	h.Resize(96, 24).WaitNot("⠿ Palette").Sleep(900 * time.Millisecond)
	h.Resize(64, 24).WaitNot("Tints").Sleep(900 * time.Millisecond)
	h.Resize(130, 24).Wait("⠿ Palette").Wait("Tints").Sleep(900 * time.Millisecond)

	h.Key("t").Wait("▸ Dracula").Sleep(1800 * time.Millisecond)
	h.Key("q")
	h.WaitFinished()
}
