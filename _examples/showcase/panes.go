// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"slices"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// paneSizes are how wide the panes are: fixed, or sharing the width by
// weight down to a minimum. Those with a priority collapse as the terminal
// narrows, the lowest first.
var paneSizes = map[bloom.ID]struct {
	fixed    int
	grow     int
	min      int
	collapse int
}{
	paneTints:    {fixed: 30, collapse: 2},
	paneActivity: {grow: 4, min: 30},
	panePalette:  {grow: 3, min: 28, collapse: 1},
	paneServices: {grow: 3, min: 26},
}

// Panes glide to their new places and sizes as they are zoomed, and a
// dragged pane trails the pointer a little, and glides into its place when
// dropped.
var glide = anim.Spring(14, 0.85)

// Panes making way for a dragged pane fly to their new places: their cells
// take off from where they were and land where they go, over this long.
var flight = []anim.Option{anim.Duration(800 * time.Millisecond), anim.Ease(anim.Linear)}

// dragLayer floats the dragged pane under the pointer, held where it was
// grabbed, over a shadow. It has the pane's ID, so it carries on from where
// the pane was, and the pane carries on from it when dropped.
func (m *model) dragLayer() bloom.LayerNode {
	at := m.at.Sub(m.grab)
	return bloom.Layer(m.pane(m.dragging)).
		Anchor(bloom.Start, bloom.Start).
		Offset(at.X, at.Y).
		Size(m.size.X, m.size.Y).
		Shadow(2, 1, fx.Recolor{Fg: m.st.shadow, Bg: m.st.shadow}).
		ID(m.dragging).
		Animate(glide)
}

// slot is pane id's place in the row: sharing the width, all of it while
// zoomed, or none while another pane is. While the pane is dragged, its
// place is left empty, for the other panes to fly into as they move.
func (m *model) slot(id bloom.ID) bloom.SlotNode {
	if id == m.dragging {
		size := paneSizes[id]
		return bloom.Grow(size.grow, nil).Min(size.min)
	}

	pane, size := m.pane(id), paneSizes[id]
	s := bloom.Fill(pane)
	switch {
	case m.zoomed == "" && size.fixed > 0:
		s = bloom.Fixed(size.fixed, pane)
	case m.zoomed == "":
		s = bloom.Grow(size.grow, pane).Min(size.min)
	case m.zoomed != id:
		s = bloom.Fixed(0, pane)
	}
	if m.zoomed == "" && size.collapse > 0 {
		s = s.Collapsible(size.collapse)
	}
	if id == paneTints || m.zooming {
		return s.ID(id).Animate(glide)
	}
	return s.ID(id).Animate(flight...).MoveWith(fx.Fly{Seed: uint32(len(id))}) //nolint:gosec // IDs are short.
}

// pane boxes the pane's content under its title, which, except for the
// sidebar's, can be dragged to move the pane, and double clicked to zoom.
func (m *model) pane(id bloom.ID) bloom.Child {
	var body bloom.Child
	switch id {
	case paneTints:
		body = tintList{st: &m.st, current: m.tint, pick: m.switchTint}
	case paneActivity:
		body = &m.activity
	case panePalette:
		body = palette{st: &m.st}
	case paneServices:
		body = &m.services
	}

	box, title := m.st.pane, m.st.title
	if id == m.dragging {
		// Lifted, it brings its own background over the panes beneath.
		box, title = m.st.lifted, m.st.grabbed
	}

	head := bloom.Text(string(id)).Style(title)
	if id != paneTints {
		// The title has an ID of its own, by which bloom follows the drag of
		// it as the pane moves from the row to its own layer.
		head = bloom.Text("⠿ "+string(id)).Style(title).HoverStyle(m.st.grabbed).ID(id.Sub("title")).On(
			on.Drag(func(e on.Event) tea.Cmd { return m.drag(id, e) }),
			on.DoubleClick(func() tea.Cmd { return m.zoom(id) }),
		)
	}
	return bloom.Box(box, bloom.Column(head, bloom.Fixed(1, nil), bloom.Fill(body)))
}

// drag lifts pane id out of the row once its title is dragged, to float
// under the pointer, and moves its place in the row to wherever the pointer
// is. The other panes glide out of the way, and when it is dropped, it
// glides into its place.
func (m *model) drag(id bloom.ID, e on.Event) tea.Cmd {
	if e.Done {
		m.dragging = ""
		return nil
	}
	if m.dragging == "" {
		// A press is not a drag until the pointer moves.
		r, ok := m.ui.Frame().Rect(id)
		if !ok || m.zoomed != "" || e.Screen == e.Start {
			return nil
		}
		m.dragging, m.grab, m.size, m.zooming = id, e.Start.Sub(r.Min), r.Size(), false
	}
	m.at = e.Screen

	// The pane goes after the last of the others on screen whose middle the
	// pointer is past, or before the first on screen if it is past none, so
	// panes collapsed out of sight keep their places around it.
	others := slices.DeleteFunc(slices.Clone(m.order), func(p bloom.ID) bool { return p == id })
	i, placed := slices.Index(m.order, id), false
	for j, p := range others {
		r, ok := m.ui.Frame().Rect(p)
		switch {
		case !ok:
		case (r.Min.X+r.Max.X)/2 < e.Screen.X:
			i, placed = j+1, true
		case !placed:
			i, placed = j, true
		}
	}
	m.order = slices.Insert(others, i, id)
	return nil
}

// zoom gives pane id the full width, or gives the others theirs back.
func (m *model) zoom(id bloom.ID) tea.Cmd {
	m.zooming = true
	if m.zoomed == id {
		m.zoomed = ""
		return nil
	}
	m.zoomed = id
	return m.toast.notify("⤢ " + string(id))
}
