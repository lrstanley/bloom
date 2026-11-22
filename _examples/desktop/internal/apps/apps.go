// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package apps has the desktop's apps, what their windows show, and the
// desktop's icons to launch them from. The apps don't open windows or
// change the theme themselves: they send a message, such as [OpenMsg] or a
// [theme.ChangeMsg], for the desktop to handle, so that each stays a
// component of its own.
package apps

import "image"

// App is an app a window can show.
type App int

const (
	Files App = iota
	Notes
	Calc
	Settings
)

// All are the apps, in the order the icons and the start menu list them.
var All = []App{Files, Notes, Calc, Settings}

// spec is what an app is: its name and its icon, on its title bar, the
// taskbar, and the desktop, where its window first opens, and the smallest
// its window can be resized to.
type spec struct {
	name  string
	glyph string
	place image.Rectangle
	min   image.Point
}

var specs = [...]spec{
	Files:    {"Files", "▤", image.Rect(14, 2, 46, 13), image.Pt(24, 6)},
	Notes:    {"Notes", "✎", image.Rect(34, 5, 80, 21), image.Pt(24, 6)},
	Calc:     {"Calculator", "▦", image.Rect(66, 1, 97, 14), image.Pt(27, 13)},
	Settings: {"Settings", "⚙", image.Rect(24, 2, 56, 17), image.Pt(26, 8)},
}

// Name returns the app's name.
func (a App) Name() string { return specs[a].name }

// Glyph returns the app's icon.
func (a App) Glyph() string { return specs[a].glyph }

// Place returns where the app's window first opens.
func (a App) Place() image.Rectangle { return specs[a].place }

// Min returns the smallest the app's window can be resized to.
func (a App) Min() image.Point { return specs[a].min }

// LaunchMsg launches an app, from its icon or the start menu.
type LaunchMsg struct{ App App }

// OpenMsg opens a file in a notes window of its own.
type OpenMsg struct{ File string }
