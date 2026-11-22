// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// fontList is the fonts' names, scrolled to keep the one shown in the
// middle, as tall as it is given, in a frame. A click picks a font, and the
// wheel scrolls through them.
type fontList struct {
	st   *styles
	font int // index into gallery, the font shown

	pick func(i int) tea.Cmd  // shows font i
	step func(by int) tea.Cmd // shows the font by fonts on from the one shown
}

func (l fontList) Layout(bloom.Context) bloom.Child {
	return bloom.Box(l.st.panel, bloom.LayoutFunc(l.names)).
		Edge(bloom.Top, bloom.Start, bloom.Text(" fonts ").Style(l.st.title)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf(" %d/%d ", l.font+1, len(gallery))).Style(l.st.dim)).
		On(on.Scroll(func(_, dy int) tea.Cmd { return l.step(dy) }))
}

// names are the rows of the list, as many as fit.
func (l fontList) names(ctx bloom.Context) bloom.Child {
	n := min(ctx.Height(), len(gallery))
	first := min(max(l.font-n/2, 0), len(gallery)-n)
	rows := make([]bloom.Child, 0, n)
	for i := first; i < first+n; i++ {
		st, name := l.st.item, fonts.Names[i]
		if i == l.font {
			st, name = l.st.selected, "› "+name
		}
		rows = append(rows, bloom.Text(name).Style(st).Truncate().On(on.Click(func() tea.Cmd {
			return l.pick(i)
		})))
	}
	return bloom.Column(rows...)
}
