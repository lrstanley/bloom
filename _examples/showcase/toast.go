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

// toast is a note that slides in from the right, decoding its text.
type toast struct {
	st    styles
	shown anim.Presence
	text  anim.Value // decodes the toast's text.
	note  string
	notes uint32 // how many toasts there have been, to vary the decode.
}

// Update moves the toast's animations along, and takes the new styles of a
// themeMsg.
func (t *toast) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(themeMsg); ok {
		t.st = *msg.st
	}
	return anim.Update(msg, &t.shown, &t.text)
}

// notify shows text in the toast, decoding it into place.
func (t *toast) notify(text string) tea.Cmd {
	t.note = text
	t.notes++
	return tea.Batch(t.shown.ShowFor(2*time.Second), t.text.Play(0, 1))
}

// Layer slides the toast in from the right, decoding its text.
func (t *toast) Layer() bloom.LayerNode {
	text := bloom.Transition(
		t.text.Float(),
		bloom.Box(t.st.raised, bloom.Text(t.note).Style(t.st.text)),
		fx.Decode{Seed: t.notes},
	)
	return bloom.Layer(bloom.Box(t.st.toast, text)).
		Anchor(bloom.End, bloom.End).
		Margin(2).
		Presence(t.shown, fx.Slide{From: fx.Right})
}
