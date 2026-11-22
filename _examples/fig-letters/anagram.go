// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// anagrams are the words shown, one after the other.
var anagrams = []string{"listen", "silent", "enlist", "tinsel", "inlets"}

// anagram is a card with one of the anagrams in it, whose letters fly to
// their places in the next word, and light up under the pointer.
type anagram struct {
	st     styles
	word   int
	hover  int // the glyph of the anagram under the pointer, or -1
	clicks int
}

func (a *anagram) Layout(bloom.Context) bloom.Child {
	word := fig.Text(anagrams[a.word]).
		Font(fonts.Big(), fonts.Small()).
		Paint(a.wordPaint()).
		ID("anagram").
		Animate(anim.Duration(900*time.Millisecond), anim.Ease(anim.Linear)).
		MoveWith(fx.Fly{Seed: 3}).
		OnGlyph(a.glyphHandlers)

	return a.st.card("anagram", bloom.Centered(word)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf(" %d/%d ", a.word+1, len(anagrams))).Style(a.st.dim)).
		Edge(bloom.Bottom, bloom.End, bloom.Text(fmt.Sprintf(" hover or click a letter · %d clicks ", a.clicks)).Style(a.st.dim))
}

// words are the anagrams, the one shown highlighted, each shown by a click.
func (a *anagram) words() bloom.Child {
	chips := make([]bloom.Child, len(anagrams))
	for i, w := range anagrams {
		st := a.st.chip
		if i == a.word {
			st = a.st.chipOn
		}
		chips[i] = bloom.Text(w).Style(st).On(on.Click(func() tea.Cmd {
			a.word = i
			return nil
		}))
	}
	return bloom.Row(chips...).Gap(1)
}

// glyphHandlers lights up a letter of the anagram while the pointer is over
// it, and counts clicks on it.
func (a *anagram) glyphHandlers(g fig.Glyph) []on.Handler {
	return []on.Handler{
		on.Hover(func(over bool) tea.Cmd {
			switch {
			case over:
				a.hover = g.Index
			case a.hover == g.Index:
				a.hover = -1
			}
			return nil
		}),
		on.Click(func() tea.Cmd {
			a.clicks++
			return nil
		}),
	}
}

// wordPaint colors each letter of the anagram, and the one under the
// pointer bold and white.
func (a *anagram) wordPaint() fig.Paint {
	if a.hover < 0 {
		return a.st.wordPaint
	}
	hover := a.hover
	return fig.PaintFunc(func(f *fig.Figure, c fig.Cell) uv.Style {
		st := a.st.wordPaint.Shade(f, c)
		if c.Glyph == hover {
			st.Fg = a.st.theme.BrightWhite
			st.Attrs |= uv.AttrBold
		}
		return st
	})
}
