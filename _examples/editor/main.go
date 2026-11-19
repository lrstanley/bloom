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
	"unicode/utf8"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNight

var (
	muted  = theme.BrightBlack
	accent = theme.Blue

	panel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	title   = lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 1)
	counter = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	status  = lipgloss.NewStyle().Foreground(theme.Fg)
	hint    = lipgloss.NewStyle().Foreground(muted)
	numbers = lipgloss.NewStyle().Foreground(muted)
	thumb   = lipgloss.NewStyle().Foreground(accent)
	onBar   = thumb.Foreground(theme.Purple)
	current = lipgloss.NewStyle().Background(lipgloss.Lighten(theme.Bg, 0.05))
)

const (
	paneText = "text"
	// undoSteps is how many steps of the undo history are kept.
	undoSteps = 1000
)

// step is a step of the undo history: the text, and what was selected in
// it, to go back to.
type step struct {
	text bloom.Snapshot
	sel  bloom.Range
}

// The kinds of edit that join the one before of the same kind into one
// step of the undo history.
const (
	joinNone   = ""
	joinTyping = "typing"
	joinDelete = "delete"
)

type model struct {
	ui   *bloom.Root
	w, h int

	buf  *bloom.Buffer
	name string // what the title calls the text
	path string // the file it is saved to, if it was opened from one

	sel    bloom.Range       // what is selected, the caret at its Head
	pos    bloom.ViewportPos // where the viewport is scrolled to
	goal   int               // the column kept while moving up and down, or -1
	wrap   bool
	cursor *tea.Cursor

	undo, redo []step
	joins      string        // the kind of the last edit, which one of the same kind joins
	cost       time.Duration // what the last edit cost
	changed    bool          // the text changed since it was opened or saved
	saving     bool          // a save is under way, which the next waits for
	clip       string        // the text last copied or cut, which ctrl+v pastes

	status string
}

// newModel returns an editor of the file at path, or of the sample text
// without one, followed by generated lines until there are lines of them.
func newModel(path string, lines int) (*model, error) {
	m := &model{
		ui:     bloom.New(bloom.WithColors(theme.Fg, theme.Bg), bloom.WithClipboard(clip.Write)),
		buf:    &bloom.Buffer{},
		name:   "sample.txt",
		goal:   -1,
		wrap:   true,
		cursor: tea.NewCursor(0, 0),
		status: "type away; ctrl+q quits",
	}
	m.cursor.Shape = tea.CursorBar
	// Words, for ctrl and the arrows and for a double click alike, are those
	// of code, joined only across "_", rather than taking in a path or a URL
	// whole, as they do by default.
	m.buf.SetWordChars("_")
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m.buf.SetContent(string(data))
		m.name, m.path = path, path
	} else {
		m.buf.SetContent(sample)
	}
	if more := lines - m.buf.Len(); more > 0 {
		start := time.Now()
		generated := make([]string, more)
		for i := range generated {
			generated[i] = generatedLine(m.buf.Len() + i)
		}
		m.buf.Append(generated...)
		m.status = fmt.Sprintf("generated %s lines in %s", count(more), time.Since(start).Round(time.Millisecond))
	}
	return m, nil
}

// sample is the text shown when no file is named.
const sample = `A small editor, built from bloom.Viewport and bloom.Buffer
===========================================================

The caret is a range kept in the model: bloom draws it, puts the terminal cursor at it, and sets it from the mouse. Everything else here is a keymap of a few dozen lines, and an undo history of snapshots.

Try it:
	- type anywhere, and undo it with ctrl+z, as one step;
	- select with shift and the arrows, or by dragging, or double clicking a word;
	- cut with ctrl+x, and paste it somewhere else with ctrl+v;
	- turn wrapping off with alt+z, and scroll sideways past the end of a long line.

Wide characters and tabs take the cells they are drawn in, and the caret never lands inside one: 日本語のテキスト, café, naïve, and	a	few	tabs.

Lines wrap at words, and the caret moves down the rows a line wraps to, keeping its column, as it does down the lines: this paragraph is one line, long enough to wrap two or three times at the width of most terminals, so that moving up and down through it shows the rows.
`

// The words generated lines are made of.
var (
	subjects = []string{"The editor", "A buffer", "Each viewport", "The caret", "This line", "A snapshot", "The terminal"}
	verbs    = []string{"draws", "keeps", "wraps", "measures", "restores", "follows", "shows"}
	objects  = []string{
		"only the rows in sight", "every line it is given", "the text under the cursor",
		"a million lines at once", "what changed since the last frame", "the column it was at",
	}
)

// generatedLine is line i of the generated text.
func generatedLine(i int) string {
	s := fmt.Sprintf("%s %s %s, on line %d.", subjects[i%len(subjects)], verbs[i/3%len(verbs)], objects[i/7%len(objects)], i+1)
	if i%5 == 0 {
		s += " It goes on a while longer than the lines around it, so that some of them wrap."
	}
	return s
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
		m.replace(m.sel.From(), m.sel.To(), msg.Content, joinNone)
		m.show()
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
		m.changed, m.status = m.buf.Rev() != msg.rev, "saved "+m.path
	}
	return m, cmd
}

// key handles a key press: the keymap.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	// The rows of the text are those of the frame on screen, which knows
	// how its lines wrap, worked out again for the text as it is now.
	v, _ := m.ui.Frame().Viewport(paneText)
	// Shift with a key that moves the caret selects as it moves.
	name := msg.String()
	extend := strings.Contains(name, "shift+")
	key := strings.Replace(name, "shift+", "", 1)
	if m.moveKey(v, key, extend) {
		return nil
	}
	switch key {
	// Editing.
	case "enter":
		m.replace(m.sel.From(), m.sel.To(), "\n"+m.indent(), joinNone)
	case "tab":
		m.replace(m.sel.From(), m.sel.To(), "\t", joinTyping)
	case "backspace":
		if from := m.sel.From(); m.sel.Empty() {
			m.replace(m.buf.Prev(from), from, "", joinDelete)
		} else {
			m.replace(from, m.sel.To(), "", joinNone)
		}
	case "delete":
		if from := m.sel.From(); m.sel.Empty() {
			m.replace(from, m.buf.Next(from), "", joinDelete)
		} else {
			m.replace(from, m.sel.To(), "", joinNone)
		}
	case "ctrl+z":
		// ctrl+shift+z redoes.
		if extend {
			m.redoStep()
		} else {
			m.undoStep()
		}
	case "ctrl+y":
		m.redoStep()

	// The clipboard, through the Root, so that what is copied flashes.
	case "ctrl+a":
		m.ui.SelectAll(paneText)
		m.status = fmt.Sprintf("selected all %s lines", count(m.buf.Len()))
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
		m.replace(m.sel.From(), m.sel.To(), "", joinNone)
		m.show()
		return copied
	case "ctrl+v":
		m.replace(m.sel.From(), m.sel.To(), m.clip, joinNone)

	case "alt+z":
		m.wrap = !m.wrap
		m.status = map[bool]string{true: "wrapping at words", false: "not wrapping; lines scroll sideways"}[m.wrap]
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
		m.replace(m.sel.From(), m.sel.To(), msg.Text, joinTyping)
	}
	m.goal = -1
	m.show()
	return nil
}

// moveKey moves the caret for key, without shift, and with extend, the
// Head of the selection, keeps it in sight, and reports whether key moves
// it. Each key moves the Head, and the selection is collapsed to it unless
// shift is held.
func (m *model) moveKey(v bloom.ViewportView, key string, extend bool) bool {
	sel, head, page := m.sel, m.sel.Head, max(v.Rows-1, 1)
	switch key {
	// Up and down keep the column the caret was at; the other keys forget it.
	case "up", "down":
		n := 1
		if key == "up" {
			n = -1
		}
		m.moveTo(v.MoveRows(sel, n, m.column(v)), extend)
		m.show()
		return true
	case "pgup", "pgdown":
		// A page, keeping the caret on the row of the viewport it was on.
		n := page
		if key == "pgup" {
			n = -page
		}
		sel, m.pos = v.ScrollRows(sel, n, m.column(v))
		m.moveTo(sel, extend)
		return true

	case "left":
		if !extend && !sel.Empty() {
			sel = sel.WithHead(sel.From())
		} else {
			sel = sel.WithHead(m.buf.Prev(head))
		}
	case "right":
		if !extend && !sel.Empty() {
			sel = sel.WithHead(sel.To())
		} else {
			sel = sel.WithHead(m.buf.Next(head))
		}
	case "ctrl+left":
		sel = sel.WithHead(m.buf.PrevWord(head))
	case "ctrl+right":
		sel = sel.WithHead(m.buf.NextWord(head))
	// Home and end go to the ends of the row the caret is drawn on, end to
	// the very end of a row the line wraps after, where the caret stays.
	case "home":
		sel = v.RowStart(sel)
	case "end":
		sel = v.RowEnd(sel)
	case "ctrl+home":
		sel = sel.WithHead(bloom.Pos{})
	case "ctrl+end":
		sel = sel.WithHead(m.buf.End())
	default:
		return false
	}
	m.goal = -1
	m.moveTo(sel, extend)
	m.show()
	return true
}

// moveTo makes sel, its Head moved, what is selected, or without extend,
// the caret at its Head.
func (m *model) moveTo(sel bloom.Range, extend bool) {
	if !extend {
		sel = sel.Collapse()
	}
	m.sel, m.joins = sel, joinNone
}

// column returns the column kept while moving up and down, that of the
// caret before it first moved up or down.
func (m *model) column(v bloom.ViewportView) int {
	if m.goal < 0 {
		m.goal = v.Column(m.sel)
	}
	return m.goal
}

// show scrolls the viewport as little as it takes for the caret to be in
// sight.
func (m *model) show() {
	if v, ok := m.ui.Frame().Viewport(paneText); ok {
		m.pos = v.Show(m.sel)
	}
}

// replace replaces the text from from up to to with text, puts the caret
// after it, and notes the step in the undo history, unless it joins the
// last one: an edit of a kind other than joinNone joins the edit before it
// of the same kind, so that typing in a row is undone at once.
func (m *model) replace(from, to bloom.Pos, text, kind string) {
	if from == to && text == "" {
		return
	}
	if kind == joinNone || kind != m.joins {
		m.undo = append(m.undo, step{text: m.buf.Snapshot(), sel: m.sel})
		if len(m.undo) > undoSteps {
			m.undo = m.undo[1:]
		}
	}
	m.redo = m.redo[:0]
	start := time.Now()
	c := m.buf.Replace(from, to, text)
	m.cost = time.Since(start)
	m.sel = bloom.Range{Anchor: c.End(), Head: c.End()}
	m.joins, m.changed = kind, true
}

// undoStep puts the text and the selection back as they were before the
// last step of the undo history.
func (m *model) undoStep() {
	if m.travel(&m.undo, &m.redo) {
		m.status = fmt.Sprintf("undone; %d more to undo", len(m.undo))
	}
}

// redoStep takes the step undone last again.
func (m *model) redoStep() {
	if m.travel(&m.redo, &m.undo) {
		m.status = fmt.Sprintf("redone; %d more to redo", len(m.redo))
	}
}

// travel goes back to the last step of from, keeping where it is now as a
// step of to, and reports whether there was a step to go to.
func (m *model) travel(from, to *[]step) bool {
	if len(*from) == 0 {
		return false
	}
	*to = append(*to, step{text: m.buf.Snapshot(), sel: m.sel})
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	start := time.Now()
	m.buf.Restore(s.text)
	m.cost = time.Since(start)
	m.sel, m.joins, m.changed = s.sel, joinNone, true
	return true
}

// indent returns the spaces and tabs the caret's line starts with, up to
// the caret, for a new line to start with too.
func (m *model) indent() string {
	at := m.sel.From()
	text := m.buf.Text(at.Line)[:at.Col]
	return text[:len(text)-len(strings.TrimLeft(text, " \t"))]
}

// save writes the text to the file it was opened from, from a snapshot,
// which may be read while the buffer goes on changing.
func (m *model) save() tea.Cmd {
	if m.path == "" {
		m.status = "nothing to save to: name a file to edit"
		return nil
	}
	if m.saving {
		// One write at a time, so that an older one cannot land last.
		m.status = "still saving: try again in a moment"
		return nil
	}
	m.saving = true
	text, rev, path := m.buf.Snapshot(), m.buf.Rev(), m.path
	return func() tea.Msg {
		return savedMsg{rev: rev, err: os.WriteFile(path, []byte(text.String()+"\n"), 0o600)}
	}
}

// caretMoved forgets the column kept while moving up and down, and ends the
// typing an undo step joins, when the mouse moves the caret.
func (m *model) caretMoved(bloom.Range) tea.Cmd {
	m.goal, m.joins = -1, joinNone
	return nil
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

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	return v
}

func (m *model) layout() bloom.Child {
	caretLine := m.sel.Head.Line
	text := bloom.Viewport(m.buf).ID(paneText).At(m.pos).LineNumbers(numbers).
		Caret(&m.sel).OnCaret(m.caretMoved).Cursor(m.cursor).
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { m.pos = p; return nil }).
		LineStyle(func(line int) lipgloss.Style {
			if line == caretLine {
				return current
			}
			return lipgloss.Style{}
		})
	if m.wrap {
		text = text.Wrap(bloom.WrapWords)
	}

	name := m.name
	if m.changed {
		name += " ●"
	}
	pane := bloom.Box(panel, text).
		Edge(bloom.Top, bloom.Start, bloom.Text(name).Style(title)).
		Edge(bloom.Top, bloom.End, bloom.Text(count(m.buf.Len())+" lines").Style(counter)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneText).
			Style(bloom.ScrollbarStyle{ThumbStyle: thumb}).HoverStyle(bloom.ScrollbarStyle{ThumbStyle: onBar}))
	if !m.wrap {
		pane = pane.Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneText).Horizontal().
			Style(bloom.ScrollbarStyle{ThumbStyle: thumb}).HoverStyle(bloom.ScrollbarStyle{ThumbStyle: onBar}))
	}

	head := m.buf.Clamp(m.sel.Head)
	col := utf8.RuneCountInString(m.buf.Text(head.Line)[:head.Col]) + 1
	where := fmt.Sprintf("Ln %s, Col %d", count(head.Line+1), col)
	if !m.sel.Empty() {
		where += fmt.Sprintf(" (%s selected)", count(utf8.RuneCountInString(m.buf.TextIn(m.sel.From(), m.sel.To()))))
	}
	cost := "no edits yet"
	if len(m.undo)+len(m.redo) > 0 {
		cost = "last edit " + m.cost.String()
	}
	right := fmt.Sprintf("%s · %s · %s ", where, cost, map[bool]string{true: "wrap", false: "no wrap"}[m.wrap])
	hints := " arrows move · shift selects · ctrl+a all · ctrl+c copy · ctrl+x cut · ctrl+v paste · ctrl+z undo · ctrl+y redo · alt+z wrap · ctrl+q quit"
	return bloom.Column(
		bloom.Fill(pane),
		bloom.Row(
			bloom.Fill(bloom.Text(" "+m.status).Style(status).Truncate()),
			bloom.Text(right).Style(hint),
		),
		bloom.Text(hints).Style(hint).Truncate(),
	)
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
