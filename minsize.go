// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strconv"

	"bloomfx.dev/bloom/fx"
)

// MinSizeNode draws its child only when it is given at least a minimum size,
// and a note saying the space is too small otherwise. Build one with
// [MinSize].
type MinSizeNode struct {
	child Child
	min   Size
}

// MinSize draws child when it is given at least w×h cells, and in a smaller
// space draws a line in its middle, faded toward the terminal's background,
// saying the window is too small and what it needs, such as "window too
// small, min 80x24", or only "window too small" where the rest does not fit.
// A w or h of zero or less has no minimum on that axis.
//
// Where it fits, its natural size is at least w×h, so in a slot sized by its
// content, or drawn at its natural height as [Root.Render] does for a
// height of zero, it is given its minimum. Wrap the root of a model's layout
// in it for an application that cannot be used below some size:
//
//	func (m *model) View() tea.View {
//		return m.ui.View(m.width, m.height, bloom.MinSize(80, 24, m.layout()))
//	}
//
// While the note is drawn the child is not laid out, so its nodes are not in
// the frame: their mouse handlers do not run and [Focus] skips them. Keys
// still reach the model, which can compare its own size with the minimum to
// ignore them.
func MinSize(w, h int, child Child) MinSizeNode {
	return MinSizeNode{child: child, min: Size{W: max(w, 0), H: max(h, 0)}}
}

// View implements [Child], drawing the node at its natural size, which is
// never too small.
func (n MinSizeNode) View() string { return Render(0, 0, n) }

// Layout implements [Layouter], returning the child, or the note when the
// space is too small.
func (n MinSizeNode) Layout(ctx Context) Child {
	if !n.tooSmall(Size{W: ctx.Width, H: ctx.Height}) {
		return n.child
	}
	return Centered(Transition(1, Text(n.note(ctx.Width)).Truncate(), tooSmallFade))
}

// tooSmall reports whether size is below the minimum on either axis.
func (n MinSizeNode) tooSmall(size Size) bool {
	return (n.min.W > 0 && size.W < n.min.W) || (n.min.H > 0 && size.H < n.min.H)
}

// tooSmallFade moves the note's text toward the terminal's background, so it
// sits quietly on an otherwise empty screen.
var tooSmallFade = fx.Dim{Amount: 0.5}

// note is the line drawn when the space is too small, w cells wide: what the
// window needs, or only that it is too small where that does not fit.
func (n MinSizeNode) note(w int) string {
	const short = "window too small"
	var need string
	switch {
	case n.min.H == 0:
		need = strconv.Itoa(n.min.W) + "w"
	case n.min.W == 0:
		need = strconv.Itoa(n.min.H) + "h"
	default:
		need = strconv.Itoa(n.min.W) + "x" + strconv.Itoa(n.min.H)
	}
	// The note is ASCII, so its length is its width.
	if full := short + ", min " + need; len(full) <= w {
		return full
	}
	return short
}
