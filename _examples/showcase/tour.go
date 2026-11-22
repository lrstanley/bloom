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

// tourSteps are what the tour says, about the pane it spotlights.
var tourSteps = []struct {
	pane  bloom.ID
	title string
	text  string
}{
	{paneTints, "Tints", "Every color is from a bubbletint theme. Click one to switch."},
	{paneActivity, "Panes", "Drag a title to move a pane. Double click to zoom."},
	{paneServices, "Responsive", "Narrow the terminal and panes collapse, lowest priority first."},
}

// tour spotlights each part of the screen in turn, with a note about it.
type tour struct {
	st    styles
	step  int // the tour step, from 1, kept while the tour fades out.
	shown anim.Presence
	text  anim.Value
}

// Update moves the tour's animations along, and takes the new styles of a
// themeMsg.
func (t *tour) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(themeMsg); ok {
		t.st = *msg.st
	}
	return anim.Update(msg, &t.shown, &t.text)
}

// next steps the tour on, starting it if it is not on, and ending it after
// the last step.
func (t *tour) next() tea.Cmd {
	if !t.shown.Shown() {
		t.step = 0
	}
	t.step++
	if t.step > len(tourSteps) {
		return t.shown.Hide()
	}
	return tea.Batch(t.shown.Show(), t.text.Play(0, 1))
}

// Layer is the tour's: a note at the bottom, over a backdrop that leaves
// only the pane it explains lit, where it is in frame.
func (t *tour) Layer(frame *bloom.Frame) bloom.LayerNode {
	// Past the last step, the last note stays while it fades out.
	step := tourSteps[min(max(t.step, 1), len(tourSteps))-1]
	note := bloom.Column(
		bloom.Text(step.title).Style(t.st.highlight),
		bloom.Text(step.text).Style(t.st.text),
	)
	decode := fx.Decode{Seed: uint32(t.step)} //nolint:gosec // the step is small.

	l := bloom.Layer(bloom.Box(t.st.tour, bloom.Transition(t.text.Float(), bloom.Box(t.st.raised, note), decode))).
		Anchor(bloom.Center, bloom.End).
		Margin(2).
		Presence(t.shown, fx.Fade{})
	if r, ok := frame.Rect(step.pane); ok {
		l = l.Backdrop(fx.Spotlight{Rect: r, Soft: 4})
	}
	return l
}
