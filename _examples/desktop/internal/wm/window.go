// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package wm

import (
	"image"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
)

// Window is a window on the desktop. The fields are set by whoever opens
// it, before giving it to [Manager.Open]; the rest is the manager's.
type Window struct {
	// Key tells the window apart from others, for [Manager.Find], such as
	// the app and the file it shows. It must be comparable.
	Key any

	Title   string  // the window's name, on its title bar and taskbar button
	Glyph   string  // the app's icon, beside the title
	Content Content // what it shows inside its frame

	// Rect is where the window first opens, relative to the desktop, which
	// is the screen less the taskbar. Once open, it is where the window goes
	// back to when it is no longer maximized or snapped.
	Rect image.Rectangle
	Min  image.Point // the smallest it can be resized to
	// Cascade opens the window a step down and to the right of the last
	// window opened.
	Cascade bool
	// Scroll is the ID of a viewport in Content, for a scrollbar on the
	// window's right edge, or "" for none.
	Scroll bloom.ID

	id    bloom.ID
	seq   int // the order it opened in, from 0
	snap  snap
	shut  bool          // closing, and removed once it has faded out
	slide bool          // it minimizes and comes back by sliding to and from the taskbar
	pres  anim.Presence // hidden when closing, or minimized to its taskbar button
}

// Content is what a window shows: a component that lays itself out, and
// takes messages, such as the new styles of a [theme.ChangeMsg].
type Content interface {
	bloom.Child
	Update(msg tea.Msg) tea.Cmd
}

// ID returns the ID of the window's layer, which it keeps as it moves up and
// down the stack, for Animate to follow it. The nodes in the window are
// known by where they are in it, so a press on a button of a window behind
// the others, which brings the window to the front, still clicks it.
func (w *Window) ID() bloom.ID { return w.id }

// Seq returns the order the window opened in: a window opened later has a
// greater one.
func (w *Window) Seq() int { return w.seq }

// Shown reports whether the window is open, rather than minimized or
// closing.
func (w *Window) Shown() bool { return w.pres.Shown() }

// Closing reports whether the window was closed, and is fading out.
func (w *Window) Closing() bool { return w.shut }

// fit returns r grown to the window's smallest size, from its top left
// corner.
func (w *Window) fit(r image.Rectangle) image.Rectangle {
	r.Max = image.Pt(max(r.Max.X, r.Min.X+w.Min.X), max(r.Max.Y, r.Min.Y+w.Min.Y))
	return r
}

// snap is where a window is held, rather than at its own rectangle.
type snap int

const (
	snapNone  snap = iota
	snapMax        // the whole desktop
	snapLeft       // the left half of the desktop
	snapRight      // the right half
)

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
