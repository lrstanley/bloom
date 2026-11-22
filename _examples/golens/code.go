// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"slices"
	"strconv"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// pane is which list of the code view has the cursor.
type pane uint8

const (
	paneCalls pane = iota
	paneCallers
)

// codeRows is how many lines of code the code view shows.
func (m *model) codeRows() int { return max(m.h-5, 1) }

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

// codeView shows the selected function's code, between the functions that
// call it and those it calls.
func (m *model) codeView() bloom.Child {
	f := m.cur()
	icon, iconSt := m.st.icon(f)
	where := f.pkgPath
	if f.local() {
		where = fmt.Sprintf("%s:%d", f.file, f.line)
	}
	header := bloom.Row(
		bloom.Text(icon+" ").Style(iconSt),
		bloom.Fill(bloom.Text(m.st.muted.Render(f.pkg+".")+m.st.bright.Render(f.name)).Truncate()),
		bloom.Text(where).Style(m.st.faint),
	)

	var body bloom.Child
	if f.local() {
		gutter := len(strconv.Itoa(f.line+len(f.src))) + 2
		body = bloom.List(len(f.src), func(i int) bloom.Child { return m.codeLine(f, i, gutter) }).
			ID(codeListID).
			Offset(m.codeOff).
			On(on.Scroll(func(_, dy int) tea.Cmd {
				m.codeOff = min(max(m.codeOff+3*dy, 0), max(len(f.src)-m.codeRows()/2, 0))
				return nil
			}))
	} else {
		note := "Declared outside the project, in " + f.pkgPath + ", so there is no source to show."
		if f.kind == kindInterface {
			note = "An interface method, so there is no body to show. Its implementations in the project are on the right."
		}
		body = bloom.Column(
			bloom.Text(m.st.signature(f)),
			bloom.Fixed(1, nil),
			bloom.Text(note).Style(m.st.muted),
		)
	}
	from := m.swapFrom
	if from == 0 {
		from = bloom.Bottom
	}
	code := bloom.Box(m.st.panel, bloom.Column(
		bloom.Fixed(1, header),
		bloom.Fixed(1, nil),
		bloom.Fill(bloom.Transition(m.swap.Float(), body, fx.Slide{From: from}, fx.Fade{})),
	))

	return bloom.Box(m.st.screen, bloom.Column(
		bloom.Fixed(1, m.topbar()),
		bloom.Fill(bloom.Row(
			bloom.Fixed(26, m.calls("Called by", f.callers, paneCallers, m.callerSel)).Collapsible(1),
			bloom.Fill(code).Min(40),
			bloom.Fixed(30, m.calls("Calls", f.calls, paneCalls, m.calleeSel)).Collapsible(2),
		).Gap(1)),
		bloom.Fixed(1, m.st.footer(
			"↑↓", "select", "enter", "walk in", "←", "back", "tab", "callers", "esc", "tree", "/", "search",
		)),
	))
}

// calls is a list of functions in the code view: those that call the
// selected one, or those it calls.
func (m *model) calls(title string, fs []*fn, p pane, sel int) bloom.Child {
	focused := m.focus == p
	titleSt := m.st.muted
	if focused {
		titleSt = m.st.bright
	}
	head := titleSt.Render(title) + "  " + m.st.accent.Render(strconv.Itoa(len(fs)))
	list := bloom.List(len(fs), func(i int) bloom.Child {
		f := fs[i]
		st, hover := m.st.row, m.st.rowHover
		switch {
		case i == sel && focused:
			st, hover = m.st.rowSel, m.st.rowSelHover
		case i == sel:
			st, hover = m.st.rowSelBlur, m.st.rowSelBlurHover
		}
		icon, iconSt := m.st.icon(f)
		name := f.name
		if f.pkg != m.cur().pkg {
			name = f.pkg + "." + name
		}
		return bloom.Box(st, bloom.Row(
			bloom.Text(icon+" ").Style(iconSt),
			bloom.Fill(bloom.Text(name).Truncate()),
		)).
			HoverStyle(hover).
			On(
				on.Click(func() tea.Cmd {
					m.focus = p
					if p == paneCalls {
						m.calleeSel = i
						m.scrollToCallee()
					} else {
						m.callerSel = i
					}
					return nil
				}),
				on.DoubleClick(func() tea.Cmd {
					if p == paneCalls {
						return m.goCallee(i)
					}
					return m.goCaller(f)
				}),
			)
	}).Keep(sel)
	return bloom.Box(m.st.callsPanel, bloom.Column(
		bloom.Fixed(1, nil),
		bloom.Fixed(1, bloom.Text(" "+head).Truncate()),
		bloom.Fixed(1, nil),
		bloom.Fill(list),
	))
}

// codeLine is line i of f's code, with the selected callee's call sites
// marked. A click on any call site walks into what it calls.
func (m *model) codeLine(f *fn, i, gutter int) bloom.Child {
	var marked *fn
	if m.calleeSel < len(f.calls) {
		marked = f.calls[m.calleeSel]
	}
	num := m.st.lineNo
	for _, s := range f.sites[marked] {
		if s.line == i {
			num = m.st.lineNoMarked
			break
		}
	}
	return bloom.Text(num.Render(fmt.Sprintf("%*d  ", gutter-2, f.line+i)) + m.st.line(f, i, marked)).
		Truncate().
		On(on.ClickAt(func(e on.Event) tea.Cmd {
			callee, ok := f.siteAt(i, byteAt(f.src[i], e.X-gutter))
			if !ok {
				return nil
			}
			return m.goCallee(slices.Index(f.calls, callee))
		}))
}

// byteAt returns the byte of s at cell x.
func byteAt(s string, x int) int {
	at := 0
	for i, r := range s {
		if at >= x {
			return i
		}
		at += max(ansi.StringWidth(string(r)), 1)
	}
	return len(s)
}
