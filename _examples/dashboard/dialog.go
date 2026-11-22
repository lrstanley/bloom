// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/flex"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// confirm is the dialog that asks before restarting a service, which slides
// up over a dimmed backdrop.
type confirm struct {
	st      styles
	shown   anim.Presence
	restart bool // the dialog's selected button

	// close closes the dialog, restarting the service if restart is true.
	close func(restart bool) tea.Cmd
}

// open shows the dialog, with restart selected.
func (c *confirm) open() tea.Cmd {
	c.restart = true
	return c.shown.Show()
}

// key handles a key while the dialog is shown.
func (c *confirm) key(k string) tea.Cmd {
	switch k {
	case "left", "right", "tab":
		c.restart = !c.restart
	case "enter":
		return c.close(c.restart)
	case "esc":
		return c.close(false)
	}
	return nil
}

// Layer is the dialog, asking whether to restart service.
func (c *confirm) Layer(service string) bloom.LayerNode {
	restart, cancel := c.st.button, c.st.active
	if c.restart {
		restart, cancel = c.st.active, c.st.button
	}

	body := bloom.Column(
		bloom.Text("Restart "+service+"?").Style(c.st.dialogTitle),
		bloom.Text("In-flight requests will be drained first.").Style(c.st.dialogText),
		bloom.Fixed(1, nil),
		bloom.Row(
			bloom.Text("restart").Style(restart).On(on.Click(func() tea.Cmd { return c.close(true) })),
			bloom.Text("cancel").Style(cancel).On(on.Click(func() tea.Cmd { return c.close(false) })),
		).Gap(2).Justify(flex.End),
	)

	return bloom.Layer(bloom.Box(c.st.dialog, body)).
		ID(paneDialog).
		Modal().
		On(on.ClickOutside(func() tea.Cmd { return c.close(false) })).
		Backdrop(fx.Dim{Amount: 0.55}, fx.Desaturate{Amount: 0.7}).
		Shadow(2, 1, fx.Dim{Amount: 0.6}).
		Presence(c.shown, fx.Slide{From: fx.Bottom}, fx.Fade{})
}
