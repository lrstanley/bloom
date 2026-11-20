// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"bytes"
	"image"
	"io"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
	"unsafe"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// wrapping is on one row when there is room for it, and on two when there
// isn't.
type wrapping struct{ NoView }

func (wrapping) Layout(ctx Context) Child {
	if ctx.Width >= 5 {
		return Row(Text("abc"), Text("de"))
	}
	return Column(Text("abc"), Text("de"))
}

func TestMeasure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		w, h int
		c    Child
		want Size
	}{
		{"natural", 0, 0, Text("ab\ncde"), Size{W: 3, H: 2}},
		{"cut", 2, 1, Text("ab\ncde"), Size{W: 2, H: 1}},
		{"layouter with room", 0, 0, wrapping{}, Size{W: 5, H: 1}},
		{"layouter without room", 4, 0, wrapping{}, Size{W: 3, H: 2}},
		{"nil", 10, 10, nil, Size{}},
	}
	for _, tt := range tests {
		if got := Measure(tt.w, tt.h, tt.c); got != tt.want {
			t.Errorf("%s: Measure(%d, %d) = %v, want %v", tt.name, tt.w, tt.h, got, tt.want)
		}
	}
}

func TestSolveMatchesRoot(t *testing.T) {
	t.Parallel()

	for _, size := range []image.Point{{200, 60}, {120, 30}, {70, 20}} {
		tree := benchTree(4, 1, 0.5)
		r := New()
		r.Render(size.X, size.Y, tree)
		solved := Solve(size.X, size.Y, tree)
		for _, id := range []any{"sidebar", "log", 4, "dialog"} {
			want, wantOK := r.Frame().Rect(id)
			got, gotOK := solved.Rect(id)
			if id == "dialog" {
				// Solve does not run effects, so a slid layer is where it
				// was laid out.
				continue
			}
			if got != want || gotOK != wantOK {
				t.Errorf("%v at %v: Solve %v %v, Root %v %v", id, size, got, gotOK, want, wantOK)
			}
		}
	}
}

func TestUnchangedFrame(t *testing.T) {
	t.Parallel()

	r := New()
	first := r.Render(80, 24, benchTree(2, 0, -1))
	frame := r.Frame()
	again := r.Render(80, 24, benchTree(2, 0, -1))
	if unsafe.StringData(first) != unsafe.StringData(again) || r.Frame() != frame {
		t.Fatal("an unchanged frame was drawn again")
	}
	if changed := r.Render(80, 24, benchTree(3, 0, -1)); changed == first {
		t.Fatal("a changed frame was not drawn")
	}
}

func TestMouseMode(t *testing.T) {
	t.Parallel()

	var f Focus[string]
	nothing := func() tea.Cmd { return nil }
	tests := []struct {
		name string
		opts []Option
		tree Child
		want tea.MouseMode
	}{
		{"nothing", nil, Text("x").ID("x"), tea.MouseModeNone},
		{"click handler", nil, Text("x").On(on.Click(nothing)), tea.MouseModeCellMotion},
		{"wheel handler", nil, Text("x").On(on.Scroll(func(_, _ int) tea.Cmd { return nil })), tea.MouseModeCellMotion},
		{"scrolled viewport", nil, Viewport(NewBuffer("a\nb\nc")).OnScroll(func(ViewportPos) tea.Cmd { return nil }), tea.MouseModeCellMotion},
		{"selectable", nil, Text("x").Selectable(), tea.MouseModeCellMotion},
		{"hover style", nil, Text("x").HoverStyle(lipgloss.NewStyle().Bold(true)), tea.MouseModeAllMotion},
		{"hover handler", nil, Column(Text("x").On(on.Click(nothing)), Text("y").On(on.Hover(func(bool) tea.Cmd { return nil }))), tea.MouseModeAllMotion},
		{"focusable without focus", nil, Text("x").ID("x").Focusable(), tea.MouseModeNone},
		{"click to focus", []Option{WithFocus(&f)}, Text("x").ID("x").Focusable(), tea.MouseModeCellMotion},
		{"tooltip with tooltips off", nil, Text("x").Tooltip("tip"), tea.MouseModeNone},
		{"tooltip", []Option{WithTooltips(Tooltips{})}, Text("x").Tooltip("tip"), tea.MouseModeAllMotion},
		{"tooltips on without any", []Option{WithTooltips(Tooltips{})}, Text("x").On(on.Click(nothing)), tea.MouseModeCellMotion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := New(tt.opts...).View(10, 2, tt.tree).MouseMode; got != tt.want {
				t.Fatalf("mouse mode %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCursor(t *testing.T) {
	t.Parallel()

	input := Text("> abc").Cursor(tea.NewCursor(3, 0))
	tree := Row(Fixed(10, Text("label")), input)

	v := New().View(30, 3, tree)
	if v.Cursor == nil || v.Cursor.X != 13 || v.Cursor.Y != 0 {
		t.Fatalf("cursor = %+v, want 13,0", v.Cursor)
	}

	covered := Overlay(tree, Layer(Text("dialog over the input")).Anchor(End, Start))
	if cur := New().View(30, 3, covered).Cursor; cur != nil {
		t.Fatalf("cursor under a layer is shown at %+v", cur)
	}
}

func TestEnvWithUncomparableColors(t *testing.T) {
	t.Parallel()

	r := New(WithColors(sliceColor{0xff}, sliceColor{0}))
	r.Render(4, 1, Text("a"))
	r.Render(4, 1, Text("a"))
}

// sliceColor is a valid [color.Color] that cannot be compared with ==.
type sliceColor []uint8

func (c sliceColor) RGBA() (r, g, b, a uint32) {
	v := uint32(c[0]) * 0x101
	return v, v, v, 0xffff
}

// goid returns the ID of the calling goroutine, from its stack trace.
func goid() string {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	// "goroutine 123 [running]:..."
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	id, _, _ := bytes.Cut(buf, []byte(" "))
	return string(id)
}

// firstWrite is an output that reports when the program first draws: the
// first write holding want, the view's text, as Bubble Tea writes its
// startup queries before it.
type firstWrite struct {
	once  sync.Once
	want  []byte
	drawn chan struct{}
}

func (w *firstWrite) Write(p []byte) (int, error) {
	if bytes.Contains(p, w.want) {
		w.once.Do(func() { close(w.drawn) })
	}
	return len(p), nil
}

type doneMsg struct{}

// loopModel records where and in what order Bubble Tea runs a click's
// handler, Update, and View, and the handler's command.
type loopModel struct {
	ui      *Root
	clicked bool
	steps   []string // "what goroutine"
	cmdOn   string
}

func (m *loopModel) Init() tea.Cmd { return nil }

func (m *loopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseReleaseMsg:
		// Before Root.Update, which would run the handler had OnMouse not.
		m.steps = append(m.steps, "update "+goid())
	case doneMsg:
		return m, tea.Quit
	}
	return m, m.ui.Update(msg)
}

func (m *loopModel) View() tea.View {
	if m.clicked && !slices.ContainsFunc(m.steps, func(s string) bool { return s[:4] == "view" }) {
		m.steps = append(m.steps, "view "+goid())
	}
	v := m.ui.View(20, 2, Text("click").On(on.Click(func() tea.Cmd {
		m.clicked = true
		m.steps = append(m.steps, "handler "+goid())
		return func() tea.Msg {
			m.cmdOn = goid()
			return doneMsg{}
		}
	})))
	return v
}

// TestBubbleTeaEventLoop pins the Bubble Tea behavior handlers rely on: it
// calls OnMouse for an event, then Update with it, then View, all on its
// event loop, and runs the command OnMouse returns there too, but not the
// commands in a batch it returns.
func TestBubbleTeaEventLoop(t *testing.T) {
	t.Parallel()

	in, feed := io.Pipe()
	out := &firstWrite{want: []byte("click"), drawn: make(chan struct{})}
	m := &loopModel{ui: New()}
	p := tea.NewProgram(m,
		tea.WithInput(in), tea.WithOutput(out), tea.WithWindowSize(20, 2),
		tea.WithoutSignalHandler(), tea.WithoutCatchPanics(),
	)
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	// One deadline for drawing, reading the input, and the command; on it,
	// the program is stopped before its steps are read.
	timeout := time.After(5 * time.Second)
	fail := func(why string) {
		t.Helper()
		p.Kill()
		<-done
		_ = feed.Close()
		t.Fatalf("%s; steps %q", why, m.steps)
	}
	// OnMouse comes from the last view drawn, so wait for one.
	select {
	case <-out.drawn:
	case <-timeout:
		fail("nothing was drawn")
	}
	// SGR mouse reports, 1-based: a left press and release at 0,0. The
	// pipe blocks until the program reads them, and fails once closed.
	go func() { _, _ = feed.Write([]byte("\x1b[<0;1;1M\x1b[<0;1;1m")) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-timeout:
		fail("the click's command never ran")
	}
	_ = feed.Close()

	if len(m.steps) != 3 {
		t.Fatalf("steps %q, want the handler, Update, and View", m.steps)
	}
	loop := m.steps[0][len("handler "):]
	want := []string{"handler " + loop, "update " + loop, "view " + loop}
	if !slices.Equal(m.steps, want) {
		t.Fatalf("steps %q, want %q", m.steps, want)
	}
	if m.cmdOn == loop {
		t.Fatal("the handler's command ran on the event loop")
	}
}

// TestViewSameContentRoutesLatestFrame checks that a view routes with the
// latest frame when it draws the same, as Bubble Tea keeps the view it
// painted over newer ones with the same content, even after one drawing
// something else that it never painted.
func TestViewSameContentRoutesLatestFrame(t *testing.T) {
	t.Parallel()

	var l log
	r := New()
	shown := r.View(10, 1, Text("same").On(on.Click(l.add("old"))))
	r.View(10, 1, Text("other").On(on.Click(l.add("other"))))
	r.View(10, 1, Text("same").On(on.Click(l.add("new"))))
	shown.OnMouse(down(0, 0))
	shown.OnMouse(release(0, 0))
	expect(t, &l, "new")
}

// TestViewSameContentTooltip checks that a node given a tooltip while the
// pointer is over it, drawing the same, arms it on the next motion, as an
// editor showing the diagnostic under the pointer does.
func TestViewSameContentTooltip(t *testing.T) {
	t.Parallel()

	over := false
	tree := func() Child {
		n := Text("alpha").On(on.Mouse(func(e on.Event) tea.Cmd {
			over = e.Inside
			return nil
		}))
		if over {
			n = n.Tooltip("tip a")
		}
		return Column(Text(" "), Row(Fixed(6, n), Fill(Text(" "))))
	}
	g := newTipRig(t, 30, 2, tree)
	// Every event goes to the first view, as it would in Bubble Tea before
	// it paints a newer one.
	shown := g.r.View(g.w, g.h, tree())
	for _, msg := range []tea.MouseMsg{move(1, 1), move(2, 1)} {
		shown.OnMouse(msg)
		g.send(msg)
	}
	g.wait(defaultTipDelay)
	g.want("tip a")
}

// swapModel has a click's handler replace itself with one that quits,
// drawing the same.
type swapModel struct {
	ui      *Root
	swapped bool
}

func (m *swapModel) Init() tea.Cmd { return nil }

func (m *swapModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.ui.Update(msg)
}

func (m *swapModel) View() tea.View {
	fn := func() tea.Cmd { m.swapped = true; return nil }
	if m.swapped {
		fn = func() tea.Cmd { return tea.Quit }
	}
	return m.ui.View(20, 2, Text("click").On(on.Click(fn)))
}

// TestBubbleTeaSameContent checks, in a running program, that a click is
// routed with the frame drawn after the last click, which draws the same.
func TestBubbleTeaSameContent(t *testing.T) {
	t.Parallel()

	in, feed := io.Pipe()
	out := &firstWrite{want: []byte("click"), drawn: make(chan struct{})}
	m := &swapModel{ui: New()}
	p := tea.NewProgram(m,
		tea.WithInput(in), tea.WithOutput(out), tea.WithWindowSize(20, 2),
		tea.WithoutSignalHandler(), tea.WithoutCatchPanics(),
	)
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()
	defer func() { _ = feed.Close() }()

	timeout := time.After(5 * time.Second)
	select {
	case <-out.drawn:
	case <-timeout:
		p.Kill()
		<-done
		t.Fatal("nothing was drawn")
	}
	// Two clicks at 0,0: the first swaps the handler, the second quits.
	go func() { _, _ = feed.Write([]byte("\x1b[<0;1;1M\x1b[<0;1;1m\x1b[<0;1;1M\x1b[<0;1;1m")) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-timeout:
		p.Kill()
		<-done
		t.Fatalf("the second click ran the first handler again (swapped %t)", m.swapped)
	}
}
