// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package taskbar is the bar along the bottom of the desktop: the start
// button, a button for each window, the status, and a clock.
package taskbar

import (
	"slices"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/_examples/desktop/internal/wm"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// clockMsg redraws the clock, as the minute it shows turns over.
type clockMsg struct{}

// Taskbar is the taskbar. A click on a window's button is what
// [wm.Manager.Task] does.
type Taskbar struct {
	st    theme.Styles
	wm    *wm.Manager
	start bloom.Child // the start button

	Status string           // what was done last
	Now    func() time.Time // the time the clock shows
}

// New returns a taskbar for the windows of mgr, with start as its start
// button.
func New(mgr *wm.Manager, start bloom.Child) *Taskbar {
	return &Taskbar{wm: mgr, start: start, Now: time.Now}
}

// SetStyles sets the styles the taskbar is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (t *Taskbar) SetStyles(st theme.Styles) *Taskbar {
	t.st = st
	return t
}

// Tick returns the command that redraws the clock at the start of the next
// minute.
func (t *Taskbar) Tick() tea.Cmd {
	now := t.Now()
	return tea.Tick(now.Truncate(time.Minute).Add(time.Minute).Sub(now), func(time.Time) tea.Msg { return clockMsg{} })
}

// Update keeps the clock ticking, and takes the styles of a new theme.
func (t *Taskbar) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		t.st = *msg.Styles
	}
	if _, ok := msg.(clockMsg); ok {
		return t.Tick()
	}
	return nil
}

func (t *Taskbar) Layout(bloom.Context) bloom.Child {
	items := []bloom.Child{t.start}
	top := t.wm.Top()
	// The buttons stay in the order the windows opened in, rather than the
	// order they stack in, so that they don't move under the pointer.
	byID := slices.SortedFunc(slices.Values(t.wm.Windows()), func(a, b *wm.Window) int { return int(a.ID() - b.ID()) })
	for _, w := range byID {
		if w.Closing() {
			continue
		}
		st := t.st.Task
		switch {
		case w == top:
			st = t.st.TaskOn
		case !w.Shown():
			st = t.st.TaskMin
		}
		items = append(items, bloom.Text(" "+w.Glyph+" "+w.Title+" ").Style(st).HoverStyle(t.st.TaskOver).
			On(on.Click(func() tea.Cmd { return t.wm.Task(w) })))
	}
	items = append(items,
		bloom.Fill(bloom.Text(t.Status).Style(t.st.Status).Truncate()),
		bloom.Text(" "+t.Now().Format(time.Kitchen)+" ").Style(t.st.Clock),
	)
	return bloom.Box(t.st.Bar, bloom.Row(items...).Gap(1))
}
