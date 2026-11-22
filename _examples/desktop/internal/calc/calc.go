// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package calc is the desktop's calculator app: a display, and a grid of
// keys that take clicks. It is the most self contained of the components:
// its keys change only its own state, so it tells the desktop nothing.
package calc

import (
	"slices"
	"strconv"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// Calc is the calculator. Its keys change it from their handlers, so it is
// placed as a pointer.
type Calc struct {
	st theme.Styles

	display string
	acc     float64
	op      string
	fresh   bool // the next digit starts a new number
}

// New returns a calculator showing 0.
func New() *Calc {
	c := &Calc{}
	c.clear()
	return c
}

// SetStyles sets the styles the calculator is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (c *Calc) SetStyles(st theme.Styles) *Calc {
	c.st = st
	return c
}

// Update takes the styles of a new theme.
func (c *Calc) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		c.st = *msg.Styles
	}
	return nil
}

// keys are the calculator's keys, in rows of four.
var keys = []string{
	"C", "⌫", "%", "÷",
	"7", "8", "9", "×",
	"4", "5", "6", "−",
	"1", "2", "3", "+",
	"±", "0", ".", "=",
}

// ops are the keys that aren't digits.
var ops = []string{"C", "⌫", "%", "÷", "×", "−", "+", "±"}

// Layout is the calculator's display, and a grid of keys that take clicks
// while the window can still be dragged by its title bar.
func (c *Calc) Layout(bloom.Context) bloom.Child {
	grid := make([]bloom.Child, len(keys))
	for i, k := range keys {
		st, hover := c.st.Key, c.st.KeyHover
		switch {
		case k == "=":
			st, hover = c.st.KeyEq, c.st.KeyEqHover
		case slices.Contains(ops, k):
			st = c.st.KeyOp
		}
		grid[i] = bloom.Box(st, bloom.Centered(bloom.Text(k))).HoverStyle(hover).
			On(on.Click(func() tea.Cmd { c.Press(k); return nil }))
	}
	return bloom.Column(
		bloom.Box(c.st.Display, bloom.Row(bloom.Space(), bloom.Auto(bloom.Text(c.display+" "+c.op)))),
		bloom.Fill(bloom.Grid(4, grid...).Gap(1, 1)),
	).Gap(1)
}

// clear puts the calculator back to 0.
func (c *Calc) clear() {
	c.display, c.acc, c.op, c.fresh = "0", 0, "", true
}

// Press presses key k of the calculator.
func (c *Calc) Press(k string) {
	switch k {
	case "C":
		c.clear()
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

func (c *Calc) value() float64 {
	v, _ := strconv.ParseFloat(c.display, 64)
	return v
}

func (c *Calc) set(v float64) {
	c.display = strconv.FormatFloat(v, 'g', 12, 64)
}

// apply runs the operator waiting for the number just typed.
func (c *Calc) apply() {
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
			c.clear()
			c.display = "can't ÷ 0"
			return
		}
		c.acc /= v
	default:
		c.acc = v
	}
	c.set(c.acc)
}
