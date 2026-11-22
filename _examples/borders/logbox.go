// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
)

// logBox is a log of long lines, which scroll sideways rather than wrap,
// with a horizontal scrollbar on its bottom edge and its name down its side.
type logBox struct {
	st    styles
	lines *bloom.Buffer
	at    bloom.ViewportPos
}

// key scrolls the log sideways with left and right, from where it was in
// frame f.
func (l *logBox) key(msg tea.KeyPressMsg, f *bloom.Frame) {
	log, _ := f.Viewport(paneLog)
	switch msg.String() {
	case "left":
		l.at = log.Left(4)
	case "right":
		l.at = log.Right(4)
	}
}

func (l *logBox) Layout(bloom.Context) bloom.Child {
	return bloom.Box(l.st.pane, bloom.Viewport(l.lines).ID(paneLog).At(l.at).Style(l.st.text).
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { l.at = p; return nil })).
		Edge(bloom.Left, bloom.Center, bloom.Text("LOG").Style(l.st.side)).
		Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneLog).Horizontal().
			Style(l.st.scrollbar).HoverStyle(l.st.onBar))
}
