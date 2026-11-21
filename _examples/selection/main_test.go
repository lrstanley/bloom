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

	h := steep.NewHarness(t, newModel(), demo.Options("selection", 100, 30, effects.Mouse())...)
	h.Wait("drag over any text").Sleep(600 * time.Millisecond)
	slow := reel.Delay(30 * time.Millisecond)

	// Across lines of the log, inside the log's box alone.
	at := h.Locate(reel.Contains("upstream payments slow"))
	at.LeftDrag(at.Offset(20, 2), slow).Wait(`selected "upstream payments slow`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "upstream payments slow`).Sleep(demo.Hold(800 * time.Millisecond))

	// Alt held as the drag starts selects a rectangle: the timestamps
	// alone.
	at = h.Locate(reel.Contains("14:02:00.000"))
	altPress(h, at.X, at.Y)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+5, at.Y+2).Sleep(150 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+11, at.Y+3).Sleep(150 * time.Millisecond)
	h.Mouse(vt.MouseRelease, vt.MouseLeft, at.X+11, at.Y+3).
		Wait(`selected "14:02:00.000⏎14:02:01.700⏎14:02:03.4`).Sleep(demo.Hold(800 * time.Millisecond))

	// Across the request's fields, a value beside its label.
	at = h.Locate(reel.Contains("method"))
	at.LeftDrag(at.Offset(40, 1), slow).
		Wait(`selected "method POST⏎path /api/v1/orders/1042/`).Sleep(demo.Hold(800 * time.Millisecond))

	// A value cut to fit is copied whole: a double click on it selects all
	// of the word cut.
	at = h.Locate(reel.Contains("7f3a9c2e"))
	at.Offset(3, 0).LeftClick(reel.Repeat(2)).
		Wait(`selected "7f3a9c2e-41b8-4d0e-9a51-c2d8e6f01b77" in request`).Sleep(demo.Hold(800 * time.Millisecond))

	// A click clears the selection; ctrl+c with none would quit.
	at.LeftClick().Sleep(demo.Hold(500 * time.Millisecond))

	// A double click selects a word.
	h.Locate(reel.Contains("rfnd_")).Offset(3, 0).LeftClick(reel.Repeat(2)).Wait(`selected "rfnd_7Hc2Lq9xVb" in request`).Sleep(demo.Hold(800 * time.Millisecond))

	// Across the rows the error wrapped to, it is copied as one line.
	end := h.Locate(reel.Contains("3); the refund"))
	h.Locate(reel.Contains("refunding order")).LeftDrag(end.Offset(2, 0), slow).Wait(`selected "refunding order 1042 (attempt 1 of 3);" in request`).Sleep(demo.Hold(800 * time.Millisecond))

	// ctrl+a selects all of the log, scrolled out of sight or not.
	h.Key("ctrl+a").Wait("selected all 36 lines of the log").Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "14:02:00.000 INFO  listening on :8080⏎`).Sleep(demo.Hold(800 * time.Millisecond))

	// Held below the log, on the note's border, a drag scrolls it to its
	// end.
	at = h.Locate(reel.Contains("listening on"))
	h.Mouse(vt.MousePress, vt.MouseLeft, at.X, at.Y).Sleep(100 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, at.X+10, 25).Wait("14:02:59.500")
	h.Mouse(vt.MouseRelease, vt.MouseLeft, at.X+10, 25).Wait(`selected "listening on :8080`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("esc").Wait("selection cleared").Sleep(demo.Hold(300 * time.Millisecond))

	// Inside the dialog, over the log it covers.
	h.Key("d").Wait("API token created").Sleep(500 * time.Millisecond)
	at = h.Locate(reel.Contains("blm_live_"))
	at.LeftDrag(at.Offset(34, 0), slow).Wait(`selected "blm_live_9Qk2Xv7rLp4TzW8nB3sHc6Yd" in dialog`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "blm_live_`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("esc").WaitNot("API token created").Sleep(demo.Hold(500 * time.Millisecond))

	// The notes wrap and scroll their own text: a selection there leaves
	// their line numbers out, and stays on its text as the wheel scrolls
	// it away.
	at = h.Locate(reel.Contains("Refunds that"))
	end = h.Locate(reel.Contains("retried three"))
	at.LeftDrag(end.Offset(12, 0), slow).Wait(`selected "Refunds that time out upstream`).Sleep(demo.Hold(800 * time.Millisecond))
	at.Wheel(vt.MouseWheelDown, reel.Repeat(3), reel.Delay(200*time.Millisecond))
	h.Wait("its retries were").Key("ctrl+c").Wait(`copied "Refunds that time out upstream`).Sleep(demo.Hold(800 * time.Millisecond))

	// The note keeps its own selection. Shift and the arrows make one,
	// which is the selection at once, and typing replaces it.
	h.Key("i").Wait("typing in the note").Key("end").Sleep(300 * time.Millisecond)
	for range 4 {
		h.Key("shift+left").Sleep(120 * time.Millisecond)
	}
	h.Wait(`selected "hand" in the note`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Type("phone", slow).Wait("retried by phone").Sleep(demo.Hold(500 * time.Millisecond))
	// The mouse selects in it as well, and ctrl+c copies what it selected,
	// leaving it selected.
	h.Locate(reel.Contains("retried by phone")).Offset(2, 0).LeftClick(reel.Repeat(2)).Wait(`selected "retried" in note`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "retried"`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("esc").Wait("stopped typing").Key("esc").Wait("selection cleared").Sleep(demo.Hold(500 * time.Millisecond))

	// The requests are a list of ten thousand rows, which builds only those
	// in sight. A selection there keeps the row it starts in as the wheel,
	// turned with the button held, scrolls it away, and copies every row it
	// covers, with a space between the cells of a row: from the path of
	// order 1002 to that of 1015, twelve rows below where it was.
	h.Key("l").Wait("showing 10000 requests").Sleep(demo.Hold(500 * time.Millisecond))
	at = h.Locate(reel.Contains("/api/v1/orders/1002/items"))
	to := at.Offset(8, 1)
	h.Mouse(vt.MousePress, vt.MouseLeft, at.X, at.Y).Sleep(100 * time.Millisecond)
	h.Mouse(vt.MouseMotion, vt.MouseLeft, to.X, to.Y).Sleep(150 * time.Millisecond)
	to.Wheel(vt.MouseWheelDown, reel.Repeat(4), reel.Delay(200*time.Millisecond))
	h.Wait("/api/v1/orders/1015/items").WaitNot("/api/v1/orders/1002/items")
	h.Mouse(vt.MouseRelease, vt.MouseLeft, to.X, to.Y).
		Wait(`selected "/api/v1/orders/1002/items 201⏎14:02:01.`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "/api/v1/orders/1002/items 201⏎14:02:01.`).Wait(`, 14 lines`).Sleep(demo.Hold(800 * time.Millisecond))

	// ctrl+a selects all of them at once, and copies every one.
	h.Key("ctrl+a").Wait("selected all 10000 requests").Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "14:02:00.000 GET /api/v1/orders/1000/`).Wait(`, 10000 lines`).Sleep(demo.Hold(800 * time.Millisecond))
	h.Key("l").Wait("showing the log").Sleep(demo.Hold(300 * time.Millisecond))

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
