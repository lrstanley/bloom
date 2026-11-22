// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package wm

import (
	"fmt"
	"image"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// grab is a window being dragged by its title bar, or resized by its corner.
type grab struct {
	win   *Window
	from  image.Rectangle // where the window was when the button went down
	at    image.Point     // where the pointer was last seen
	moved bool            // the pointer has moved since
	zone  snap            // where the window goes if dropped now
}

// zoneAt returns where a window dropped with the pointer at p snaps to: the
// whole desktop against its top, and half of it against either side.
func (m *Manager) zoneAt(p image.Point) snap {
	d := m.desk()
	switch {
	case p.Y <= d.Min.Y:
		return snapMax
	case p.X <= d.Min.X:
		return snapLeft
	case p.X >= d.Max.X-1:
		return snapRight
	default:
		return snapNone
	}
}

// mover returns the drag handler of the title bar of w, which moves it by as
// much as the pointer has moved since the button went down.
func (m *Manager) mover(w *Window) on.Handler {
	return on.Drag(func(e on.Event) tea.Cmd {
		g := m.grabbed(w, &e)
		if g == nil {
			return nil
		}
		if w.snap != snapNone {
			// Dragging a maximized or snapped window tears it off at its own
			// size, under the pointer as far along its title bar as it was.
			x := e.Screen.X - (e.Start.X-g.from.Min.X)*w.Rect.Dx()/max(g.from.Dx(), 1)
			at := image.Pt(x, g.from.Min.Y).Sub(e.Screen.Sub(e.Start))
			g.from, w.snap = image.Rectangle{Min: at, Max: at.Add(w.Rect.Size())}, snapNone
		}
		// A release may be where no motion was seen, as motion is throttled.
		w.Rect = g.from.Add(e.Screen.Sub(e.Start))
		if e.Done {
			g.zone = m.zoneAt(e.Screen)
			return m.drop(false)
		}
		m.status(fmt.Sprintf("moving %s: %d,%d", w.Title, w.Rect.Min.X, w.Rect.Min.Y))
		return m.aim(m.zoneAt(e.Screen))
	})
}

// resizer returns the drag handler of the corner of w, which resizes it.
func (m *Manager) resizer(w *Window) on.Handler {
	return on.Drag(func(e on.Event) tea.Cmd {
		g := m.grabbed(w, &e)
		if g == nil {
			return nil
		}
		r := g.from
		r.Max = r.Max.Add(e.Screen.Sub(e.Start))
		w.Rect, w.snap = w.fit(r), snapNone
		if e.Done {
			return m.drop(true)
		}
		m.status(fmt.Sprintf("resizing %s: %d×%d", w.Title, w.Rect.Dx(), w.Rect.Dy()))
		return nil
	})
}

// grabbed returns the grab of w for drag event e, starting it when the
// button goes down, or nil while the window should not follow the pointer:
// not before it has moved, so that a click or a double click on a title bar
// leaves the window where it is. A drag that ends without a release, which
// was lost, ends where the pointer was last seen.
func (m *Manager) grabbed(w *Window, e *on.Event) *grab {
	g := m.grab
	if g == nil || g.win != w {
		if !e.Done {
			// Where the window was drawn, in the frame pressed on, which one
			// still gliding has not reached, to move it by as much as the
			// pointer moves.
			from, _ := m.frame().Rect(w.id)
			m.grab = &grab{win: w, from: from, at: e.Start}
		}
		return nil
	}
	if _, up := e.Msg.(tea.MouseReleaseMsg); e.Done && !up {
		e.Screen = g.at
	}
	g.at, g.moved = e.Screen, g.moved || e.Screen != e.Start
	if !g.moved {
		if e.Done {
			m.grab = nil
		}
		return nil
	}
	return g
}

// aim shows the preview of where a dragged window would snap to, or hides
// it when it would not.
func (m *Manager) aim(z snap) tea.Cmd {
	if z == m.grab.zone {
		return nil
	}
	m.grab.zone = z
	if z == snapNone {
		return m.preview.Hide()
	}
	m.target = m.place(&Window{snap: z})
	return m.preview.Show()
}

// drop ends the grab of a window that was moved, or resized: it stays where
// it was let go, snapped to the side it was dropped against, if any.
func (m *Manager) drop(resize bool) tea.Cmd {
	g := m.grab
	m.grab = nil
	w := g.win
	w.Rect = m.clamp(w.Rect)
	switch {
	case resize:
		m.status(fmt.Sprintf("resized %s to %d×%d", w.Title, w.Rect.Dx(), w.Rect.Dy()))
	case g.zone != snapNone:
		w.snap = g.zone
		m.status(fmt.Sprintf("snapped %s to %s", w.Title, g.zone))
	default:
		m.status(fmt.Sprintf("dropped %s at %d,%d", w.Title, w.Rect.Min.X, w.Rect.Min.Y))
	}
	return m.preview.Hide()
}
