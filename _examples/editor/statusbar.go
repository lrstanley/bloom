// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"unicode/utf8"

	"bloomfx.dev/bloom"
)

// statusbar is the two lines under the text: what happened last, beside
// where the caret is, what the last edit cost and whether lines wrap, and
// then the keys. The model builds one each frame.
type statusbar struct {
	st     *styles
	status string
	text   *textarea
}

// hints are the keys, as the last line shows them.
const hints = " arrows move · shift selects · ctrl+a all · ctrl+c copy · ctrl+x cut · ctrl+v paste · ctrl+z undo · ctrl+y redo · alt+z wrap · ctrl+q quit"

func (s statusbar) Layout(bloom.Context) bloom.Child {
	t := s.text
	head := t.buf.Clamp(t.sel.Head)
	col := utf8.RuneCountInString(t.buf.Text(head.Line)[:head.Col]) + 1
	where := fmt.Sprintf("Ln %s, Col %d", count(head.Line+1), col)
	if !t.sel.Empty() {
		where += fmt.Sprintf(" (%s selected)", count(utf8.RuneCountInString(t.buf.TextIn(t.sel.From(), t.sel.To()))))
	}
	cost := "no edits yet"
	if len(t.undo)+len(t.redo) > 0 {
		cost = "last edit " + t.cost.String()
	}
	right := fmt.Sprintf("%s · %s · %s ", where, cost, map[bool]string{true: "wrap", false: "no wrap"}[t.wrap])
	return bloom.Column(
		bloom.Row(
			bloom.Fill(bloom.Text(" "+s.status).Style(s.st.status).Truncate()),
			bloom.Text(right).Style(s.st.hint),
		),
		bloom.Text(hints).Style(s.st.hint).Truncate(),
	)
}
