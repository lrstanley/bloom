// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// toast is a FIGlet toast over the panels, shown for a while after a
// deploy, sliding in from the top: a transparent layer, so the panels show
// through between its strokes, or an opaque one.
type toast struct {
	st       styles
	presence anim.Presence
	opaque   bool
}

// newToast makes the toast, hidden.
func newToast() toast {
	return toast{presence: anim.NewPresence(anim.Duration(450*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60))}
}

// setStyles sets the styles the toast is drawn with, of which it keeps a copy.
func (t *toast) setStyles(st styles) *toast {
	t.st = st
	return t
}

// show shows the toast for a while.
func (t *toast) show() tea.Cmd { return t.presence.ShowFor(2200 * time.Millisecond) }

// Layer is the toast's layer, over the screen.
func (t *toast) Layer() bloom.LayerNode {
	// The toast dims the screen beneath it, which shows through the blanks
	// between its strokes unless it is opaque.
	text := fig.Text("Deployed!").Font(fonts.Doom(), fonts.Small(), fonts.Mini()).Paint(t.st.toastPaint)
	layer := bloom.Layer(text).
		Anchor(bloom.Center, bloom.Center).
		Backdrop(fx.Dim{Amount: 0.7}).
		Presence(t.presence, fx.Slide{From: fx.Top})
	if !t.opaque {
		layer = layer.Transparent()
	}
	return layer
}

// kind says whether the toast is opaque or transparent.
func (t *toast) kind() string {
	if t.opaque {
		return "opaque"
	}
	return "transparent"
}
