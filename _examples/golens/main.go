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
	"slices"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	tea "charm.land/bubbletea/v2"
)

// pane is which list of the code view has the cursor.
type pane uint8

const (
	paneCalls pane = iota
	paneCallers
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

type model struct {
	ui   *bloom.Root
	st   *styles
	w, h int

	dir      string
	patterns []string
	prog     *program
	err      error
	shimmer  anim.Sequence // the loading skeleton's shimmer
	intro    anim.Value    // the tree's entrance once loaded

	// path is the selected function: the index of a root, then of a callee
	// of each function in turn. Every function on it is expanded.
	path       []int
	camX, camY int        // the graph's scroll, in cells
	reveal     anim.Value // the preview column's entrance, played on each move

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
	return &model{
		ui:       bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		st:       newStyles(),
		dir:      dir,
		patterns: patterns,
		shimmer:  anim.NewLoop(1400 * time.Millisecond),
		intro:    effect(600 * time.Millisecond),
		reveal:   effect(220 * time.Millisecond),
		swap:     effect(260 * time.Millisecond),
		code:     anim.NewPresence(anim.Duration(320*time.Millisecond), anim.Ease(anim.InOutCubic)),
		search:   anim.NewPresence(anim.Duration(180 * time.Millisecond)),
	}
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

// depth is the column of the selected function.
func (m *model) depth() int { return len(m.path) - 1 }

// level returns the functions in column k: the roots, or what the function
// selected in column k-1 calls.
func (m *model) level(k int) []*fn {
	if k == 0 {
		return m.prog.roots
	}
	return m.nodeAt(k - 1).calls
}

// nodeAt returns the function selected in column k.
func (m *model) nodeAt(k int) *fn { return m.level(k)[m.path[k]] }

// cur returns the selected function.
func (m *model) cur() *fn { return m.nodeAt(m.depth()) }

// onPath reports whether f is selected in a column before k, as a call
// back up the path is recursion.
func (m *model) onPath(f *fn, k int) bool {
	fs := m.prog.roots
	for _, i := range m.path[:min(k, len(m.path))] {
		if fs[i] == f {
			return true
		}
		fs = fs[i].calls
	}
	return false
}

// selectPath selects the function at path, playing the preview's entrance
// when it changed.
func (m *model) selectPath(path []int) tea.Cmd {
	if slices.Equal(path, m.path) || len(path) == 0 {
		return nil
	}
	m.path = path
	m.calleeSel, m.callerSel, m.codeOff = 0, 0, 0
	m.follow()
	m.scrollToCallee()
	return m.reveal.Play(0, 1)
}

func (m *model) moveSibling(delta int) tea.Cmd {
	d := m.depth()
	i := min(max(m.path[d]+delta, 0), len(m.level(d))-1)
	return m.selectPath(append(slices.Clone(m.path[:d]), i))
}

// enter selects the first function the selected one calls.
func (m *model) enter() tea.Cmd {
	if len(m.cur().calls) == 0 {
		return nil
	}
	return m.selectPath(append(slices.Clone(m.path), 0))
}

// back selects the function that called the selected one.
func (m *model) back() tea.Cmd {
	if len(m.path) < 2 {
		return nil
	}
	return m.selectPath(slices.Clone(m.path[:len(m.path)-1]))
}

// jump selects f: up the path if it is on it, or else by the shortest path
// to it from a root.
func (m *model) jump(f *fn) tea.Cmd {
	for k := range m.path {
		if m.nodeAt(k) == f {
			return m.selectPath(slices.Clone(m.path[:k+1]))
		}
	}
	if path := m.prog.pathTo(f); path != nil {
		return m.selectPath(path)
	}
	return nil
}

// follow scrolls the graph as little as it can to show the selected function
// with its callees, and the column before it when there is room.
func (m *model) follow() {
	if m.prog == nil || len(m.path) == 0 {
		return
	}
	viewW, viewH := m.graphSize()
	d := m.depth()

	right := colX(d) + itemW
	if len(m.cur().calls) > 0 {
		right = colX(d+1) + itemW
	}
	lo, hi := right+graphPad-viewW, colX(d)-graphPad
	if m.camX < lo {
		m.camX = lo
	}
	if prev := colX(max(d-1, 0)) - graphPad; m.camX > prev {
		m.camX = max(prev, lo)
	}
	// Scrolled to the start of a column, rather than showing a sliver of
	// one at the left.
	m.camX = max(min(m.camX, hi), 0)
	if m.camX > 0 {
		const step = itemW + wireW
		m.camX = colX((m.camX - graphPad + step - 1) / step)
	}

	y := graphPad
	for _, i := range m.path {
		y += i
	}
	margin := min(4, viewH/4)
	// The callees open below the selected function: show as many as fit.
	bottom := y + min(len(m.cur().calls), viewH/2)
	if bottom+margin >= m.camY+viewH {
		m.camY = bottom + margin - viewH + 1
	}
	if y-margin < m.camY {
		m.camY = y - margin
	}
	m.camY = max(m.camY, 0)
}

// openCode shows the code view of the selected function.
func (m *model) openCode() tea.Cmd {
	m.focus = paneCalls
	m.scrollToCallee()
	return m.code.Show()
}

// codeKey handles a key in the code view.
func (m *model) codeKey(act action) tea.Cmd {
	f := m.cur()
	switch act {
	case actNone:
	case actQuit:
		return tea.Quit
	case actClose:
		m.follow()
		return m.code.Hide()
	case actSwitch:
		if m.focus == paneCalls && len(f.callers) > 0 {
			m.focus = paneCallers
		} else {
			m.focus = paneCalls
		}
	case actUp:
		m.moveCodeSel(-1)
	case actDown:
		m.moveCodeSel(1)
	case actFirst:
		m.moveCodeSel(-len(f.calls) - len(f.callers))
	case actLast:
		m.moveCodeSel(len(f.calls) + len(f.callers))
	case actPageUp:
		m.codeOff = max(m.codeOff-m.codeRows(), 0)
	case actPageDown:
		m.codeOff = min(m.codeOff+m.codeRows(), max(len(f.src)-1, 0))
	case actOpen, actIn:
		if m.focus == paneCallers {
			if m.callerSel < len(f.callers) {
				return m.goCaller(f.callers[m.callerSel])
			}
			return nil
		}
		return m.goCallee(m.calleeSel)
	case actBack:
		if len(m.path) > 1 {
			back := m.path[m.depth()]
			cmd := m.walk(slices.Clone(m.path[:len(m.path)-1]), bloom.Left)
			m.calleeSel = back
			m.scrollToCallee()
			return cmd
		}
	case actSearch:
		return m.openSearch()
	}
	return nil
}

func (m *model) moveCodeSel(delta int) {
	f := m.cur()
	if m.focus == paneCallers {
		m.callerSel = min(max(m.callerSel+delta, 0), max(len(f.callers)-1, 0))
		return
	}
	m.calleeSel = min(max(m.calleeSel+delta, 0), max(len(f.calls)-1, 0))
	m.scrollToCallee()
}

// goCallee walks into callee i of the selected function in the code view.
func (m *model) goCallee(i int) tea.Cmd {
	if i < 0 || i >= len(m.cur().calls) {
		return nil
	}
	return m.walk(append(slices.Clone(m.path), i), bloom.Right)
}

// goCaller walks to caller in the code view: back up the path if it is the
// function before, or else to it by the shortest path from a root.
func (m *model) goCaller(caller *fn) tea.Cmd {
	from := m.cur()
	m.swapFrom = bloom.Left
	cmd := m.jump(caller)
	m.focus = paneCalls
	m.calleeSel = max(slices.Index(caller.calls, from), 0)
	m.scrollToCallee()
	return tea.Batch(cmd, m.swap.Play(0, 1))
}

// walk selects path from the code view, sliding the new code in from side.
func (m *model) walk(path []int, side bloom.Direction) tea.Cmd {
	m.swapFrom = side
	m.focus = paneCalls
	return tea.Batch(m.selectPath(path), m.swap.Play(0, 1))
}

// scrollToCallee scrolls the code so that the first call of the selected
// callee is a few lines from the top.
func (m *model) scrollToCallee() {
	if m.prog == nil || len(m.path) == 0 {
		return
	}
	f := m.cur()
	if m.calleeSel >= len(f.calls) {
		return
	}
	sites := f.sites[f.calls[m.calleeSel]]
	if len(sites) == 0 {
		return
	}
	line, rows := sites[0].line, m.codeRows()
	if line < m.codeOff || line >= m.codeOff+rows {
		m.codeOff = max(line-rows/3, 0)
	}
}

// openSearch opens the search dialog, with the last query.
func (m *model) openSearch() tea.Cmd {
	m.runSearch()
	return m.search.Show()
}

func (m *model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return m.search.Hide()
	case "enter":
		if m.resultSel < len(m.results) {
			m.swapFrom = bloom.Top
			return tea.Batch(m.search.Hide(), m.jump(m.results[m.resultSel]), m.swap.Play(0, 1))
		}
		return nil
	case "up", "ctrl+p", "shift+tab":
		m.resultSel = max(m.resultSel-1, 0)
	case "down", "ctrl+n", "tab":
		m.resultSel = min(m.resultSel+1, max(len(m.results)-1, 0))
	case "backspace":
		if q := []rune(m.query); len(q) > 0 {
			m.query = string(q[:len(q)-1])
			m.runSearch()
		}
	case "ctrl+u":
		m.query = ""
		m.runSearch()
	default:
		if msg.Text != "" {
			m.query += msg.Text
			m.runSearch()
		}
	}
	return nil
}

// runSearch finds the functions matching the query: names containing it
// first, then names with its letters in order.
func (m *model) runSearch() {
	m.resultSel = 0
	q := strings.ToLower(strings.TrimSpace(m.query))
	type hit struct {
		f     *fn
		score int
	}
	var hits []hit
	for _, f := range m.prog.all {
		name := strings.ToLower(f.qualified())
		score := 0
		switch i := strings.Index(name, q); {
		case q == "":
		case i >= 0:
			score = i
			if !strings.Contains(strings.ToLower(f.name), q) {
				score += 100 // matched only with the package
			}
		case fuzzy(name, q):
			score = 1000
		default:
			continue
		}
		if !f.local() {
			score += 10000
		}
		hits = append(hits, hit{f, score})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.score - b.score })
	m.results = m.results[:0]
	for _, h := range hits {
		m.results = append(m.results, h.f)
	}
}

// fuzzy reports whether the letters of q appear in s in order.
func fuzzy(s, q string) bool {
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
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
