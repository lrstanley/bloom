// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// banner is an alert that slides in at the top right, holds, and slides out
// again.
type banner struct {
	st       styles
	presence anim.Presence // the banner, shown for a while
	alerts   int           // banners shown
}

// newBanner makes the banner, hidden.
func newBanner() banner {
	return banner{
		presence: anim.NewPresence(
			anim.Duration(400*time.Millisecond),
			anim.Ease(anim.OutCubic),
			anim.FPS(60),
		),
	}
}

// setStyles sets the styles the banner is drawn with, of which it keeps a copy.
func (b *banner) setStyles(st styles) *banner {
	b.st = st
	return b
}

// show shows the next alert for a while.
func (b *banner) show() tea.Cmd {
	b.alerts++
	return b.presence.ShowFor(2400 * time.Millisecond)
}

// Layer is the banner's layer, with an alert about the stock named name.
func (b *banner) Layer(name string) bloom.LayerNode {
	alert := fmt.Sprintf("◆ Alert %d: %s", b.alerts, name)
	return bloom.Layer(bloom.Box(b.st.banner, bloom.Text(alert).Style(b.st.onPanel))).
		Anchor(bloom.End, bloom.Start).
		Offset(0, 3).
		Margin(1).
		Presence(b.presence, fx.Slide{From: fx.Right}, fx.Fade{})
}
