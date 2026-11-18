// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command tooltips is a deployments view whose tooltips come from bloom.
// The toolbar's icon buttons say what they do: the pointer resting on one
// shows its tooltip after a moment, and the next one along shows at once.
// Most place themselves where they hide the least, and the settings button
// asks for its right side. Images too long for their column are cut with
// "…", and unfold in place while the pointer is over them: moving on along
// the row, onto the status the unfold covers, closes it. A status that
// needs explaining has a tooltip of its own, which finds empty space to go
// in, and a failed one is drawn in red. The secret column is cut too, but is
// marked NoTooltip, so it never shows in full.
//
// Tooltips can go to the status line instead, which holds a TooltipSink.
// The selected row's image is pinned open, so it shows in full as the
// selection moves with the keyboard, and tab moves the focus along the
// toolbar, which shows each button's tooltip.
//
// Keys: tab and shift+tab move the focus along the toolbar, enter and space
// press the focused button, up and down (or k and j) move the selection, t
// sends tooltips to the status line and back, esc clears the selection and
// the focus, q quits.
// A click on a row selects it.
package main

import (
	"fmt"
	"os"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the example's colors come from.
var theme = tint.TintCatppuccinMocha

var (
	fg     = theme.Fg
	muted  = theme.BrightBlack
	accent = theme.Purple
	dark   = theme.Bg
	button = lipgloss.Lighten(theme.Bg, 0.1)

	page      = lipgloss.NewStyle().Padding(1, 2, 0)
	title     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	text      = lipgloss.NewStyle().Foreground(fg)
	label     = lipgloss.NewStyle().Foreground(muted)
	header    = lipgloss.NewStyle().Foreground(muted).Bold(true)
	marker    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	tool      = lipgloss.NewStyle().Foreground(fg).Background(button).Padding(0, 1)
	toolHover = tool.Background(lipgloss.Lighten(theme.Bg, 0.2))
	toolOn    = tool.Foreground(dark).Background(accent).Bold(true)
	statusBar = lipgloss.NewStyle().Foreground(fg).Background(lipgloss.Lighten(theme.Bg, 0.05)).Padding(0, 1)
	hint      = statusBar.Foreground(muted).Padding(0, 1, 0, 0)

	healthy  = lipgloss.NewStyle().Foreground(theme.Green)
	degraded = lipgloss.NewStyle().Foreground(theme.Yellow)
	failed   = lipgloss.NewStyle().Foreground(theme.Red)
	rolling  = lipgloss.NewStyle().Foreground(theme.Blue)

	// failedTip is the tooltip of a failed deploy, which stands out from
	// the others.
	failedTip = lipgloss.NewStyle().Foreground(dark).Background(theme.Red).Padding(0, 1)
)

// The widths of the table's columns but the image's, which takes the rest.
const (
	serviceWidth = 10
	statusWidth  = 11
	secretWidth  = 14
)

// action is the ID of a toolbar button, which focus moves between.
type action string

const (
	actionRefresh  action = "refresh"
	actionDeploy   action = "deploy"
	actionRollback action = "rollback"
	actionStop     action = "stop"
	actionSettings action = "settings"
)

// toolButton is a button of the toolbar: its icon, and its tooltip.
type toolButton struct {
	id   action
	icon string
	tip  string
	opts []bloom.TipOption
}

var tools = []toolButton{
	{id: actionRefresh, icon: "⟳", tip: "Refresh"},
	{id: actionDeploy, icon: "▶", tip: "Deploy"},
	{id: actionRollback, icon: "↶", tip: "Roll back"},
	{id: actionStop, icon: "■", tip: "Stop"},
	{id: actionSettings, icon: "⚙", tip: "Settings", opts: []bloom.TipOption{bloom.TipSide(bloom.Right)}},
}

// deployment is a row of the table. A status that needs explaining has a
// note, its tooltip.
type deployment struct {
	service string
	image   string
	status  string
	note    string
	secret  string
}

var deployments = []deployment{
	{"api", "registry.internal/platform/api:v2.14.3-rc.1+build.8812", "healthy", "", "sk_live_4eC39HqLyjWDarjtT1"},
	{"web", "registry.internal/web:v5.2.0", "healthy", "", "sk_live_9fA02LmQpZbXcVt7Rk"},
	{"payments", "registry.internal/payments/ledger-api:2026.11.3-hotfix.2", "degraded", "2 of 3 replicas ready · restarted 4m ago", "sk_live_T7yH1nBvC3xZqW8e"},
	{"search", "registry.internal/search:v1.9.4", "healthy", "", "sk_live_Mn5Kp2Qs8Xr4Tv1w"},
	{"worker", "registry.internal/jobs/worker-batch:v3.0.0-beta.7-arm64", "rolling", "rolling out · 4 of 6 pods updated", "sk_live_Hb6Gc9Jd2Ke5Lf8m"},
	{"mailer", "registry.internal/notify/mailer:v0.8.1-patch.3", "failed", "CrashLoopBackOff · exit code 137 (out of memory)", "sk_live_Qa3Wz7Ex1Rc5Tv9y"},
}

type model struct {
	ui    *bloom.Root
	w, h  int
	focus bloom.Focus[action]
	tips  bloom.Tooltips
	sink  bool // tooltips go to the status line
	row   int  // the selected row, or -1
	note  string
}

func newModel() *model {
	m := &model{row: -1, note: "prod-eu-1 · 6 services"}
	m.ui = bloom.New(
		bloom.WithColors(theme.Fg, theme.Bg),
		bloom.WithFocus(&m.focus),
		bloom.WithTooltips(m.tips),
	)
	return m
}

// Init does not return the root's Init, which asks the terminal for its
// colors: the theme's are used instead.
func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.ui.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		cmd, ok := m.focus.Update(msg, m.ui.Frame())
		if !ok {
			cmd = m.key(msg)
		}
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "up", "k":
		m.row = max(m.row-1, 0)
	case "down", "j":
		m.row = min(m.row+1, len(deployments)-1)
	case "esc":
		m.row = -1
		m.focus.Clear()
	case "t":
		m.sink = !m.sink
		mode := bloom.TipDefault
		if m.sink {
			mode = bloom.TipSink
		}
		m.tips.Explicit, m.tips.Truncated = mode, mode
		m.ui.SetTooltips(m.tips)
	}
	return nil
}

// press runs the toolbar button with ID id.
func (m *model) press(id action) tea.Cmd {
	name := "every service"
	if m.row >= 0 {
		name = deployments[m.row].service
	}
	switch id {
	case actionRefresh:
		m.note = "refreshed just now"
	case actionDeploy:
		m.note = "deploying " + name
	case actionRollback:
		m.note = "rolling back " + name
	case actionStop:
		m.note = "stopping " + name
	case actionSettings:
		m.note = "no settings yet"
	}
	return nil
}

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it, and reports focus, which tooltips close on losing.
	v.BackgroundColor = theme.Bg
	v.ReportFocus = true
	return v
}

func (m *model) layout() bloom.Child {
	bar := []bloom.Child{bloom.Text("Deployments").Style(title), bloom.Fixed(1, nil)}
	for _, t := range tools {
		bar = append(bar, m.toolButton(t))
	}
	bar = append(bar, bloom.Space(), bloom.Text(m.note).Style(label))

	rows := []bloom.Child{tableRow(bloom.Text(""),
		bloom.Text("SERVICE").Style(header), bloom.Text("IMAGE").Style(header),
		bloom.Text("STATUS").Style(header), bloom.Text("SECRET").Style(header))}
	for i := range deployments {
		rows = append(rows, m.deploymentRow(i))
	}

	return bloom.Column(
		bloom.Box(page, bloom.Column(
			bloom.Row(bar...).Gap(1),
			bloom.Fixed(1, nil),
			bloom.Column(rows...),
		)),
		bloom.Space(),
		m.statusLine(),
	)
}

// toolButton is a button of the toolbar, an icon with a tooltip.
func (m *model) toolButton(t toolButton) bloom.Child {
	st := tool
	if m.focus.Is(t.id) {
		st = toolOn
	}
	return bloom.Text(t.icon).Style(st).HoverStyle(toolHover).
		ID(t.id).Focusable().
		On(on.Press(func() tea.Cmd { return m.press(t.id) })).
		Tooltip(t.tip, t.opts...)
}

// tableRow is a row of the table, with its cells in their columns, after
// the mark of the selected row.
func tableRow(mark, service, image, status, secret bloom.TextNode) bloom.FlexNode {
	return bloom.Row(
		bloom.Fixed(1, mark),
		bloom.Fixed(serviceWidth, service.Truncate()),
		bloom.Fill(image.Truncate()),
		bloom.Fixed(statusWidth, status),
		bloom.Fixed(secretWidth, secret.Truncate()),
	).Gap(1)
}

// deploymentRow is row i of the table. Its image and secret are cut to fit:
// the image unfolds under the pointer, and is pinned open while the row is
// selected, and the secret never shows in full.
func (m *model) deploymentRow(i int) bloom.Child {
	d := deployments[i]
	mark := bloom.Text("")
	if i == m.row {
		mark = bloom.Text("›").Style(marker)
	}
	image := bloom.Text(d.image).Style(text).TooltipOpen(i == m.row)
	secret := bloom.Text(d.secret).Style(label).NoTooltip()
	row := tableRow(mark, bloom.Text(d.service).Style(text), image, status(d), secret)
	return row.On(on.Click(func() tea.Cmd {
		m.row = i
		return nil
	}))
}

// status is the status cell of d, with its note as a tooltip.
func status(d deployment) bloom.TextNode {
	var st lipgloss.Style
	var icon string
	var opts []bloom.TipOption
	switch d.status {
	case "healthy":
		st, icon = healthy, "●"
	case "degraded":
		st, icon = degraded, "◐"
	case "rolling":
		st, icon = rolling, "◌"
	case "failed":
		st, icon = failed, "✕"
		opts = append(opts, bloom.TipStyle(failedTip))
	}
	return bloom.Text(icon+" "+d.status).Style(st).Tooltip(d.note, opts...)
}

// statusLine is the line at the bottom, with the keys: a TooltipSink while
// t sends tooltips there, and a hint otherwise.
func (m *model) statusLine() bloom.Child {
	where := "float"
	var left bloom.Child = bloom.Text("hover an icon, or press tab").Style(statusBar.Foreground(muted)).Truncate()
	if m.sink {
		where = "sink"
		left = bloom.TooltipSink().Style(statusBar)
	}
	keys := fmt.Sprintf("tips: %s (t) · q quit", where)
	return bloom.Row(bloom.Fill(left), bloom.Text(keys).Style(hint))
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
