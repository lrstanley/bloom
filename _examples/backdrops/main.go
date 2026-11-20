// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command backdrops shows every backdrop effect. Behind the screen, an
// ambient effect draws a moving texture of glyphs into its empty cells:
// rings that ripple out from clicks and from the pointer as it moves,
// patches of glyphs that drift and gather around the pointer, or falling
// rain, all on one anim.Ticker. Over it, a dialog's backdrop changes the
// screen beneath it, fading in and out with the dialog: dimmed,
// desaturated, recolored, spotlit, redrawn in braille, or with an ambient
// effect of its own.
//
// Keys: tab and the arrows switch the screen's backdrop, b the dialog's, d
// opens and closes the dialog, space pauses, q quits. Click anywhere to drop
// a ripple.
package main

import (
	"fmt"
	"image"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

var theme = tint.TintTokyoNight

var (
	surface = lipgloss.Darken(theme.Bg, 0.25)
	muted   = theme.BrightBlack

	card     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).BorderBackground(surface).Background(surface).Padding(1, 3)
	title    = lipgloss.NewStyle().Bold(true).Foreground(theme.Purple).Background(surface)
	text     = lipgloss.NewStyle().Foreground(theme.Fg).Background(surface)
	dim      = lipgloss.NewStyle().Foreground(muted).Background(surface)
	label    = dim.Width(8)
	item     = lipgloss.NewStyle().Foreground(theme.Fg).Background(surface).Padding(0, 1)
	selected = item.Foreground(theme.Bg).Background(theme.Cyan).Bold(true)
	picked   = item.Foreground(theme.Bg).Background(theme.Purple).Bold(true)
	hover    = item.Background(lipgloss.Lighten(surface, 0.12))
	status   = lipgloss.NewStyle().Foreground(muted)
	dialog   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Purple).BorderBackground(surface).Background(surface).Padding(1, 4)
)

// panelID names the panel, which the spotlight backdrop lights.
type panelID struct{}

// backdrop is one of the backdrops the example switches between.
type backdrop struct {
	name, about string
	effects     func(m *model) []fx.Effect
}

// screens are the ambient effects drawn behind the whole screen.
var screens = []backdrop{
	{
		name:  "Ripple",
		about: "Rings of glyphs ripple out from each click, and from the\npointer as it moves. Drops of rain keep it moving.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Rain: 0.7, Color: theme.Cyan}}
		},
	},
	{
		name:  "Drift",
		about: "Faint patches of glyphs drift and fade, gathering around\nthe pointer. Clicks still ripple through them.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{
				fx.Drift{Time: m.clock.Elapsed(), Focus: m.pointer, Radius: m.lens(), Color: theme.Fg},
				fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Color: theme.Fg},
			}
		},
	},
	{
		name:  "Rain",
		about: "Streams of glyphs fall, each led by one that scrambles\nas it falls, and trailed by glyphs that settle.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{
				fx.Rain{Time: m.clock.Elapsed(), Color: theme.Green, Head: theme.Fg},
				fx.Ripple{Time: m.clock.Elapsed(), Drops: m.drops, Color: theme.Green},
			}
		},
	},
}

// dialogs are the backdrops of the dialog, which change the screen beneath
// it.
var dialogs = []backdrop{
	{
		name:  "Dim",
		about: "fx.Dim moves colors toward the terminal's background.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Dim{Amount: 0.6}}
		},
	},
	{
		name:  "Desaturate",
		about: "fx.Desaturate moves colors toward gray.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Desaturate{Amount: 1}}
		},
	},
	{
		name:  "Tint",
		about: "fx.Recolor, with the same color for both, tints\neverything toward it.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Recolor{Fg: theme.Purple, Bg: theme.Purple, Amount: 0.45}}
		},
	},
	{
		name:  "Two colors",
		about: "fx.Recolor, all the way, draws every cell in two colors.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Recolor{Fg: lipgloss.Lighten(theme.Bg, 0.17), Bg: lipgloss.Darken(theme.Bg, 0.3)}}
		},
	},
	{
		name:  "Spotlight",
		about: "fx.Spotlight dims everything but the panel, with a\nsoft edge.",
		effects: func(m *model) []fx.Effect {
			r, ok := m.ui.Frame().Rect(panelID{})
			if !ok {
				return nil
			}
			return []fx.Effect{fx.Spotlight{Rect: r, Soft: 4}}
		},
	},
	{
		name:  "Braille",
		about: "fx.Braille redraws everything as braille dots, which\nchange as what is beneath them does.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Braille{Seed: 7}}
		},
	},
	{
		name:  "Braille tint",
		about: "fx.Braille draws in grays, and an fx.Recolor after it\ntints them.",
		effects: func(*model) []fx.Effect {
			return []fx.Effect{fx.Braille{Amount: 0.5, Seed: 7}, fx.Recolor{Fg: theme.Purple, Amount: 0.3}}
		},
	},
	{
		name:  "Dim + Drift",
		about: "fx.Dim, with an ambient fx.Drift drawn over it, both\nfading in with the dialog.",
		effects: func(m *model) []fx.Effect {
			return []fx.Effect{fx.Dim{Amount: 0.6}, fx.Drift{Time: m.clock.Elapsed(), Color: theme.Purple}}
		},
	},
}

type model struct {
	ui      *bloom.Root
	w, h    int
	clock   anim.Ticker
	drops   fx.Drops
	pointer image.Point // where the pointer is, and where it last left a drop
	trail   image.Point
	hovered bool // whether the pointer has moved since starting
	screen  int  // index into screens
	backed  int  // index into dialogs
	dialog  anim.Presence
}

func newModel() *model {
	return &model{
		ui:     bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		clock:  anim.NewTicker(),
		dialog: anim.NewPresence(anim.Duration(350 * time.Millisecond)),
	}
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.clock.Start() }

// lens is the radius the drift gathers in around the pointer, once it has
// moved.
func (m *model) lens() int {
	if !m.hovered {
		return 0
	}
	return 16
}

// showDialog opens the dialog with backdrop i.
func (m *model) showDialog(i int) tea.Cmd {
	m.backed = i
	return m.dialog.Show()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.MouseClickMsg:
		m.drops.Add(fx.Drop{At: image.Pt(msg.Mouse().X, msg.Mouse().Y), Time: m.clock.Elapsed()})
	case tea.MouseMotionMsg:
		m.pointer, m.hovered = image.Pt(msg.Mouse().X, msg.Mouse().Y), true
		// The pointer leaves small drops behind it as it moves.
		if d := m.pointer.Sub(m.trail); d.X*d.X+4*d.Y*d.Y >= 25 {
			m.drops.Add(fx.Drop{At: m.pointer, Time: m.clock.Elapsed(), Size: 0.3})
			m.trail = m.pointer
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l", "down", "j":
			m.screen = (m.screen + 1) % len(screens)
		case "shift+tab", "left", "h", "up", "k":
			m.screen = (m.screen + len(screens) - 1) % len(screens)
		case "b":
			// The first press opens the dialog; the rest switch its backdrop.
			next := m.backed
			if m.dialog.Shown() {
				next = (next + 1) % len(dialogs)
			}
			cmds = append(cmds, m.showDialog(next))
		case "d":
			cmds = append(cmds, m.dialog.Toggle())
		case "esc":
			cmds = append(cmds, m.dialog.Hide())
		case "space":
			cmds = append(cmds, m.clock.Toggle())
		}
	}
	return m, tea.Batch(append(cmds, anim.Update(msg, &m.clock, &m.dialog))...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.BackgroundColor = theme.Bg
	// The clock slows while the terminal is not focused.
	v.ReportFocus = true
	// The pointer is followed everywhere, not just over hoverable nodes.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// tabs is a row of clickable backdrop names after name, with current drawn
// in style.
func tabs(name string, list []backdrop, current int, style lipgloss.Style, pick func(i int) tea.Cmd) bloom.Child {
	items := make([]bloom.Child, 0, len(list)+1)
	items = append(items, bloom.Text(name).Style(label))
	for i, d := range list {
		s := item
		if i == current {
			s = style
		}
		items = append(items, bloom.Text(d.name).Style(s).HoverStyle(hover).On(on.Click(func() tea.Cmd { return pick(i) })))
	}
	return bloom.Row(items...).Gap(1)
}

func (m *model) layout() bloom.Child {
	screen, backed := screens[m.screen], dialogs[m.backed]

	panel := bloom.Box(card, bloom.Column(
		bloom.Text("bloom · backdrops").Style(title),
		bloom.Text(" ").Style(text),
		tabs("Screen", screens, m.screen, selected, func(i int) tea.Cmd {
			m.screen = i
			return nil
		}),
		tabs("Dialog", dialogs, m.backed, picked, m.showDialog),
		bloom.Text(" ").Style(text),
		bloom.Text(screen.about).Style(text),
		bloom.Text(" ").Style(text),
		bloom.Text("click anywhere to drop a ripple, or a dialog backdrop to open it").Style(dim),
	)).ID(panelID{})

	state := "running"
	if !m.clock.Animating() {
		state = "paused"
	}
	hints := fmt.Sprintf(" %s · tab: screen backdrop · b: dialog backdrop · d: dialog · space: pause (%s) · q: quit",
		screen.name, state)
	page := bloom.Column(
		bloom.Fill(bloom.Centered(panel)),
		bloom.Text(hints).Style(status).Truncate(),
	)

	// The screen's backdrop draws into the empty cells of the whole screen,
	// behind the panel, whose own background keeps it out.
	base := bloom.Transition(1, page, screen.effects(m)...)

	// The dialog sits below the panel, so its backdrop shows on the panel.
	modal := bloom.Layer(bloom.Box(dialog, bloom.Column(
		bloom.Text("Backdrop: "+backed.name).Style(title),
		bloom.Text(" ").Style(text),
		bloom.Text(backed.about).Style(text),
		bloom.Text(" ").Style(text),
		bloom.Text("b: next backdrop · esc or d closes it").Style(dim),
	))).Modal().
		Anchor(bloom.Center, bloom.End).
		Margin(2).
		Backdrop(backed.effects(m)...).
		Presence(m.dialog, fx.Fade{}).
		On(on.ClickOutside(func() tea.Cmd { return m.dialog.Hide() }))

	return bloom.Overlay(base, modal)
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFPS(60), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
