// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// menuWidth is the width of a menu and its items.
const menuWidth = 16

// action is an item of a menu, or a rule between items when run is nil.
type action struct {
	label  string
	danger bool
	run    func()
}

var rule = action{}

// menu is the right click menu, a layer that is open while it has items.
type menu struct {
	st    styles
	items []action    // the open menu's items, or none while it is closed
	at    image.Point // where the right button came up for the menu
}

func (mn *menu) open(at image.Point, items ...action) tea.Cmd {
	mn.items, mn.at = items, at
	return nil
}

func (mn *menu) close() tea.Cmd {
	mn.items = nil
	return nil
}

// Layer is the open menu, with its top-left corner where the right
// button came up, unless it would run off the screen, such as after the
// terminal shrinks. The screen is w×h.
func (mn *menu) Layer(w, h int) bloom.LayerNode {
	items := make([]bloom.Child, len(mn.items))
	for i, a := range mn.items {
		if a.run == nil {
			items[i] = bloom.Text(strings.Repeat("─", menuWidth-2)).Style(mn.st.menuRule)
			continue
		}
		st, hover := mn.st.menuItem, mn.st.menuHover
		if a.danger {
			st, hover = mn.st.menuDanger, mn.st.menuDangerHover
		}
		items[i] = bloom.Text(a.label).Style(st).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			a.run()
			return mn.close()
		}))
	}

	mw, mh := menuWidth, len(mn.items)
	at := mn.at
	if at.X+mw > w {
		at.X -= mw - 1
	}
	if at.Y+mh > h {
		at.Y -= mh - 1
	}
	return bloom.Layer(bloom.Column(items...)).
		Anchor(bloom.Start, bloom.Start).
		Offset(max(min(at.X, w-mw), 0), max(min(at.Y, h-mh), 0)).
		Shadow(1, 1, fx.Dim{Amount: 0.6}).
		On(on.ClickOutside(mn.close))
}
