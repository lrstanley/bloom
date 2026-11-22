// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image"
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/demo"
	tea "charm.land/bubbletea/v2"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

func TestDesktop(t *testing.T) {
	t.Parallel()

	m := newModel()
	// The clock shows Go's reference time, for the snapshot to match.
	m.bar.Now = func() time.Time { return time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC) }
	h := steep.NewHarness(t, m, demo.Options("desktop", 100, 30, effects.Mouse())...)
	h.Wait("drag a title").Sleep(demo.Hold(800 * time.Millisecond))

	drag := reel.Delay(25 * time.Millisecond)

	// Files is behind the notes: grabbing its title brings it to the front, and
	// it stays where it is dropped.
	h.LeftDrag(24, 2, 30, 15, drag).Wait("dropped Files at 20,15").Sleep(demo.Hold(600 * time.Millisecond))

	// The notes, held against the left edge, show where they will snap to,
	// and snap to that half when let go.
	h.Mouse(vt.MousePress, vt.MouseLeft, 55, 7)
	for x := 50; x >= 0; x -= 5 {
		h.Mouse(vt.MouseMotion, vt.MouseLeft, x, 7+(55-x)/10).Sleep(25 * time.Millisecond)
	}
	h.Wait("moving README.md").Sleep(demo.Hold(900 * time.Millisecond))
	h.Mouse(vt.MouseRelease, vt.MouseLeft, 0, 12)
	h.Wait("snapped README.md to the left half").Sleep(demo.Hold(600 * time.Millisecond))

	// The calculator's keys take clicks: 7 × 6 =.
	for _, at := range [][2]int{{70, 6}, {91, 6}, {84, 8}, {91, 12}} {
		h.LeftClick(at[0], at[1]).Sleep(demo.Hold(250 * time.Millisecond))
	}
	h.Wait("42 ").Sleep(demo.Hold(500 * time.Millisecond))

	// Its corner resizes it, and its keys grow with it.
	h.LeftDrag(96, 13, 98, 22, drag).Wait("resized Calculator to 33×22").Sleep(demo.Hold(600 * time.Millisecond))

	// A double click on a title maximizes the window, and another puts it back.
	h.LeftClick(80, 1, reel.Repeat(2)).Wait("maximized Calculator").Sleep(800 * time.Millisecond)
	h.LeftClick(30, 0, reel.Repeat(2)).Wait("restored the size of Calculator").Sleep(800 * time.Millisecond)

	// The taskbar minimizes the window in front, and brings it back.
	h.LeftClick(40, 29).Wait("minimized Calculator").Sleep(demo.Hold(600 * time.Millisecond))
	h.LeftClick(40, 29).Wait("restored Calculator").Sleep(demo.Hold(600 * time.Millisecond))

	// Dragging a snapped window tears it off at its own size.
	h.LeftDrag(8, 0, 40, 6, drag).Wait("dropped README.md").Sleep(demo.Hold(600 * time.Millisecond))

	// Tab brings the window at the back to the front, and the arrows move it.
	h.Key("tab").Wait("Files is in front").Sleep(demo.Hold(400 * time.Millisecond))
	h.Key("left", reel.Repeat(4), reel.Delay(demo.Hold(80*time.Millisecond))).Wait("moved Files to 12,15")

	// The calculator closes from its title bar.
	h.LeftClick(96, 1).Wait("closed Calculator").WaitNot("▦ Calculator")

	// The start menu opens the settings, which change the theme.
	h.LeftClick(3, 29).Wait("Log out").Sleep(300 * time.Millisecond).Sleep(demo.Hold(300 * time.Millisecond))
	h.LeftClick(5, 25).Wait("opened Settings").Sleep(300 * time.Millisecond).Sleep(demo.Hold(300 * time.Millisecond))
	h.LeftClick(30, 5).Wait("changed the theme to Amber CRT Retro").Sleep(demo.Hold(900 * time.Millisecond))
	h.LeftClick(30, 6).Wait("changed the theme to Green Phosphor CRT").Sleep(demo.Hold(900 * time.Millisecond))
	h.LeftClick(30, 10).Wait("changed the theme to Outrun Electric").Sleep(demo.Hold(900 * time.Millisecond))
	h.AssertSnapshot()

	// Logging out shuts the desktop down, and the program quits.
	h.LeftClick(3, 29).Wait("Log out").Sleep(300 * time.Millisecond).Sleep(demo.Hold(200 * time.Millisecond))
	h.LeftClick(5, 27).Wait("shutting down")
	h.WaitFinished()
}

// TestClickBehind clicks the close button of a window behind the others,
// with a frame drawn between the press and the release, as there is for a
// person: the press brings the window to the front, which moves it in the
// tree, and the release still closes it.
func TestClickBehind(t *testing.T) {
	t.Parallel()

	m := newModel()
	m.splash.Skip() // past the splash
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.View()

	files := m.wm.Windows()[0] // at the back
	r, ok := m.ui.Frame().Rect(files.ID())
	if !ok {
		t.Fatal("the files window was not drawn")
	}
	// Its close button, " × ", is the last on its title bar, before the
	// corner of its border.
	at := image.Pt(r.Max.X-3, r.Min.Y)
	m.Update(tea.MouseClickMsg{X: at.X, Y: at.Y, Button: tea.MouseLeft})
	m.View()
	if m.wm.Top() != files {
		t.Fatal("the press did not bring the files window to the front")
	}
	m.Update(tea.MouseReleaseMsg{X: at.X, Y: at.Y, Button: tea.MouseLeft})
	if !files.Closing() {
		t.Fatalf("the click did not close the files window: %s", m.bar.Status)
	}
}
