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
	"getsteep.dev/steep/reel/effects"
)

// movedPast holds once the Activity pane's title is right of the Services
// pane's, on whichever rows they are.
type movedPast struct{}

func (movedPast) Check(screen reel.ScreenText) ([]reel.Match, bool) {
	a, aok := reel.Contains("⠿ Activity").Check(screen)
	s, sok := reel.Contains("⠿ Services").Check(screen)
	return nil, aok && sok && a[0].Col > s[0].Col
}

func (movedPast) String() string { return "Activity right of Services" }

// checkMovedPast waits for Activity to be dragged past Services, then checks
// it is still there once the panes have glided into their places. A slow
// (such as -race, or a busy CI) run can still be mid-glide after the sleep,
// so the second check waits for Activity to land.
func checkMovedPast(h *steep.Harness) {
	h.WaitFor(movedPast{}).Sleep(1200 * time.Millisecond).WaitFor(movedPast{})
}

// dragActivity drags the Activity pane by its title to dx columns right of
// the Services pane's title, and 4 rows below it.
func dragActivity(h *steep.Harness, dx int) {
	to := h.Locate(reel.Contains("⠿ Services")).Offset(dx, 4)
	h.Locate(reel.Contains("⠿ Activity")).Offset(3, 0).LeftDrag(to, reel.Delay(20*time.Millisecond))
}

// pick clicks the tint named name in the sidebar, and waits for the panes
// to wave back in with its colors.
func pick(h *steep.Harness, name string) {
	h.Locate(reel.Contains(name)).Offset(2, 0).LeftClick().Wait("▸ " + name).Sleep(1800 * time.Millisecond)
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
	h.Locate(reel.Contains("billing")).LeftClick().Wait("billing is slow").Sleep(demo.Hold(500 * time.Millisecond))
	pick(h, "Rose Pine")

	// A pane dragged by its title floats under the pointer, and the others
	// make room as it passes them.
	dragActivity(h, 20)
	checkMovedPast(h)
	pick(h, "Catppuccin Mocha")

	// A double click zooms a pane, and another gives the rest their width
	// back.
	h.Locate(reel.Contains("⠿ Palette")).Offset(3, 0).LeftClick(reel.Repeat(2)).WaitNot("⠿ Services").Sleep(1200 * time.Millisecond)
	h.Locate(reel.Contains("⠿ Palette")).Offset(3, 0).LeftClick(reel.Repeat(2)).Wait("⠿ Services").Sleep(demo.Hold(1000 * time.Millisecond))

	// The tour spotlights each part in turn.
	h.Key("?").Wait("Click one to switch").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("?").Wait("Double click to zoom").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("?").Wait("lowest priority first").Sleep(demo.Hold(1600 * time.Millisecond))
	h.Key("esc").WaitNot("lowest priority first").Sleep(demo.Hold(300 * time.Millisecond))
	pick(h, "Kanagawa Wave")

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
	dragActivity(h, 24)
	checkMovedPast(h)
	h.Key("q")
	h.WaitFinished()
}
