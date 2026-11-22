// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package wm is the desktop's window manager. It keeps the windows in the
// order they stack in, and draws each as a layer of the desktop's overlay,
// put at a rectangle it keeps with Place, so where a window was dropped is
// only state: the layers are built again from it each frame. The window on
// top has the focus.
//
// Windows are dragged by their title bars and stay where they are dropped,
// are resized from their bottom right corners, come to the front when
// clicked, minimize to the taskbar, maximize with a double click on the
// title, snap to half of the desktop when dragged against its left or right
// edge, or fill it when dragged against the top, and close. They glide with
// Animate when they are maximized, snapped, or moved with the keyboard, and
// follow the pointer cell for cell while dragged, leaving Animate off then:
// it carries on from there once the window is dropped, so a window let go
// against an edge glides into its half of the desktop.
package wm

import (
	"fmt"
	"image"
	"slices"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
)

// reach is how much of a window stays on the desktop when it is dragged off
// a side, so that its title bar can be grabbed again.
const reach = 10

// Manager is the window manager.
type Manager struct {
	st     theme.Styles
	frame  func() *bloom.Frame // the frame on screen, for where windows were drawn
	status func(string)        // reports what was done, for the taskbar to show

	w, h    int       // the size of the desktop
	windows []*Window // bottom to top: the last one is in front, with the focus
	nextID  WindowID
	grab    *grab
	preview anim.Presence   // the outline of where a dragged window would snap
	target  image.Rectangle // where the preview is, kept as it fades out
}

// New returns a window manager with no windows. frame returns the frame on
// screen, such as [bloom.Root.Frame], and status is told what the manager
// did, such as "minimized Files".
func New(frame func() *bloom.Frame, status func(string)) *Manager {
	return &Manager{
		frame:   frame,
		status:  status,
		preview: anim.NewPresence(anim.Duration(120 * time.Millisecond)),
	}
}

// SetStyles sets the styles the window manager is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (m *Manager) SetStyles(st theme.Styles) *Manager {
	m.st = st
	return m
}

// Resize sets the size of the desktop, which windows are kept on.
func (m *Manager) Resize(w, h int) { m.w, m.h = w, h }

// Update moves the windows' animations along, takes the styles of a new
// theme, and removes a closed window once it has faded out. The windows'
// content is given every message too, for it to take the new theme as well.
func (m *Manager) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		m.st = *msg.Styles
	}
	anims := []anim.Animation{&m.preview}
	var cmds []tea.Cmd
	for _, w := range m.windows {
		anims = append(anims, &w.pres)
		if cmd := w.Content.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	cmd := tea.Batch(append(cmds, anim.Update(msg, anims...))...)
	m.windows = slices.DeleteFunc(m.windows, func(w *Window) bool { return w.shut && !w.pres.Visible() })
	return cmd
}

// Key handles the keys for the window in front: the arrows move it, and
// shift with the arrows resizes it; f maximizes it, m minimizes it, and w
// closes it. Tab brings the next window to the front.
func (m *Manager) Key(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "tab" {
		return m.Cycle()
	}
	w := m.Top()
	if w == nil {
		return nil
	}
	switch msg.String() {
	case "left":
		m.nudge(w, image.Pt(-2, 0), false)
	case "right":
		m.nudge(w, image.Pt(2, 0), false)
	case "up":
		m.nudge(w, image.Pt(0, -1), false)
	case "down":
		m.nudge(w, image.Pt(0, 1), false)
	case "shift+left":
		m.nudge(w, image.Pt(-2, 0), true)
	case "shift+right":
		m.nudge(w, image.Pt(2, 0), true)
	case "shift+up":
		m.nudge(w, image.Pt(0, -1), true)
	case "shift+down":
		m.nudge(w, image.Pt(0, 1), true)
	case "f":
		m.ToggleMax(w)
	case "m":
		return m.Minimize(w)
	case "w":
		return m.Close(w)
	}
	return nil
}

// Windows returns the windows, bottom to top.
func (m *Manager) Windows() []*Window { return m.windows }

// desk is the desktop: the screen above the taskbar.
func (m *Manager) desk() image.Rectangle { return image.Rect(0, 0, m.w, m.h) }

// Top returns the window in front, which has the focus, or nil when every
// window is minimized or closing.
func (m *Manager) Top() *Window {
	for _, w := range slices.Backward(m.windows) {
		if w.pres.Shown() {
			return w
		}
	}
	return nil
}

// Find returns the open window with key, or nil.
func (m *Manager) Find(key any) *Window {
	for _, w := range m.windows {
		if w.Key == key && !w.shut {
			return w
		}
	}
	return nil
}

// Open opens window w in front of the others.
func (m *Manager) Open(w *Window) tea.Cmd {
	w.id = m.nextID
	w.pres = anim.NewPresence(anim.Duration(180 * time.Millisecond))
	m.nextID++
	if w.Cascade {
		// Each opens a step down and to the right of the last.
		w.Rect = w.Rect.Add(image.Pt(3, 1).Mul(int(m.nextID) % 4))
	}
	m.windows = append(m.windows, w)
	m.status("opened " + w.Title)
	return w.pres.Show()
}

// Raise brings w to the front.
func (m *Manager) Raise(w *Window) {
	if i := slices.Index(m.windows, w); i >= 0 && i != len(m.windows)-1 {
		m.windows = append(slices.Delete(m.windows, i, i+1), w)
	}
}

// Cycle brings the window at the back to the front, so that pressing it
// again and again goes through every window.
func (m *Manager) Cycle() tea.Cmd {
	for _, w := range m.windows {
		if w.pres.Shown() && w != m.Top() {
			m.Raise(w)
			m.status(w.Title + " is in front")
			break
		}
	}
	return nil
}

// Restore brings w back from the taskbar, if minimized, and to the front.
func (m *Manager) Restore(w *Window) tea.Cmd {
	m.Raise(w)
	if w.pres.Shown() {
		return nil
	}
	m.status("restored " + w.Title)
	return w.pres.Show()
}

// Minimize hides w, leaving its button on the taskbar.
func (m *Manager) Minimize(w *Window) tea.Cmd {
	w.slide = true
	m.status("minimized " + w.Title)
	return w.pres.Hide()
}

// Close fades w out, and removes it once it has.
func (m *Manager) Close(w *Window) tea.Cmd {
	w.shut, w.slide = true, false
	m.status("closed " + w.Title)
	return w.pres.Hide()
}

// ToggleMax maximizes w, or puts it back where it was.
func (m *Manager) ToggleMax(w *Window) {
	if w.snap != snapNone {
		w.snap = snapNone
		m.status("restored the size of " + w.Title)
		return
	}
	w.snap = snapMax
	m.status("maximized " + w.Title)
}

// Task is what a click on the taskbar button of w does: it brings back a
// minimized window, minimizes the one in front, and brings any other to the
// front.
func (m *Manager) Task(w *Window) tea.Cmd {
	if w == m.Top() {
		return m.Minimize(w)
	}
	return m.Restore(w)
}

// place returns where w goes on the desktop: snapped, or its own rectangle
// kept in reach.
func (m *Manager) place(w *Window) image.Rectangle {
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
	return m.clamp(w.Rect)
}

// clamp returns r no bigger than the desktop, with its title bar on it and
// at least reach cells of it showing, so that it can be dragged back. The
// window's own rectangle is left as it was dropped, so that a window pushed
// in by the terminal shrinking goes back out when it grows again.
func (m *Manager) clamp(r image.Rectangle) image.Rectangle {
	d := m.desk()
	size := image.Pt(min(r.Dx(), d.Dx()), min(r.Dy(), d.Dy()))
	at := image.Pt(
		min(max(r.Min.X, reach-size.X), d.Dx()-reach),
		min(max(r.Min.Y, 0), d.Dy()-1),
	)
	return image.Rectangle{Min: at, Max: at.Add(size)}
}

// nudge moves w by d, or resizes it by d, from the keyboard.
func (m *Manager) nudge(w *Window, d image.Point, resize bool) {
	r := m.place(w)
	w.snap = snapNone
	if resize {
		r.Max = r.Max.Add(d)
		w.Rect = m.clamp(w.fit(r))
		m.status(fmt.Sprintf("resized %s to %d×%d", w.Title, w.Rect.Dx(), w.Rect.Dy()))
		return
	}
	w.Rect = m.clamp(r.Add(d))
	m.status(fmt.Sprintf("moved %s to %d,%d", w.Title, w.Rect.Min.X, w.Rect.Min.Y))
}
