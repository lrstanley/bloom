// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package theme has the desktop's colors and styles, all made from one
// bubbletint tint by NewStyles. The model and each component keep a copy of
// their own. To change the theme, [Change] sends a [ChangeMsg] with the new
// styles, which the model and every component take in Update.
package theme

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// Tints are the tints the settings app offers: old screens and the neon of
// the eighties. The first is the one the desktop starts with.
var Tints = []*tint.Tint{
	tint.TintRosePineMoon, tint.TintAmberCrtRetro, tint.TintGreenPhosphorCrt, tint.TintIbm5153Cga,
	tint.TintVacuumTube, tint.TintTerminalGreen1999, tint.TintOutrunElectric, tint.TintCursedVhs,
	tint.TintCyberdyne, tint.TintMiamiHeat,
}

// Styles are the desktop's colors and styles.
type Styles struct {
	Tint *tint.Tint // the tint the styles are made from

	// The desktop: its wallpaper, and its icons.
	Paper     lipgloss.Style
	Icon      lipgloss.Style
	IconHover lipgloss.Style
	IconOn    lipgloss.Style

	// A window: its frame and title bar, which are brighter on the window
	// in front, and the controls on its title bar.
	Frame   lipgloss.Style
	FrameOn lipgloss.Style
	Title   lipgloss.Style
	TitleOn lipgloss.Style
	Grip    lipgloss.Style
	GripOn  lipgloss.Style
	Control lipgloss.Style
	CtlOver lipgloss.Style
	CtlShut lipgloss.Style
	Corner  lipgloss.Style

	// The outline of where a dragged window would snap to.
	Outline lipgloss.Style

	// What the apps show inside their windows: text, and rows to pick from.
	Body     lipgloss.Style
	Faint    lipgloss.Style
	Row      lipgloss.Style
	RowHover lipgloss.Style
	RowOn    lipgloss.Style

	// The calculator's display and keys.
	Display    lipgloss.Style
	Key        lipgloss.Style
	KeyOp      lipgloss.Style
	KeyHover   lipgloss.Style
	KeyEq      lipgloss.Style
	KeyEqHover lipgloss.Style

	// The start menu: an item, as wide as the menu, the item under the
	// pointer, the rule above logging out, and the banner beside them.
	MenuItem   lipgloss.Style
	MenuHover  lipgloss.Style
	MenuRule   lipgloss.Style
	Banner     lipgloss.Style
	BannerText lipgloss.Style

	// The taskbar: the start button, a button for each window, the status,
	// and the clock.
	Bar      lipgloss.Style
	Start    lipgloss.Style
	StartOn  lipgloss.Style
	Task     lipgloss.Style
	TaskOn   lipgloss.Style
	TaskMin  lipgloss.Style
	TaskOver lipgloss.Style
	Status   lipgloss.Style
	Clock    lipgloss.Style

	// The splash: the logo, the progress bar under it, in its done and to do
	// parts, and the word under that.
	SplashLogo lipgloss.Style
	SplashLine lipgloss.Style
	SplashDone lipgloss.Style
	SplashToDo lipgloss.Style
	SplashWord lipgloss.Style
}

// NewStyles makes the desktop's styles from tint t.
func NewStyles(t *tint.Tint) Styles {
	fg, muted, accent, hot, dark := t.Fg, t.BrightBlack, t.Purple, t.Red, t.Bg
	winBg, raised, barBg := lipgloss.Lighten(t.Bg, 0.06), lipgloss.Lighten(t.Bg, 0.12), lipgloss.Darken(t.Bg, 0.25)

	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).BorderBackground(winBg).Background(winBg).Padding(0, 1)
	title := lipgloss.NewStyle().Foreground(muted).Background(winBg)
	grip := lipgloss.NewStyle().Foreground(muted).Background(winBg)
	control := lipgloss.NewStyle().Foreground(fg).Background(winBg)
	body := lipgloss.NewStyle().Foreground(fg).Background(winBg)
	key := lipgloss.NewStyle().Foreground(fg).Background(raised)
	keyEq := key.Foreground(dark).Background(accent).Bold(true)
	icon := lipgloss.NewStyle().Foreground(fg).Width(12).Align(lipgloss.Center).Padding(0, 1)
	rowOn := body.Foreground(dark).Background(t.Blue).Bold(true)
	menuItem := lipgloss.NewStyle().Foreground(fg).Background(raised)
	banner := lipgloss.NewStyle().Foreground(dark).Background(accent).Bold(true)
	start := lipgloss.NewStyle().Foreground(dark).Background(accent).Bold(true)
	task := lipgloss.NewStyle().Foreground(fg).Background(raised)
	line := lipgloss.NewStyle().Width(SplashWidth).Align(lipgloss.Center)

	return Styles{
		Tint: t,

		Paper:     lipgloss.NewStyle().Foreground(lipgloss.Lighten(t.Bg, 0.14)),
		Icon:      icon,
		IconHover: icon.Background(raised),
		IconOn:    icon.Foreground(dark).Background(accent),

		Frame:   frame,
		FrameOn: frame.BorderForeground(accent),
		Title:   title,
		TitleOn: title.Foreground(dark).Background(accent).Bold(true),
		Grip:    grip,
		GripOn:  grip.Foreground(accent),
		Control: control,
		CtlOver: control.Foreground(dark).Background(accent),
		CtlShut: control.Foreground(dark).Background(hot),
		Corner:  grip.Foreground(accent),

		Outline: lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accent),

		Body:     body,
		Faint:    body.Foreground(muted),
		Row:      body,
		RowHover: body.Background(raised),
		RowOn:    rowOn,

		Display:    lipgloss.NewStyle().Foreground(fg).Background(barBg).Bold(true).Padding(0, 1),
		Key:        key,
		KeyOp:      key.Foreground(t.Cyan),
		KeyHover:   key.Foreground(dark).Background(t.Cyan),
		KeyEq:      keyEq,
		KeyEqHover: keyEq.Background(t.BrightPurple),

		MenuItem:   menuItem.Width(MenuWidth),
		MenuHover:  rowOn.Width(MenuWidth),
		MenuRule:   menuItem.Foreground(muted),
		Banner:     banner,
		BannerText: banner.Width(3).Align(lipgloss.Center),

		Bar:      lipgloss.NewStyle().Background(barBg),
		Start:    start,
		StartOn:  start.Background(t.BrightPurple),
		Task:     task,
		TaskOn:   task.Foreground(dark).Background(t.Blue).Bold(true),
		TaskMin:  task.Foreground(muted).Background(barBg),
		TaskOver: task.Background(lipgloss.Lighten(t.Bg, 0.2)),
		Status:   lipgloss.NewStyle().Foreground(muted).Background(barBg),
		Clock:    lipgloss.NewStyle().Foreground(fg).Background(raised),

		SplashLogo: line.Foreground(accent).Bold(true),
		SplashLine: line,
		SplashDone: lipgloss.NewStyle().Foreground(accent),
		SplashToDo: lipgloss.NewStyle().Foreground(muted),
		SplashWord: line.Foreground(muted),
	}
}

// MenuWidth is how wide the start menu's items are.
const MenuWidth = 18

// SplashWidth is how wide the splash's logo and progress bar are.
const SplashWidth = 24

// ChangeMsg changes the desktop's theme to the one Styles are made from. The
// model takes it first, and then gives it to each of its components, which
// pass it on to theirs. Each copies the styles: they are held by pointer so
// that the message stays small, as every component checks every message for
// it.
type ChangeMsg struct{ Styles *Styles }

// Change returns the command that changes the theme to tint t.
func Change(t *tint.Tint) tea.Cmd {
	return func() tea.Msg {
		st := NewStyles(t)
		return ChangeMsg{Styles: &st}
	}
}
