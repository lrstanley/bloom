// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
)

// input is a one-line field of ASCII text, a byte to a cell. It implements
// bloom.Editable: it keeps its own selection, whose Head is its caret, which
// the arrows move and shift and the arrows extend, and which the mouse sets
// with Select. Typing replaces what is selected. It is placed in the layout
// by pointer, so that Select reaches it.
type input struct {
	st     styles
	text   string
	sel    bloom.Range
	typing bool
}

func (f *input) View() string {
	if !f.typing {
		return f.st.value.Render(f.text)
	}
	s, at := f.text+" ", f.sel.Head.Col
	return f.st.value.Render(s[:at]) + f.st.caret.Render(s[at:at+1]) + f.st.value.Render(s[at+1:])
}

func (f *input) col(p bloom.Pos) int {
	if p.Line > 0 {
		return len(f.text)
	}
	return min(max(p.Col, 0), len(f.text))
}

// PosAt implements bloom.TextMap.
func (f *input) PosAt(cell image.Point, _ bloom.Size) bloom.Pos {
	return bloom.Pos{Col: f.col(bloom.Pos{Col: cell.X})}
}

// Cells implements bloom.TextMap.
func (f *input) Cells(from, to bloom.Pos, _ bloom.Size, cells func(y, x0, x1 int)) {
	cells(0, f.col(from), f.col(to))
}

// Text implements bloom.TextMap.
func (f *input) Text(from, to bloom.Pos) string { return f.text[f.col(from):f.col(to)] }

// Selection implements bloom.Editable.
func (f *input) Selection() bloom.Range { return f.sel }

// Select implements bloom.Editable.
func (f *input) Select(r bloom.Range) {
	f.sel = bloom.Range{Anchor: bloom.Pos{Col: f.col(r.Anchor)}, Head: bloom.Pos{Col: f.col(r.Head)}}
}

// key handles a key pressed while typing in the field.
func (f *input) key(msg tea.KeyPressMsg) {
	from, to, head := f.sel.From().Col, f.sel.To().Col, f.sel.Head.Col
	move := func(col int, extend bool) {
		f.sel.Head = bloom.Pos{Col: min(max(col, 0), len(f.text))}
		if !extend {
			f.sel.Anchor = f.sel.Head
		}
	}
	switch k := msg.String(); k {
	case "left", "right":
		left := k == "left"
		switch {
		case from != to && left:
			move(from, false)
		case from != to:
			move(to, false)
		case left:
			move(head-1, false)
		default:
			move(head+1, false)
		}
	case "shift+left":
		move(head-1, true)
	case "shift+right":
		move(head+1, true)
	case "home", "shift+home":
		move(0, k != "home")
	case "end", "shift+end":
		move(len(f.text), k != "end")
	case "backspace":
		if from == to && from > 0 {
			from--
		}
		f.replace(from, to, "")
	default:
		if msg.Text != "" {
			f.replace(from, to, msg.Text)
		}
	}
}

// replace replaces the text from from up to to with s, leaving the caret
// after it.
func (f *input) replace(from, to int, s string) {
	f.text = f.text[:from] + s + f.text[to:]
	f.sel = bloom.Range{Anchor: bloom.Pos{Col: from + len(s)}, Head: bloom.Pos{Col: from + len(s)}}
}
