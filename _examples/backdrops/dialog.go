// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// dialog is a modal dialog about one of the dialogs' backdrops, which
// changes the screen beneath it as it fades in and out.
type dialog struct {
	st       styles
	presence anim.Presence
}

// Layer is the dialog's layer, about backdrop b, with b's effects as its
// backdrop.
func (d *dialog) Layer(b backdrop, effects ...fx.Effect) bloom.LayerNode {
	// The dialog sits below the panel, so its backdrop shows on the panel.
	return bloom.Layer(bloom.Box(d.st.dialog, bloom.Column(
		bloom.Text("Backdrop: "+b.name).Style(d.st.dialogTitle),
		bloom.Text(" ").Style(d.st.dialogText),
		bloom.Text(b.about).Style(d.st.dialogText),
		bloom.Text(" ").Style(d.st.dialogText),
		bloom.Text("b: next backdrop · esc or d closes it").Style(d.st.dialogDim),
	))).Modal().
		Anchor(bloom.Center, bloom.End).
		Margin(2).
		Backdrop(effects...).
		Presence(d.presence, fx.Fade{}).
		On(on.ClickOutside(func() tea.Cmd { return d.presence.Hide() }))
}
