// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
)

// statusline is the line at the bottom, with the keys: a TooltipSink while
// t sends tooltips there, and a hint otherwise.
type statusline struct {
	st   *styles
	sink bool // tooltips go to the status line
}

func (s statusline) Layout(bloom.Context) bloom.Child {
	where := "float"
	var left bloom.Child = bloom.Text("hover an icon, or press tab").Style(s.st.statusMuted).Truncate()
	if s.sink {
		where = "sink"
		left = bloom.TooltipSink().Style(s.st.statusBar)
	}
	keys := fmt.Sprintf("tips: %s (t) · q quit", where)
	return bloom.Row(bloom.Fill(left), bloom.Text(keys).Style(s.st.hint))
}
