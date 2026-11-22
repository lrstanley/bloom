// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// tourSteps are what the tour says about each card.
var tourSteps = [...][2]string{
	{"Traffic", "Requests served each minute, live."},
	{"Latency", "How long the slowest 5% of requests take."},
	{"Errors", "Failed requests in the past hour."},
}

// tour spotlights each card in turn, with a note that decodes into place.
type tour struct {
	st styles

	step     int // the tour step: 0 is off, then 1 for the first card.
	presence anim.Presence
	text     anim.Value
}

// next moves the tour on to the next card, or ends it after the last.
func (t *tour) next() tea.Cmd {
	t.step++
	if t.step > len(cards) {
		return t.end()
	}
	return tea.Batch(t.presence.Show(), t.text.Play(0, 1))
}

// end ends the tour, and fades its note out.
func (t *tour) end() tea.Cmd {
	t.step = 0
	return t.presence.Hide()
}

// Layer is the tour's: a note at the bottom, over a backdrop that leaves
// only the card it explains lit, which it finds in frame.
func (t *tour) Layer(frame *bloom.Frame) bloom.LayerNode {
	// While the last note animates out, it stays on the last card.
	step := min(max(t.step, 1), len(cards))
	title, text := tourSteps[step-1][0], tourSteps[step-1][1]

	note := bloom.Column(
		bloom.Text(title).Style(t.st.heading),
		bloom.Text(text),
	)
	decode := fx.Decode{Seed: uint32(step)} //nolint:gosec // step is small.

	l := bloom.Layer(bloom.Box(t.st.tour, bloom.Transition(t.text.Float(), note, decode))).
		Anchor(bloom.Center, bloom.End).
		Margin(2).
		Presence(t.presence, fx.Fade{})
	if r, ok := frame.Rect(cardID.Sub(step - 1)); ok {
		l = l.Backdrop(fx.Spotlight{Rect: r.Inset(-1), Soft: 5})
	}
	return l
}
