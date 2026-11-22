// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
)

// logs is the log stream: the last 200 lines logged.
type logs struct {
	st    styles
	lines []string
}

// add logs a line, dropping the oldest past 200.
func (l *logs) add(line string) {
	l.lines = append(l.lines, line)
	if len(l.lines) > 200 {
		l.lines = l.lines[len(l.lines)-200:]
	}
}

// Layout is the latest lines of the log, kept at the bottom of the panel.
func (l *logs) Layout(bloom.Context) bloom.Child {
	recent := l.lines[max(len(l.lines)-60, 0):]
	return bloom.Text(strings.Join(recent, "\n")).Style(l.st.logText)
}
