// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command logs tails a server log of a quarter of a million lines, which
// grows as it is shown, in a bloom.Viewport. The viewport lays out and
// draws only the lines in sight, so appending to the log and scrolling
// through it cost the same at any length.
//
//   - It follows the end of the log while it is there, as tail -f does,
//     and stays put when scrolled away from it.
//   - Lines wrap at words, or are cut off and scroll sideways, and either
//     way the top line stays at the top when the wrapping changes.
//   - A search marks every match of a regular expression in the lines in
//     sight, and moves between them, through the lines out of sight too.
//   - The scrollbars on the border follow it, and drag it, and the wheel
//     scrolls it, with the same function.
//   - Text is selected with the mouse, and the rows of a wrapped line are
//     copied as the one line it is, without the line numbers beside it.
//
// Keys: up/down or j/k scroll a line, pgup/pgdown a page, home/end or g/G
// go to the start or the end; left/right or h/l scroll sideways; w wraps;
// # shows line numbers; / searches, n and N move between matches, and esc
// clears the search; p pauses the log; ctrl+c copies the selection, or quits
// when there is none; q quits.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

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
	match   = lipgloss.NewStyle().Background(theme.Yellow).Foreground(theme.Bg)
	current = lipgloss.NewStyle().Background(theme.Purple).Foreground(theme.Bg).Bold(true)
	focused = lipgloss.NewStyle().Background(lipgloss.Lighten(theme.Bg, 0.06))
)

const (
	paneLog = "log"
	// preloaded is how many lines the log starts with.
	preloaded = 250_000
	// every is how often a line is added to the log.
	every = 150 * time.Millisecond
)

type tickMsg struct{}

type model struct {
	ui   *bloom.Root
	w, h int

	log     *bloom.Buffer
	lines   int // how many lines were written, to number the next
	pos     bloom.ViewportPos
	wrap    bool
	numbers bool
	paused  bool

	searching bool   // the query is being typed
	query     string // what was typed
	re        *regexp.Regexp
	found     bloom.Match
	hasFound  bool

	status string
}

func newModel() *model {
	m := &model{
		ui:      bloom.New(bloom.WithColors(theme.Fg, theme.Bg), bloom.WithClipboard(clip.Write)),
		log:     &bloom.Buffer{},
		pos:     bloom.ViewportEnd,
		wrap:    true,
		numbers: true,
		status:  "following the log",
	}
	lines := make([]string, preloaded)
	for i := range lines {
		lines[i] = logLine(i)
	}
	m.log.Append(lines...)
	m.lines = preloaded
	return m
}

// The levels of a log line.
const (
	info  = "INFO"
	warn  = "WARN"
	fail  = "ERROR"
	debug = "DEBUG"
)

// The styles of a log line, as the escape sequences that start them, made
// once rather than rendered for each line.
var (
	stamp  = sgr(lipgloss.NewStyle().Foreground(muted))
	reset  = "\x1b[m"
	levels = map[string]string{
		info:  sgr(lipgloss.NewStyle().Foreground(theme.Blue)),
		warn:  sgr(lipgloss.NewStyle().Foreground(theme.Yellow)),
		fail:  sgr(lipgloss.NewStyle().Foreground(theme.Red).Bold(true)),
		debug: sgr(lipgloss.NewStyle().Foreground(muted)),
	}
	order = []string{info, info, debug, info, warn, info, info, fail, debug, info, info, info, warn}
)

// sgr returns the escape sequence that starts style.
func sgr(style lipgloss.Style) string {
	s, _, _ := strings.Cut(style.Render("x"), "x")
	return s
}

// logLine is line i of a made-up server log.
func logLine(i int) string {
	at := time.Date(2026, 11, 8, 14, 0, 0, 0, time.UTC).Add(time.Duration(i) * 137 * time.Millisecond)
	level := order[i%len(order)]
	var msg string
	switch level {
	case fail:
		msg = fmt.Sprintf("POST /api/v1/orders/%d/refund 502 in %dms: payments: dial tcp 10.0.4.%d:443: i/o timeout "+
			"(attempt %d of 3, retrying with the same idempotency key rfnd_%x)", 1000+i%9000, 900+i%400, i%250, 1+i%3, i*7919)
	case warn:
		msg = fmt.Sprintf("upstream payments slow: %dms for order %d", 700+i%600, 1000+i%9000)
	case debug:
		msg = fmt.Sprintf("pool: %d idle, %d busy", i%8, i%5)
	default:
		msg = fmt.Sprintf("GET /api/v1/orders/%d 200 in %dms", 1000+i%9000, 3+i%40)
	}
	return stamp + at.Format("15:04:05.000") + reset + " " + levels[level] + fmt.Sprintf("%-5s", level) + reset + " " + msg
}

func tick() tea.Cmd { return tea.Tick(every, func(time.Time) tea.Msg { return tickMsg{} }) }

func (m *model) Init() tea.Cmd { return tea.Batch(m.ui.Init(), tick()) }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		if !m.paused {
			m.log.Append(logLine(m.lines))
			m.lines++
		}
		return m, tea.Batch(cmd, tick())
	case bloom.SelectionMsg:
		m.status = fmt.Sprintf("selected %q; ctrl+c copies", preview(msg.Text))
	case bloom.CopyMsg:
		m.status = fmt.Sprintf("copied %q", preview(msg.Text))
	case tea.KeyPressMsg:
		if m.searching {
			m.typeQuery(msg)
			return m, cmd
		}
		return m, tea.Batch(cmd, m.key(msg))
	}
	return m, cmd
}

// key handles a key pressed while not searching.
func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	// Scrolling goes from where the viewport was in the frame on screen,
	// which knows how its lines wrapped.
	v, _ := m.ui.Frame().Viewport(paneLog)
	switch msg.String() {
	case "up", "k":
		m.pos = v.Up(1)
	case "down", "j":
		m.pos = v.Down(1)
	case "pgup":
		m.pos = v.PageUp()
	case "pgdown":
		m.pos = v.PageDown()
	case "home", "g":
		m.pos = v.Top()
	case "end", "G":
		m.pos = bloom.ViewportEnd
	case "left", "h":
		m.pos = v.Left(8)
	case "right", "l":
		m.pos = v.Right(8)
	case "w":
		m.wrap = !m.wrap
		m.status = map[bool]string{true: "wrapping at words", false: "cut off; left and right scroll"}[m.wrap]
	case "#":
		m.numbers = !m.numbers
	case "p":
		m.paused = !m.paused
		m.status = map[bool]string{true: "paused", false: "resumed"}[m.paused]
	case "/":
		m.searching, m.query = true, ""
	case "n":
		m.find(v, true)
	case "N":
		m.find(v, false)
	case "esc":
		m.re, m.hasFound = nil, false
		m.ui.ClearSelection()
		m.status = "search cleared"
	case "ctrl+c":
		if copied := m.ui.CopySelection(); copied != nil {
			return copied
		}
		return tea.Quit
	case "q":
		return tea.Quit
	}
	if m.pos.AtEnd() {
		m.status = "following the log"
	}
	return nil
}

// typeQuery handles a key pressed while typing the query.
func (m *model) typeQuery(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		m.searching = false
		re, err := regexp.Compile("(?i)" + m.query)
		if err != nil || m.query == "" {
			m.status = "not a search"
			return
		}
		m.re, m.hasFound = re, false
		v, _ := m.ui.Frame().Viewport(paneLog)
		m.find(v, true)
	case "esc":
		m.searching = false
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
	default:
		m.query += msg.Text
	}
}

// find moves to the next match of the search, or the one before, from the
// last one found, or from the top of v, wrapping around the log.
func (m *model) find(v bloom.ViewportView, forward bool) {
	if m.re == nil {
		return
	}
	line, at := v.First, 0
	if m.hasFound {
		line, at = m.found.Line, m.found.End
		if !forward {
			at = m.found.Start
		}
	}
	var found bloom.Match
	var ok bool
	wrapped := ""
	if forward {
		if found, ok = m.log.Find(m.re, line, at); !ok {
			found, ok = m.log.Find(m.re, 0, 0)
			wrapped = ", from the top"
		}
	} else if found, ok = m.log.FindBefore(m.re, line, at); !ok {
		found, ok = m.log.FindBefore(m.re, m.log.Len()-1, 1<<30)
		wrapped = ", from the bottom"
	}
	if !ok {
		m.status = fmt.Sprintf("no match for %q", m.query)
		return
	}
	m.found, m.hasFound = found, true
	m.pos = v.Reveal(found.Line)
	m.status = fmt.Sprintf("%q on line %d%s", m.query, found.Line+1, wrapped)
}

// marks marks every match of the search in a line, and the one found in
// another style.
func (m *model) marks(line int, text string) []bloom.Mark {
	locs := m.re.FindAllStringIndex(text, -1)
	marks := make([]bloom.Mark, len(locs))
	for i, l := range locs {
		marks[i] = bloom.Mark{Start: l[0], End: l[1], Style: match}
		if m.hasFound && line == m.found.Line && l[0] == m.found.Start {
			marks[i].Style = current
		}
	}
	return marks
}

// preview is s cut to a line short enough for the status line.
func preview(s string) string {
	s = strings.ReplaceAll(s, "\n", "⏎")
	if r := []rune(s); len(r) > 40 {
		return string(r[:39]) + "…"
	}
	return s
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.BackgroundColor = theme.Bg
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) layout() bloom.Child {
	log := bloom.Viewport(m.log).ID(paneLog).At(m.pos).Follow().Selectable().
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { m.pos = p; return nil })
	if m.wrap {
		log = log.Wrap(bloom.WrapWords)
	}
	if m.numbers {
		log = log.LineNumbers(numbers)
	}
	if m.re != nil {
		log = log.Marks(m.marks).LineStyle(func(line int) lipgloss.Style {
			if m.hasFound && line == m.found.Line {
				return focused
			}
			return lipgloss.Style{}
		})
	}

	v, _ := m.ui.Frame().Viewport(paneLog)
	pane := bloom.Box(panel, log).
		Edge(bloom.Top, bloom.Start, bloom.Text("server.log").Style(title)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf("%d lines", m.log.Len())).Style(counter)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneLog).Thumb("┃", thumb))
	if !m.wrap {
		pane = pane.Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneLog).Horizontal().Thumb("━", thumb))
	}

	left := " " + m.status
	if m.searching {
		left = " /" + m.query + "█"
	}
	where := fmt.Sprintf("line %d of %d · %d%% ", v.First+1, m.log.Len(), int(v.Percent()*100))
	switch {
	case m.paused:
		where += "· paused "
	case v.Pos.AtEnd():
		where += "· following "
	}
	hints := " ↑↓ pgup pgdn g G scroll · ←→ sideways · w wrap · # numbers · / search · n N matches · p pause · q quit"
	return bloom.Column(
		bloom.Fill(pane),
		bloom.Row(
			bloom.Fill(bloom.Text(left).Style(status).Truncate()),
			bloom.Text(where).Style(hint),
		),
		bloom.Text(hints).Style(hint).Truncate(),
	)
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
