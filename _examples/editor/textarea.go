// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// undoSteps is how many steps of the undo history are kept.
const undoSteps = 1000

// step is a step of the undo history: the text, and what was selected in
// it, to go back to.
type step struct {
	text bloom.Snapshot
	sel  bloom.Range
}

// The kinds of edit that join the one before of the same kind into one
// step of the undo history.
const (
	joinNone   = ""
	joinTyping = "typing"
	joinDelete = "delete"
)

// textarea is the text being edited, in its pane: the buffer it is kept
// in, the caret and where the viewport is scrolled to, and the undo
// history. Its handlers change it, so it is placed by pointer.
type textarea struct {
	st styles

	buf  *bloom.Buffer
	name string // what the title calls the text
	path string // the file it is saved to, if it was opened from one

	sel    bloom.Range       // what is selected, the caret at its Head
	pos    bloom.ViewportPos // where the viewport is scrolled to
	goal   int               // the column kept while moving up and down, or -1
	wrap   bool
	cursor *tea.Cursor

	undo    []step
	redo    []step
	joins   string        // the kind of the last edit, which one of the same kind joins
	cost    time.Duration // what the last edit cost
	changed bool          // the text changed since it was opened or saved
}

// newTextarea returns an empty text, called sample.txt.
func newTextarea() textarea {
	t := textarea{
		buf:    &bloom.Buffer{},
		name:   "sample.txt",
		goal:   -1,
		wrap:   true,
		cursor: tea.NewCursor(0, 0),
	}
	t.cursor.Shape = tea.CursorBar
	// Words, for ctrl and the arrows and for a double click alike, are those
	// of code, joined only across "_", rather than taking in a path or a URL
	// whole, as they do by default.
	t.buf.SetWordChars("_")
	return t
}

// setStyles sets the styles the text is drawn with, of which it keeps a copy.
func (t *textarea) setStyles(st styles) *textarea {
	t.st = st
	return t
}

func (t *textarea) Layout(bloom.Context) bloom.Child {
	caretLine := t.sel.Head.Line
	text := bloom.Viewport(t.buf).ID(paneText).At(t.pos).LineNumbers(t.st.numbers).
		Caret(&t.sel).OnCaret(t.caretMoved).Cursor(t.cursor).
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { t.pos = p; return nil }).
		LineStyle(func(line int) lipgloss.Style {
			if line == caretLine {
				return t.st.current
			}
			return lipgloss.Style{}
		})
	if t.wrap {
		text = text.Wrap(bloom.WrapWords)
	}

	name := t.name
	if t.changed {
		name += " ●"
	}
	pane := bloom.Box(t.st.panel, text).
		Edge(bloom.Top, bloom.Start, bloom.Text(name).Style(t.st.title)).
		Edge(bloom.Top, bloom.End, bloom.Text(count(t.buf.Len())+" lines").Style(t.st.counter)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneText).
			Style(t.st.bar).HoverStyle(t.st.barHover))
	if !t.wrap {
		pane = pane.Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneText).Horizontal().
			Style(t.st.bar).HoverStyle(t.st.barHover))
	}
	return pane
}

// moveKey moves the caret for key, without shift, and with extend, the
// Head of the selection, keeps it in sight, and reports whether key moves
// it. Each key moves the Head, and the selection is collapsed to it unless
// shift is held.
func (t *textarea) moveKey(f *bloom.Frame, key string, extend bool) bool {
	v, _ := f.Viewport(paneText)
	sel, head, page := t.sel, t.sel.Head, max(v.Rows-1, 1)
	switch key {
	// Up and down keep the column the caret was at; the other keys forget it.
	case "up", "down":
		n := 1
		if key == "up" {
			n = -1
		}
		t.moveTo(v.MoveRows(sel, n, t.column(v)), extend)
		t.show(f)
		return true
	case "pgup", "pgdown":
		// A page, keeping the caret on the row of the viewport it was on.
		n := page
		if key == "pgup" {
			n = -page
		}
		sel, t.pos = v.ScrollRows(sel, n, t.column(v))
		t.moveTo(sel, extend)
		return true

	case "left":
		if !extend && !sel.Empty() {
			sel = sel.WithHead(sel.From())
		} else {
			sel = sel.WithHead(t.buf.Prev(head))
		}
	case "right":
		if !extend && !sel.Empty() {
			sel = sel.WithHead(sel.To())
		} else {
			sel = sel.WithHead(t.buf.Next(head))
		}
	case "ctrl+left":
		sel = sel.WithHead(t.buf.PrevWord(head))
	case "ctrl+right":
		sel = sel.WithHead(t.buf.NextWord(head))
	// Home and end go to the ends of the row the caret is drawn on, end to
	// the very end of a row the line wraps after, where the caret stays.
	case "home":
		sel = v.RowStart(sel)
	case "end":
		sel = v.RowEnd(sel)
	case "ctrl+home":
		sel = sel.WithHead(bloom.Pos{})
	case "ctrl+end":
		sel = sel.WithHead(t.buf.End())
	default:
		return false
	}
	t.goal = -1
	t.moveTo(sel, extend)
	t.show(f)
	return true
}

// moveTo makes sel, its Head moved, what is selected, or without extend,
// the caret at its Head.
func (t *textarea) moveTo(sel bloom.Range, extend bool) {
	if !extend {
		sel = sel.Collapse()
	}
	t.sel, t.joins = sel, joinNone
}

// column returns the column kept while moving up and down, that of the
// caret before it first moved up or down.
func (t *textarea) column(v bloom.ViewportView) int {
	if t.goal < 0 {
		t.goal = v.Column(t.sel)
	}
	return t.goal
}

// show scrolls the viewport as little as it takes for the caret to be in
// sight.
func (t *textarea) show(f *bloom.Frame) {
	if v, ok := f.Viewport(paneText); ok {
		t.pos = v.Show(t.sel)
	}
}

// replace replaces the text from from up to to with text, puts the caret
// after it, and notes the step in the undo history, unless it joins the
// last one: an edit of a kind other than joinNone joins the edit before it
// of the same kind, so that typing in a row is undone at once.
func (t *textarea) replace(from, to bloom.Pos, text, kind string) {
	if from == to && text == "" {
		return
	}
	if kind == joinNone || kind != t.joins {
		t.undo = append(t.undo, step{text: t.buf.Snapshot(), sel: t.sel})
		if len(t.undo) > undoSteps {
			t.undo = t.undo[1:]
		}
	}
	t.redo = t.redo[:0]
	start := time.Now()
	c := t.buf.Replace(from, to, text)
	t.cost = time.Since(start)
	t.sel = bloom.Range{Anchor: c.End(), Head: c.End()}
	t.joins, t.changed = kind, true
}

// undoStep puts the text and the selection back as they were before the
// last step of the undo history. It returns what the status line says of
// it, and whether there was a step to undo.
func (t *textarea) undoStep() (string, bool) {
	if t.travel(&t.undo, &t.redo) {
		return fmt.Sprintf("undone; %d more to undo", len(t.undo)), true
	}
	return "", false
}

// redoStep takes the step undone last again. It returns what the status
// line says of it, and whether there was a step to redo.
func (t *textarea) redoStep() (string, bool) {
	if t.travel(&t.redo, &t.undo) {
		return fmt.Sprintf("redone; %d more to redo", len(t.redo)), true
	}
	return "", false
}

// travel goes back to the last step of from, keeping where it is now as a
// step of to, and reports whether there was a step to go to.
func (t *textarea) travel(from, to *[]step) bool {
	if len(*from) == 0 {
		return false
	}
	*to = append(*to, step{text: t.buf.Snapshot(), sel: t.sel})
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	start := time.Now()
	t.buf.Restore(s.text)
	t.cost = time.Since(start)
	t.sel, t.joins, t.changed = s.sel, joinNone, true
	return true
}

// indent returns the spaces and tabs the caret's line starts with, up to
// the caret, for a new line to start with too.
func (t *textarea) indent() string {
	at := t.sel.From()
	text := t.buf.Text(at.Line)[:at.Col]
	return text[:len(text)-len(strings.TrimLeft(text, " \t"))]
}

// caretMoved forgets the column kept while moving up and down, and ends the
// typing an undo step joins, when the mouse moves the caret.
func (t *textarea) caretMoved(bloom.Range) tea.Cmd {
	t.goal, t.joins = -1, joinNone
	return nil
}
