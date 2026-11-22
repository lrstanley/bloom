// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package splash is what the screen shows while the desktop starts up and
// shuts down: a logo and a progress bar, after which the desktop opens from
// its middle row and fades in, like an old CRT turning on, and closes back
// to it as one turning off.
package splash

import (
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// on is where the power rests while the desktop is on. Past 1, the desktop
// turns on, which takes a fifth of the time the power takes to get here.
const on = 1.25

// Splash is the splash, and the power of the desktop behind it.
type Splash struct {
	st theme.Styles

	// power runs from 0 to 1 as the splash starts up, and on to on as the
	// desktop turns on, and back down to 0 once off, to shut down.
	power anim.Value
}

// New returns the splash, with the desktop off.
func New() *Splash {
	s := &Splash{}
	s.power = anim.NewValue(0, s.options()...)
	return s
}

// SetStyles sets the styles the splash is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (s *Splash) SetStyles(st theme.Styles) *Splash {
	s.st = st
	return s
}

func (s *Splash) options() []anim.Option {
	return []anim.Option{anim.Duration(2 * time.Second), anim.Ease(anim.Linear)}
}

// Skip turns the desktop on at once, past the splash, as tests do.
func (s *Splash) Skip() { s.power = anim.NewValue(on, s.options()...) }

// Start starts the desktop up.
func (s *Splash) Start() tea.Cmd { return s.power.To(on) }

// Shutdown shuts the desktop down: it shades out to the splash, which runs
// back down.
func (s *Splash) Shutdown() tea.Cmd { return s.power.To(0) }

// Off reports whether the desktop is shutting down, once it has started up.
func (s *Splash) Off() bool { return s.power.Target() == 0 }

// Done reports whether the desktop has shut down, for the program to quit.
func (s *Splash) Done() bool { return s.Off() && s.power.Float() <= 0 }

// Update moves the power along, and takes the styles of a new theme.
func (s *Splash) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		s.st = *msg.Styles
	}
	return s.power.Update(msg)
}

// Screen returns what the terminal shows: the splash, or the desktop that
// desktop lays out, as it turns on or off, or once on. The desktop is laid
// out only once it shows.
func (s *Splash) Screen(desktop func() bloom.Child) bloom.Child {
	p := s.power.Float()
	if p >= 1 {
		// Once on, the CRT effects would only cost a pass over the screen on
		// every frame. It stays in the Transition, for its nodes to keep
		// their paths.
		var crt []fx.Effect
		if p < on {
			crt = []fx.Effect{fx.Unfold{}, fx.Fade{}}
		}
		return bloom.Transition(anim.OutCubic(min((p-1)/(on-1), 1)), desktop(), crt...)
	}
	const width = theme.SplashWidth
	n, word := int(p*width), "starting up…"
	if s.Off() {
		word = "shutting down…"
	}
	return bloom.Centered(bloom.Column(
		bloom.Transition(min(p*3, 1), bloom.Text("❀ bloom").Style(s.st.SplashLogo), fx.Decode{}),
		bloom.Text(""),
		bloom.Text(s.st.SplashDone.Render(strings.Repeat("━", n))+s.st.SplashToDo.Render(strings.Repeat("─", width-n))).Style(s.st.SplashLine),
		bloom.Text(word).Style(s.st.SplashWord),
	))
}
