// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package wm

import (
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// previewID is the ID of the outline showing where a window would snap.
type previewID struct{}

// Layers returns the windows as layers of the desktop's overlay, in the
// order they stack in, with the window in front last.
func (m *Manager) Layers() []bloom.LayerNode {
	layers := make([]bloom.LayerNode, 0, len(m.windows)+1)
	top := m.Top()
	for i, w := range m.windows {
		if i == len(m.windows)-1 {
			// The outline of where a window would snap goes beneath the window
			// in front, which is the one being dragged.
			layers = append(layers, bloom.Layer(bloom.Box(m.st.Outline, bloom.Space())).
				ID(previewID{}).Transparent().Place(m.target).
				Presence(m.preview, fx.Fade{}))
		}
		layers = append(layers, m.window(w, w == top))
	}
	return layers
}

// window returns the layer of window w, placed where the manager says it is.
func (m *Manager) window(w *Window, focused bool) bloom.LayerNode {
	r := m.place(w)
	st, ti, gr := m.st.Frame, m.st.Title, m.st.Grip
	if focused {
		st, ti, gr = m.st.FrameOn, m.st.TitleOn, m.st.GripOn
	}
	glyph := "□"
	if w.snap != snapNone {
		glyph = "❐"
	}
	// The title bar, the title and the border beside it, drags the window,
	// and a double click on it maximizes the window. It is one edge child, as
	// the cell between two of them belongs to the box.
	bar := bloom.Row(
		bloom.Auto(bloom.Text(" "+w.Glyph+" "+w.Title+" ").Style(ti).Truncate()),
		bloom.Fill(bloom.Text(strings.Repeat("─", max(r.Dx(), 0))).Style(gr)),
	).On(m.mover(w), on.DoubleClick(func() tea.Cmd { m.ToggleMax(w); return nil }))

	box := bloom.Box(st, w.Content).
		Edge(bloom.Top, bloom.End, m.button("×", m.st.CtlShut, func() tea.Cmd { return m.Close(w) })).
		Edge(bloom.Top, bloom.End, m.button(glyph, m.st.CtlOver, func() tea.Cmd { m.ToggleMax(w); return nil })).
		Edge(bloom.Top, bloom.End, m.button("−", m.st.CtlOver, func() tea.Cmd { return m.Minimize(w) })).
		Edge(bloom.Top, bloom.Stretch, bar).
		EdgeAt(bloom.Bottom, -1, bloom.Text("◢").Style(gr).HoverStyle(m.st.Corner).On(m.resizer(w)))
	if w.Scroll != nil {
		box = box.Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(w.Scroll).
			Style(bloom.ScrollbarStyle{ThumbStyle: gr, TrackStyle: gr}))
	}

	layer := bloom.Layer(box).ID(w.id).Place(r).
		Shadow(2, 1, fx.Dim{Amount: 0.6}).
		// Any press on a window brings it to the front, including one on a
		// button or on its title bar as a drag starts.
		On(on.Down(func() tea.Cmd { m.Raise(w); return nil }))
	// A window follows the pointer cell for cell while it is dragged, and
	// glides otherwise: to where it is maximized, snapped, or moved with the
	// keyboard, and from where it was dropped, should that be against an
	// edge, or off the screen.
	if g := m.grab; g == nil || g.win != w || !g.moved {
		layer = layer.Animate(anim.Spring(22, 1))
	}
	if w.slide {
		return layer.Presence(w.pres, fx.Slide{From: bloom.Bottom}, fx.Fade{})
	}
	return layer.Presence(w.pres, fx.Fade{})
}

// button is a control on a title bar.
func (m *Manager) button(glyph string, hover lipgloss.Style, fn func() tea.Cmd) bloom.Child {
	return bloom.Text(" " + glyph + " ").Style(m.st.Control).HoverStyle(hover).On(on.Click(fn))
}
