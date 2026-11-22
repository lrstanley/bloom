// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package startmenu is the start button on the desktop's taskbar, and the
// menu it opens above it: the apps, and logging out, beside a banner, as
// start menus were. An item hides the menu and sends a message, such as
// [apps.LaunchMsg], for the desktop to handle.
package startmenu

import (
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/apps"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// LogoutMsg logs out, which shuts the desktop down.
type LogoutMsg struct{}

// startID is the ID of the start button, which the start menu opens above.
type startID struct{}

// menuID is the ID of the start menu, so that a press on any of it, its
// banner too, is on it rather than on the desktop beneath.
type menuID struct{}

// Menu is the start button and its menu.
type Menu struct {
	st   theme.Styles
	open anim.Presence // the menu
}

// New returns the start button, with its menu closed.
func New() *Menu {
	return &Menu{open: anim.NewPresence(anim.Duration(150 * time.Millisecond))}
}

// SetStyles sets the styles the start menu is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (m *Menu) SetStyles(st theme.Styles) *Menu {
	m.st = st
	return m
}

// Update moves the menu's animation along, and takes the styles of a new
// theme.
func (m *Menu) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		m.st = *msg.Styles
	}
	return m.open.Update(msg)
}

// Hide closes the menu.
func (m *Menu) Hide() tea.Cmd { return m.open.Hide() }

func (m *Menu) Layout(bloom.Context) bloom.Child {
	menu := bloom.Layer(m.panel()).ID(menuID{}).AnchorTo(startID{}, bloom.Top).Float().
		Presence(m.open, fx.Wipe{From: bloom.Bottom})

	st := m.st.Start
	if m.open.Shown() {
		st = m.st.StartOn
	}
	// The menu is declared in the button, so a click on either isn't outside
	// it, and a click anywhere else closes it.
	return bloom.Overlay(bloom.Text(" ❀ bloom ").Style(st).ID(startID{}).On(on.Click(m.open.Toggle)), menu).
		On(on.ClickOutside(func() tea.Cmd {
			if m.open.Shown() {
				return m.open.Hide()
			}
			return nil
		}))
}

// panel is the menu itself: the banner, and the items beside it.
func (m *Menu) panel() bloom.Child {
	rows := make([]bloom.Child, 0, len(apps.All)+4)
	blank := bloom.Text(" ").Style(m.st.MenuItem)
	rows = append(rows, blank)
	for _, a := range apps.All {
		rows = append(rows, m.item(a.Glyph()+"  "+a.Name(), apps.LaunchMsg{App: a}))
	}
	rows = append(rows,
		bloom.Text(strings.Repeat("─", theme.MenuWidth)).Style(m.st.MenuRule),
		m.item("⏻  Log out", LogoutMsg{}),
		blank,
	)
	return bloom.Row(
		bloom.Fixed(3, bloom.Box(m.st.Banner, bloom.Column(bloom.Space(), bloom.Text("b\nl\no\no\nm\n").Style(m.st.BannerText)))),
		bloom.Auto(bloom.Column(rows...)),
	)
}

// item is an item of the menu, which closes it and sends msg.
func (m *Menu) item(label string, msg tea.Msg) bloom.Child {
	return bloom.Text(" " + label).Style(m.st.MenuItem).HoverStyle(m.st.MenuHover).
		On(on.Click(func() tea.Cmd { return tea.Batch(m.open.Hide(), func() tea.Msg { return msg }) }))
}
