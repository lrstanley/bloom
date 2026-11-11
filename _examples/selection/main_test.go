// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"testing"
	"time"

	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/_examples/internal/demo"
	tea "charm.land/bubbletea/v2"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/vt"
)

func init() { clip.Native = false }

func TestSelection(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel(), demo.Options("selection", 100, 20, effects.Mouse())...)
	h.Wait("drag over any text").Sleep(600 * time.Millisecond)
	slow := reel.Delay(30 * time.Millisecond)

	// Across lines of the log, inside the log's box alone.
	at := h.Locate(reel.Contains("upstream payments slow"))
	h.LeftDrag(at.X, at.Y, at.X+20, at.Y+2, slow).Wait(`selected "upstream payments slow`).Sleep(800 * time.Millisecond)
	h.Key("ctrl+c").Wait(`copied "upstream payments slow`).Sleep(800 * time.Millisecond)

	// Alt held as the drag starts selects a rectangle: the timestamps
	// alone.
	at = h.Locate(reel.Contains("14:02:00.000"))
	altPress(h, at.X, at.Y)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+5, at.Y+2).Sleep(150 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+11, at.Y+3).Sleep(150 * time.Millisecond)
	h.Mouse(vt.MouseRelease, vt.MouseLeft, at.X+11, at.Y+3).
		Wait(`selected "14:02:00.000⏎14:02:01.700⏎14:02:03.4`).Sleep(800 * time.Millisecond)

	// Across the request's fields, a value beside its label.
	at = h.Locate(reel.Contains("method"))
	h.LeftDrag(at.X, at.Y, at.X+40, at.Y+1, slow).
		Wait(`selected "method POST⏎path /api/v1/orders/1042/`).Sleep(800 * time.Millisecond)

	// A value cut to fit is copied whole: a double click on it selects all
	// of the word cut.
	at = h.Locate(reel.Contains("7f3a9c2e"))
	h.LeftClick(at.X+3, at.Y, reel.Repeat(2)).
		Wait(`selected "7f3a9c2e-41b8-4d0e-9a51-c2d8e6f01b77" in request`).Sleep(800 * time.Millisecond)

	// A click clears the selection; ctrl+c with none would quit.
	h.LeftClick(at.X, at.Y).Sleep(500 * time.Millisecond)

	// A double click selects a word.
	at = h.Locate(reel.Contains("rfnd_"))
	h.LeftClick(at.X+3, at.Y, reel.Repeat(2)).Wait(`selected "rfnd_7Hc2Lq9xVb" in request`).Sleep(800 * time.Millisecond)

	// Across the rows the error wrapped to, it is copied as one line.
	at = h.Locate(reel.Contains("refunding order"))
	end := h.Locate(reel.Contains("3); the refund"))
	h.LeftDrag(at.X, at.Y, end.X+2, end.Y, slow).Wait(`selected "refunding order 1042 (attempt 1 of 3);" in request`).Sleep(800 * time.Millisecond)

	// ctrl+a selects all of the log, scrolled out of sight or not.
	h.Key("ctrl+a").Wait("selected all 36 lines of the log").Sleep(600 * time.Millisecond)
	h.Key("ctrl+c").Wait(`copied "14:02:00.000 INFO  listening on :8080⏎`).Sleep(800 * time.Millisecond)

	// Held below the log, a drag scrolls it to its end.
	at = h.Locate(reel.Contains("listening on"))
	h.Mouse(vt.MousePress, vt.MouseLeft, at.X, at.Y).Sleep(100 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+10, 18).Wait("14:02:59.500")
	h.Mouse(vt.MouseRelease, vt.MouseLeft, at.X+10, 18).Wait(`selected "listening on :8080`).Sleep(800 * time.Millisecond)
	h.Key("esc").Wait("selection cleared").Sleep(300 * time.Millisecond)

	// Inside the dialog, over the log it covers.
	h.Key("d").Wait("API token created").Sleep(500 * time.Millisecond)
	at = h.Locate(reel.Contains("blm_live_"))
	h.LeftDrag(at.X, at.Y, at.X+34, at.Y, slow).Wait(`selected "blm_live_9Qk2Xv7rLp4TzW8nB3sHc6Yd" in dialog`).Sleep(800 * time.Millisecond)
	h.Key("ctrl+c").Wait(`copied "blm_live_`).Sleep(800 * time.Millisecond)
	h.Key("esc").WaitNot("API token created").Sleep(500 * time.Millisecond)

	// Nothing is selected, so ctrl+c quits.
	h.Key("ctrl+c")
	h.WaitFinished()
}

// altPress presses the left button at x, y with alt held, which the
// harness's mouse cannot send through the terminal, so it goes to the
// program as the terminal would report it.
func altPress(h *steep.Harness, x, y int) {
	h.SendProgram(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: tea.ModAlt}).Sleep(150 * time.Millisecond)
}
