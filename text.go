// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

// TextNode is a string, optionally styled. Build one with [Text].
type TextNode struct {
	meta

	s        string
	style    lipgloss.Style
	styled   bool
	truncate bool
}

// Text returns a node showing s, a string that may hold ANSI styles.
func Text(s string) TextNode { return TextNode{s: s} }

// Style renders the text with style at the size the node is given, so a
// background fills the node and long lines wrap to its width. A width or
// height set on style itself is kept.
func (n TextNode) Style(style lipgloss.Style) TextNode {
	n.style, n.styled = style, true
	return n
}

// Truncate shows one line, replacing newlines with spaces and cutting text
// that does not fit with "…". Padding and borders count against its width.
func (n TextNode) Truncate() TextNode {
	n.s, n.truncate = lineBreaks.Replace(n.s), true
	return n
}

// ID sets the ID that focus and [Frame.Rect] refer to the node by.
func (n TextNode) ID[K comparable](id K) TextNode { n.id = id; return n }

// On adds mouse handlers to the node, made by package [on].
func (n TextNode) On(handlers ...on.Handler) TextNode {
	n.handlers = with(n.handlers, handlers)
	return n
}

// HoverStyle renders the text with style instead of its own while the
// pointer is over it, which needs no state in the model. It should take the
// same space as the node's style, changing only how it looks. See
// [on.Hover] for when the pointer is followed.
func (n TextNode) HoverStyle(style lipgloss.Style) TextNode {
	n.hover = &style
	return n
}

// Focusable makes the node one that [Focus] can move to.
func (n TextNode) Focusable() TextNode { n.focusable = true; return n }

// Cursor shows the terminal cursor at c, relative to the node's top-left
// cell, while that cell is on screen and not covered by a later layer.
func (n TextNode) Cursor(c *tea.Cursor) TextNode { n.cursor = c; return n }

// View implements [Child], drawing the node at its natural size.
func (n TextNode) View() string { return Render(n, 0, 0) }

// MarqueeNode is a line of text that scrolls when it does not fit. Build one
// with [Marquee].
type MarqueeNode struct {
	s, gap string
	w, gw  int // the cell widths of s and gap, measured once
	phase  float64
	style  lipgloss.Style
	styled bool
}

// Marquee returns a single line showing s, a string that may hold ANSI
// styles and whose newlines are shown as spaces. Text that fits its width is
// shown as it is, at its natural width, so the node shrinks in a [Fill] slot.
// Text that does not fit is cut off with "…" while phase is 0, and otherwise
// scrolls: phase from 0 to 1 moves it one full loop, the text and the gap
// after it, wrapping around seamlessly. A phase outside that range wraps.
//
// Show a marquee at phase 0 until something, such as focus, gives it a
// phase to scroll with. A looping Sequence from package anim can drive it:
//
//	anim.NewSequence(0, anim.Jump(0), anim.To(1, anim.Duration(8*time.Second), anim.Ease(anim.Linear)))
func Marquee(s string, phase float64) MarqueeNode {
	if math.IsNaN(phase) || math.IsInf(phase, 0) {
		phase = 0
	}
	if strings.ContainsAny(s, "\r\n") {
		s = lineBreaks.Replace(s)
	}
	return MarqueeNode{s: s, gap: "   ", w: ansi.StringWidth(s), gw: 3, phase: phase}
}

// Style renders the text with style, at the size the node is given as
// [TextNode.Style] does, so a background fills it. The style's padding and
// borders count against the width the text has to scroll in.
func (n MarqueeNode) Style(style lipgloss.Style) MarqueeNode {
	n.style, n.styled = style, true
	return n
}

// Gap sets what is shown between the end of the text and its start again
// while scrolling, three spaces by default.
func (n MarqueeNode) Gap(s string) MarqueeNode {
	n.gap, n.gw = s, ansi.StringWidth(s)
	return n
}

// View implements [Child], drawing the marquee at its natural size.
func (n MarqueeNode) View() string { return Render(n, 0, 0) }

// Layout implements [Layouter].
func (n MarqueeNode) Layout(ctx Context) Child {
	inner := lineWidth(n.style, ctx.Width)
	if inner <= 0 {
		return nil
	}
	if n.w <= inner {
		return n.text(n.s)
	}
	if n.phase == 0 {
		return n.text(truncateLine(n.s, inner))
	}

	period := n.w + n.gw
	off := int((n.phase - math.Floor(n.phase)) * float64(period))
	off = min(off, period-1)
	loop := n.s + n.gap + n.s + n.gap // two loops, so a window starting anywhere in the first fits

	// Cut keeps a wide glyph that starts before its left edge, which would
	// push everything after it out of place, so a glyph cut in half at the
	// left is replaced by a space.
	lead := ""
	if ansi.StringWidth(ansi.Cut(loop, 0, off)) < off {
		lead, off = " ", off+1
	}
	return n.text(padTo(lead+ansi.Cut(loop, off, off+inner-len(lead)), inner))
}

func (n MarqueeNode) text(s string) TextNode {
	if n.styled {
		return Text(s).Style(n.style)
	}
	return Text(s)
}

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

// lineBreaks replaces line breaks with spaces, for text shown on one line.
var lineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// stack returns s with a line for each of its graphemes, keeping its escape
// sequences, for text running down a side. Its own line breaks are dropped.
func stack(s string) string {
	if seq, _, n, _ := ansi.DecodeSequence(s, 0, nil); n == len(s) && seq != "\n" && seq != "\r" {
		return s // one glyph, as a button usually is
	}
	var b strings.Builder
	b.Grow(2 * len(s))
	var state byte
	for glyphs := 0; s != ""; {
		seq, width, n, next := ansi.DecodeSequence(s, state, nil)
		switch {
		case width > 0:
			if glyphs > 0 {
				b.WriteByte('\n')
			}
			glyphs++
			b.WriteString(seq)
		case seq != "\n" && seq != "\r":
			b.WriteString(seq)
		}
		s, state = s[n:], next
	}
	return b.String()
}

// padTo pads s with spaces to w cells, as truncating may leave it short when
// a wide glyph does not fit.
func padTo(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// truncateLine cuts s to width cells with "…", keeping whole graphemes, and
// pads it back to width when a wide glyph that did not fit leaves it short.
func truncateLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return padTo(ansi.Truncate(s, width, "…"), width)
}

// lineWidth is the room for a line of text inside width, kept within the
// style's own width and MaxWidth, less its padding, borders, and margins.
func lineWidth(style lipgloss.Style, width int) int {
	if w := style.GetWidth(); w > 0 {
		width = min(width, w+style.GetHorizontalMargins())
	}
	if w := style.GetMaxWidth(); w > 0 {
		// Lip Gloss cuts the rendered lines, margins included, to MaxWidth.
		width = min(width, w)
	}
	return max(width-style.GetHorizontalFrameSize(), 0)
}
