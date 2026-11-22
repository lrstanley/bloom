// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// toastEffects are what the toast shows for keys 1-6.
var toastEffects = []struct {
	name string
	fx   []fx.Effect
}{
	{"fade", []fx.Effect{fx.Fade{}}},
	{"slide", []fx.Effect{fx.Slide{From: fx.Right}}},
	{"wipe", []fx.Effect{fx.Wipe{From: fx.Left}}},
	{"dissolve", []fx.Effect{fx.Dissolve{Seed: 7}}},
	{"typewriter", []fx.Effect{fx.Typewriter{}}},
	{"scramble", []fx.Effect{fx.Scramble{Seed: 3}, fx.Fade{}}},
}

// toast is a note in the corner, shown with one of toastEffects.
type toast struct {
	st     styles
	shown  anim.Presence
	effect int // the toast's effect
}

// show shows the toast with effect i of toastEffects for a moment.
func (t *toast) show(i int) tea.Cmd {
	t.effect = i
	return t.shown.ShowFor(1500 * time.Millisecond)
}

// Layer is the toast, in the bottom right corner.
func (t *toast) Layer() bloom.LayerNode {
	e := toastEffects[t.effect]
	text := bloom.Text("✓ " + e.name + "\nan effect on a layer's cells")

	return bloom.Layer(bloom.Box(t.st.toast, text)).
		Anchor(bloom.End, bloom.End).
		Margin(2).
		Presence(t.shown, e.fx...)
}
