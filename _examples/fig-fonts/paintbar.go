// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// paintBar is the row of paints under the stage.
type paintBar struct {
	st    *styles
	paint int // index into st.paints, the paint in use

	pick func(i int) tea.Cmd // uses paint i
}

func (b paintBar) Layout(bloom.Context) bloom.Child {
	return bloom.Row(bloom.Text("paint").Style(b.st.paintLabel), bloom.Fill(b.chips()))
}

// chips are the paints, the one in use highlighted, each picked by a click.
func (b paintBar) chips() bloom.Child {
	chips := make([]bloom.Child, len(b.st.paints))
	for i, p := range b.st.paints {
		st := b.st.chip
		if i == b.paint {
			st = b.st.chipOn
		}
		chips[i] = bloom.Text(p.name).Style(st).On(on.Click(func() tea.Cmd { return b.pick(i) }))
	}
	return bloom.Wrap(chips...).Gap(1, 0)
}
