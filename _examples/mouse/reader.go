// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"cmp"
	"fmt"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
)

// reader shows the opened file under its name. Its scroll handler changes
// where it is scrolled to, so it has pointer receivers, and is placed as
// &m.reader.
type reader struct {
	st     styles
	opened string
	buf    *bloom.Buffer // the text of the opened file
	at     bloom.ViewportPos
}

// open opens file f in the reader, from its top.
func (r *reader) open(f string) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("%2d  // line %d of %s", i+1, i+1, f)
	}
	r.buf.SetLines(lines...)
	r.opened, r.at = f, bloom.ViewportPos{}
}

func (r *reader) Layout(bloom.Context) bloom.Child {
	return bloom.Column(
		bloom.Text(cmp.Or(r.opened, "reader")).Style(r.st.heading),
		// The text wraps when the reader is narrow, and the wheel scrolls it.
		bloom.Fill(bloom.Box(r.st.page, bloom.Viewport(r.buf).At(r.at).
			Wrap(bloom.WrapWords).Style(r.st.code).Selectable().
			OnScroll(func(p bloom.ViewportPos) tea.Cmd { r.at = p; return nil }))),
	)
}
