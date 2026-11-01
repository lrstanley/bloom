// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/fx"
)

// skeletonFade is how far an unstyled skeleton's bars are faded from the
// terminal's foreground into its background.
const skeletonFade = 0.8

// SkeletonNode is a placeholder for content that is still loading. Build one
// with [Skeleton].
type SkeletonNode struct {
	lines     []int
	style     lipgloss.Style
	styled    bool
	phase     float64
	shimmered bool
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
	n.phase, n.shimmered = phase, true
	return n
}

// View implements [Child], drawing the skeleton at its natural size.
func (n SkeletonNode) View() string { return Render(n, 0, 0) }

// Layout implements [Layouter].
func (n SkeletonNode) Layout(ctx Context) Child {
	width := lineWidth(n.style, ctx.Width)
	var b strings.Builder
	for i, w := range n.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat("█", min(max(w, 0), width)))
	}

	var c Child
	if n.styled {
		c = Text(b.String()).Style(n.style)
	} else {
		c = Transition(1, Text(b.String()), fx.Dim{Amount: skeletonFade})
	}
	if n.shimmered {
		c = Transition(n.phase, c, fx.Shimmer{})
	}
	return c
}
