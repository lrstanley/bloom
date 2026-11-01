// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
)

// SkeletonNode is a placeholder for content that is still loading. Build one
// with [Skeleton].
type SkeletonNode struct {
	lines  []int
	style  lipgloss.Style
	styled bool
	phase  float64
}

// Skeleton returns a placeholder for content that is still loading: a bar
// of blocks for each of lines, that many cells wide, with 0 for a blank
// line. Give it a line for each line of the content it stands in for, so
// that nothing moves when the content arrives:
//
//	if !m.loaded {
//		return bloom.Box(card, bloom.Skeleton(8, 0, 5, 11).Shimmer(m.shimmer.Float()))
//	}
//	return bloom.Box(card, bloom.Column(title, bloom.Fixed(1, nil), value, unit))
//
// Bars are cut to the width the skeleton is given. They are drawn in the
// terminal's foreground, faded most of the way into its background, unless
// given a [SkeletonNode.Style].
func Skeleton(lines ...int) SkeletonNode {
	return SkeletonNode{lines: lines}
}

// Style draws the bars with style, whose foreground colors them.
func (n SkeletonNode) Style(style lipgloss.Style) SkeletonNode {
	n.style, n.styled = style, true
	return n
}

// Shimmer sweeps a band of light across the skeleton, as [fx.Shimmer] does,
// at phase from 0 to 1. A looping sequence from package anim can drive it,
// started in Init and stopped once the content arrives:
//
//	shimmer: anim.NewLoop(1400 * time.Millisecond),
//
// For a band of another color or width, draw the skeleton in a [Transition]
// with an [fx.Shimmer] instead.
func (n SkeletonNode) Shimmer(phase float64) SkeletonNode {
	n.phase = phase
	return n
}

// View implements [Child], drawing the skeleton at its natural size.
func (n SkeletonNode) View() string { return Render(n, 0, 0) }

// Layout implements [Layouter].
func (n SkeletonNode) Layout(ctx Context) Child {
	width := lineWidth(n.style, ctx.Width)
	bar := func(w int) int { return min(max(w, 0), width) }

	size := max(len(n.lines)-1, 0) // the line breaks
	for _, w := range n.lines {
		size += bar(w) * len("█")
	}
	var b strings.Builder
	b.Grow(size)
	for i, w := range n.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for range bar(w) {
			b.WriteString("█")
		}
	}

	// The shimmer's band is off the skeleton at phase 0, so it is only drawn
	// once it moves.
	text := Text(b.String())
	switch {
	case !n.styled:
		return Transition(n.phase, text, skeletonBars{})
	case n.phase != 0:
		return Transition(n.phase, text.Style(n.style), fx.Shimmer{})
	default:
		return text.Style(n.style)
	}
}

// skeletonBars draws an unstyled skeleton's bars, in the terminal's
// foreground, faded most of the way into its background, with the shimmer's
// band at t over them. It is one effect, so they are drawn on one surface.
type skeletonBars struct{}

// Apply implements [fx.Effect].
func (skeletonBars) Apply(s *canvas.Surface, t float64) {
	fx.Dim{Amount: 0.8}.Apply(s, 1)
	fx.Shimmer{}.Apply(s, t)
}
