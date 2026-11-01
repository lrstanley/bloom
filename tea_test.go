// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"bytes"
	"io"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bloom/on"
)

// goid returns the ID of the calling goroutine, from its stack trace.
func goid() string {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	// "goroutine 123 [running]:..."
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	id, _, _ := bytes.Cut(buf, []byte(" "))
	return string(id)
}

// firstWrite is an output that reports when the program first draws.
type firstWrite struct {
	once  sync.Once
	drawn chan struct{}
}

func (w *firstWrite) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.drawn) })
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
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// TestBubbleTeaEventLoop pins the Bubble Tea behavior handlers rely on: it
// calls OnMouse for an event, then Update with it, then View, all on its
// event loop, and runs the command OnMouse returns there too, but not the
// commands in a batch it returns.
func TestBubbleTeaEventLoop(t *testing.T) {
	t.Parallel()

	in, feed := io.Pipe()
	out := &firstWrite{drawn: make(chan struct{})}
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

	// OnMouse comes from the last view drawn, so wait for one.
	<-out.drawn
	// SGR mouse reports, 1-based: a left press and release at 0,0.
	if _, err := feed.Write([]byte("\x1b[<0;1;1M\x1b[<0;1;1m")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatalf("the click's command never ran; steps %q", m.steps)
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
