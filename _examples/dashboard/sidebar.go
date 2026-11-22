// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// sidebar is the list of services, which can be clicked, and which springs
// open and shut. It keeps which service is selected, which the rest of the
// screen shows.
type sidebar struct {
	st       styles
	open     bool // whether the sidebar is open
	selected int  // the selected service
}

// Layout is the sidebar's list of services.
func (s *sidebar) Layout(bloom.Context) bloom.Child {
	items := make([]bloom.Child, 0, len(services)+1)
	for i, name := range services {
		st := s.st.item
		if i == s.selected {
			st = s.st.selected
		}
		items = append(items, bloom.Text("● "+name).Style(st).Truncate().
			On(on.Click(func() tea.Cmd { s.selected = i; return nil })))
	}
	items = append(items, bloom.Space())
	return bloom.Column(items...)
}
