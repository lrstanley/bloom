// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const service = "payments"

// form is the deploy form, where the service's name is typed in to confirm,
// and what deploying sets off: the confetti, and the LIVE badge.
type form struct {
	st    styles
	focus *bloom.Focus // the model's, which moves between the input and the button

	text  string // what was typed into the input.
	err   string
	shake anim.Value

	deployed bool
	burst    anim.Value
	badge    anim.Value
	bursts   uint32
}

// key deploys with enter, starts over with ctrl+r, and edits the input.
func (f *form) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		return f.deploy()
	case "ctrl+r":
		f.text, f.err, f.deployed = "", "", false
		f.focus.Set(inputConfirm)
		f.burst.Set(1)
		f.badge.Set(1)
	case "backspace":
		if f.focus.Is(inputConfirm) && !f.deployed {
			r := []rune(f.text)
			f.text = string(r[:max(len(r)-1, 0)])
		}
	default:
		if f.focus.Is(inputConfirm) && !f.deployed && msg.Text != "" {
			f.text += msg.Text
			f.err = ""
		}
	}
	return nil
}

// deploy deploys the service if its name was typed in, and shakes the input
// if not.
func (f *form) deploy() tea.Cmd {
	if f.deployed {
		return nil
	}
	if f.text != service {
		f.err = fmt.Sprintf("✗ type %q to confirm", service)
		return f.shake.Play(0, 1)
	}
	f.deployed, f.err = true, ""
	f.bursts++
	return tea.Batch(f.burst.Play(0, 1), f.badge.Play(0, 1))
}

func (f *form) Layout(bloom.Context) bloom.Child {
	text, st := f.text, f.st.input
	if text == "" {
		text, st = service, f.st.hint
	}
	in := bloom.Text(text).Style(st).ID(inputConfirm).Focusable()
	if f.focus.Is(inputConfirm) {
		if f.text == "" {
			st = f.st.inputOnHint
		} else {
			st = f.st.inputOn
		}
		in = in.Style(st).Cursor(tea.NewCursor(1+ansi.StringWidth(f.text), 0))
	}
	field := bloom.Child(in)
	if f.shake.Animating() {
		field = bloom.Transition(f.shake.Float(), field, fx.Shake{Amount: 3, Times: 3})
	}

	btn, hover := f.st.button, f.st.buttonHover
	if f.focus.Is(buttonDeploy) {
		btn, hover = f.st.buttonOn, f.st.buttonOnHover
	}
	msg := bloom.Text(" ")
	switch {
	case f.deployed:
		msg = bloom.Text("✓ " + service + " is live").Style(f.st.success)
	case f.err != "":
		msg = bloom.Text(f.err).Style(f.st.failure)
	}
	deploy := bloom.Text("deploy").Style(btn).HoverStyle(hover).ID(buttonDeploy).Focusable().On(on.Press(f.deploy))

	return bloom.Column(
		bloom.Text("Deploy "+service+" to production").Style(f.st.heading),
		bloom.Text("Type the service name to confirm.").Style(f.st.label),
		bloom.Fixed(1, nil),
		bloom.Row(bloom.Fill(field), deploy).Gap(2),
		msg,
	)
}

// badgeNode is the badge in the header: STAGED, then LIVE once deployed,
// which glitches into place.
func (f *form) badgeNode() bloom.Child {
	if !f.deployed {
		return bloom.Text("STAGED").Style(f.st.staged)
	}
	b := bloom.Child(bloom.Text("LIVE").Style(f.st.live))
	if f.badge.Animating() {
		b = bloom.Transition(f.badge.Float(), b, fx.Glitch{Seed: f.bursts})
	}
	return b
}
