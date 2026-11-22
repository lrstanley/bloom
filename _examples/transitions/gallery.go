// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
)

const sampleText = "The quick brown fox\njumps over the lazy\ndog. 0123456789 ✓\n" +
	"Wide glyphs: 世界 ✓\nthe border stays whole"

var items = []string{"Inbox", "Drafts", "Sent", "Archive", "Spam", "Trash"}

// cards are what the gallery's cards show. Body is a list instead of a
// panel when list is set.
var cards = []struct {
	name    string
	list    bool
	effects []fx.Effect
}{
	{"Unfold", false, []fx.Effect{fx.Unfold{}}},
	{"Iris", false, []fx.Effect{fx.Iris{}}},
	{"Shade", false, []fx.Effect{fx.Shade{Seed: 7}}},
	{"Blinds from the top", false, []fx.Effect{fx.Blinds{From: fx.Top}}},
	{"Wave from the bottom", false, []fx.Effect{fx.Wave{From: fx.Bottom}}},
	{"Wave from the left", false, []fx.Effect{fx.Wave{From: fx.Left}}},
	{"Stagger: Slide, by rows", true, []fx.Effect{
		fx.Stagger{Effect: fx.Slide{From: fx.Left}, From: fx.Top},
	}},
	{"Stagger: Shade, by columns", false, []fx.Effect{
		fx.Stagger{Effect: fx.Shade{Seed: 3}, From: fx.Left, Spread: 0.7},
	}},
	{"Delay: Fade, then Scramble", false, []fx.Effect{
		fx.Delay{Effect: fx.Fade{}, To: 0.5},
		fx.Delay{Effect: fx.Scramble{Seed: 5}, From: 0.5},
	}},
	{"Reverse: Wipe, leaving", false, []fx.Effect{
		fx.Reverse{Effect: fx.Wipe{From: fx.Left}},
	}},
	{"Ease: Slide with OutBack", false, []fx.Effect{
		fx.Ease{Effect: fx.Slide{From: fx.Left}, Easing: anim.OutBack},
	}},
	{"Blinds from the left", false, []fx.Effect{fx.Blinds{From: fx.Left, Size: 2}}},
}

// gallery is the grid of cards, each running its transition at t, the
// gallery's progress.
type gallery struct {
	st *styles
	t  float64
}

func (g gallery) Layout(bloom.Context) bloom.Child {
	kids := make([]bloom.Child, len(cards))
	for i, c := range cards {
		kids[i] = bloom.Box(g.st.card, bloom.Column(
			bloom.Text(c.name).Style(g.st.name),
			bloom.Fill(bloom.Column(bloom.Transition(g.t, g.body(c.list), c.effects...))),
		))
	}
	return bloom.Grid(4, kids...).Gap(1, 0)
}

// body returns what a card transitions: a bordered panel, or a list.
func (g gallery) body(list bool) bloom.Child {
	if !list {
		return bloom.Box(g.st.panel, bloom.Text(sampleText))
	}
	rows := make([]bloom.Child, len(items))
	for i, item := range items {
		st := g.st.rowEven
		if i%2 == 1 {
			st = g.st.rowOdd
		}
		rows[i] = bloom.Text(fmt.Sprintf(" %-*s", 26, item)).Style(st)
	}
	return bloom.Column(rows...)
}
