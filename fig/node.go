// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	_ "embed"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

//go:embed fonts/data/standard.flf.gz
var standardData string

// standard is the font a TextNode uses when given none: figlet's standard
// font, the same as fonts.Standard.
var standard = sync.OnceValue(func() *Font { return Lazy("standard", standardData) })

// TextNode shows text in a FIGlet font, as a bloom node. Build one with
// [Text]. It implements [bloom.Layouter] and [bloom.Measurer].
//
// A node whose glyphs do not move is drawn as one string, which bloom
// parses once and does not draw again while it is unchanged. Once glyphs
// move, with [TextNode.Each], [TextNode.Animate], [TextNode.OnGlyph], or a
// [Morph], each glyph is drawn as a transparent [bloom.LayerNode] of its own,
// over the ones before it, so that bloom's effects and layout animation work
// on single glyphs. Where glyphs were smushed together, each glyph's layer
// holds its own strokes, and the merged characters are shown only while the
// glyphs that met there are at rest: glyphs moving apart never leave holes.
type TextNode struct {
	text     string
	fonts    []*Font
	opts     Options
	paint    Paint
	fallback bloom.Child
	wrap     bool
	id       any
	handlers []on.Handler

	each    bool
	t       float64
	effects []fx.Effect
	spread  float64
	order   Order
	anim    []anim.Option
	animate bool
	moves   []fx.Effect
	keying  Keying
	onGlyph func(Glyph) []on.Handler
	morph   *morphState
	enter   []fx.Effect
	leave   []fx.Effect
}

// Text returns a node showing s, in figlet's standard font unless given
// others with [TextNode.Font].
func Text(s string) TextNode { return TextNode{text: s, t: 1, spread: 0.5} }

// Font sets the fonts the text may be shown in. The first whose figure fits
// the space the node is given is used, and [TextNode.Fallback] when none
// fits, so a header can step down to smaller fonts as space runs short.
func (n TextNode) Font(chain ...*Font) TextNode { n.fonts = chain; return n }

// Options sets how the text is laid out. With [TextNode.Wrap], its Width is
// the node's width.
func (n TextNode) Options(o Options) TextNode { n.opts = o; return n }

// Paint sets how the text is colored.
func (n TextNode) Paint(p Paint) TextNode { n.paint = p; return n }

// Fallback sets what is shown when the text fits in none of the fonts. The
// default is the text itself, on one line, cut off with "…" if it does not
// fit.
func (n TextNode) Fallback(c bloom.Child) TextNode { n.fallback = c; return n }

// Wrap wraps the text onto more lines, at the node's width, as
// [Options.Width] does, before stepping down to the next font. A font is
// still skipped if the wrapped text is too tall.
func (n TextNode) Wrap() TextNode { n.wrap = true; return n }

// ID sets the ID that [bloom.Frame.Rect] refers to the node by. It is also
// what the IDs of its glyphs' layers are made from, which
// [TextNode.Animate] needs.
func (n TextNode) ID[K comparable](id K) TextNode { n.id = id; return n }

// On adds mouse handlers to the node, made by package on.
func (n TextNode) On(handlers ...on.Handler) TextNode {
	n.handlers = append(slices.Clip(n.handlers), handlers...)
	return n
}

// Each draws every glyph with effects applied for its own progress, as
// [bloom.Transition] does a node, the glyphs starting one after another as t
// runs from 0 to 1, as [TextNode.Spread] says. At t of 1, the text is drawn
// whole again.
func (n TextNode) Each(t float64, effects ...fx.Effect) TextNode {
	n.each, n.t, n.effects = true, min(max(t, 0), 1), effects
	return n
}

// Spread sets how much of [TextNode.Each]'s progress is spent starting the
// glyphs, from 0, all at once, to 1, each in an instant, as [fx.Stagger]'s
// Spread does, and the order they start in. Entering and leaving glyphs of a
// [Morph] start the same way. The default is 0.5, in [Forward] order.
func (n TextNode) Spread(s float64, order Order) TextNode {
	n.spread, n.order = min(max(s, 0), 1), order
	return n
}

// Animate glides each glyph to its new place when the text changes, with
// bloom's layout animation, as [bloom.SlotNode.Animate] does a slot. It needs
// an ID, and a retained [bloom.Root]. Which glyph is which is decided by
// [TextNode.Key].
func (n TextNode) Animate(opts ...anim.Option) TextNode {
	n.animate, n.anim = true, opts
	return n
}

// MoveWith has [TextNode.Animate] move glyphs with effects, such as
// [fx.Fly], rather than by gliding, as [bloom.SlotNode.MoveWith] does.
func (n TextNode) MoveWith(effects ...fx.Effect) TextNode { n.moves = effects; return n }

// Key sets how glyphs of the text before and after a change are matched, for
// [TextNode.Animate] and [Morph].
func (n TextNode) Key(k Keying) TextNode { n.keying = k; return n }

// OnGlyph adds the mouse handlers fn returns to each glyph's layer, such as
// to shake a letter while the pointer is over it. Where glyphs overlap, the
// one on top gets the event.
func (n TextNode) OnGlyph(fn func(g Glyph) []on.Handler) TextNode { n.onGlyph = fn; return n }

// Enter sets the effects glyphs entering the text of a [Morph] are drawn
// with, for their progress from 0 to 1. The default is [fx.Fade].
func (n TextNode) Enter(effects ...fx.Effect) TextNode { n.enter = effects; return n }

// Leave sets the effects glyphs leaving the text of a [Morph] are drawn
// with, for their progress from 1 down to 0, so an effect that brings a node
// in from the top, such as fx.Slide{From: bloom.Top}, takes it out the top.
// The default is [fx.Fade].
func (n TextNode) Leave(effects ...fx.Effect) TextNode { n.leave = effects; return n }

// View implements [bloom.Child], drawing the node at its natural size.
func (n TextNode) View() string { return bloom.Render(n, 0, 0) }

// Measure implements [bloom.Measurer].
func (n TextNode) Measure(avail bloom.Size) bloom.Size {
	if _, f, _ := n.pick(avail); f != nil {
		w, h := f.Size()
		return bloom.Size{W: w, H: h}
	}
	fb := n.fallbackChild()
	if m, ok := fb.(bloom.Measurer); ok {
		return m.Measure(avail)
	}
	w, h := canvas.Measure(fb.View(), ansi.GraphemeWidth)
	return bloom.Size{W: min(w, avail.W), H: min(h, avail.H)}
}

// Layout implements [bloom.Layouter].
func (n TextNode) Layout(ctx bloom.Context) bloom.Child {
	font, f, o := n.pick(bloom.Size{W: ctx.Width, H: ctx.Height})
	if f == nil {
		return n.fallbackChild()
	}
	if n.morph != nil && n.morph.moving() {
		return n.wrapped(n.morphed(f, font.Figure(n.morph.prev, o)))
	}
	if !n.glyphwise() {
		return bloom.Text(f.Render(n.paint)).ID(n.id).On(n.handlers...)
	}
	ts := n.progress(f)
	return n.wrapped(n.glyphs(f, ts, n.restFor(f, func(i int) bool { return ts[i] >= 1 })))
}

// glyphwise reports whether the node is drawn a layer per glyph.
func (n TextNode) glyphwise() bool {
	return n.each && n.t < 1 || n.animate || len(n.moves) > 0 || n.onGlyph != nil
}

// wrapped gives a tree of layers the node's ID and handlers.
func (n TextNode) wrapped(c bloom.Child) bloom.Child {
	if n.id == nil && len(n.handlers) == 0 {
		return c
	}
	return bloom.Auto(c).ID(n.id).On(n.handlers...)
}

// pick returns the first font, its figure of the text, and the options it
// was laid out with, that fits avail.
func (n TextNode) pick(avail bloom.Size) (*Font, *Figure, Options) {
	chain := n.fonts
	if len(chain) == 0 {
		chain = []*Font{standard()}
	}
	o := n.opts
	if n.wrap {
		o.Width = avail.W
	}
	for _, font := range chain {
		if font == nil {
			continue
		}
		f := font.Figure(n.text, o)
		if w, h := f.Size(); w <= avail.W && h <= avail.H {
			return font, f, o
		}
	}
	return nil, nil, o
}

func (n TextNode) fallbackChild() bloom.Child {
	if n.fallback != nil {
		return n.fallback
	}
	return bloom.Text(n.text).Truncate()
}

// glyphID is the ID of a glyph's layer: the node's ID and the glyph's key.
type glyphID struct {
	node any
	key  glyphKey
}

// glyphs returns f drawn a layer per glyph, each glyph's effects applied for
// its progress, ts, and the merged characters each glyph is drawn over shown
// only where rest says.
func (n TextNode) glyphs(f *Figure, ts []float64, rest []bool) bloom.Child {
	keys := n.keys(f)
	layers := make([]bloom.LayerNode, 0, len(f.glyphs))
	for _, i := range f.order() {
		var effects []fx.Effect
		t := 1.0
		if n.each {
			t, effects = ts[i], n.effects
		}
		if l, ok := n.layer(f, i, rest[i], keys[i], t, effects); ok {
			layers = append(layers, l)
		}
	}
	return bloom.Overlay(blankBlock(f.Size()), layers...)
}

// layer returns glyph i's layer, at progress t of effects.
func (n TextNode) layer(f *Figure, i int, rest bool, key glyphKey, t float64, effects []fx.Effect) (bloom.LayerNode, bool) {
	s, at := f.layer(i, n.paint, rest)
	if s == "" {
		return bloom.LayerNode{}, false
	}
	l := bloom.Layer(bloom.Text(s)).Anchor(bloom.Start, bloom.Start).Offset(at.X, at.Y).Transparent()
	if len(effects) > 0 {
		l = l.Transition(t, effects...)
	}
	if n.id != nil {
		l = l.ID(glyphID{node: n.id, key: key})
	}
	if n.animate {
		l = l.Animate(n.anim...)
		if len(n.moves) > 0 {
			l = l.MoveWith(n.moves...)
		}
	}
	if n.onGlyph != nil {
		l = l.On(n.onGlyph(f.glyphs[i])...)
	}
	return l, true
}

// restFor returns, for each glyph of f, whether the merged characters it is
// drawn over are shown: when it and every glyph it met there are at rest.
func (n TextNode) restFor(f *Figure, still func(i int) bool) []bool {
	rest := make([]bool, len(f.glyphs))
	for i := range rest {
		rest[i] = still(i)
	}
	for _, s := range f.seams {
		top := s.Later
		if f.stack == EarlierOnTop {
			top = s.Earlier
		}
		if !still(s.Earlier) || !still(s.Later) {
			rest[top] = false
		}
	}
	return rest
}

// progress returns each glyph's progress of [TextNode.Each].
func (n TextNode) progress(f *Figure) []float64 {
	ts := make([]float64, len(f.glyphs))
	for i := range ts {
		ts[i] = 1
	}
	if !n.each || n.t >= 1 {
		return ts
	}
	ranks := n.order.ranks(f.shown)
	for i := range ts {
		// A glyph showing nothing has no rank, and nothing to animate.
		if f.visible[i] < f.shown {
			ts[i] = n.window(n.t, ranks[f.visible[i]], f.shown)
		}
	}
	return ts
}

// window returns the progress, for t, of the part starting rank-th of count,
// as [fx.Stagger] splits t between parts.
func (n TextNode) window(t float64, rank, count int) float64 {
	var start float64
	if count > 1 {
		start = n.spread * float64(rank) / float64(count-1)
	}
	end := start + (1 - n.spread)
	if end <= start {
		if t >= start {
			return 1
		}
		return 0
	}
	return min(max((t-start)/(end-start), 0), 1)
}

// blankBlock returns a node of blanks, w×h, the size of a figure drawn
// layer by layer.
func blankBlock(w, h int) bloom.Child {
	row := strings.Repeat(" ", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return bloom.Text(strings.Join(rows, "\n"))
}
