// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import "image"

// Viewer is a component drawn from a string, such as a Bubbles component.
// Place one in a layout with [View].
type Viewer interface {
	View() string
}

// ViewNode draws a [Viewer] from its View string. Build one with [View].
type ViewNode struct {
	v Viewer
}

// View returns a node that draws v, a component such as a Bubbles one, from
// its View string, which is called at most once a frame, and only when the
// node is measured or drawn. The node is as big as the string, clipped to
// the space it is given, and its lines are not wrapped.
//
// Pass a pointer to a component kept in a model, such as &m.input, so that
// it is not copied each frame. A v that is also a [Child] is still drawn from
// its View, so a component can lay itself out as a View of itself; place it
// directly to have it laid out.
//
// v is checked for the interfaces a component that draws its own cells can
// implement: [Measurer], [TextSource], [TextMap], and [Editable].
//
// A view is drawn as it is, with any commands in it that are written to the
// terminal rather than drawn, such as images, which [Text] and [Viewport]
// drop unless they are trusted. A component that shows text from outside the
// program, such as a file, should clean it with
// [bloomfx.dev/bloom/canvas.Sanitize] first.
func View(v Viewer) ViewNode { return ViewNode{v: v} }

// Layout implements [Child], returning the node itself.
func (n ViewNode) Layout(Context) Child { return n }

// paintView adds the op for box i, view n, drawn at vis, and for an
// [Editable], the op that draws its range over it.
func (r *renderer) paintView(i int, n ViewNode, vis image.Rectangle) {
	r.blit(r.viewOf(i), r.boxes[i].rect, vis)
	if ed, ok := n.v.(Editable); ok && r.parse {
		r.paintHeld(i, ed, vis)
	}
}
