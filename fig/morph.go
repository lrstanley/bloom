// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"cmp"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fx"
)

// Order is the order glyphs start in, for [TextNode.Spread].
type Order uint64

const (
	// Forward starts glyphs from the first to the last.
	Forward Order = iota
	// Backward starts glyphs from the last to the first.
	Backward
	// Outward starts glyphs from the middle out to both ends.
	Outward
	// Inward starts glyphs from both ends in to the middle.
	Inward
)

// Random starts glyphs in an order that is the same for the same seed.
func Random(seed uint32) Order { return Order(uint64(seed)<<8 | 0xff) }

// ranks returns where each of n parts starts, in the order.
func (o Order) ranks(n int) []int {
	out := make([]int, n)
	mid := float64(n-1) / 2
	for i := range out {
		switch o {
		case Forward:
			out[i] = i
		case Backward:
			out[i] = n - 1 - i
		case Outward, Inward:
			// Distance from the middle, so both halves move together.
			d := int(abs(float64(i)-mid) * 2)
			if o == Inward {
				d = n - 1 - d
			}
			out[i] = d
		}
	}
	if o <= Inward {
		return out
	}
	// A shuffle of 0..n-1 by a hash of the seed: each part's place among
	// the parts' hashes.
	seed := uint64(o >> 8)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(hash(uint64(a), seed), hash(uint64(b), seed)), cmp.Compare(a, b)) //nolint:gosec // indexes are never negative.
	})
	for r, i := range order {
		out[i] = r
	}
	return out
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func hash(x, seed uint64) uint64 {
	h := x*0x9e3779b97f4a7c15 ^ seed*0xc2b2ae3d27d4eb4f
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	return h
}

// Keying is how the glyphs of a text before and after a change are matched,
// so that a glyph in both is the same glyph, moving if its place changed.
type Keying uint8

const (
	// ByOccurrence matches the nth of each character to the nth of the same
	// character: changing "listen" to "silent" moves every letter. It suits
	// words.
	ByOccurrence Keying = iota
	// ByPosition matches the characters at the same place in the text, if
	// they are the same character: changing "12:59" to "13:00" keeps the
	// "1" and the ":", and replaces the rest. It suits numbers and clocks.
	ByPosition
)

// glyphKey is which glyph a glyph is, across changes to the text.
type glyphKey struct {
	r rune
	n int
}

// keys returns each glyph's key in f, as the node's Keying says.
func (n TextNode) keys(f *Figure) []glyphKey {
	keys := make([]glyphKey, len(f.glyphs))
	seen := map[rune]int{}
	for i, g := range f.glyphs {
		if n.keying == ByPosition {
			keys[i] = glyphKey{r: g.Rune, n: i}
			continue
		}
		keys[i] = glyphKey{r: g.Rune, n: seen[g.Rune]}
		seen[g.Rune]++
	}
	return keys
}

// Morph is text that animates its glyphs as it changes: glyphs that leave
// the text are drawn where they were, going out with [TextNode.Leave]'s
// effects, glyphs that join it come in with [TextNode.Enter]'s, and glyphs
// in both move to their new places, gliding with [TextNode.Animate]. Keep a
// Morph in the model, pass it messages, and draw [Morph.Text]:
//
//	m.clock = fig.NewMorph("12:59", anim.Duration(400*time.Millisecond))
//	cmd := m.clock.Set("13:00")
//	// in View:
//	m.clock.Text().Font(fonts.Big()).Key(fig.ByPosition).ID("clock").Animate()
//
// Setting new text while a change is still animating starts over from the
// text shown last.
type Morph struct {
	text, prev string
	v          anim.Value
}

// NewMorph returns a Morph showing text, animating changes with opts, which
// default to those of [anim.NewValue].
func NewMorph(text string, opts ...anim.Option) Morph {
	return Morph{text: text, v: anim.NewValue(1, opts...)}
}

// Set changes the text, starting the animation from the text before.
func (m *Morph) Set(text string) tea.Cmd {
	if text == m.text {
		return nil
	}
	m.prev, m.text = m.text, text
	return m.v.Play(0, 1)
}

// Update advances the animation. It implements [anim.Animation].
func (m *Morph) Update(msg tea.Msg) tea.Cmd { return m.v.Update(msg) }

// Animating reports whether a change is still animating.
func (m *Morph) Animating() bool { return m.v.Animating() }

// Current returns the text.
func (m *Morph) Current() string { return m.text }

// Text returns a node showing the text, with the glyphs of a change still
// animating.
func (m *Morph) Text() TextNode {
	n := Text(m.text)
	n.morph = &morphState{prev: m.prev, t: min(max(m.v.Float(), 0), 1)}
	return n
}

type morphState struct {
	prev string
	t    float64
}

func (s *morphState) moving() bool { return s.t < 1 }

// morphed draws a change of text, from old to f, laid out the same way,
// matched glyph by glyph. While it runs, merged characters are shown only
// between glyphs that stay where they were.
func (n TextNode) morphed(f, old *Figure) bloom.Child {
	nowKeys, oldKeys := n.keys(f), n.keys(old)
	oldAt := make(map[glyphKey]int, len(oldKeys))
	for i, k := range oldKeys {
		oldAt[k] = i
	}
	enter, leave := orFade(n.enter), orFade(n.leave)
	t := n.morph.t

	// Each glyph of the text is either one that was there before, or one
	// entering, at its own progress.
	entering := 0
	for _, k := range nowKeys {
		if _, ok := oldAt[k]; !ok {
			entering++
		}
	}
	progress := make([]float64, len(nowKeys))
	ranks, rank := n.order.ranks(entering), 0
	for _, i := range f.order() {
		progress[i] = -1
		if _, ok := oldAt[nowKeys[i]]; !ok {
			progress[i] = n.window(t, ranks[rank], entering)
			rank++
		}
	}
	// A glyph is still once it has entered, or if it is the same glyph, in
	// the same place, as before.
	rest := n.restFor(f, func(i int) bool {
		if progress[i] >= 0 {
			return progress[i] >= 1
		}
		return old.marks[oldAt[nowKeys[i]]].at(f.marks[i])
	})

	var layers []bloom.LayerNode
	for _, i := range f.order() {
		effects, gt := []fx.Effect(nil), 1.0
		if progress[i] >= 0 {
			effects, gt = enter, progress[i]
		}
		if l, ok := n.layer(f, i, rest[i], nowKeys[i], gt, effects); ok {
			layers = append(layers, l)
		}
	}
	inNow := make(map[glyphKey]bool, len(nowKeys))
	for _, k := range nowKeys {
		inNow[k] = true
	}
	leaving := 0
	for _, k := range oldKeys {
		if !inNow[k] {
			leaving++
		}
	}
	ranks, rank = n.order.ranks(leaving), 0
	for _, i := range old.order() {
		if inNow[oldKeys[i]] {
			continue
		}
		if l, ok := n.layer(old, i, false, oldKeys[i], 1-n.window(t, ranks[rank], leaving), leave); ok {
			layers = append(layers, l)
		}
		rank++
	}
	w, h := f.Size()
	ow, oh := old.Size()
	return bloom.Overlay(blankBlock(max(w, ow), max(h, oh)), layers...)
}

func orFade(effects []fx.Effect) []fx.Effect {
	if len(effects) == 0 {
		return []fx.Effect{fx.Fade{}}
	}
	return effects
}
