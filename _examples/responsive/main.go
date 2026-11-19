// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command responsive is a reader that adapts to the terminal's size. The
// details panel collapses first and the article list second as the terminal
// narrows; the toolbar is a Layouter that rebuilds itself for the space it
// gets; the tags wrap onto as many lines as they need; the reader is a
// Viewport, which pages by its height on screen; the status bar is a
// Measurer, which says what bloom.Solve finds in the frame about to be
// drawn; and below the size the reader needs, bloom.MinSize shows a note
// saying the window is too small in place of the layout.
//
// Keys: up/down pick an article, pgup/pgdown or j/k scroll, q quits. The
// mouse wheel scrolls the reader and a click picks an article.
package main

import (
	"fmt"
	"os"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintGruvboxDark

var (
	fg     = theme.Fg
	text   = theme.White
	accent = theme.Yellow
	purple = theme.Purple
	muted  = theme.BrightBlack
	dark   = theme.Bg
	barBg  = lipgloss.Lighten(theme.Bg, 0.05)

	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
	bar      = lipgloss.NewStyle().Foreground(fg).Background(barBg)
	title    = bar.Bold(true).Foreground(accent).Padding(0, 1)
	action   = bar.Foreground(text).Padding(0, 1)
	item     = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	selected = item.Foreground(dark).Background(accent).Bold(true)
	prose    = lipgloss.NewStyle().Foreground(fg)
	chip     = lipgloss.NewStyle().Foreground(dark).Background(purple).Padding(0, 1)
	bold     = lipgloss.NewStyle().Bold(true)
	note     = lipgloss.NewStyle().Foreground(muted)
)

// paneID is the ID of a pane, for finding it in a frame.
type paneID string

const (
	paneNav     paneID = "nav"
	paneReader  paneID = "reader"
	paneDetails paneID = "details"
	paneText    paneID = "text" // the reader's viewport, to scroll it
)

// minWidth and minHeight are the smallest window the reader can be used in:
// the reader's minimum width with its border, and the toolbar, status bar,
// and a few rows of text.
const minWidth, minHeight = 40, 10

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
	text     *bloom.Buffer // the selected article's body
	at       bloom.ViewportPos
}

func newModel() *model {
	return &model{
		ui:   bloom.New(bloom.WithColors(theme.Fg, theme.Bg)),
		text: bloom.NewBuffer(articles[0].body),
	}
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
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
		}
		// The reader scrolls from where it was on screen, by its height there
		// for a page. While it isn't on screen, as when the window is too
		// small for it, keys that would change the layout wait.
		v, ok := m.ui.Frame().Viewport(paneText)
		if !ok {
			break
		}
		switch msg.String() {
		case "up":
			m.pick(m.selected - 1)
		case "down":
			m.pick(m.selected + 1)
		case "j":
			m.at = v.Down(1)
		case "k":
			m.at = v.Up(1)
		case "pgdown", "space":
			m.at = v.PageDown()
		case "pgup":
			m.at = v.PageUp()
		}
	}
	return m, cmd
}

// pick opens article i, from its top.
func (m *model) pick(i int) {
	m.selected = min(max(i, 0), len(articles)-1)
	m.text.SetContent(articles[m.selected].body)
	m.at = bloom.ViewportPos{}
}

func (m *model) layout(status statusbar) bloom.Child {
	art := articles[m.selected]

	// The details collapse first as the terminal narrows, then the list.
	nav := bloom.Fixed(24, bloom.Box(panel, m.articleList())).
		ID(paneNav).
		Collapsible(2)

	// The text wraps to the reader's width. The wheel scrolls it from
	// anywhere over the reader, its border included, three rows a notch.
	text := bloom.Viewport(m.text).ID(paneText).At(m.at).Wrap(bloom.WrapWords).Style(prose)
	reader := bloom.Fill(bloom.Box(panel.Padding(0, 1), text)).
		ID(paneReader).
		Min(40).
		On(on.Scroll(func(_, dy int) tea.Cmd {
			if v, ok := m.ui.Frame().Viewport(paneText); ok {
				m.at = v.Down(dy * 3)
			}
			return nil
		}))

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

// status is the status bar, which says which panes fit and where the reader
// is. They are found with bloom.Solve, which lays out the frame about to be
// drawn without drawing it. The status bar is solved empty: as a Measurer,
// it is the same size whatever it says.
func (m *model) status() statusbar {
	frame := bloom.Solve(m.w, m.h, m.layout(statusbar{}))
	var panes []string
	for _, p := range []paneID{paneNav, paneReader, paneDetails} {
		if _, ok := frame.Rect(p); ok {
			panes = append(panes, string(p))
		}
	}
	v, _ := frame.Viewport(paneText)
	text := fmt.Sprintf(
		"%d×%d · showing %s · row %d of %d · page %d",
		m.w, m.h, strings.Join(panes, ", "), v.Row+1, max(v.TotalRows-v.Rows, 0)+1, v.Rows,
	)
	return statusbar{text: text}
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, bloom.MinSize(minWidth, minHeight, m.layout(m.status())))
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	return v
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
