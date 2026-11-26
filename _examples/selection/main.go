// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command selection shows selecting text with the mouse, and copying it,
// in six kinds of node:
//
//   - The log is a selectable viewport, so a selection there takes lines
//     scrolled out of sight, and wheels with the text. Dragging past its
//     top or bottom scrolls it.
//   - The requests, shown in the log's place, are a selectable list of ten
//     thousand rows, which builds only the rows in sight. A selection there
//     keeps the rows it starts and ends in as the wheel scrolls them away,
//     and copies every row it covers, a line to a row, with a space between
//     the cells of a row.
//   - The request is one selectable column of fields, so a selection runs
//     across them, a value beside its label, and leaves the pane's border
//     out. A value cut to fit is copied whole. Its error is a styled text
//     that wraps to the pane's width; a selection across its rows is
//     copied as the one line it is.
//   - The dialog is a selectable layer holding several nodes, so a
//     selection there runs across them, and stays inside the dialog, over
//     the log it covers. Its token is a component that shows it masked and
//     implements bloom.TextSource, so that selecting it copies the token.
//   - The notes are a component that wraps and scrolls its own text, beside
//     a gutter of line numbers, and implements bloom.TextMap, so that a
//     selection there stays on its text as the wheel scrolls it, and leaves
//     the gutter out.
//   - The note is a one-line input that implements bloom.Editable: it keeps
//     its own selection, which the mouse sets and shift and the arrows make,
//     and typing replaces it. Either way, it is the selection, and ctrl+c
//     copies it.
//
// A drag selects a run of text, as a terminal does, and alt held as the
// drag starts selects a rectangle. A double click selects a word, and a
// triple click a line. A click clears the selection. Copying sweeps a band
// across what was copied.
//
// Keys: ctrl+c copies the selection, or quits when there is none; ctrl+a
// selects all of the log, or of the requests; l shows the requests in the
// log's place, and back; a turns copying as soon as text is selected on and
// off; d opens the dialog; i types in the note, until esc; esc closes the
// dialog, or clears the selection; q quits.
package main

import (
	"fmt"
	"os"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// The IDs of the log's viewport and the list of requests, to select all of
// them, of the notes, to find how big they are, and of the note, to select
// all of it.
const (
	logText      = "log"
	requestsList = "requests"
	notesText    = "notes"
	noteText     = "note"
)

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	log      logpane     // the log, or the requests in its place
	request  requestPane // the request's fields
	notes    *reader
	note     *input
	dialog   dialog
	autoCopy bool
	status   string
}

func newModel() *model {
	m := &model{st: newStyles(tint.TintTokyoNight), status: "drag over any text to select it"}
	m.ui = bloom.New(
		bloom.WithColors(m.st.fit.Color(roles.Fg), m.st.fit.Color(roles.Bg)),
		// A background alone keeps the colors of the log's levels. The
		// dim timestamps, hard to read on it, take the theme's
		// foreground instead.
		bloom.WithSelectionStyle(m.st.selection),
		bloom.WithClipboard(clip.Write),
	)
	m.log = newLogpane()
	m.log.setStyles(m.st)
	m.request = requestPane{st: m.st}
	m.notes = &reader{lines: notesLines, numStyle: m.st.label}
	m.note = &input{st: m.st, text: "refund 1042 retried by hand"}
	m.dialog = dialog{st: m.st}
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// The notes wrap to the width they are given, and show as many rows
		// as fit.
		if r, ok := bloom.Solve(m.w, m.h, m.layout()).Rect(notesText); ok {
			m.notes.w, m.notes.h = r.Dx(), r.Dy()
			m.notes.scroll(0)
		}
	case bloom.SelectionMsg:
		if m.autoCopy {
			return m, tea.Batch(cmd, m.ui.CopySelection())
		}
		m.status = fmt.Sprintf("selected %q in %v; ctrl+c copies", preview(msg.Text), msg.ID)
	case bloom.CopyMsg:
		m.status = fmt.Sprintf("copied %q", preview(msg.Text))
		if n := strings.Count(msg.Text, "\n") + 1; n > 1 {
			m.status += fmt.Sprintf(", %d lines", n)
		}
		if msg.Err != nil {
			m.status += " with OSC 52 only: " + msg.Err.Error()
		}
	case tea.KeyPressMsg:
		if m.note.typing && msg.String() != "ctrl+c" {
			m.typeKey(msg)
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c":
			if copied := m.ui.CopySelection(); copied != nil {
				return m, tea.Batch(cmd, copied)
			}
			return m, tea.Quit
		case "ctrl+a":
			switch {
			case m.log.listed && m.ui.SelectAll(requestsList):
				// Selecting all builds none of the rows; they are built when
				// the text is read, as when it is copied.
				m.status = fmt.Sprintf("selected all %d requests; ctrl+c copies", requests)
			case !m.log.listed && m.ui.SelectAll(logText):
				text, _ := m.ui.Selection()
				m.status = fmt.Sprintf("selected all %d lines of the log; ctrl+c copies", strings.Count(text, "\n")+1)
			}
		case "l":
			m.log.listed = !m.log.listed
			m.status = "showing the log"
			if m.log.listed {
				m.status = fmt.Sprintf("showing %d requests", requests)
			}
		case "a":
			m.autoCopy = !m.autoCopy
			m.status = fmt.Sprintf("copy on select: %v", m.autoCopy)
		case "d":
			m.dialog.open = true
		case "i":
			m.note.typing = true
			m.status = "typing in the note; shift+arrows select, esc stops"
		case "esc":
			if m.dialog.open {
				m.dialog.open = false
			} else {
				m.ui.ClearSelection()
				m.status = "selection cleared"
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.fit.Color(roles.Bg)
	return v
}

// typeKey handles a key pressed while typing in the note.
func (m *model) typeKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.note.typing = false
		m.status = "stopped typing"
		return
	case "ctrl+a":
		m.ui.SelectAll(noteText)
	default:
		m.note.key(msg)
	}
	// A range the note made itself is the selection at once.
	if text, ok := m.ui.Selection(); ok {
		m.status = fmt.Sprintf("selected %q in the note; ctrl+c copies", preview(text))
	} else {
		m.status = "typing in the note; shift+arrows select, esc stops"
	}
}

// preview is s cut to a line short enough for the status line.
func preview(s string) string {
	s = strings.ReplaceAll(s, "\n", "⏎")
	if r := []rune(s); len(r) > 40 {
		return string(r[:39]) + "…"
	}
	return s
}

// layout places the panes, the status line and the keys under them, and
// the dialog over them while it is open.
func (m *model) layout() bloom.Child {
	// The notes scroll their own text with the wheel, and are selectable as
	// a bloom.TextMap, without being made so. They draw their own cells, so
	// they are placed with bloom.View.
	notesPane := m.st.pane("notes", bloom.Column(bloom.Fill(bloom.View(m.notes)).ID(notesText).
		On(on.Scroll(func(_, dy int) tea.Cmd { m.notes.scroll(dy); return nil }))))

	// The note keeps its own selection, as a bloom.Editable, and is placed
	// by pointer. A click on it starts typing.
	notePane := m.st.pane("note", bloom.Row(bloom.Fill(bloom.View(m.note)).ID(noteText).
		On(on.Click(func() tea.Cmd { m.note.typing = true; return nil }))))

	hints := "ctrl+c copy · ctrl+a all · 2×/3× word/line · alt block · l list · i type · a auto-copy · d dialog · q quit"
	base := bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(&m.log).Min(30),
			bloom.Fixed(44, bloom.Column(&m.request, bloom.Fill(notesPane))).Collapsible(1),
		)),
		notePane,
		bloom.Text(" "+m.status).Style(m.st.message).Truncate(),
		bloom.Text(" "+hints).Style(m.st.status).Truncate(),
	)

	var layers []bloom.LayerNode
	if m.dialog.open {
		layers = append(layers, m.dialog.Layer())
	}
	return bloom.Overlay(base, layers...)
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
