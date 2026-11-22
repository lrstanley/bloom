// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// filters are the tabs over the list, each a prefix of the paths it shows.
var filters = []string{"all", "kv", "pki", "db"}

// statusMsg is what the status line says, sent by a click on one of the
// list's icons.
type statusMsg string

// secrets is the scrolled list of paths, with its title, filter tabs, and
// position on its border, icons down its left side, and a scrollbar on its
// right side that can be dragged.
type secrets struct {
	st     styles
	phase  *anim.Sequence // the loop its border effect runs on, which the model keeps
	filter int
	paths  []string
	listed *bloom.Buffer // the paths the filter shows
	at     bloom.ViewportPos
}

// key scrolls the list with up and down, from where it was in frame f, and
// changes the filter with tab.
func (s *secrets) key(msg tea.KeyPressMsg, f *bloom.Frame) {
	list, _ := f.Viewport(paneList)
	switch msg.String() {
	case "up":
		s.at = list.Up(1)
	case "down":
		s.at = list.Down(1)
	case "tab":
		s.setFilter((s.filter + 1) % len(filters))
	}
}

// shown returns the paths the filter shows.
func (s *secrets) shown() []string {
	if s.filter == 0 {
		return s.paths
	}
	var out []string
	for _, p := range s.paths {
		if strings.HasPrefix(p, filters[s.filter]+"/") {
			out = append(out, p)
		}
	}
	return out
}

func (s *secrets) setFilter(i int) {
	s.filter, s.at = i, bloom.ViewportPos{}
	s.listed.SetLines(s.shown()...)
}

func (s *secrets) Layout(ctx bloom.Context) bloom.Child {
	// The position is where the list is now, a page as tall as it was in
	// the last frame, which the frame itself is a step behind while nothing
	// else draws a new one.
	v, _ := ctx.Frame().Viewport(paneList)
	n, rows := s.listed.Len(), max(v.Rows, 1)
	first := min(max(s.at.Line, 0), max(n-rows, 0))
	// The scrollbar drags the list with the list's OnScroll.
	box := bloom.Box(s.st.pane, bloom.Viewport(s.listed).ID(paneList).At(s.at).Style(s.st.text).
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { s.at = p; return nil })).
		Edge(bloom.Top, bloom.Start, bloom.Text("◆ secrets").Style(s.st.title)).
		Edge(bloom.Top, bloom.End, s.tabs()).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf("%d-%d of %d",
			min(first+1, n), min(first+rows, n), n)).Style(s.st.label)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneList).
			Style(s.st.scrollbar).HoverStyle(s.st.onBar))
	// Single glyphs down the side, each one a button.
	for _, a := range sideButtons {
		box = box.Edge(bloom.Left, bloom.Start, bloom.Text(a.glyph).Style(s.st.icon).HoverStyle(s.st.onTip).
			On(on.Click(on.Msg(statusMsg("clicked "+a.glyph+": "+a.does)))))
	}
	return box.BorderFx(s.phase.Float(), fx.Chase{From: s.st.accent, To: s.st.pink})
}

// tabs are the filters, which can be clicked.
func (s *secrets) tabs() bloom.Child {
	kids := make([]bloom.Child, 0, len(filters))
	for i, f := range filters {
		st := s.st.tab
		if i == s.filter {
			st = s.st.onTab
		}
		kids = append(kids, bloom.Text(f).Style(st).HoverStyle(s.st.onTab).
			On(on.Click(func() tea.Cmd { s.setFilter(i); return nil })))
	}
	return bloom.Row(kids...).Gap(1)
}

// sideButtons are the glyphs down the side of the list, and what each does.
var sideButtons = []struct {
	glyph string
	does  string
}{{"+", "new secret"}, {"✎", "edit"}, {"⧉", "copy path"}, {"✕", "delete"}}
