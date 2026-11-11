// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
	"github.com/charmbracelet/x/ansi"
)

// locate returns the cell text starts at in view.
func locate(view, text string) (x, y int, ok bool) {
	for y, line := range strings.Split(view, "\n") {
		if before, _, ok := strings.Cut(line, text); ok {
			return ansi.StringWidth(before), y, true
		}
	}
	return 0, 0, false
}

// find returns the cell text starts at on screen, waiting for it first: a
// tint's wave can still be scrambling the panes when a slow (such as -race)
// run gets here.
func find(t *testing.T, h *steep.Harness, text string) (x, y int) {
	t.Helper()
	view := h.Wait(text).View()
	x, y, ok := locate(view, text)
	if !ok {
		t.Fatalf("%q is not on screen:\n%s", text, view)
	}
	return x, y
}

// movedPast holds once the Activity pane's title is right of the Services
// pane's.
type movedPast struct{}

func (movedPast) Check(screen reel.ScreenText) ([]reel.Match, bool) {
	a, _, aok := locate(screen.Text, "⠿ Activity")
	s, _, sok := locate(screen.Text, "⠿ Services")
	return nil, aok && sok && a > s
}

func (movedPast) String() string { return "Activity right of Services" }

// checkMovedPast waits for Activity to be dragged past Services, then checks
// it is still there once the panes have glided into their places. A slow
// (such as -race, or a busy CI) run can still be mid-glide after the sleep,
// so the second check waits for Activity to land.
func checkMovedPast(h *steep.Harness) {
	h.WaitFor(movedPast{}).Sleep(1200 * time.Millisecond).WaitFor(movedPast{})
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
	h.LeftClick(x, y).Wait("billing is slow").Sleep(demo.Hold(500 * time.Millisecond))
	pick(t, h, "Rose Pine")

	// A pane dragged by its title floats under the pointer, and the others
	// make room as it passes them.
	x1, y1 := find(t, h, "⠿ Activity")
	x2, y2 := find(t, h, "⠿ Services")
	h.MouseDrag(vt.MouseLeft, x1+3, y1, x2+20, y2+4, reel.Delay(20*time.Millisecond))
	checkMovedPast(h)
	pick(t, h, "Catppuccin Mocha")

	// A double click zooms a pane, and another gives the rest their width
	// back.
	x, y = find(t, h, "⠿ Palette")
	h.LeftClick(x+3, y, reel.Repeat(2)).WaitNot("⠿ Services").Sleep(1200 * time.Millisecond)
	x, y = find(t, h, "⠿ Palette")
	h.LeftClick(x+3, y, reel.Repeat(2)).Wait("⠿ Services").Sleep(demo.Hold(1000 * time.Millisecond))

	// The tour spotlights each part in turn.
	h.Key("?").Wait("Click one to switch").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("?").Wait("Double click to zoom").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("?").Wait("lowest priority first").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("esc").WaitNot("lowest priority first").Sleep(demo.Hold(300 * time.Millisecond))
	pick(t, h, "Kanagawa Wave")

	// Narrowing the terminal collapses the palette, then the sidebar.
	h.Resize(96, 24).WaitNot("⠿ Palette").Sleep(demo.Hold(900 * time.Millisecond))
	h.Resize(64, 24).WaitNot("Tints").Sleep(demo.Hold(900 * time.Millisecond))
	h.Resize(130, 24).Wait("⠿ Palette").Wait("Tints").Sleep(demo.Hold(900 * time.Millisecond))

	h.Key("t").Wait("▸ Dracula").Sleep(demo.Hold(1800 * time.Millisecond))
	h.Key("q")
	h.WaitFinished()
}

// TestDragPastCollapsed drags a pane past another while the terminal is
// narrow enough that a pane between them is collapsed out of sight.
func TestDragPastCollapsed(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), steep.WithWindowSize(96, 24), steep.WithTimeout(20*time.Second))
	h.Wait("billing").WaitNot("⠿ Palette").Sleep(800 * time.Millisecond)
	x1, y1 := find(t, h, "⠿ Activity")
	x2, y2 := find(t, h, "⠿ Services")
	h.MouseDrag(vt.MouseLeft, x1+3, y1, x2+24, y2+4, reel.Delay(20*time.Millisecond))
	checkMovedPast(h)
	h.Key("q")
	h.WaitFinished()
}
