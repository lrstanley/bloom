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
	"image"
	"os"
	"strconv"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/internal/clip"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
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

// The IDs of the log's viewport and the list of requests, to select all of
// them, of the notes, to find how big they are, and of the note, to select
// all of it.
const (
	logText      = "log"
	requestsList = "requests"
	notesText    = "notes"
	noteText     = "note"
)

// requests is how many requests the list holds, far more than it shows.
const requests = 10_000

// requestRow is request i of the list: a row of cells, each a node of its
// own, so that a selection joins them with a space. The path is cut to fit,
// but copied whole.
func requestRow(i int) bloom.Child {
	method, code := "GET", "200"
	switch i % 5 {
	case 2:
		method, code = "POST", "201"
	case 3:
		code = "404"
	case 4:
		method, code = "DELETE", "502"
	}
	at := time.Date(2026, 11, 8, 14, 2, 0, 0, time.UTC).Add(time.Duration(i) * 350 * time.Millisecond)
	st := value
	if code[0] != '2' {
		st = levels["ERROR"]
	}
	return bloom.Row(
		bloom.Fixed(12, bloom.Text(at.Format("15:04:05.000")).Style(label)),
		bloom.Fixed(6, bloom.Text(method).Style(levels["INFO"])),
		bloom.Fill(bloom.Text(fmt.Sprintf("/api/v1/orders/%d/items", 1000+i)).Style(value).Truncate()),
		bloom.Fixed(3, bloom.Text(code).Style(st)),
	).Gap(1)
}

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

// notesLines are the notes, longer than the pane shows.
var notesLines = []string{
	"Refunds that time out upstream are retried three times, 2s apart, with the same idempotency key.",
	"A 502 from payments is not a failed refund: check the retry queue before refunding again.",
	"Order 1042 was refunded by hand at 14:05, and its retries were cancelled.",
	"Latency over 1s on POST /refund pages whoever is on call.",
	"Logs older than 30 days move to cold storage at midnight UTC.",
}

// gutter is how wide the reader's line numbers are.
const gutter = 3

// reader shows lines of text wrapped at words to w cells, beside a gutter
// of line numbers, h rows at a time from row top of all that they wrap to,
// as a component that scrolls its own text does. It implements
// bloom.TextMap, so that a selection in it is kept on its text as it
// scrolls, takes what is scrolled out of sight, and leaves the gutter out.
type reader struct {
	lines    []string
	top      int
	w, h     int
	numStyle lipgloss.Style
}

// row is a row of a reader: the bytes of a line from from up to to.
type row struct{ line, from, to int }

// rows returns all of the rows the lines wrap to.
func (r *reader) rows() []row {
	var rows []row
	w := max(r.w-gutter, 1)
	for i, l := range r.lines {
		for from := 0; ; {
			to := min(from+w, len(l))
			if sp := strings.LastIndexByte(l[from:to], ' '); to < len(l) && sp > 0 {
				to = from + sp + 1
			}
			rows = append(rows, row{line: i, from: from, to: to})
			if to >= len(l) {
				break
			}
			from = to
		}
	}
	return rows
}

// shown returns the rows in sight.
func (r *reader) shown() []row {
	rows := r.rows()
	top := min(r.top, max(len(rows)-1, 0))
	return rows[top:min(top+r.h, len(rows))]
}

func (r *reader) scroll(dy int) { r.top = min(max(r.top+dy, 0), max(len(r.rows())-r.h, 0)) }

func (r *reader) View() string {
	out := make([]string, 0, r.h)
	for _, s := range r.shown() {
		num := ""
		if s.from == 0 {
			num = strconv.Itoa(s.line + 1)
		}
		out = append(out, r.numStyle.Render(fmt.Sprintf("%*s ", gutter-1, num))+r.lines[s.line][s.from:s.to])
	}
	return strings.Join(out, "\n")
}

func (r *reader) clamp(p bloom.Pos) bloom.Pos {
	last := len(r.lines) - 1
	if p.Line > last {
		return bloom.Pos{Line: last, Col: len(r.lines[last])}
	}
	p.Line = max(p.Line, 0)
	p.Col = min(max(p.Col, 0), len(r.lines[p.Line]))
	return p
}

// PosAt implements bloom.TextMap: a cell in the gutter is the start of its
// row, and one past the text of a row its end.
func (r *reader) PosAt(cell image.Point, _ bloom.Size) bloom.Pos {
	rows := r.shown()
	if cell.Y >= len(rows) {
		return r.clamp(bloom.Pos{Line: len(r.lines)})
	}
	s := rows[max(cell.Y, 0)]
	return bloom.Pos{Line: s.line, Col: s.from + min(max(cell.X-gutter, 0), s.to-s.from)}
}

// Cells implements bloom.TextMap.
func (r *reader) Cells(from, to bloom.Pos, _ bloom.Size, cells func(y, x0, x1 int)) {
	for y, s := range r.shown() {
		if s.line < from.Line || s.line > to.Line {
			continue
		}
		lo, hi := s.from, s.to
		if s.line == from.Line {
			lo = max(lo, from.Col)
		}
		if s.line == to.Line {
			hi = min(hi, to.Col)
		}
		if lo < hi {
			cells(y, gutter+lo-s.from, gutter+hi-s.from)
		}
	}
}

// Text implements bloom.TextMap.
func (r *reader) Text(from, to bloom.Pos) string {
	from, to = r.clamp(from), r.clamp(to)
	var b strings.Builder
	for i := from.Line; i <= to.Line; i++ {
		lo, hi := 0, len(r.lines[i])
		if i == from.Line {
			lo = from.Col
		}
		if i == to.Line {
			hi = to.Col
		}
		if i > from.Line {
			b.WriteByte('\n')
		}
		b.WriteString(r.lines[i][lo:max(hi, lo)])
	}
	return b.String()
}

// input is a one-line field of ASCII text, a byte to a cell. It implements
// bloom.Editable: it keeps its own selection, whose Head is its caret, which
// the arrows move and shift and the arrows extend, and which the mouse sets
// with Select. Typing replaces what is selected. It is placed in the layout
// by pointer, so that Select reaches it.
type input struct {
	text   string
	sel    bloom.Range
	typing bool
}

func (f *input) View() string {
	if !f.typing {
		return value.Render(f.text)
	}
	s, at := f.text+" ", f.sel.Head.Col
	return value.Render(s[:at]) + value.Reverse(true).Render(s[at:at+1]) + value.Render(s[at+1:])
}

func (f *input) col(p bloom.Pos) int {
	if p.Line > 0 {
		return len(f.text)
	}
	return min(max(p.Col, 0), len(f.text))
}

// PosAt implements bloom.TextMap.
func (f *input) PosAt(cell image.Point, _ bloom.Size) bloom.Pos {
	return bloom.Pos{Col: f.col(bloom.Pos{Col: cell.X})}
}

// Cells implements bloom.TextMap.
func (f *input) Cells(from, to bloom.Pos, _ bloom.Size, cells func(y, x0, x1 int)) {
	cells(0, f.col(from), f.col(to))
}

// Text implements bloom.TextMap.
func (f *input) Text(from, to bloom.Pos) string { return f.text[f.col(from):f.col(to)] }

// Selection implements bloom.Editable.
func (f *input) Selection() bloom.Range { return f.sel }

// Select implements bloom.Editable.
func (f *input) Select(r bloom.Range) {
	f.sel = bloom.Range{Anchor: bloom.Pos{Col: f.col(r.Anchor)}, Head: bloom.Pos{Col: f.col(r.Head)}}
}

// key handles a key pressed while typing in the field.
func (f *input) key(msg tea.KeyPressMsg) {
	from, to, head := f.sel.From().Col, f.sel.To().Col, f.sel.Head.Col
	move := func(col int, extend bool) {
		f.sel.Head = bloom.Pos{Col: min(max(col, 0), len(f.text))}
		if !extend {
			f.sel.Anchor = f.sel.Head
		}
	}
	switch k := msg.String(); k {
	case "left", "right":
		left := k == "left"
		switch {
		case from != to && left:
			move(from, false)
		case from != to:
			move(to, false)
		case left:
			move(head-1, false)
		default:
			move(head+1, false)
		}
	case "shift+left":
		move(head-1, true)
	case "shift+right":
		move(head+1, true)
	case "home", "shift+home":
		move(0, k != "home")
	case "end", "shift+end":
		move(len(f.text), k != "end")
	case "backspace":
		if from == to && from > 0 {
			from--
		}
		f.replace(from, to, "")
	default:
		if msg.Text != "" {
			f.replace(from, to, msg.Text)
		}
	}
}

// replace replaces the text from from up to to with s, leaving the caret
// after it.
func (f *input) replace(from, to int, s string) {
	f.text = f.text[:from] + s + f.text[to:]
	f.sel = bloom.Range{Anchor: bloom.Pos{Col: from + len(s)}, Head: bloom.Pos{Col: from + len(s)}}
}

type model struct {
	ui       *bloom.Root
	w, h     int
	at       bloom.ViewportPos // where the log is scrolled to
	listed   bool              // the requests are shown in the log's place
	first    int               // the first request shown
	dialog   bool
	autoCopy bool
	status   string
	log      *bloom.Buffer
	notes    *reader
	note     *input
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
		notes:  &reader{lines: notesLines, numStyle: label},
		note:   &input{text: "refund 1042 retried by hand"},
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
			case m.listed && m.ui.SelectAll(requestsList):
				// Selecting all builds none of the rows; they are built when
				// the text is read, as when it is copied.
				m.status = fmt.Sprintf("selected all %d requests; ctrl+c copies", requests)
			case !m.listed && m.ui.SelectAll(logText):
				text, _ := m.ui.Selection()
				m.status = fmt.Sprintf("selected all %d lines of the log; ctrl+c copies", strings.Count(text, "\n")+1)
			}
		case "l":
			m.listed = !m.listed
			m.status = "showing the log"
			if m.listed {
				m.status = fmt.Sprintf("showing %d requests", requests)
			}
		case "a":
			m.autoCopy = !m.autoCopy
			m.status = fmt.Sprintf("copy on select: %v", m.autoCopy)
		case "d":
			m.dialog = true
		case "i":
			m.note.typing = true
			m.status = "typing in the note; shift+arrows select, esc stops"
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

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	v.BackgroundColor = theme.Bg
	return v
}

func (m *model) layout() bloom.Child {
	// All of the log is selectable, so a selection takes what is scrolled
	// out of sight too.
	logPane := bloom.Box(panel, bloom.Viewport(m.log).ID(logText).At(m.at).Selectable().
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { m.at = p; return nil })).
		Edge(bloom.Top, bloom.Start, bloom.Text("server.log").Style(title))
	if m.listed {
		// Only the rows in sight are built for a frame, and a selection
		// builds those it covers out of sight when its text is read. The
		// wheel scrolls from where the list was, three rows a notch, and so
		// does a drag held past its edge.
		logPane = bloom.Box(panel, bloom.List(requests, requestRow).ID(requestsList).Offset(m.first).Selectable().
			On(on.Scroll(func(_, dy int) tea.Cmd {
				v, _ := m.ui.Frame().List(requestsList)
				m.first = min(max(v.First+dy*3, 0), requests-v.Visible)
				return nil
			}))).
			Edge(bloom.Top, bloom.Start, bloom.Text("requests").Style(title))
	}

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

	// The notes scroll their own text with the wheel, and are selectable as
	// a bloom.TextMap, without being made so.
	notesPane := bloom.Box(panel, bloom.Column(bloom.Fill(m.notes).ID(notesText).
		On(on.Scroll(func(_, dy int) tea.Cmd { m.notes.scroll(dy); return nil })))).
		Edge(bloom.Top, bloom.Start, bloom.Text("notes").Style(title))

	// The note keeps its own selection, as a bloom.Editable, and is placed
	// by pointer. A click on it starts typing.
	notePane := bloom.Box(panel, bloom.Row(bloom.Fill(m.note).ID(noteText).
		On(on.Click(func() tea.Cmd { m.note.typing = true; return nil })))).
		Edge(bloom.Top, bloom.Start, bloom.Text("note").Style(title))

	hints := "ctrl+c copy · ctrl+a all · 2×/3× word/line · alt block · l list · i type · a auto-copy · d dialog · q quit"
	base := bloom.Column(
		bloom.Fill(bloom.Row(
			bloom.Fill(logPane).Min(30),
			bloom.Fixed(44, bloom.Column(requestPane, bloom.Fill(notesPane))).Collapsible(1),
		)),
		notePane,
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
