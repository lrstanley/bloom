// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command responsive is a reader that adapts to the terminal's size. The
// details panel collapses first and the article list second as the terminal
// narrows; the toolbar is a Layouter that rebuilds itself for the space it
// gets; the tags wrap onto as many lines as they need; the status bar is a
// Measurer; and paging uses
// bloom.Solve in Update to know how tall the reader is.
//
// Keys: up/down pick an article, pgup/pgdown or j/k scroll, q quits. The
// mouse wheel scrolls the reader and a click picks an article.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom"
	"github.com/lrstanley/bloom/on"
)

var (
	fg     = lipgloss.Color("#c0caf5")
	text   = lipgloss.Color("#a9b1d6")
	subtle = lipgloss.Color("#9aa5ce")
	accent = lipgloss.Color("#7dcfff")
	purple = lipgloss.Color("#bb9af7")
	muted  = lipgloss.Color("#565f89")
	dark   = lipgloss.Color("#1a1b26")
	barBg  = lipgloss.Color("#24283b")

	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	bar      = lipgloss.NewStyle().Foreground(fg).Background(barBg)
	title    = bar.Bold(true).Foreground(accent).Padding(0, 1)
	action   = bar.Foreground(subtle).Padding(0, 1)
	item     = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	selected = item.Foreground(dark).Background(accent).Bold(true)
	prose    = lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	chip     = lipgloss.NewStyle().Foreground(dark).Background(purple).Padding(0, 1)
	bold     = lipgloss.NewStyle().Bold(true)
	note     = lipgloss.NewStyle().Foreground(muted)
)

// paneID is the ID of a pane, for finding its size with bloom.Solve.
type paneID string

const (
	paneNav     paneID = "nav"
	paneReader  paneID = "reader"
	paneDetails paneID = "details"
	paneContent paneID = "content" // the scrolled text, to know its height
)

type article struct {
	title string
	tags  []string
	body  string
}

var articles = []article{
	{
		title: "Constraints go down",
		tags:  []string{"layout", "design", "flutter", "gio"},
		body:  essay("constraints", 18),
	},
	{
		title: "Sizes come up",
		tags:  []string{"sizing", "measure"},
		body:  essay("measurement", 26),
	},
	{
		title: "Parents place",
		tags:  []string{"arrange", "flex", "slots", "collapse", "ownership"},
		body:  essay("placement", 22),
	},
	{
		title: "Cells, not pixels",
		tags:  []string{"terminal", "unicode", "compositor"},
		body:  essay("cells", 30),
	},
}

// essay returns numbered paragraphs about topic, long enough to wrap at most
// widths, so that the reader reflows them as the terminal narrows and widens.
func essay(topic string, paragraphs int) string {
	const text = "§%d. A paragraph about %s, long enough to wrap at most widths so that " +
		"the reader has to reflow it as the terminal narrows and widens.\n\n"

	var b strings.Builder
	for i := range paragraphs {
		fmt.Fprintf(&b, text, i+1, topic)
	}
	return strings.TrimSpace(b.String())
}

// toolbar is a [bloom.Layouter]: it shows its actions with labels when there
// is room, and only their icons when there is not.
type toolbar struct{ title string }

func (t toolbar) View() string { return t.title }

func (t toolbar) Layout(ctx bloom.Context) bloom.Child {
	actions := []string{"⟳ refresh", "✎ edit", "⇪ share", "⚙ settings"}
	children := []bloom.Child{
		bloom.Text("◆ " + t.title).Style(title),
		bloom.Fill(bloom.Text("").Style(bar)),
	}
	for _, a := range actions {
		if ctx.Width < 70 {
			a, _, _ = strings.Cut(a, " ")
		}
		children = append(children, bloom.Text(a).Style(action))
	}
	return bloom.Row(children...)
}

// chips returns tags as chips that wrap onto lines to fit.
func chips(tags []string) bloom.Child {
	cs := make([]bloom.Child, len(tags))
	for i, tag := range tags {
		cs[i] = bloom.Text("#" + tag).Style(chip)
	}
	return bloom.Wrap(cs...).Gap(1, 1)
}

// statusbar is a [bloom.Measurer]: one row, of any width, as the column
// it is in stretches it across.
type statusbar struct{ text string }

func (s statusbar) View() string { return bar.Render(" " + s.text) }

func (statusbar) Measure(bloom.Size) bloom.Size { return bloom.Size{H: 1} }

type model struct {
	ui       *bloom.Root
	w, h     int
	selected int
	offset   int // the reader's scroll position, in rows
}

func newModel() *model { return &model{ui: bloom.New()} }

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.ui.Update(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			m.pick(m.selected - 1)
		case "down":
			m.pick(m.selected + 1)
		case "j":
			m.offset++
		case "k":
			m.offset--
		case "pgdown", "space":
			m.offset += m.geometry().page
		case "pgup":
			m.offset -= m.geometry().page
		}
	}
	// The wheel's handler scrolled before Update, which clamps it here too.
	m.offset = min(max(m.offset, 0), m.geometry().maxOffset)
	return m, cmd
}

func (m *model) pick(i int) {
	m.selected = min(max(i, 0), len(articles)-1)
	m.offset = 0
}

// geometry is what Update needs to know about the layout: how many rows the
// reader shows, how far it can scroll, and which panels fit.
type geometry struct {
	page, maxOffset int
	panes           []string
}

func (m *model) geometry() geometry {
	// The status bar reports the geometry, so it is solved with an empty one,
	// which as a Measurer is the same size.
	frame := bloom.Solve(m.layout(statusbar{}), m.w, m.h)
	var g geometry
	if r, ok := frame.Rect(paneReader); ok {
		g.page = max(r.Dy()-panel.GetVerticalFrameSize(), 1)
	}
	if r, ok := frame.Rect(paneContent); ok {
		g.maxOffset = max(r.Dy()-g.page, 0)
	}
	for _, p := range []paneID{paneNav, paneReader, paneDetails} {
		if _, ok := frame.Rect(p); ok {
			g.panes = append(g.panes, string(p))
		}
	}
	return g
}

func (m *model) layout(status statusbar) bloom.Child {
	art := articles[m.selected]

	// The details collapse first as the terminal narrows, then the list.
	nav := bloom.Fixed(24, bloom.Box(panel, m.articleList())).
		ID(paneNav).
		Collapsible(2)

	content := bloom.Text(art.body).Style(prose).ID(paneContent)
	reader := bloom.Fill(bloom.Box(panel, bloom.Scroll(content, m.offset))).
		ID(paneReader).
		Min(40).
		On(on.Scroll(func(_, dy int) tea.Cmd { m.offset += dy * 3; return nil }))

	paragraphs := fmt.Sprintf("%d paragraphs", strings.Count(art.body, "§"))
	info := bloom.Column(
		bloom.Text(art.title).Style(bold),
		bloom.Fixed(1, nil),
		chips(art.tags),
		bloom.Space(),
		bloom.Text(paragraphs).Style(note),
	)
	details := bloom.Percent(28, bloom.Box(panel.Padding(0, 1), info)).
		ID(paneDetails).
		Min(28).
		Collapsible(1)

	return bloom.Column(
		toolbar{title: "reader"},
		bloom.Fill(bloom.Row(nav, reader, details).Gap(1)),
		status,
	)
}

// articleList is the list of articles, which can be clicked.
func (m *model) articleList() bloom.Child {
	items := make([]bloom.Child, 0, len(articles)+1)
	for i, a := range articles {
		st := item
		if i == m.selected {
			st = selected
		}
		items = append(items, bloom.Text(a.title).Style(st).On(on.Click(func() tea.Cmd { m.pick(i); return nil })))
	}
	items = append(items, bloom.Space())
	return bloom.Column(items...)
}

func (m *model) status() statusbar {
	g := m.geometry()
	text := fmt.Sprintf(
		"%d×%d · showing %s · row %d of %d · page %d",
		m.w, m.h, strings.Join(g.panes, ", "), m.offset+1, g.maxOffset+1, g.page,
	)
	return statusbar{text: text}
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout(m.status()))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
