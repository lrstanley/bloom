// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
)

// statusbar is the two lines under the log: what happened last, or the
// query as it is typed, beside where the log is, and then the keys. The
// model builds one each frame from its own state and the log's.
type statusbar struct {
	st        *styles
	status    string
	searching bool               // the query is being typed
	query     string             // what was typed
	view      bloom.ViewportView // where the log is, in the frame on screen
	lines     int                // how many lines the log has
	paused    bool
}

// hints are the keys, as the last line shows them.
const hints = " ↑↓ pgup pgdn g G scroll · ←→ sideways · w wrap · # numbers · / search · n N matches · p pause · q quit"

func (s statusbar) Layout(bloom.Context) bloom.Child {
	left := " " + s.status
	if s.searching {
		left = " /" + s.query + "█"
	}
	where := fmt.Sprintf("line %d of %d · %d%% ", s.view.First+1, s.lines, int(s.view.Percent()*100))
	switch {
	case s.paused:
		where += "· paused "
	case s.view.Pos.AtEnd():
		where += "· following "
	}
	return bloom.Column(
		bloom.Row(
			bloom.Fill(bloom.Text(left).Style(s.st.status).Truncate()),
			bloom.Text(where).Style(s.st.hint),
		),
		bloom.Text(hints).Style(s.st.hint).Truncate(),
	)
}
