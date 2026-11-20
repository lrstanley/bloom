// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// startID is the ID of the start button, which the start menu opens above.
type startID struct{}

// menuID is the ID of the start menu, so that a press on any of it, its
// banner too, is on it rather than on the desktop beneath.
type menuID struct{}

// screen is what the terminal shows: the splash while the desktop starts up
// or shuts down, and the desktop, which opens from its middle row and fades
// in after it, like an old CRT turning on, and closes back to it as one
// turning off.
func (m *model) screen() bloom.Child {
	p := m.power.Float()
	if p >= 1 {
		// Once on, the CRT effects would only cost a pass over the screen on
		// every frame. It stays in the Transition, for its nodes to keep
		// their paths.
		var crt []fx.Effect
		if p < poweredOn {
			crt = []fx.Effect{fx.Unfold{}, fx.Fade{}}
		}
		return bloom.Transition(anim.OutCubic(min((p-1)/(poweredOn-1), 1)), m.layout(), crt...)
	}
	const width = 24
	n, word := int(p*width), "starting up…"
	if m.off() {
		word = "shutting down…"
	}
	line := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	return bloom.Centered(bloom.Column(
		bloom.Transition(min(p*3, 1), bloom.Text("❀ bloom").Style(line.Foreground(accent).Bold(true)), fx.Decode{}),
		bloom.Text(""),
		bloom.Text(paint(accent, strings.Repeat("━", n))+paint(muted, strings.Repeat("─", width-n))).Style(line),
		bloom.Text(word).Style(line.Foreground(muted)),
	))
}

// paint returns s in color c.
func paint(c color.Color, s string) string { return lipgloss.NewStyle().Foreground(c).Render(s) }

// off reports whether the desktop is shutting down, once it has started up.
func (m *model) off() bool { return m.power.Target() == 0 }

// logout shuts the desktop down: it shades out to the splash, which runs
// back down, and the program quits once it has.
func (m *model) logout() tea.Cmd {
	if m.off() {
		return nil
	}
	return tea.Batch(m.menu.Hide(), m.power.To(0))
}

// launch opens app a, and the README for notes.
func (m *model) launch(a app) tea.Cmd {
	if a == appNotes {
		return m.open(a, "README.md")
	}
	return m.open(a, "")
}

// startMenu is the start button, which opens the start menu above it: the
// apps, and logging out, beside a banner, as start menus were.
func (m *model) startMenu() bloom.Child {
	const width = 18
	item := func(label string, fn func() tea.Cmd) bloom.Child {
		return bloom.Text(" " + label).Style(menuItem.Width(width)).HoverStyle(rowOn.Width(width)).
			On(on.Click(func() tea.Cmd { return tea.Batch(m.menu.Hide(), fn()) }))
	}
	rows := make([]bloom.Child, 0, len(apps)+4)
	blank := bloom.Text(" ").Style(menuItem.Width(width))
	rows = append(rows, blank)
	for _, a := range apps {
		rows = append(rows, item(a.glyph()+"  "+a.name(), func() tea.Cmd { return m.launch(a) }))
	}
	rows = append(rows,
		bloom.Text(strings.Repeat("─", width)).Style(menuItem.Foreground(muted)),
		item("⏻  Log out", m.logout),
		blank,
	)
	panel := bloom.Row(
		bloom.Fixed(3, bloom.Box(banner, bloom.Column(bloom.Space(), bloom.Text("b\nl\no\no\nm\n").Style(banner.Width(3).Align(lipgloss.Center))))),
		bloom.Auto(bloom.Column(rows...)),
	)
	menu := bloom.Layer(panel).ID(menuID{}).AnchorTo(startID{}, bloom.Top).Float().
		Presence(m.menu, fx.Wipe{From: bloom.Bottom})

	st := start
	if m.menu.Shown() {
		st = startOn
	}
	// The menu is declared in the button, so a click on either isn't outside
	// it, and a click anywhere else closes it.
	return bloom.Overlay(bloom.Text(" ❀ bloom ").Style(st).ID(startID{}).On(on.Click(m.menu.Toggle)), menu).
		On(on.ClickOutside(func() tea.Cmd {
			if m.menu.Shown() {
				return m.menu.Hide()
			}
			return nil
		}))
}

// setTint changes the desktop's theme to t. The root is told its colors
// by a themeMsg, outside the click that changed it, which runs within the
// root, and whichever theme is current then.
func (m *model) setTint(t *tint.Tint) tea.Cmd {
	setTheme(t)
	m.status = "changed the theme to " + t.DisplayName
	return func() tea.Msg { return themeMsg{} }
}
