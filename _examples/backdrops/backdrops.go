// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom/fx"
	"github.com/lrstanley/bubbletint/roles/v2"
)

// backdrop is one of the backdrops the example switches between.
type backdrop struct {
	name    string
	about   string
	effects func(m *model) []fx.Effect
}

// screens are the ambient effects drawn behind the whole screen.
var screens = []backdrop{
	{
		name:  "Ripple",
		about: "Rings of glyphs ripple out from each click, and from the\npointer as it moves. Drops of rain keep it moving.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Rain: 0.7, Color: m.st.fit.Color(roles.Cyan)}}
		},
	},
	{
		name:  "Drift",
		about: "Faint patches of glyphs drift and fade, gathering around\nthe pointer. Clicks still ripple through them.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{
				fx.Drift{Time: m.clock.Elapsed(), Focus: m.pointer, Radius: m.lens(), Color: m.st.fit.Color(roles.Fg)},
				fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Color: m.st.fit.Color(roles.Fg)},
			}
		},
	},
	{
		name:  "Rain",
		about: "Streams of glyphs fall, each led by one that scrambles\nas it falls, and trailed by glyphs that settle.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{
				fx.Rain{Time: m.clock.Elapsed(), Color: m.st.fit.Color(roles.Green), Head: m.st.fit.Color(roles.Fg)},
				fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Color: m.st.fit.Color(roles.Green)},
			}
		},
	},
}

// dialogs are the backdrops of the dialog, which change the screen beneath
// it.
var dialogs = []backdrop{
	{
		name:  "Dim",
		about: "fx.Dim moves colors toward the terminal's background.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Dim{Amount: 0.6}}
		},
	},
	{
		name:  "Desaturate",
		about: "fx.Desaturate moves colors toward gray.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Desaturate{Amount: 1}}
		},
	},
	{
		name:  "Tint",
		about: "fx.Recolor, with the same color for both, tints\neverything toward it.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Recolor{Fg: m.st.fit.Color(roles.Accent), Bg: m.st.fit.Color(roles.Accent), Amount: 0.45}}
		},
	},
	{
		name:  "Two colors",
		about: "fx.Recolor, all the way, draws every cell in two colors.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Recolor{Fg: m.st.fit.Color(roles.Border), Bg: m.st.fit.Color(roles.Sunken)}}
		},
	},
	{
		name:  "Spotlight",
		about: "fx.Spotlight dims everything but the panel, with a\nsoft edge.",
		effects: func(m *model) []fx.Effect {
			r, ok := m.ui.Frame().Rect(panelID)
			if !ok {
				return nil
			}
			return []fx.Effect{fx.Spotlight{Rect: r, Soft: 4}}
		},
	},
	{
		name:  "Braille",
		about: "fx.Braille redraws everything as braille dots, which\nchange as what is beneath them does.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Braille{Seed: 7}}
		},
	},
	{
		name:  "Braille tint",
		about: "fx.Braille draws in grays, and an fx.Recolor after it\ntints them.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Braille{Amount: 0.5, Seed: 7}, fx.Recolor{Fg: m.st.fit.Color(roles.Accent), Amount: 0.3}}
		},
	},
	{
		name:  "Dim + Drift",
		about: "fx.Dim, with an ambient fx.Drift drawn over it, both\nfading in with the dialog.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Dim{Amount: 0.6}, fx.Drift{Time: m.clock.Elapsed(), Color: m.st.fit.Color(roles.Accent)}}
		},
	},
}
