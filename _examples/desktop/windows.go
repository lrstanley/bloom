// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// reach is how much of a window stays on the desktop when it is dragged off
// a side, so that its title bar can be grabbed again.
const reach = 10

// desk is the desktop: the screen above the taskbar.
func (m *model) desk() image.Rectangle { return image.Rect(0, 0, m.w, max(m.h-1, 0)) }

// top returns the window in front, which has the focus, or nil when every
// window is minimized or closing.
func (m *model) top() *window {
	for _, w := range slices.Backward(m.windows) {
		if w.pres.Shown() {
			return w
		}
	}
	return nil
}

// find returns the open window of app a, and file f for notes.
func (m *model) find(a app, f string) *window {
	for _, w := range m.windows {
		if w.app == a && w.file == f && !w.shut {
			return w
		}
	}
	return nil
}

// open opens a window of app a, showing file f for notes, or brings the one
// already open to the front.
func (m *model) open(a app, f string) tea.Cmd {
	if w := m.find(a, f); w != nil {
		return m.restore(w)
	}
	w := &window{
		id:    m.nextID,
		app:   a,
		title: a.name(),
		file:  f,
		pres:  anim.NewPresence(anim.Duration(180 * time.Millisecond)),
	}
	m.nextID++
	r := a.place()
	if a == appNotes {
		w.title = f
		w.text = bloom.NewBuffer(strings.Join(files[f], "\n"))
		// Each notes window opens a step down and to the right of the last.
		r = r.Add(image.Pt(3, 1).Mul(m.nextID % 4))
	}
	w.rect = r
	m.windows = append(m.windows, w)
	m.status = "opened " + w.title
	return w.pres.Show()
}

// raise brings w to the front.
func (m *model) raise(w *window) {
	if i := slices.Index(m.windows, w); i >= 0 && i != len(m.windows)-1 {
		m.windows = append(slices.Delete(m.windows, i, i+1), w)
	}
}

// cycle brings the window at the back to the front, so that pressing it
// again and again goes through every window.
func (m *model) cycle() tea.Cmd {
	for _, w := range m.windows {
		if w.pres.Shown() && w != m.top() {
			m.raise(w)
			m.status = w.title + " is in front"
			break
		}
	}
	return nil
}

// restore brings w back from the taskbar, if minimized, and to the front.
func (m *model) restore(w *window) tea.Cmd {
	m.raise(w)
	if w.pres.Shown() {
		return nil
	}
	m.status = "restored " + w.title
	return w.pres.Show()
}

// minimize hides w, leaving its button on the taskbar.
func (m *model) minimize(w *window) tea.Cmd {
	w.slide = true
	m.status = "minimized " + w.title
	return w.pres.Hide()
}

// close fades w out, and removes it once it has.
func (m *model) close(w *window) tea.Cmd {
	w.shut, w.slide = true, false
	m.status = "closed " + w.title
	return w.pres.Hide()
}

// toggleMax maximizes w, or puts it back where it was.
func (m *model) toggleMax(w *window) {
	if w.snap != snapNone {
		w.snap = snapNone
		m.status = "restored the size of " + w.title
		return
	}
	w.snap = snapMax
	m.status = "maximized " + w.title
}

// task is what a click on the taskbar button of w does: it brings back a
// minimized window, minimizes the one in front, and brings any other to the
// front.
func (m *model) task(w *window) tea.Cmd {
	if w == m.top() {
		return m.minimize(w)
	}
	return m.restore(w)
}

// place returns where w goes on the desktop: snapped, or its own rectangle
// kept in reach.
func (m *model) place(w *window) image.Rectangle {
	d := m.desk()
	switch w.snap {
	case snapMax:
		return d
	case snapLeft:
		return image.Rect(0, 0, d.Dx()/2, d.Dy())
	case snapRight:
		return image.Rect(d.Dx()/2, 0, d.Dx(), d.Dy())
	case snapNone:
	}
	return m.clamp(w.rect)
}

// clamp returns r no bigger than the desktop, with its title bar on it and
// at least reach cells of it showing, so that it can be dragged back. The
// window's own rectangle is left as it was dropped, so that a window pushed
// in by the terminal shrinking goes back out when it grows again.
func (m *model) clamp(r image.Rectangle) image.Rectangle {
	d := m.desk()
	size := image.Pt(min(r.Dx(), d.Dx()), min(r.Dy(), d.Dy()))
	at := image.Pt(
		min(max(r.Min.X, reach-size.X), d.Dx()-reach),
		min(max(r.Min.Y, 0), d.Dy()-1),
	)
	return image.Rectangle{Min: at, Max: at.Add(size)}
}

// nudge moves w by d, or resizes it by d, from the keyboard.
func (m *model) nudge(w *window, d image.Point, resize bool) {
	r := m.place(w)
	w.snap = snapNone
	if resize {
		r.Max = r.Max.Add(d)
		w.rect = m.clamp(w.app.fit(r))
		m.status = fmt.Sprintf("resized %s to %d×%d", w.title, w.rect.Dx(), w.rect.Dy())
		return
	}
	w.rect = m.clamp(r.Add(d))
	m.status = fmt.Sprintf("moved %s to %d,%d", w.title, w.rect.Min.X, w.rect.Min.Y)
}

// zoneAt returns where a window dropped with the pointer at p snaps to: the
// whole desktop against its top, and half of it against either side.
func (m *model) zoneAt(p image.Point) snap {
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
func (m *model) mover(w *window) on.Handler {
	return on.Drag(func(e on.Event) tea.Cmd {
		g := m.grabbed(w, &e)
		if g == nil {
			return nil
		}
		if w.snap != snapNone {
			// Dragging a maximized or snapped window tears it off at its own
			// size, under the pointer as far along its title bar as it was.
			x := e.Screen.X - (e.Start.X-g.from.Min.X)*w.rect.Dx()/max(g.from.Dx(), 1)
			at := image.Pt(x, g.from.Min.Y).Sub(e.Screen.Sub(e.Start))
			g.from, w.snap = image.Rectangle{Min: at, Max: at.Add(w.rect.Size())}, snapNone
		}
		// A release may be where no motion was seen, as motion is throttled.
		w.rect = g.from.Add(e.Screen.Sub(e.Start))
		if e.Done {
			g.zone = m.zoneAt(e.Screen)
			return m.drop(false)
		}
		m.status = fmt.Sprintf("moving %s: %d,%d", w.title, w.rect.Min.X, w.rect.Min.Y)
		return m.aim(m.zoneAt(e.Screen))
	})
}

// resizer returns the drag handler of the corner of w, which resizes it.
func (m *model) resizer(w *window) on.Handler {
	return on.Drag(func(e on.Event) tea.Cmd {
		g := m.grabbed(w, &e)
		if g == nil {
			return nil
		}
		r := g.from
		r.Max = r.Max.Add(e.Screen.Sub(e.Start))
		w.rect, w.snap = w.app.fit(r), snapNone
		if e.Done {
			return m.drop(true)
		}
		m.status = fmt.Sprintf("resizing %s: %d×%d", w.title, w.rect.Dx(), w.rect.Dy())
		return nil
	})
}

// grabbed returns the grab of w for drag event e, starting it when the
// button goes down, or nil while the window should not follow the pointer:
// not before it has moved, so that a click or a double click on a title bar
// leaves the window where it is. A drag that ends without a release, which
// was lost, ends where the pointer was last seen.
func (m *model) grabbed(w *window, e *on.Event) *grab {
	g := m.grab
	if g == nil || g.win != w {
		if !e.Done {
			// Where the window was drawn, in the frame pressed on, which one
			// still gliding has not reached, to move it by as much as the
			// pointer moves.
			from, _ := m.ui.Frame().Rect(winID(w.id))
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
func (m *model) aim(z snap) tea.Cmd {
	if z == m.grab.zone {
		return nil
	}
	m.grab.zone = z
	if z == snapNone {
		return m.preview.Hide()
	}
	m.target = m.place(&window{snap: z})
	return m.preview.Show()
}

// drop ends the grab of a window that was moved, or resized: it stays where
// it was let go, snapped to the side it was dropped against, if any.
func (m *model) drop(resize bool) tea.Cmd {
	g := m.grab
	m.grab = nil
	w := g.win
	w.rect = m.clamp(w.rect)
	switch {
	case resize:
		m.status = fmt.Sprintf("resized %s to %d×%d", w.title, w.rect.Dx(), w.rect.Dy())
	case g.zone != snapNone:
		w.snap = g.zone
		m.status = fmt.Sprintf("snapped %s to %s", w.title, g.zone)
	default:
		m.status = fmt.Sprintf("dropped %s at %d,%d", w.title, w.rect.Min.X, w.rect.Min.Y)
	}
	return m.preview.Hide()
}

func (s snap) String() string {
	switch s {
	case snapMax:
		return "the whole desktop"
	case snapLeft:
		return "the left half"
	case snapRight:
		return "the right half"
	case snapNone:
	}
	return "nothing"
}
