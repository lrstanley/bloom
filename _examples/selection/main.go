// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command selection shows selecting text with the mouse, and copying it,
// in four kinds of node:
//
//   - The log is a selectable viewport, so a selection there takes lines
//     scrolled out of sight, and wheels with the text. Dragging past its
//     top or bottom scrolls it.
//   - The request is one selectable column of fields, so a selection runs
//     across them, a value beside its label, and leaves the pane's border
//     out. A value cut to fit is copied whole. Its error is a styled text
//     that wraps to the pane's width; a selection across its rows is
//     copied as the one line it is.
//   - The dialog is a selectable layer holding several nodes, so a
//     selection there runs across them, and stays inside the dialog, over
//     the log it covers. Its token is a component that shows it masked and
//     implements bloom.TextSource, so that selecting it copies the token.
//
// A drag selects a run of text, as a terminal does, and alt held as the
// drag starts selects a rectangle. A double click selects a word, and a
// triple click a line. A click clears the selection. Copying sweeps a band
// across what was copied.
//
// Keys: ctrl+c copies the selection, or quits when there is none; ctrl+a
// selects all of the log; a turns copying as soon as text is selected on
// and off; d opens the dialog; esc closes the dialog, or clears the
// selection; q quits.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintTokyoNight

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Blue

	panel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
	title  = lipgloss.NewStyle().Foreground(accent).Bold(true).Padding(0, 1)
	label  = lipgloss.NewStyle().Foreground(muted)
	value  = lipgloss.NewStyle().Foreground(fg)
	status = lipgloss.NewStyle().Foreground(muted)
	dialog = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).
		Padding(1, 2).Background(lipgloss.Lighten(theme.Bg, 0.06))
	token = lipgloss.NewStyle().Foreground(theme.Green).Bold(true)

	levels = map[string]lipgloss.Style{
		"INFO":  lipgloss.NewStyle().Foreground(theme.Blue),
		"WARN":  lipgloss.NewStyle().Foreground(theme.Yellow),
		"ERROR": lipgloss.NewStyle().Foreground(theme.Red),
		"DEBUG": lipgloss.NewStyle().Foreground(muted),
	}
)

// logText is the ID of the log's viewport, to select all of it.
const logText = "log"

type field struct{ name, value string }

var request = []field{
	{"id", "7f3a9c2e-41b8-4d0e-9a51-c2d8e6f01b77"},
	{"method", "POST"},
	{"path", "/api/v1/orders/1042/refund"},
	{"status", "502 Bad Gateway"},
	{"latency", "1.284s"},
	{"client", "203.0.113.42"},
	{"agent", "curl/8.11.0"},
	{"trace", "00-4bf92f3577b34da6a3ce929d0e0e4736-01"},
}

// failure is the error the request failed with, long enough to wrap.
const failure = "payments did not answer within 1000ms while refunding order 1042 " +
	"(attempt 1 of 3); the refund will be retried in 2s with the same idempotency key rfnd_7Hc2Lq9xVb"

// secret is a token shown masked, but for its first and last few
// characters. It implements bloom.TextSource, so that selecting it copies
// the token itself rather than what is shown.
type secret struct {
	value      string
	head, tail int
	style      lipgloss.Style
}

func (s secret) View() string {
	masked := strings.Repeat("•", len(s.value)-s.head-s.tail)
	return s.style.Render(s.value[:s.head] + masked + s.value[len(s.value)-s.tail:])
}

// SelectedText implements bloom.TextSource: the token is one row, a
// character to a cell.
func (s secret) SelectedText(span bloom.Span) string {
	from, to := span.Start.X, span.End.X+1
	if span.Start.Y > 0 {
		from = len(s.value)
	}
	if span.End.Y > 0 {
		to = len(s.value)
	}
	from, to = min(from, len(s.value)), min(to, len(s.value))
	return s.value[from:max(to, from)]
}

type model struct {
	ui       *bloom.Root
	w, h     int
	at       bloom.ViewportPos // where the log is scrolled to
	dialog   bool
	autoCopy bool
	status   string
	log      *bloom.Buffer
}

func newModel() *model {
	return &model{
		ui: bloom.New(
			bloom.WithColors(theme.Fg, theme.Bg),
			// A background alone keeps the colors of the log's levels. The
			// dim timestamps, hard to read on it, take the theme's
			// foreground instead.
			bloom.WithSelectionStyle(lipgloss.NewStyle().Background(theme.SelectionBg)),
			bloom.WithClipboard(clip.Write),
		),
		log:    bloom.NewBuffer(logLines()),
		status: "drag over any text to select it",
	}
}

// logLines is a made-up server log.
func logLines() string {
	msgs := []struct{ level, msg string }{
		{"INFO", "listening on :8080"},
		{"DEBUG", "pool: 4 idle, 0 busy"},
		{"INFO", "GET /api/v1/orders 200 12ms"},
		{"INFO", "GET /api/v1/orders/1042 200 8ms"},
		{"WARN", "upstream payments slow: 950ms"},
		{"INFO", "POST /api/v1/orders/1042/refund started"},
		{"ERROR", "payments: dial tcp 10.0.4.12:443: i/o timeout"},
		{"ERROR", "POST /api/v1/orders/1042/refund 502 1284ms"},
		{"INFO", "retrying refund 1042 in 2s"},
		{"DEBUG", "pool: 3 idle, 1 busy"},
	}
	start := time.Date(2026, 11, 8, 14, 2, 0, 0, time.UTC)
	var b strings.Builder
	for i := range 36 {
		m := msgs[i%len(msgs)]
		if i > 0 {
			b.WriteByte('\n')
		}
		at := start.Add(time.Duration(i) * 1700 * time.Millisecond).Format("15:04:05.000")
		fmt.Fprintf(&b, "%s %s %s", label.Render(at), levels[m.level].Render(fmt.Sprintf("%-5s", m.level)), value.Render(m.msg))
	}
	return b.String()
}

func (m *model) Init() tea.Cmd { return m.ui.Init() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case bloom.SelectionMsg:
		if m.autoCopy {
			return m, tea.Batch(cmd, m.ui.CopySelection())
		}
		m.status = fmt.Sprintf("selected %q in %v; ctrl+c copies", preview(msg.Text), msg.ID)
	case bloom.CopyMsg:
		m.status = fmt.Sprintf("copied %q", preview(msg.Text))
		if msg.Err != nil {
			m.status += " with OSC 52 only: " + msg.Err.Error()
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if copied := m.ui.CopySelection(); copied != nil {
				return m, tea.Batch(cmd, copied)
			}
			return m, tea.Quit
		case "ctrl+a":
			if m.ui.SelectAll(logText) {
				text, _ := m.ui.Selection()
				m.status = fmt.Sprintf("selected all %d lines of the log; ctrl+c copies", strings.Count(text, "\n")+1)
			}
		case "a":
			m.autoCopy = !m.autoCopy
			m.status = fmt.Sprintf("copy on select: %v", m.autoCopy)
		case "d":
			m.dialog = true
		case "esc":
			if m.dialog {
				m.dialog = false
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
	// Motion is only reported while a button is down, which is all a drag
	// needs.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) layout() bloom.Child {
	// All of the log is selectable, so a selection takes what is scrolled
	// out of sight too.
	logPane := bloom.Box(panel, bloom.Viewport(m.log).ID(logText).At(m.at).Selectable().
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { m.at = p; return nil })).
		Edge(bloom.Top, bloom.Start, bloom.Text("server.log").Style(title))

	rows := make([]bloom.Child, 0, len(request))
	for _, f := range request {
		rows = append(rows, bloom.Row(
			bloom.Fixed(9, bloom.Text(f.name).Style(label)),
			// Cut to fit, but copied whole.
			bloom.Fill(bloom.Text(f.value).Style(value).Truncate()),
		))
	}
	rows = append(rows,
		bloom.Fixed(1, nil),
		bloom.Text("error").Style(label),
		// Wrapped to the pane's width, but copied as the one line it is.
		bloom.Fill(bloom.Text(failure).Style(value)),
	)
	// The fields are selectable together, and the pane's border is not.
	requestPane := bloom.Box(panel, bloom.Column(rows...).ID("request").Selectable()).
		Edge(bloom.Top, bloom.Start, bloom.Text("request").Style(title))

	hints := "ctrl+c copy · ctrl+a all · 2×/3× click word/line · alt+drag block · a auto-copy · d dialog · q quit"
	base := bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(logPane).Min(30),
			bloom.Fixed(44, requestPane).Collapsible(1),
		)),
		bloom.Text(" "+m.status).Style(status.Foreground(fg)).Truncate(),
		bloom.Text(" "+hints).Style(status).Truncate(),
	)

	var layers []bloom.LayerNode
	if m.dialog {
		// The whole dialog is selectable: a selection there runs across
		// the nodes inside it.
		layers = append(layers, bloom.Layer(bloom.Box(dialog, bloom.Column(
			bloom.Text("API token created").Style(title.Padding(0)),
			bloom.Fixed(1, nil),
			secret{value: "blm_live_9Qk2Xv7rLp4TzW8nB3sHc6Yd", head: 9, tail: 4, style: token},
			bloom.Fixed(1, nil),
			bloom.Text("Drag over the token, then ctrl+c:\nthe whole token is copied. It is\nnot shown again.").Style(label),
		))).ID("dialog").Selectable().Modal().Backdrop(fx.Dim{Amount: 0.5}))
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
