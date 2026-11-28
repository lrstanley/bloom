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
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/roles/v2"
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
	Tint *tint.Tint    // the tint the styles are made from, which the settings app names
	Fit  *roles.Fitted // the tint's roles, which the desktop's colors come from

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

// NewStyles makes the desktop's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or a window's surface,
// from the tint, so the desktop reads on any tint, even one of a single hue.
func NewStyles(t *tint.Tint) Styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	win := f.On(roles.Surface)  // the roles for drawing in a window, and on the taskbar
	key := f.On(roles.Raised)   // the roles for drawing on a key or a taskbar button
	menu := f.On(roles.Overlay) // the roles for drawing on the start menu

	frame := win.Fill(roles.Surface).Border(lipgloss.RoundedBorder()).
		BorderForeground(win.Color(roles.Border)).BorderBackground(win.Color(roles.Surface)).Padding(0, 1)
	title := win.Text(roles.Muted)
	grip := win.Text(roles.BorderStrong)
	control := win.Fill(roles.Surface)
	body := win.Fill(roles.Surface)
	keyEq := f.Fill(roles.Accent).Bold(true)
	icon := func(s lipgloss.Style) lipgloss.Style { return s.Width(12).Align(lipgloss.Center).Padding(0, 1) }
	rowOn := f.Fill(roles.Accent2).Bold(true)
	menuItem := f.Fill(roles.Overlay)
	banner := f.Fill(roles.Accent).Bold(true)
	button := f.Fill(roles.Raised) // a key, a taskbar button, and the clock
	line := lipgloss.NewStyle().Width(SplashWidth).Align(lipgloss.Center)

	return Styles{
		Tint: t,
		Fit:  f,

		Paper:     f.Text(roles.Border),
		Icon:      icon(f.Text(roles.Fg)),
		IconHover: icon(f.Fill(roles.Bg.Hover())),
		IconOn:    icon(f.Fill(roles.Accent)),

		Frame:   frame,
		FrameOn: frame.BorderForeground(win.Color(roles.Accent)),
		Title:   title,
		TitleOn: win.Fill(roles.Accent).Bold(true),
		Grip:    grip,
		GripOn:  win.Text(roles.Accent),
		Control: control,
		CtlOver: win.Fill(roles.Accent),
		CtlShut: win.Fill(roles.Danger),
		Corner:  win.Text(roles.Accent),

		Outline: lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(f.Color(roles.Accent)),

		Body:     body,
		Faint:    win.Text(roles.Muted),
		Row:      body,
		RowHover: win.Fill(roles.Surface.Hover()),
		RowOn:    rowOn,

		Display:    f.Fill(roles.Sunken).Bold(true).Padding(0, 1),
		Key:        button,
		KeyOp:      key.Text(roles.Accent2),
		KeyHover:   f.Fill(roles.Accent2),
		KeyEq:      keyEq,
		KeyEqHover: f.Fill(roles.Accent.Hover()).Bold(true),

		MenuItem:   menuItem.Width(MenuWidth),
		MenuHover:  rowOn.Width(MenuWidth),
		MenuRule:   menu.Text(roles.Border),
		Banner:     banner,
		BannerText: banner.Width(3).Align(lipgloss.Center),

		Bar:      f.Fill(roles.Surface),
		Start:    banner,
		StartOn:  f.Fill(roles.Accent.Soft()).Bold(true),
		Task:     button,
		TaskOn:   rowOn,
		TaskMin:  win.Text(roles.Muted),
		TaskOver: f.Fill(roles.Raised.Hover()),
		Status:   win.Text(roles.Muted),
		Clock:    button,

		SplashLogo: line.Foreground(f.Color(roles.Accent)).Bold(true),
		SplashLine: line,
		SplashDone: f.Text(roles.Accent),
		SplashToDo: f.Text(roles.Subtle),
		SplashWord: line.Foreground(f.Color(roles.Muted)),
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
