// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command showcase puts much of bloom on one screen, colored with
// bubbletint themes. Choosing a tint in the sidebar recolors everything: the
// panes wave back in with the new colors, and a toast decodes the tint's
// name. A pane dragged by its title floats under the pointer while the
// others fly out of its way, and a double click on the title zooms the pane
// to the full width. As the terminal narrows, panes collapse by
// priority, and a tour spotlights each part of the screen in turn.
//
// Keys: t and T cycle the tints, ? steps the tour, esc ends it, and q or
// ctrl+c quits. Everything else is the mouse.
package main

import (
	"fmt"
	"image"
	"os"
	"slices"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// tints are the themes the sidebar offers.
var tints = []*tint.Tint{
	tint.TintTokyoNight,
	tint.TintRosePine,
	tint.TintCatppuccinMocha,
	tint.TintKanagawaWave,
	tint.TintDracula,
	tint.TintNord,
}

// paneID is the ID of a pane, for layout animation, the tour, and telling
// which pane a dragged title is over.
type paneID string

const (
	paneTints    paneID = "Tints"
	paneActivity paneID = "Activity"
	panePalette  paneID = "Palette"
	paneServices paneID = "Services"
)

// tourSteps are what the tour says, about the pane it spotlights.
var tourSteps = []struct {
	pane        paneID
	title, text string
}{
	{paneTints, "Tints", "Every color is from a bubbletint theme. Click one to switch."},
	{paneActivity, "Panes", "Drag a title to move a pane. Double click to zoom."},
	{paneServices, "Responsive", "Narrow the terminal and panes collapse, lowest priority first."},
}

type model struct {
	ui   *bloom.Root
	w, h int
	tint int
	st   styles

	order    []paneID // the panes right of the sidebar, left to right.
	zoomed   paneID   // the pane taking the full width, if any.
	zooming  bool     // the panes glide, through a zoom, rather than fly.
	selected int      // the selected service.

	// The pane being dragged by its title, if any: where it was grabbed,
	// its size when picked up, and where the pointer is.
	dragging       paneID
	grab, size, at image.Point

	bars anim.Sequence // loops, moving the activity bars.
	wave anim.Value    // the panes' entrance, played again for each tint.

	toast     anim.Presence
	toastText anim.Value // decodes the toast's text.
	note      string
	notes     uint32 // how many toasts there have been, to vary the decode.

	step     int // the tour step, from 1, kept while the tour fades out.
	tourOn   anim.Presence
	tourText anim.Value
}

func newModel() *model {
	// Effects that play once run from 0 to 1 at a steady pace, and rest at
	// 1, where they have finished.
	effect := func(d time.Duration) anim.Value {
		return anim.NewValue(1, anim.Duration(d), anim.Ease(anim.Linear))
	}
	m := &model{
		order:     []paneID{paneActivity, panePalette, paneServices},
		bars:      anim.NewLoop(6 * time.Second),
		wave:      effect(500 * time.Millisecond),
		toast:     anim.NewPresence(anim.Spring(9, 0.6)),
		toastText: effect(700 * time.Millisecond),
		tourOn:    anim.NewPresence(anim.Duration(250 * time.Millisecond)),
		tourText:  effect(800 * time.Millisecond),
	}
	m.ui = bloom.New(bloom.WithColors(tints[0].Fg, tints[0].Bg))
	m.st = newStyles(tints[0])
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the tint's colors are used instead.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.bars.Loop(), m.wave.Play(0, 1))
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, anim.Update(msg, &m.bars, &m.wave, &m.toast, &m.toastText, &m.tourOn, &m.tourText))
	return m, tea.Batch(cmds...)
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "t":
		return m.switchTint(m.tint + 1)
	case "T", "shift+t":
		return m.switchTint(m.tint - 1)
	case "?":
		if !m.tourOn.Shown() {
			m.step = 0
		}
		m.step++
		if m.step > len(tourSteps) {
			return m.tourOn.Hide()
		}
		return tea.Batch(m.tourOn.Show(), m.tourText.Play(0, 1))
	case "esc":
		return m.tourOn.Hide()
	}
	return nil
}

// switchTint changes to tint i, wrapping around, waving the panes back in
// with its colors.
func (m *model) switchTint(i int) tea.Cmd {
	m.tint = (i + len(tints)) % len(tints)
	t := tints[m.tint]
	m.st = newStyles(t)
	// The view gives the terminal the tint's colors. The root is told, as
	// the terminal would tell it, for effects such as the tour's spotlight
	// that blend toward them.
	return tea.Batch(
		m.ui.Update(tea.ForegroundColorMsg{Color: t.Fg}),
		m.ui.Update(tea.BackgroundColorMsg{Color: t.Bg}),
		m.wave.Play(0, 1),
		m.notify("✦ "+t.DisplayName),
	)
}

// notify shows text in the toast, decoding it into place.
func (m *model) notify(text string) tea.Cmd {
	m.note = text
	m.notes++
	return tea.Batch(m.toast.ShowFor(2*time.Second), m.toastText.Play(0, 1))
}

// drag lifts pane id out of the row once its title is dragged, to float
// under the pointer, and moves its place in the row to wherever the pointer
// is. The other panes glide out of the way, and when it is dropped, it
// glides into its place.
func (m *model) drag(id paneID, e on.Event) tea.Cmd {
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
	others := slices.DeleteFunc(slices.Clone(m.order), func(p paneID) bool { return p == id })
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
func (m *model) zoom(id paneID) tea.Cmd {
	m.zooming = true
	if m.zoomed == id {
		m.zoomed = ""
		return nil
	}
	m.zoomed = id
	return m.notify("⤢ " + string(id))
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFPS(60), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
