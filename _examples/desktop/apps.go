// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// app is what a window shows.
type app int

const (
	appFiles app = iota
	appNotes
	appCalc
	appSettings
)

var apps = []app{appFiles, appNotes, appCalc, appSettings}

// spec is what an app is: its name and its icon, on its title bar, the
// taskbar, and the desktop, where its window first opens, and the smallest
// its window can be resized to.
type spec struct {
	name, glyph string
	place       image.Rectangle
	min         image.Point
}

var specs = [...]spec{
	appFiles:    {"Files", "▤", image.Rect(14, 2, 46, 13), image.Pt(24, 6)},
	appNotes:    {"Notes", "✎", image.Rect(34, 5, 80, 21), image.Pt(24, 6)},
	appCalc:     {"Calculator", "▦", image.Rect(66, 1, 97, 14), image.Pt(27, 13)},
	appSettings: {"Settings", "⚙", image.Rect(24, 2, 56, 17), image.Pt(26, 8)},
}

func (a app) name() string           { return specs[a].name }
func (a app) glyph() string          { return specs[a].glyph }
func (a app) place() image.Rectangle { return specs[a].place }

// fit returns r grown to the app's smallest size, from its top left corner.
func (a app) fit(r image.Rectangle) image.Rectangle {
	s := specs[a].min
	r.Max = image.Pt(max(r.Max.X, r.Min.X+s.X), max(r.Max.Y, r.Min.Y+s.Y))
	return r
}

// content returns what window w shows inside its frame.
func (m *model) content(w *window) bloom.Child {
	switch w.app {
	case appFiles:
		return m.fileList(w)
	case appNotes:
		return bloom.Viewport(w.text).ID(textID(w.id)).At(w.at).
			Wrap(bloom.WrapWords).Style(body).
			OnScroll(func(p bloom.ViewportPos) tea.Cmd { w.at = p; return nil })
	case appCalc:
		return m.calculator()
	case appSettings:
		return m.settings()
	}
	return bloom.Space()
}

// names are the files in the files window, in order.
var names = func() []string {
	n := make([]string, 0, len(files))
	for f := range files {
		n = append(n, f)
	}
	slices.Sort(n)
	return n
}()

// fileList is the files window: a click selects a file, and a double click
// opens it in a notes window of its own.
func (m *model) fileList(w *window) bloom.Child {
	rows := make([]bloom.Child, 0, len(names)+2)
	rows = append(rows, bloom.Text(fmt.Sprintf("%d files, double click to open", len(names))).Style(faint).Truncate())
	for i, f := range names {
		st := row
		if i == w.selected {
			st = rowOn
		}
		label := fmt.Sprintf("%s %-12s %3d lines", appNotes.glyph(), f, len(files[f]))
		rows = append(rows, bloom.Text(label).Style(st).HoverStyle(rowHover).Truncate().On(
			on.Click(func() tea.Cmd { w.selected = i; return nil }),
			on.DoubleClick(func() tea.Cmd { return m.open(appNotes, f) }),
		))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}

// themes are the tints the settings window offers: old screens and the
// neon of the eighties.
var themes = []*tint.Tint{
	tint.TintRosePineMoon, tint.TintAmberCrtRetro, tint.TintGreenPhosphorCrt, tint.TintIbm5153Cga,
	tint.TintVacuumTube, tint.TintTerminalGreen1999, tint.TintOutrunElectric, tint.TintCursedVhs,
	tint.TintCyberdyne, tint.TintMiamiHeat,
}

// settings is the settings window: a click on a theme changes the desktop's
// colors to it.
func (m *model) settings() bloom.Child {
	rows := make([]bloom.Child, 0, len(themes)+2)
	rows = append(rows, bloom.Text("Theme").Style(faint))
	for _, t := range themes {
		st, mark := row, "  "
		if t == theme {
			st, mark = rowOn, "✓ "
		}
		rows = append(rows, bloom.Text(mark+t.DisplayName).Style(st).HoverStyle(rowHover).Truncate().
			On(on.Click(func() tea.Cmd { return m.setTint(t) })))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}

// calc is the calculator's state.
type calc struct {
	display string
	acc     float64
	op      string
	fresh   bool // the next digit starts a new number
}

func newCalc() calc { return calc{display: "0", fresh: true} }

// calcKeys are the calculator's keys, in rows of four.
var calcKeys = []string{
	"C", "⌫", "%", "÷",
	"7", "8", "9", "×",
	"4", "5", "6", "−",
	"1", "2", "3", "+",
	"±", "0", ".", "=",
}

// calcOps are the keys that aren't digits.
var calcOps = []string{"C", "⌫", "%", "÷", "×", "−", "+", "±"}

// press presses key k of the calculator.
func (c *calc) press(k string) {
	switch k {
	case "C":
		*c = newCalc()
	case "⌫":
		if !c.fresh && len(c.display) > 1 {
			c.display = c.display[:len(c.display)-1]
		} else {
			c.display, c.fresh = "0", true
		}
	case "%":
		c.set(c.value() / 100)
	case "±":
		c.set(-c.value())
	case "+", "−", "×", "÷":
		c.apply()
		c.op, c.fresh = k, true
	case "=":
		c.apply()
		c.op, c.fresh = "", true
	case ".":
		switch {
		case c.fresh:
			c.display, c.fresh = "0.", false
		case !strings.Contains(c.display, "."):
			c.display += "."
		}
	default:
		switch {
		case c.fresh || c.display == "0":
			c.display, c.fresh = k, false
		case len(c.display) < 14:
			c.display += k
		}
	}
}

func (c *calc) value() float64 {
	v, _ := strconv.ParseFloat(c.display, 64)
	return v
}

func (c *calc) set(v float64) {
	c.display = strconv.FormatFloat(v, 'g', 12, 64)
}

// apply runs the operator waiting for the number just typed.
func (c *calc) apply() {
	if c.fresh && c.op != "" {
		return // another operator replaces the last
	}
	v := c.value()
	switch c.op {
	case "+":
		c.acc += v
	case "−":
		c.acc -= v
	case "×":
		c.acc *= v
	case "÷":
		if v == 0 {
			*c = newCalc()
			c.display = "can't ÷ 0"
			return
		}
		c.acc /= v
	default:
		c.acc = v
	}
	c.set(c.acc)
}

// calculator is the calculator window: its display, and a grid of keys that
// take clicks while the window can still be dragged by its title bar.
func (m *model) calculator() bloom.Child {
	keys := make([]bloom.Child, len(calcKeys))
	for i, k := range calcKeys {
		st, hover := key, keyHover
		switch {
		case k == "=":
			st, hover = keyEq, keyEqHover
		case slices.Contains(calcOps, k):
			st = keyOp
		}
		keys[i] = bloom.Box(st, bloom.Centered(bloom.Text(k))).HoverStyle(hover).
			On(on.Click(func() tea.Cmd { m.calc.press(k); return nil }))
	}
	return bloom.Column(
		bloom.Box(display, bloom.Row(bloom.Space(), bloom.Auto(bloom.Text(m.calc.display+" "+m.calc.op)))),
		bloom.Fill(bloom.Grid(4, keys...).Gap(1, 1)),
	).Gap(1)
}

// files are the text of each file the files window lists, a line each.
var files = map[string][]string{
	"README.md": {
		"# desktop",
		"",
		"Every window here is a layer of one bloom overlay. Its place and size come from a rectangle in the model, given to the layer with Place, so a window stays wherever it is dropped: the tree is built again from the model on every frame, and nothing else remembers where anything is.",
		"",
		"Drag a window by its title bar to move it. Drag it against the left or right edge of the screen to snap it to that half, or against the top to fill the desktop. Drag the ◢ in its corner to resize it, and double click its title to maximize it.",
		"",
		"A click anywhere on a window brings it to the front, and the order of the layers is the order the windows stack in. The taskbar shows every window: click one to bring it forward, or the one in front to minimize it.",
		"",
		"Click ❀ bloom for the start menu, to open apps or log out. Settings changes the theme.",
		"",
		"The wheel scrolls this text, and the scrollbar on the right edge can be dragged.",
	},
	"todo.txt": {
		"[x] drag windows by their title bars",
		"[x] keep them where they are dropped",
		"[x] resize from the corner",
		"[x] snap to the edges of the screen",
		"[x] minimize to the taskbar",
		"[ ] water the plants",
		"[ ] write a window manager in a terminal",
	},
	"haiku.txt": {
		"windows drift and stack",
		"a title bar, held and moved",
		"the cells remember",
		"",
		"dropped against the edge",
		"half the screen is suddenly",
		"all that it needs",
	},
	"lorem.txt": strings.Split(strings.Repeat(
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.\n", 24), "\n"),
}
