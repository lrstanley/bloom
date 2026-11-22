// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"
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

func TestEditor(t *testing.T) {
	t.Parallel()
	// The system clipboard of whoever runs the tour is left alone.
	clip.Native = false

	m, err := newModel("", 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	h := steep.NewHarness(t, m, demo.Options("editor", 100, 26, effects.Mouse())...)
	h.Wait("sample.txt").Wait("1,000,000 lines").Sleep(demo.Hold(800 * time.Millisecond))
	slow := reel.Delay(45 * time.Millisecond)
	steps := func(key string, n int) {
		for range n {
			h.Key(key).Sleep(demo.Hold(140 * time.Millisecond))
		}
	}

	// A click puts the caret before the character under it, and typing goes
	// in there.
	h.Locate(reel.Contains("Try it:")).Offset(len("Try it"), 0).LeftClick().Wait("Ln 6, Col 7").Sleep(demo.Hold(400 * time.Millisecond))
	h.Type(", it is quick", slow).Wait("Try it, it is quick:").Sleep(demo.Hold(500 * time.Millisecond))

	// Shift and ctrl with the arrows select by words, and the selection is
	// cut, the band sweeping across it.
	steps("ctrl+shift+left", 3)
	h.Wait("(11 selected)").Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("ctrl+x").Wait(`cut "it is quick"`).Wait("Try it, :").Sleep(demo.Hold(500 * time.Millisecond))
	steps("backspace", 2)
	h.WaitNot("Try it, :").Sleep(demo.Hold(400 * time.Millisecond))

	// Pasted at the end of the first line.
	h.Key("ctrl+home").Key("end").Wait("Ln 1, Col 59")
	h.Type(" and ", slow).Key("ctrl+v").Wait("bloom.Buffer and it is quick").Sleep(demo.Hold(700 * time.Millisecond))

	// A double click selects a word, and ctrl+c copies it.
	h.Locate(reel.Contains("snapshots.")).Offset(3, 0).LeftClick(reel.Repeat(2)).Wait(`selected "snapshots"`).Sleep(demo.Hold(500 * time.Millisecond))
	h.Key("ctrl+c").Wait(`copied "snapshots"`).Sleep(demo.Hold(700 * time.Millisecond))

	// A drag across the rows a line wraps to, and shift+click to carry the
	// selection on.
	at := h.Locate(reel.Contains("this paragraph is one line"))
	at.LeftDrag(at.Offset(30, 1), slow).Wait(`selected "this paragraph is one line`).Sleep(demo.Hold(600 * time.Millisecond))
	shiftClick(h, at.X+12, at.Y+2)
	h.Wait("(152 selected)").Sleep(demo.Hold(700 * time.Millisecond))
	h.Key("esc").WaitNot("selected)").Sleep(demo.Hold(300 * time.Millisecond))

	// Undo goes back a step at a time, the selection with the text, and redo
	// comes forward again.
	steps("ctrl+z", 4)
	h.Wait("Try it, it is quick:").Wait("(11 selected)").Sleep(demo.Hold(700 * time.Millisecond))
	steps("ctrl+y", 4)
	h.Wait("bloom.Buffer and it is quick").WaitNot("Try it, :").Sleep(demo.Hold(700 * time.Millisecond))

	// Without wrapping, the end of a long line scrolls the text sideways to
	// keep the caret in sight; with it, the caret moves down the rows.
	h.Key("alt+z").Wait("not wrapping").Sleep(demo.Hold(500 * time.Millisecond))
	h.Locate(reel.Contains("Lines wrap at words")).LeftClick().Key("end").Wait("shows the rows.").Sleep(demo.Hold(700 * time.Millisecond))
	h.Key("alt+z").Wait("wrapping at words").Sleep(demo.Hold(400 * time.Millisecond))
	steps("up", 2)
	h.Sleep(demo.Hold(400 * time.Millisecond))
	// End goes to the very end of a row the line wraps after, past the
	// space it wraps at, and the caret stays on that row, in the column kept
	// for it; home goes back to the start of the same row.
	h.Key("end").Wait("Ln 14, Col 175").Sleep(demo.Hold(700 * time.Millisecond))
	row := h.Locate(reel.Contains("Lines wrap at words")).Y + 1
	if c := h.CursorPosition(); c.Y != row || c.X != 98 {
		t.Errorf("end put the cursor at %v, want at the end of row %d", c, row)
	}
	h.Key("home").Wait("Ln 14, Col 85").Sleep(demo.Hold(500 * time.Millisecond))
	if c := h.CursorPosition(); c.Y != row || c.X != 9 {
		t.Errorf("home put the cursor at %v, want at the start of row %d", c, row)
	}

	// To the end of a million lines, and typing there, at once.
	start := time.Now()
	h.Key("ctrl+end").Wait("Ln 1,000,000")
	t.Logf("ctrl+end to line a million drawn in %s", time.Since(start))
	h.Sleep(demo.Hold(600 * time.Millisecond))
	h.Key("enter").Type("The end, typed at line a million.", slow).Wait("typed at line a million.").Wait("Ln 1,000,001")
	start = time.Now()
	h.Key("!").Wait("a million.!")
	t.Logf("a key typed at line a million drawn in %s", time.Since(start))
	h.Key("backspace").WaitNot("a million.!").Sleep(demo.Hold(900 * time.Millisecond))
	steps("ctrl+left", 3)
	h.Sleep(demo.Hold(300 * time.Millisecond))

	// The wheel scrolls it, and ctrl+home goes back to the top.
	h.Locate(reel.Contains("typed at line")).Wheel(vt.MouseWheelUp, reel.Repeat(3), reel.Delay(demo.Hold(150*time.Millisecond)))
	h.Key("ctrl+home").Wait("Ln 1, Col 1").Sleep(demo.Hold(800 * time.Millisecond))

	// A page down and back up keeps the caret on its row of the screen, as
	// the text scrolls under it.
	steps("down", 3)
	h.Wait("Ln 4, Col 1")
	row = h.CursorPosition().Y
	h.Key("pgdown").Wait("Ln 18, Col 1").Sleep(demo.Hold(700 * time.Millisecond))
	if c := h.CursorPosition(); c.Y != row {
		t.Errorf("a page down put the cursor on row %d, from %d", c.Y, row)
	}
	h.Key("pgup").Wait("Ln 4, Col 1").Sleep(demo.Hold(700 * time.Millisecond))
	if c := h.CursorPosition(); c.Y != row {
		t.Errorf("a page up put the cursor on row %d, from %d", c.Y, row)
	}

	// Nothing is selected, so ctrl+c quits.
	h.Key("ctrl+c")
	h.WaitFinished()

	if n := m.buf.Len(); n != 1_000_001 {
		t.Fatalf("finished with %d lines", n)
	}
	if got := m.buf.Text(0); !strings.HasSuffix(got, "bloom.Buffer and it is quick") {
		t.Fatalf("first line %q", got)
	}
	if got := m.buf.Text(m.buf.Len() - 1); got != "The end, typed at line a million." {
		t.Fatalf("last line %q", got)
	}
	t.Logf("the last edit to a million lines took %s", m.cost)
	if m.cost > 10*time.Millisecond {
		t.Errorf("the last edit took %s", m.cost)
	}
}

// shiftClick clicks the left button at x, y with shift held, which the
// harness's mouse cannot send through the terminal, so it goes to the
// program as the terminal would report it.
func shiftClick(h *steep.Harness, x, y int) {
	h.SendProgram(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: tea.ModShift}).Sleep(80 * time.Millisecond)
	h.SendProgram(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: tea.ModShift}).Sleep(150 * time.Millisecond)
}
