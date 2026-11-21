// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package bloom lays out, composites, and routes input for Bubble Tea
// views.
//
// A view is described each frame as a tree of nodes built from model state:
// [Row] and [Column] share space between children, [Box] draws a Lip Gloss
// frame around one, [Overlay] puts [Layer]s such as dialogs over a base, and
// a Bubble Tea component is placed with [View].
//
//	bloom.Column(
//		bloom.Row(
//			bloom.Fixed(20, m.sidebar).ID(paneSidebar).Collapsible(1),
//			bloom.Fill(m.reader).ID(paneReader).Min(40),
//		),
//		m.statusbar,
//	)
//
// # Who sizes what
//
// Constraints go down, sizes come up, and parents place. A parent decides
// each child's rectangle, whether it is collapsed, and how it is clipped. How
// a child takes part in sharing space is set where it is placed, by wrapping
// it in a slot such as [Fixed] or [Fill], not by the child itself. A child
// only knows the size it was given: a component builds its layout from it
// in Layout, and one that knows its natural size can implement [Measurer].
//
// # Rendering
//
// [Root] turns a tree into a [tea.View] each frame. Leaf strings are parsed
// into cells once and cached, nodes are composited in tree order with every
// node clipped to its parent, and [fx] effects run on a node's cells or on
// the frame beneath a layer. A frame identical to the previous one is not
// composited again. Mouse events run the handlers of the nodes under the
// pointer, given with On and made by package [on], using the geometry of
// the frame on screen.
//
// Layout is a pure function of the tree, so [Solve] can compute the same
// rectangles in Update, for components that need their size there.
//
// # Documentation
//
// See the [documentation] for guides and examples. It is also available for
// LLMs and agents as [llms.txt].
//
// [documentation]: https://bloomfx.dev/
// [llms.txt]: https://bloomfx.dev/llms.txt
package bloom

import (
	"slices"
	"sync/atomic"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Child is anything that can be placed in a layout: every node of this
// package, and any component that builds its own layout for the space it
// is given, which lets it change with its size. A component drawn from a
// View string, such as a Bubbles component, is placed with [View], and a
// plain string with [Text].
//
// Layout must not change the component: it is called while measuring as
// well as for the final layout, and its result is reused for any space that
// differs only in a size it did not read from ctx. A node's Layout returns
// the node itself, as bloom lays nodes out without calling it, so a
// component can return a node, another component, or nil for nothing.
//
// A component that is also used where a string is wanted, such as by a
// program that doesn't use bloom, can draw itself with [Render], or with a
// [Root] it keeps, which skips drawing a frame that did not change.
type Child interface {
	Layout(ctx Context) Child
}

// LayoutFunc is a function that builds a layout for the space it is given,
// placed as a [Child]: a responsive piece of a layout that needs no type of
// its own. A nil LayoutFunc lays out nothing, as a nil child does.
//
//	bloom.LayoutFunc(func(ctx bloom.Context) bloom.Child {
//		if ctx.Width() < 80 {
//			return bloom.Column(sidebar, body)
//		}
//		return bloom.Row(bloom.Fixed(30, sidebar), bloom.Fill(body))
//	})
type LayoutFunc func(ctx Context) Child

// Layout implements [Child], calling f.
func (f LayoutFunc) Layout(ctx Context) Child {
	if f == nil {
		return nil
	}
	return f(ctx)
}

// Measurer is a component that knows its natural size, so it is not
// measured from what it lays out or draws. It is checked on a component, on
// the [Viewer] of a [View], and on the drawable of a [Drawable]. Measure returns the size the component wants within avail; the
// result is clamped to avail. Return what the component needs, not avail
// itself: when a layout is drawn at its natural size, avail is unbounded.
type Measurer interface {
	Measure(avail Size) Size
}

// Size is a width and height in cells.
type Size struct {
	W, H int
}

// Context is what a component is given to build its layout for: the space
// it has, and where it is.
//
// The sizes a component reads are noted, so that bloom calls Layout again
// only when a size it read changes. A component that never reads its size,
// such as one that leaves sizing to the nodes it returns, is laid out once a
// frame wherever it is. A size read later, such as by a component it
// returned, or by the item builder of a list, is the one the component is
// laid out in by then.
type Context struct {
	space  *laidSpace
	wc     bool          // text is measured with ansi.WcWidth rather than ansi.GraphemeWidth
	screen func() *Frame // the Root's frame on screen, or nil outside a Root
	side   Direction     // the side the layer the component is in went on; see Side
}

// laidSpace is the space a component is laid out in, which the [Context] it
// was given reads, and the sizes it read, as readW and readH. A size it has
// not read follows the space its layout is used again in. It is atomic, as
// a component may keep its Context and read it from anywhere, which only
// makes its next layout read more.
type laidSpace struct {
	w, h  atomic.Int64
	reads atomic.Uint32
}

// The sizes of a [Context] a component read.
const (
	readW = 1 << iota
	readH
)

// Width returns the width available. Once layout is done it is the exact
// width the component was given.
func (c Context) Width() int {
	if c.space == nil {
		return 0
	}
	c.space.reads.Or(readW)
	return int(c.space.w.Load())
}

// Height returns the height available. Once layout is done it is the exact
// height the component was given.
func (c Context) Height() int {
	if c.space == nil {
		return 0
	}
	c.space.reads.Or(readH)
	return int(c.space.h.Load())
}

// Size returns the space available, its width and height together.
func (c Context) Size() Size {
	return Size{W: c.Width(), H: c.Height()}
}

// Side returns the side or corner of its node that the layer the component
// is in was placed against with [LayerNode.AnchorTo], which is the opposite
// of the one asked for when the layer did not fit there. A menu can use it
// to draw its edge facing the node. It reports false for a component that
// is not in a layer, or is in one that is not anchored to a node.
func (c Context) Side() (Direction, bool) { return c.side, c.side != 0 }

// Frame returns the frame on screen when it is called, rather than when the
// component was laid out, for the mouse handlers it makes to read where its
// parts ended up when they run, such as which items of a [List] were shown
// or whether a node was laid out at all. In a handler, it is the frame the
// event was found in. Called from Layout itself, it is the frame before the
// one being laid out. Outside a [Root], such as with [Render], it is nil,
// which finds nothing.
func (c Context) Frame() *Frame {
	if c.screen == nil {
		return nil
	}
	return c.screen()
}

// unbounded is the space given to content that may be larger than its
// viewport, such as the content of a [Scroll].
const unbounded = 1 << 20

// Align is where a child goes on the cross axis of a [Row] or [Column], or
// on either axis of a [Layer].
type Align uint8

const (
	// Stretch makes a child as big as the space it is in. It is the default
	// for children of a row or column.
	Stretch Align = iota
	// Start puts a child at the top or left.
	Start
	// Center puts a child in the middle.
	Center
	// End puts a child at the bottom or right.
	End
)

// offset returns where a child of size n goes in space of size avail.
func (a Align) offset(n, avail int) int {
	switch a {
	case Center:
		return (avail - n) / 2
	case End:
		return avail - n
	case Stretch, Start:
	}
	return 0
}

// meta is what any node can be given for input: an ID for focus and lookups,
// whether it can be focused, the cursor it shows, its mouse handlers, the
// style it has while hovered, its tooltip, whether its tooltip is pinned
// open, and whether the text cut to fit inside it shows none.
type meta struct {
	id         any
	focusable  bool
	scope      bool
	selectable bool
	noTip      bool // set on every box inside one as it is built
	tipOpen    bool // its tooltip is pinned open
	cursor     *tea.Cursor
	handlers   []on.Handler
	hover      *lipgloss.Style
	tip        *tooltip
}

// with returns handlers with more added, without changing a slice that a
// copy of the node may share.
func with(handlers, more []on.Handler) []on.Handler {
	return append(slices.Clip(handlers), more...)
}
