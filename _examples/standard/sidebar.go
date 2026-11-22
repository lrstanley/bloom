// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
)

// openMsg opens the page at an index of pages.
type openMsg int

// sidebar lists the pages; it is a component with a Layout, so its items
// fill the sidebar's width.
type sidebar struct {
	st       styles
	selected int
}

func (s sidebar) Layout(bloom.Context) bloom.Child {
	items := make([]bloom.Child, 0, len(pages)+2)
	for i, name := range pages {
		st := s.st.item
		if i == s.selected {
			st = s.st.selected
		}
		items = append(items, bloom.Text(name).Style(st).On(on.Click(on.Msg(openMsg(i)))))
	}
	items = append(items, bloom.Space(), bloom.Text("v1.0").Style(s.st.item))
	return bloom.Box(s.st.panel, bloom.Column(items...))
}
