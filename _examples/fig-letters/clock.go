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

// start is the time the clock starts at: it rolls over a minute, and an
// hour, soon after.
var start = time.Date(2026, 11, 8, 9, 59, 54, 0, time.UTC)

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// clock is a card with the time in it, whose digits roll over as the
// seconds change: new digits slide in from the top as old ones leave from
// the bottom, and those that stay put do not move.
type clock struct {
	st     styles
	now    time.Time
	paused bool
	morph  fig.Morph
}

// newClock makes a clock at start.
func newClock() clock {
	return clock{
		now:   start,
		morph: fig.NewMorph(start.Format("15:04:05"), anim.Duration(450*time.Millisecond), anim.Ease(anim.OutCubic), anim.FPS(60)),
	}
}

// setStyles sets the styles the clock is drawn with, of which it keeps a copy.
func (c *clock) setStyles(st styles) *clock {
	c.st = st
	return c
}

// advance moves the clock on a second, unless it is paused.
func (c *clock) advance() tea.Cmd {
	if c.paused {
		return nil
	}
	c.now = c.now.Add(time.Second)
	return c.morph.Set(c.now.Format("15:04:05"))
}

func (c *clock) Layout(bloom.Context) bloom.Child {
	digits := c.morph.Text().
		Font(fonts.Big(), fonts.Small()).
		Paint(c.st.clockPaint).
		Key(fig.ByPosition).
		Enter(fx.Slide{From: fx.Top}, fx.Fade{}).
		Leave(fx.Slide{From: fx.Bottom}, fx.Fade{}).
		Spread(0.3, fig.Backward)

	state := c.st.running.Render(" ● running ")
	if c.paused {
		state = c.st.paused.Render(" ❚❚ paused ")
	}
	return c.st.card("clock", bloom.Row(bloom.Fixed(2, bloom.Space()), bloom.Fill(digits))).
		Edge(bloom.Top, bloom.End, bloom.Text(state).Style(c.st.onBg)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(c.now.Format(" Mon 2 Jan 2006 ")).Style(c.st.dim))
}
