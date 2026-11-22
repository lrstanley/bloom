// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// The IDs of the toolbar buttons, which focus moves between.
const (
	actionRefresh  bloom.ID = "refresh"
	actionDeploy   bloom.ID = "deploy"
	actionRollback bloom.ID = "rollback"
	actionStop     bloom.ID = "stop"
	actionSettings bloom.ID = "settings"
)

// toolButton is a button of the toolbar: its icon, and its tooltip.
type toolButton struct {
	id   bloom.ID
	icon string
	tip  string
	opts []bloom.TipOption
}

var tools = []toolButton{
	{id: actionRefresh, icon: "⟳", tip: "Refresh"},
	{id: actionDeploy, icon: "▶", tip: "Deploy"},
	{id: actionRollback, icon: "↶", tip: "Roll back"},
	{id: actionStop, icon: "■", tip: "Stop"},
	{id: actionSettings, icon: "⚙", tip: "Settings", opts: []bloom.TipOption{bloom.TipSide(bloom.Right)}},
}

// toolbar is the line at the top: the title, the buttons, and a note of what
// was done last. The model builds it each frame.
type toolbar struct {
	st    *styles
	focus *bloom.Focus // the model's, which moves along the buttons
	note  string
	press func(id bloom.ID) tea.Cmd // runs the button with ID id
}

func (b toolbar) Layout(bloom.Context) bloom.Child {
	bar := []bloom.Child{bloom.Text("Deployments").Style(b.st.title), bloom.Fixed(1, nil)}
	for _, t := range tools {
		bar = append(bar, b.button(t))
	}
	bar = append(bar, bloom.Space(), bloom.Text(b.note).Style(b.st.label))
	return bloom.Row(bar...).Gap(1)
}

// button is a button of the toolbar, an icon with a tooltip.
func (b toolbar) button(t toolButton) bloom.Child {
	st := b.st.tool
	if b.focus.Is(t.id) {
		st = b.st.toolOn
	}
	return bloom.Text(t.icon).Style(st).HoverStyle(b.st.toolHover).
		ID(t.id).Focusable().
		On(on.Press(func() tea.Cmd { return b.press(t.id) })).
		Tooltip(t.tip, t.opts...)
}
