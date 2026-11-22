// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
)

// panelID names the panel, which the spotlight backdrop lights.
const panelID bloom.ID = "panel"

// panel is the card in the middle of the screen, with a row of tabs for the
// screen's backdrops and one for the dialog's, and what the screen's
// backdrop does.
type panel struct {
	st     *styles
	screen int // index into screens
	backed int // index into dialogs

	pickScreen func(i int) tea.Cmd // switches the screen's backdrop to i
	pickDialog func(i int) tea.Cmd // opens the dialog with backdrop i
}

func (p panel) Layout(bloom.Context) bloom.Child {
	return bloom.Box(p.st.card, bloom.Column(
		bloom.Text("bloom · backdrops").Style(p.st.title),
		bloom.Text(" ").Style(p.st.text),
		p.st.tabs("Screen", screens, p.screen, p.st.selected, p.pickScreen),
		p.st.tabs("Dialog", dialogs, p.backed, p.st.picked, p.pickDialog),
		bloom.Text(" ").Style(p.st.text),
		bloom.Text(screens[p.screen].about).Style(p.st.text),
		bloom.Text(" ").Style(p.st.text),
		bloom.Text("click anywhere to drop a ripple, or a dialog backdrop to open it").Style(p.st.dim),
	)).ID(panelID)
}
