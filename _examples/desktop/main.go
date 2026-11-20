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
// Each window is a layer of one overlay, put at a rectangle in the model
// with Place, so where a window was dropped is only state: the tree is built
// again from the model each frame. The layers are in the order
// the windows stack in, and the window on top has the focus. Windows glide
// with Animate when they are maximized, snapped, or moved with the keyboard,
// and follow the pointer cell for cell while dragged, leaving Animate off
// then: it carries on from there once the window is dropped, so a window let
// go against an edge glides into its half of the screen.
//
// Keys: tab brings the next window to the front; the arrows move the window
// in front, and shift with the arrows resizes it; f maximizes it, m minimizes
// it, and w closes it; 1 to 4 open the apps; q logs out.
package main

import (
	"fmt"
	"image"
	"os"
	"slices"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the desktop's colors come from, which the
// settings app changes.
var theme *tint.Tint

// snap is where a window is held, rather than at its own rectangle.
type snap int

const (
	snapNone  snap = iota
	snapMax        // the whole desktop
	snapLeft       // the left half of the desktop
	snapRight      // the right half
)

// window is a window on the desktop. Its rectangle is relative to the
// desktop, which is the screen less the taskbar, and is where it goes back
// to when it is no longer maximized or snapped.
type window struct {
	id    int
	app   app
	title string
	rect  image.Rectangle
	snap  snap
	shut  bool          // closing, and removed once it has faded out
	slide bool          // it minimizes and comes back by sliding to and from the taskbar
	pres  anim.Presence // hidden when closing, or minimized to its taskbar button

	// What the window's app shows.
	file     string            // the file a notes window has open
	text     *bloom.Buffer     // its text
	at       bloom.ViewportPos // where it is scrolled to
	selected int               // the file selected in the files window
}

// grab is a window being dragged by its title bar, or resized by its corner.
type grab struct {
	win   *window
	from  image.Rectangle // where the window was when the button went down
	at    image.Point     // where the pointer was last seen
	moved bool            // the pointer has moved since
	zone  snap            // where the window goes if dropped now
}

type model struct {
	ui      *bloom.Root
	w, h    int
	windows []*window // bottom to top: the last one is in front, with the focus
	nextID  int
	icon    app // the desktop icon selected, or -1 for none
	grab    *grab
	preview anim.Presence   // the outline of where a dragged window would snap
	target  image.Rectangle // where the preview is, kept as it fades out
	calc    calc
	paper   string // the wallpaper, drawn for the desktop's size
	status  string
	start   tea.Cmd       // opens the first windows
	menu    anim.Presence // the start menu

	// power runs from 0 to 1 as the splash starts up, and on to poweredOn as
	// the desktop turns on, and back down to 0 once off, to shut down.
	power anim.Value

	now func() time.Time // the time the taskbar's clock shows
}

// poweredOn is where power rests while the desktop is on. Past 1, the
// desktop turns on, which takes a fifth of the time power takes to get here.
const poweredOn = 1.25

// clockMsg redraws the taskbar's clock, as the minute it shows turns over.
type clockMsg struct{}

// themeMsg tells the root the colors of the theme, once it has changed, for
// the effects that blend them.
type themeMsg struct{}

// tick returns the command for the next clockMsg, at the start of the next
// minute.
func (m *model) tick() tea.Cmd {
	now := m.now()
	return tea.Tick(now.Truncate(time.Minute).Add(time.Minute).Sub(now), func(time.Time) tea.Msg { return clockMsg{} })
}

func newModel() *model {
	setTheme(tint.TintRosePineMoon)
	m := &model{
		ui:      bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		preview: anim.NewPresence(anim.Duration(120 * time.Millisecond)),
		menu:    anim.NewPresence(anim.Duration(150 * time.Millisecond)),
		power:   anim.NewValue(0, anim.Duration(2*time.Second), anim.Ease(anim.Linear)),
		now:     time.Now,
		icon:    -1,
		calc:    newCalc(),
	}
	m.start = tea.Batch(m.open(appFiles, ""), m.open(appNotes, "README.md"), m.open(appCalc, ""))
	m.raise(m.windows[1])
	m.status = "drag a title to move a window, or its ◢ to resize it; tab cycles; q logs out"
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return tea.Batch(m.start, m.power.To(poweredOn), m.tick()) }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		d := m.desk()
		m.paper = wallpaper(d.Dx(), d.Dy())
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	case clockMsg:
		cmds = append(cmds, m.tick())
	case themeMsg:
		cmds = append(cmds, m.ui.Update(tea.BackgroundColorMsg{Color: theme.Bg}), m.ui.Update(tea.ForegroundColorMsg{Color: theme.Fg}))
	}

	anims := []anim.Animation{&m.preview, &m.menu, &m.power}
	for _, w := range m.windows {
		anims = append(anims, &w.pres)
	}
	cmds = append(cmds, anim.Update(msg, anims...))

	// A closed window goes once it has faded out.
	m.windows = slices.DeleteFunc(m.windows, func(w *window) bool { return w.shut && !w.pres.Visible() })
	if m.off() && m.power.Float() <= 0 {
		return m, tea.Quit
	}
	return m, tea.Batch(cmds...)
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	if m.off() && msg.String() != "ctrl+c" {
		return nil
	}
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "q":
		return m.logout()
	case "tab":
		return m.cycle()
	case "1", "2", "3", "4":
		return m.launch(apps[msg.String()[0]-'1'])
	}

	w := m.top()
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
		m.toggleMax(w)
	case "m":
		return m.minimize(w)
	case "w":
		return m.close(w)
	}
	return nil
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.screen())
	v.AltScreen = true
	v.WindowTitle = "desktop"
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	return v
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithFilter(bloom.ThrottleMouse(10*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
