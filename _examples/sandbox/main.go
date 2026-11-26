// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command sandbox is a falling-sand toy drawn with bloom.Drawable. The
// world is a grid of pixels, two to a cell, that a uv.Drawable paints with
// half blocks straight onto the frame, every cell, every frame. Pour rainbow
// sand, water, stone, and lava with the mouse: sand piles up in bands of
// color, water spreads, and lava meeting water turns to stone and steam.
//
// The world takes its size from where it was drawn, read back from the
// frame in Update, and its mouse handlers are on the Drawable itself, with
// the pointer given relative to it. A sidebar meters what a frame costs: a
// step of the simulation, the world's Draw, and the whole frame, with a
// second Drawable graphing the frame times.
//
// Keys: 1-5 pick a material, [ and ] or the wheel size the brush, r makes it
// rain, c clears, space pauses, q quits. Drag over the world to pour.
package main

import (
	"fmt"
	"image"
	"os"
	"strconv"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// fps is how many frames a second the example draws: the most Bubble Tea
// draws, which a frame of the whole world fits in many times over.
const fps = 120

// stepRate is how many steps the simulation takes a second, whatever the
// frame rate. Gravity and drift are counted in steps, so more steps would
// make everything fall faster, not more smoothly.
const stepRate = 60

// worldID is the ID of the world's Drawable, which the world reads its size
// from.
const worldID bloom.ID = "world"

// tools are the materials to pour, in the order of their keys.
var tools = []struct {
	name string
	m    material
}{
	{"Sand", sand},
	{"Water", water},
	{"Stone", stone},
	{"Lava", lava},
	{"Eraser", empty},
}

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	world   *world
	clock   anim.Ticker
	steps   int // the steps taken, stepRate for each second of the clock
	tool    int // index into tools
	radius  int
	pointer brush  // the pointer over the world, in pixels
	pouring stroke // the left button, which pours the material picked
	erasing stroke // the right button, which erases
	raining bool
	poured  int // steps spent pouring, which turn sand's hue

	perf  perf
	times []time.Duration // scratch for the frame time graph
}

func newModel() *model {
	m := &model{
		st:     newStyles(tint.TintRosePine),
		world:  newWorld(1),
		clock:  anim.NewTicker(anim.FPS(fps)),
		radius: 3,
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return m.clock.Start() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch k := msg.String(); k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1", "2", "3", "4", "5":
			m.tool = int(k[0] - '1')
		case "[":
			m.resize(-1)
		case "]":
			m.resize(1)
		case "r":
			m.raining = !m.raining
		case "c":
			m.world.clear()
		case "space":
			cmds = append(cmds, m.clock.Toggle())
		}
	}
	cmds = append(cmds, anim.Update(msg, &m.clock))
	m.fit()
	m.advance()
	return m, tea.Batch(cmds...)
}

// fit sizes the world to the cells its Drawable was given in the last
// frame: the first frame draws it at nothing, and lays it out, and the
// next message sizes it. The first time, it lays out the scene.
func (m *model) fit() {
	r, ok := m.ui.Frame().Rect(worldID)
	if !ok {
		return
	}
	first := m.world.w == 0
	m.world.resize(r.Dx(), 2*r.Dy())
	if first && m.world.w > 0 {
		m.world.scene()
	}
}

// advance takes the steps the clock has gone on by since the last, at most
// a few at once, so that a slow frame doesn't snowball.
func (m *model) advance() {
	want := int(m.clock.Seconds() * stepRate)
	n := min(want-m.steps, 4)
	m.steps = want
	for range n {
		start := time.Now()
		if m.pouring.held {
			m.pour(m.pouring.last, m.pouring.last, tools[m.tool].m)
		}
		if m.raining {
			m.world.rain(m.hue())
		}
		if m.pouring.held || m.raining {
			m.poured++
		}
		m.world.update()
		m.perf.sim.add(time.Since(start))
	}
}

// hue is the hue sand is poured in, which turns as it is poured.
func (m *model) hue() int { return m.poured / 5 }

// stroke is a button held down over the world.
type stroke struct {
	held bool
	last image.Point // where the pointer was at the last event, in pixels
}

// to moves the stroke to p, returning where it was, which is p when it
// starts there.
func (s *stroke) to(p image.Point) image.Point {
	from := p
	if s.held {
		from = s.last
	}
	s.held, s.last = true, p
	return from
}

// pour pours mat along the line from a to b, in pixels, so that a fast
// drag leaves no gaps.
func (m *model) pour(a, b image.Point, mat material) {
	d := b.Sub(a)
	steps := max(abs(d.X), abs(d.Y), 1)
	for s := 0; s <= steps; s += max(m.radius/2, 1) {
		m.world.paint(a.X+d.X*s/steps, a.Y+d.Y*s/steps, m.radius, mat, m.hue())
	}
	m.world.paint(b.X, b.Y, m.radius, mat, m.hue())
}

// resize changes the brush's radius by d.
func (m *model) resize(d int) { m.radius = min(max(m.radius+d, 1), 12) }

func (m *model) View() tea.View {
	start := time.Now()
	v := m.ui.View(m.w, m.h, m.layout())
	m.perf.framed(time.Since(start))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	v.WindowTitle = "bloom · sandbox"
	return v
}

// layout places the world beside the sidebar, which goes when the screen is
// narrow, over the status line.
func (m *model) layout() bloom.Child {
	state := ""
	if !m.clock.Animating() {
		state = " (paused)"
	}
	rain := "off"
	if m.raining {
		rain = "on"
	}
	hints := fmt.Sprintf(" drag: pour · right drag: erase · wheel/[ ]: brush · 1-5: material · r: rain (%s) · c: clear · space: pause%s · q: quit",
		rain, state)
	return bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(bloom.Box(m.st.panel, m.worldNode())),
			bloom.Fixed(30, bloom.Box(m.st.panel.Padding(0, 1), m.sidebar())).Collapsible(1),
		)),
		bloom.Text(hints).Style(m.st.status).Truncate(),
	)
}

// worldNode is the world's Drawable, with its mouse handlers, which are
// given the pointer relative to it. A cell is two pixels tall, so the
// pointer is on the top one of its cell.
func (m *model) worldNode() bloom.Child {
	b := m.pointer
	b.r = m.radius
	view := worldView{w: m.world, brush: b, ghost: m.st.ghost, took: &m.perf.drew}
	at := func(e on.Event) image.Point { return image.Pt(e.X, 2*e.Y) }
	return bloom.Drawable(view).ID(worldID).On(
		on.Drag(func(e on.Event) tea.Cmd {
			p := at(e)
			m.pour(m.pouring.to(p), p, tools[m.tool].m)
			m.pouring.held = !e.Done
			return nil
		}),
		// The pointer is followed while over the world, to draw the brush.
		on.Hover(func(over bool) tea.Cmd {
			m.pointer.shown = over
			m.erasing.held = m.erasing.held && over
			return nil
		}),
		// The right button erases, whatever the material, as Drag follows
		// only the left. A stroke stops at the edge of the world, as these
		// events come only while the pointer is over it.
		on.Mouse(func(e on.Event) tea.Cmd {
			p := at(e)
			m.pointer.x, m.pointer.y, m.pointer.shown = p.X, p.Y, true
			switch e.Msg.(type) {
			case tea.MouseClickMsg, tea.MouseMotionMsg:
				if e.Msg.Mouse().Button == tea.MouseRight {
					m.pour(m.erasing.to(p), p, empty)
				} else {
					m.erasing.held = false
				}
			case tea.MouseReleaseMsg:
				// The stroke ends where the button came up, which the last
				// move, dropped by the throttle, may not have reached. Older
				// terminals don't say which button came up.
				if b := e.Msg.Mouse().Button; b != tea.MouseRight && b != tea.MouseNone {
					break
				}
				if m.erasing.held {
					m.pour(m.erasing.to(p), p, empty)
				}
				m.erasing.held = false
			}
			return nil
		}),
		on.Scroll(func(_, dy int) tea.Cmd {
			m.resize(-dy)
			return nil
		}),
	)
}

// sidebar has the materials to pick, the brush, and the meters.
func (m *model) sidebar() bloom.Child {
	st := &m.st
	rows := []bloom.Child{
		bloom.Text("bloom · sandbox").Style(st.title),
		bloom.Text("Material").Style(st.heading),
	}
	for i, t := range tools {
		style := st.item
		if i == m.tool {
			style = st.picked
		}
		swatch := "██"
		if t.m == empty {
			swatch = "░░"
		}
		rows = append(rows, bloom.Row(
			bloom.Text(swatch).Style(st.swatch(t.m, m.hue())),
			bloom.Fill(bloom.Text(fmt.Sprintf("%d %s", i+1, t.name)).Style(style).HoverStyle(st.hovered)),
		).On(on.Click(func() tea.Cmd {
			m.tool = i
			return nil
		})))
	}

	wd := m.world
	meter := func(name, value string) bloom.Child {
		return bloom.Row(bloom.Fixed(9, bloom.Text(name).Style(st.label)), bloom.Fill(bloom.Text(value).Style(st.value).Truncate()))
	}
	m.times = m.perf.frame.samples(m.times[:0])
	rows = append(rows,
		bloom.Text("Brush").Style(st.heading),
		meter("radius", fmt.Sprintf("%d px", m.radius)),
		bloom.Text("Performance").Style(st.heading),
		meter("world", fmt.Sprintf("%d×%d px", wd.w, wd.h)),
		meter("cells", fmt.Sprintf("%d×%d = %d", wd.w, wd.h/2, wd.w*(wd.h/2))),
		meter("grains", strconv.Itoa(wd.grains)),
		meter("step", ms(m.perf.sim.avg())),
		meter("draw", ms(m.perf.draw.avg())),
		meter("frame", ms(m.perf.frame.avg())),
		meter("views/s", strconv.Itoa(m.perf.views)),
		bloom.Text("").Style(st.label),
		bloom.Fill(bloom.Drawable(graph{samples: m.times, ramp: st.ramp})),
		bloom.Text("frame times, 0 to "+graphScale.String()).Style(st.muted).Truncate(),
	)
	return bloom.Column(rows...)
}

// options are the program's options: drawing at fps, and taking motion no
// faster than that.
func options() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithFPS(fps), tea.WithFilter(bloom.ThrottleMouse(time.Second / fps))}
}

func main() {
	p := tea.NewProgram(newModel(), options()...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
