// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command golens explores the call graph of a Go project. The project's entry
// points, its functions that nothing else in it calls, are the first column
// of a tree that grows to the right: each step in shows what the selected
// function calls, with the columns before it kept for context, and siblings
// gliding out of the way as a branch opens. The sidebar shows the selected
// function's signature and doc comment.
//
// Enter opens the function's code between who calls it and what it calls,
// with each call site of the selected callee marked; enter again, or a click
// on a marked name, walks into the callee. / searches every function and
// jumps to it by the shortest path from an entry point.
//
//	go run ./golens [dir] [packages]
//
// dir defaults to the current directory and packages to ./... from it.
//
// Keys: arrows or hjkl walk the tree, enter opens the code, tab switches
// between callers and callees there, esc goes back, / searches, and q or
// ctrl+c quits. Clicks select, double clicks open, and the wheel scrolls.
package main

import (
	"fmt"
	"os"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// action is what a key does in the tree and the code view.
type action uint8

const (
	actNone action = iota
	actQuit
	actClose
	actUp
	actDown
	actPageUp
	actPageDown
	actFirst
	actLast
	actIn   // walk into the selected callee
	actBack // walk back to the caller
	actOpen // open the code view, or walk into the callee there
	actSwitch
	actSearch
)

// keys maps keys to what they do in the tree and the code view. The search
// dialog takes keys itself, as most are typed into its query.
var keys = map[string]action{
	"q": actQuit, "esc": actClose,
	"up": actUp, "k": actUp, "down": actDown, "j": actDown,
	"pgup": actPageUp, "pgdown": actPageDown,
	"home": actFirst, "g": actFirst, "end": actLast, "G": actLast,
	"right": actIn, "l": actIn, "left": actBack, "h": actBack, "backspace": actBack,
	"enter": actOpen, "space": actOpen,
	"tab": actSwitch, "shift+tab": actSwitch,
	"/": actSearch,
}

type loadedMsg struct {
	prog *program
	err  error
}

// model is the program: the project once loaded, the function selected in
// the tree (tree.go), and the state of the code view (code.go) and of search
// (search.go), which open over the tree.
type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	dir      string
	patterns []string
	prog     *program
	err      error
	shimmer  anim.Sequence // the loading skeleton's shimmer
	intro    anim.Value    // the tree's entrance once loaded

	// path is the selected function: the index of a root, then of a callee
	// of each function in turn. Every function on it is expanded.
	path   []int
	camX   int        // the graph's scroll across, in cells
	camY   int        // the graph's scroll down, in cells
	reveal anim.Value // the preview column's entrance, played on each move

	code      anim.Presence // the code view
	focus     pane
	calleeSel int // the selected callee in the code view
	callerSel int // the selected caller in the code view
	codeOff   int // the first line of code shown
	swap      anim.Value
	swapFrom  bloom.Direction // where the code slides in from as it changes

	search    anim.Presence
	query     string
	results   []*fn
	resultSel int
}

func newModel(dir string, patterns ...string) *model {
	effect := func(d time.Duration) anim.Value {
		return anim.NewValue(1, anim.Duration(d), anim.Ease(anim.OutCubic))
	}
	m := &model{
		st:       newStyles(tint.TintKanagawaDragon),
		dir:      dir,
		patterns: patterns,
		shimmer:  anim.NewLoop(1400 * time.Millisecond),
		intro:    effect(600 * time.Millisecond),
		reveal:   effect(220 * time.Millisecond),
		swap:     effect(260 * time.Millisecond),
		code:     anim.NewPresence(anim.Duration(320*time.Millisecond), anim.Ease(anim.InOutCubic)),
		search:   anim.NewPresence(anim.Duration(180 * time.Millisecond)),
	}
	m.ui = bloom.New(bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)))
	return m
}

// Init loads the project in the background while the skeleton shimmers. It
// does not return the root's Init, which asks the terminal for its colors:
// the theme's are used instead.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.shimmer.Loop(), func() tea.Msg {
		prog, err := analyze(m.dir, m.patterns...)
		return loadedMsg{prog: prog, err: err}
	})
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.follow()
	case loadedMsg:
		m.shimmer.Stop()
		m.prog, m.err = msg.prog, msg.err
		if m.prog != nil && len(m.prog.roots) > 0 {
			m.path = []int{0}
			m.follow()
			cmds = append(cmds, m.intro.Play(0, 1))
		}
	case tea.KeyPressMsg:
		cmds = append(cmds, m.key(msg))
	}
	cmds = append(cmds, anim.Update(msg, &m.shimmer, &m.intro, &m.reveal, &m.code, &m.swap, &m.search))
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	act := keys[msg.String()]
	switch {
	case m.prog == nil || len(m.prog.roots) == 0:
		if act == actQuit || act == actClose {
			return tea.Quit
		}
		return nil
	case m.search.Shown():
		return m.searchKey(msg)
	case m.code.Shown():
		return m.codeKey(act)
	}
	switch act { //nolint:exhaustive // The tree has no use for the rest.
	case actQuit:
		return tea.Quit
	case actUp:
		return m.moveSibling(-1)
	case actDown:
		return m.moveSibling(1)
	case actPageUp:
		return m.moveSibling(-10)
	case actPageDown:
		return m.moveSibling(10)
	case actFirst:
		return m.moveSibling(-len(m.level(m.depth())))
	case actLast:
		return m.moveSibling(len(m.level(m.depth())))
	case actIn:
		return m.enter()
	case actBack:
		return m.back()
	case actOpen:
		return m.openCode()
	case actSearch:
		return m.openSearch()
	}
	return nil
}

// layout places the bars, the sidebar, and the tree, with the code view and
// search in layers over them. Until the project has loaded, it shows the
// loading skeleton, or why there is nothing to show.
func (m *model) layout() bloom.Child {
	switch {
	case m.err != nil:
		return bloom.Centered(bloom.Box(m.st.dialog.Width(min(80, m.w-4)), bloom.Column(
			bloom.Text("couldn't load "+m.dir).Style(m.st.accent),
			bloom.Fixed(1, nil),
			bloom.Text(m.err.Error()).Style(m.st.text),
		)))
	case m.prog == nil:
		return m.loading()
	case len(m.prog.roots) == 0:
		return bloom.Centered(bloom.Text("no functions in " + m.dir).Style(m.st.muted))
	}

	base := bloom.Column(
		bloom.Fixed(1, m.topbar()),
		bloom.Fill(bloom.Row(
			bloom.Fixed(sidebarW, sidebar{st: &m.st, f: m.cur()}).Collapsible(1),
			bloom.Fill(m.graph()).Min(minGraph),
		).Gap(1)),
		bloom.Fixed(1, m.st.footer(
			"↑↓", "move", "←→", "walk", "enter", "code", "/", "search", "q", "quit",
		)),
	)
	// The code view and search are built only while they show.
	code, search := bloom.Child(bloom.Text("")), bloom.Child(bloom.Text(""))
	if m.code.Visible() {
		code = m.codeView()
	}
	if m.search.Visible() {
		search = m.searchView()
	}
	return bloom.Overlay(base,
		bloom.Layer(code).
			ID(codeID).
			Size(m.w, m.h).
			Modal().
			Presence(m.code, fx.Iris{At: m.selectedAt()}, fx.Fade{}),
		bloom.Layer(search).
			Size(min(72, m.w-4), 0).
			Anchor(bloom.Center, bloom.Start).
			Offset(0, max(m.h/6, 1)).
			Modal().
			Backdrop(fx.Dim{Amount: 0.5}).
			Presence(m.search, fx.Fade{}, fx.Slide{From: bloom.Top}).
			On(on.ClickOutside(m.search.Hide)),
	)
}

func main() {
	dir, patterns := ".", []string(nil)
	if len(os.Args) > 1 {
		dir, patterns = os.Args[1], os.Args[2:]
	}
	p := tea.NewProgram(newModel(dir, patterns...), tea.WithFPS(60), tea.WithFilter(bloom.ThrottleMouse(15*time.Millisecond)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
