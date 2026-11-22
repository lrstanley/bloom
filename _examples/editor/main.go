// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command editor is a small text editor built from a bloom.Viewport and
// the bloom.Buffer it shows. The viewport is given a caret, a range kept in
// the model, and bloom does the rest of what the mouse does with it; the
// editor is the keymap, the undo history, and what it says in its status
// line. Every key costs a few microseconds of editing and the frame,
// however long the text is.
//
//	editor [-lines n] [file]
//
// It opens the file named, or a sample text without one. With -lines, the
// sample is followed by generated lines until there are n, such as a
// million, to show that typing and jumping about stay instant.
//
//   - The caret moves by the rows the lines wrap to, keeps its column
//     while moving up and down, and the terminal cursor is drawn at it.
//     End takes it to the very end of a row a line wraps after, where it
//     stays on that row, and a page up or down keeps it on its row of the
//     screen as the text scrolls under it.
//   - The mouse puts the caret under the pointer, shift+click selects to
//     it, a drag selects, a double click selects a word and a triple click
//     a line, and the wheel scrolls. Dragging past the edge scrolls too.
//   - Copying sweeps a band across what was copied. Pasting from the
//     terminal, as with ctrl+shift+v, puts in what it sends.
//   - Undo and redo restore the caret as well as the text, from snapshots
//     of the buffer that cost a step to take and to restore; typing in a
//     row is one step.
//
// Keys: arrows move the caret, by words with ctrl; home and end go to the
// ends of the row, pgup and pgdown a page, ctrl+home and ctrl+end the ends
// of the text; with shift, each of them selects. ctrl+a selects all; ctrl+c
// copies, or quits when nothing is selected; ctrl+x cuts, and ctrl+v pastes
// what was last copied or cut; ctrl+z undoes, and ctrl+y or ctrl+shift+z
// redoes; alt+z turns wrapping on and off; ctrl+s saves a file opened; esc
// clears the selection; ctrl+q quits.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// paneText is the ID of the text's viewport.
const paneText = "text"

type model struct {
	ui   *bloom.Root
	st   styles
	w, h int

	text   textarea // the text, in its pane
	saving bool     // a save is under way, which the next waits for
	clip   string   // the text last copied or cut, which ctrl+v pastes

	status string
}

// newModel returns an editor of the file at path, or of the sample text
// without one, followed by generated lines until there are lines of them.
func newModel(path string, lines int) (*model, error) {
	m := &model{st: newStyles(tint.TintTokyoNight), status: "type away; ctrl+q quits"}
	m.ui = bloom.New(bloom.WithColors(m.st.theme.Fg, m.st.theme.Bg), bloom.WithClipboard(clip.Write))
	m.text = newTextarea()
	m.text.setStyles(m.st)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m.text.buf.SetContent(string(data))
		m.text.name, m.text.path = path, path
	} else {
		m.text.buf.SetContent(sample)
	}
	if more := lines - m.text.buf.Len(); more > 0 {
		start := time.Now()
		generated := make([]string, more)
		for i := range generated {
			generated[i] = generatedLine(m.text.buf.Len() + i)
		}
		m.text.buf.Append(generated...)
		m.status = fmt.Sprintf("generated %s lines in %s", count(more), time.Since(start).Round(time.Millisecond))
	}
	return m, nil
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

// savedMsg reports that the text was saved, as it was at revision rev of
// the buffer, or why not.
type savedMsg struct {
	rev uint64
	err error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m, tea.Batch(cmd, m.key(msg))
	case tea.PasteMsg:
		// What the terminal pastes, such as with ctrl+shift+v.
		m.text.replace(m.text.sel.From(), m.text.sel.To(), msg.Content, joinNone)
		m.text.show(m.ui.Frame())
		m.status = fmt.Sprintf("pasted %q", preview(msg.Content))
	case bloom.SelectionMsg:
		m.status = fmt.Sprintf("selected %q", preview(msg.Text))
	case bloom.CopyMsg:
		if msg.Err != nil {
			m.status = "the clipboard did not take it: " + msg.Err.Error()
		}
	case savedMsg:
		m.saving = false
		if msg.err != nil {
			m.status = "not saved: " + msg.err.Error()
			break
		}
		// What was typed while it was being written is not saved yet.
		m.text.changed, m.status = m.text.buf.Rev() != msg.rev, "saved "+m.text.path
	}
	return m, cmd
}

func (m *model) View() tea.View {
	// The text, and the status bar under it.
	v := m.ui.View(m.w, m.h, bloom.Column(
		bloom.Fill(&m.text),
		statusbar{st: &m.st, status: m.status, text: &m.text},
	))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = m.st.theme.Bg
	return v
}

// key handles a key press: the keymap.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	// The rows of the text are those of the frame on screen, which knows
	// how its lines wrap, worked out again for the text as it is now.
	f := m.ui.Frame()
	t := &m.text
	// Shift with a key that moves the caret selects as it moves.
	name := msg.String()
	extend := strings.Contains(name, "shift+")
	key := strings.Replace(name, "shift+", "", 1)
	if t.moveKey(f, key, extend) {
		return nil
	}
	switch key {
	// Editing.
	case "enter":
		t.replace(t.sel.From(), t.sel.To(), "\n"+t.indent(), joinNone)
	case "tab":
		t.replace(t.sel.From(), t.sel.To(), "\t", joinTyping)
	case "backspace":
		if from := t.sel.From(); t.sel.Empty() {
			t.replace(t.buf.Prev(from), from, "", joinDelete)
		} else {
			t.replace(from, t.sel.To(), "", joinNone)
		}
	case "delete":
		if from := t.sel.From(); t.sel.Empty() {
			t.replace(from, t.buf.Next(from), "", joinDelete)
		} else {
			t.replace(from, t.sel.To(), "", joinNone)
		}
	case "ctrl+z":
		// ctrl+shift+z redoes.
		step := t.undoStep
		if extend {
			step = t.redoStep
		}
		if s, ok := step(); ok {
			m.status = s
		}
	case "ctrl+y":
		if s, ok := t.redoStep(); ok {
			m.status = s
		}

	// The clipboard, through the Root, so that what is copied flashes.
	case "ctrl+a":
		m.ui.SelectAll(paneText)
		m.status = fmt.Sprintf("selected all %s lines", count(t.buf.Len()))
		return nil
	case "ctrl+c":
		text, _ := m.ui.Selection()
		if copied := m.ui.CopySelection(); copied != nil {
			m.clip, m.status = text, fmt.Sprintf("copied %q", preview(text))
			return copied
		}
		return tea.Quit
	case "ctrl+x":
		text, _ := m.ui.Selection()
		copied := m.ui.CopySelection()
		if copied == nil {
			return nil
		}
		m.clip, m.status = text, fmt.Sprintf("cut %q", preview(text))
		t.replace(t.sel.From(), t.sel.To(), "", joinNone)
		t.show(f)
		return copied
	case "ctrl+v":
		t.replace(t.sel.From(), t.sel.To(), m.clip, joinNone)

	case "alt+z":
		t.wrap = !t.wrap
		m.status = map[bool]string{true: "wrapping at words", false: "not wrapping; lines scroll sideways"}[t.wrap]
		return nil
	case "esc":
		m.ui.ClearSelection()
		return nil
	case "ctrl+s":
		return m.save()
	case "ctrl+q":
		return tea.Quit

	default:
		// Anything else that types text, typed in place of what is selected.
		if msg.Text == "" || msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
			return nil
		}
		t.replace(t.sel.From(), t.sel.To(), msg.Text, joinTyping)
	}
	t.goal = -1
	t.show(f)
	return nil
}

// save writes the text to the file it was opened from, from a snapshot,
// which may be read while the buffer goes on changing.
func (m *model) save() tea.Cmd {
	if m.text.path == "" {
		m.status = "nothing to save to: name a file to edit"
		return nil
	}
	if m.saving {
		// One write at a time, so that an older one cannot land last.
		m.status = "still saving: try again in a moment"
		return nil
	}
	m.saving = true
	text, rev, path := m.text.buf.Snapshot(), m.text.buf.Rev(), m.text.path
	return func() tea.Msg {
		return savedMsg{rev: rev, err: os.WriteFile(path, []byte(text.String()+"\n"), 0o600)}
	}
}

// count returns n with its thousands separated by commas.
func count(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// preview is s cut to a line short enough for the status line.
func preview(s string) string {
	s = strings.ReplaceAll(s, "\n", "⏎")
	if r := []rune(s); len(r) > 32 {
		return string(r[:31]) + "…"
	}
	return s
}

func main() {
	lines := flag.Int("lines", 0, "follow the text with generated lines until there are `n`, such as 1000000")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: editor [-lines n] [file]")
		flag.PrintDefaults()
	}
	flag.Parse()
	m, err := newModel(flag.Arg(0), *lines)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err = tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
