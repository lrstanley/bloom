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

// toast says the profile was saved, in the bottom right corner, and hides
// itself after a moment.
type toast struct {
	st       styles
	presence anim.Presence
}

// show shows the toast for two seconds.
func (t *toast) show() tea.Cmd { return t.presence.ShowFor(2 * time.Second) }

// Layer is the toast's layer, which types itself in as it fades in.
func (t *toast) Layer() bloom.LayerNode {
	return bloom.Layer(bloom.Box(t.st.toastBox, bloom.Text("✓ Profile saved"))).
		Anchor(bloom.End, bloom.End).
		Margin(1).
		Presence(t.presence, fx.Typewriter{}, fx.Fade{})
}
