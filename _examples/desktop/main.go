// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command desktop is a small desktop environment. Its windows are dragged by
// their title bars and stay where they are dropped, are resized from their
// bottom right corners, come to the front when clicked, minimize to a
// taskbar, maximize with a double click on the title, snap to half of the
// screen when dragged against its left or right edge, or fill it when dragged
// against the top, and close. It starts up behind a splash, has a start menu
// on the taskbar to open apps from and to log out, which shuts it down behind
// the splash again, and a settings app that changes its theme.
//
// Each part of the desktop is a component of its own, in internal:
//
//   - theme: the colors and styles, made from a tint. The model and every
//     component keep a copy, and a theme.ChangeMsg, passed down through
//     Update, gives each the new ones.
//   - wm: the window manager, which keeps the windows, draws each as a layer
//     of the desktop's overlay, and moves, resizes, and snaps them.
//   - apps: the apps the windows show, and the desktop's icons.
//   - calc: the calculator app.
//   - taskbar and startmenu: the bar along the bottom, and its start menu.
//   - splash: the splash, and the desktop turning on and off behind it.
//
// This file has the model, which keeps the components, places them in
// View, and handles what they ask for with messages, such as an app to
// launch or a theme to change to.
//
// Keys: tab brings the next window to the front; the arrows move the window
// in front, and shift with the arrows resizes it; f maximizes it, m minimizes
// it, and w closes it; 1 to 4 open the apps; q logs out.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/apps"
	"bloomfx.dev/bloom/_examples/desktop/internal/calc"
	"bloomfx.dev/bloom/_examples/desktop/internal/splash"
	"bloomfx.dev/bloom/_examples/desktop/internal/startmenu"
	"bloomfx.dev/bloom/_examples/desktop/internal/taskbar"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/_examples/desktop/internal/wm"
	tea "charm.land/bubbletea/v2"
)

type model struct {
	ui   *bloom.Root
	st   theme.Styles
	w, h int

	splash *splash.Splash
	wm     *wm.Manager
	icons  *apps.Icons
	menu   *startmenu.Menu
	bar    *taskbar.Taskbar
	// calc is the one calculator, which keeps its sum while its window is
	// closed.
	calc *calc.Calc

	paper string  // the wallpaper, drawn for the desktop's size
	start tea.Cmd // opens the first windows
}

// windowKey tells the windows apart, for an app, or a file, to open in one
// window only.
type windowKey struct {
	app  apps.App
	file string // the file a notes window has open
}

func newModel() *model {
	m := &model{st: theme.NewStyles(theme.Tints[0])}
	m.ui = bloom.New(bloom.WithColors(m.st.Tint.Fg, m.st.Tint.Bg))
	m.splash = splash.New().SetStyles(m.st)
	m.wm = wm.New(m.ui.Frame, m.say).SetStyles(m.st)
	m.icons = apps.NewIcons().SetStyles(m.st)
	m.menu = startmenu.New().SetStyles(m.st)
	m.bar = taskbar.New(m.wm, m.menu).SetStyles(m.st)
	m.calc = calc.New() // given its styles as its window opens

	m.start = tea.Batch(m.open(apps.Files, ""), m.open(apps.Notes, "README.md"), m.open(apps.Calc, ""))
	m.wm.Raise(m.wm.Windows()[1])
	m.say("drag a title to move a window, or its ◢ to resize it; tab cycles; q logs out")
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return tea.Batch(m.start, m.splash.Start(), m.bar.Tick()) }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// The desktop is the screen above the taskbar.
		dw, dh := m.w, max(m.h-1, 0)
		m.wm.Resize(dw, dh)
		m.paper = wallpaper(dw, dh)
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	case apps.LaunchMsg:
		cmds = append(cmds, m.launch(msg.App))
	case apps.OpenMsg:
		cmds = append(cmds, m.open(apps.Notes, msg.File))
	case theme.ChangeMsg:
		cmds = append(cmds, m.setTheme(*msg.Styles))
	case startmenu.LogoutMsg:
		cmds = append(cmds, m.logout())
	}

	// Each component takes every message too, for its animations, and for
	// the styles of a new theme.
	cmds = append(cmds,
		m.splash.Update(msg), m.wm.Update(msg), m.icons.Update(msg), m.menu.Update(msg), m.bar.Update(msg),
	)
	if m.splash.Done() {
		return m, tea.Quit
	}
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	// The desktop, its icons and windows over the wallpaper, above the
	// taskbar, behind the splash.
	v := m.ui.View(
		m.w, m.h,
		m.splash.Screen(func() bloom.Child {
			return bloom.Column(
				bloom.Fill(bloom.Overlay(
					bloom.Text(m.paper).Style(m.st.Paper),
					append([]bloom.LayerNode{m.icons.Layer()}, m.wm.Layers()...)...,
				)),
				bloom.Fixed(1, m.bar),
			)
		}),
	)
	v.AltScreen = true
	v.WindowTitle = "desktop"
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.Tint.Bg
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	if m.splash.Off() && msg.String() != "ctrl+c" {
		return nil
	}
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "q":
		return m.logout()
	case "1", "2", "3", "4":
		return m.launch(apps.All[msg.String()[0]-'1'])
	}
	return m.wm.Key(msg)
}

// say shows what was done last on the taskbar.
func (m *model) say(status string) { m.bar.Status = status }

// launch opens app a, and the README for notes.
func (m *model) launch(a apps.App) tea.Cmd {
	if a == apps.Notes {
		return m.open(a, "README.md")
	}
	return m.open(a, "")
}

// open opens a window of app a, showing file f for notes, or brings the one
// already open to the front.
func (m *model) open(a apps.App, f string) tea.Cmd {
	key := windowKey{app: a, file: f}
	if w := m.wm.Find(key); w != nil {
		return m.wm.Restore(w)
	}
	w := &wm.Window{Key: key, Title: a.Name(), Glyph: a.Glyph(), Rect: a.Place(), Min: a.Min()}
	switch a {
	case apps.Files:
		w.Content = apps.NewFileList().SetStyles(m.st)
	case apps.Notes:
		notes := apps.NewNotepad(f).SetStyles(m.st)
		w.Title, w.Content, w.Scroll, w.Cascade = f, notes, notes.ScrollID(), true
	case apps.Calc:
		// The calculator is given the styles each time its window opens, as it
		// misses any theme change while its window is closed.
		w.Content = m.calc.SetStyles(m.st)
	case apps.Settings:
		w.Content = apps.NewSettings().SetStyles(m.st)
	}
	return m.wm.Open(w)
}

// setTheme takes the styles of a new theme, which Update then gives each
// component too, and tells the root the theme's colors, for the effects that
// blend them.
func (m *model) setTheme(st theme.Styles) tea.Cmd {
	m.st = st
	t := st.Tint
	m.say("changed the theme to " + t.DisplayName)
	return tea.Batch(m.ui.Update(tea.BackgroundColorMsg{Color: t.Bg}), m.ui.Update(tea.ForegroundColorMsg{Color: t.Fg}))
}

// logout shuts the desktop down: it shades out to the splash, which runs
// back down, and the program quits once it has.
func (m *model) logout() tea.Cmd {
	if m.splash.Off() {
		return nil
	}
	return tea.Batch(m.menu.Hide(), m.splash.Shutdown())
}

// wallpaper is a w×h field of dots, for the desktop's background.
func wallpaper(w, h int) string {
	var b strings.Builder
	for y := range h {
		for x := range w {
			if (x+y*3)%8 == 0 && y%2 == 0 {
				b.WriteString("·")
			} else {
				b.WriteByte(' ')
			}
		}
		if y < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFilter(bloom.ThrottleMouse(10*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
