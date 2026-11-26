// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lrstanley/bubbletint/chromatint/v2"
	"github.com/lrstanley/bubbletint/roles/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// trail is the path to the selected function in the tree, and the selected
// row of a list without focus: a role the example adds to bubbletint's, a
// step above the panel it sits on, and a step short of a row under the
// pointer.
var trail = roles.Define(roles.Spec{
	Name: "trail",
	Kind: roles.KindSurface,
	Derive: func(p *roles.Palette) tint.Color {
		return p.Color(roles.Surface).Mix(p.Color(roles.Hover), 0.6)
	},
	Distinct: []roles.Role{roles.Surface, roles.Hover},
})

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	fit *roles.Fitted // the tint's roles, which the example's colors come from

	text   lipgloss.Style
	muted  lipgloss.Style
	faint  lipgloss.Style
	bright lipgloss.Style
	accent lipgloss.Style

	item         lipgloss.Style
	itemPath     lipgloss.Style
	itemSel      lipgloss.Style
	itemPreview  lipgloss.Style
	itemHover    lipgloss.Style
	bar          lipgloss.Style
	name         lipgloss.Style
	nameSel      lipgloss.Style
	nameExternal lipgloss.Style
	meta         lipgloss.Style
	iconFunc     lipgloss.Style
	iconMethod   lipgloss.Style
	iconIface    lipgloss.Style
	iconExternal lipgloss.Style
	wire         lipgloss.Style
	wireHot      lipgloss.Style

	screen       lipgloss.Style
	panel        lipgloss.Style
	dialog       lipgloss.Style
	row          lipgloss.Style
	rowSel       lipgloss.Style
	rowSelBlur   lipgloss.Style
	code         map[chroma.TokenType]lipgloss.Style
	mark         lipgloss.Style
	lineNo       lipgloss.Style
	lineNoMarked lipgloss.Style

	// The lists of the code view, and the results of search, which are rows
	// without padding, and how each row looks under the pointer.
	callsPanel      lipgloss.Style
	rowHover        lipgloss.Style
	rowSelHover     lipgloss.Style
	rowSelBlurHover lipgloss.Style
	result          lipgloss.Style
	resultSel       lipgloss.Style
	resultHover     lipgloss.Style
	resultSelHover  lipgloss.Style

	// codeStyle is the chroma style made from the tint by chromatint,
	// without backgrounds, so the code takes its pane's.
	codeStyle *chroma.Style

	// painted caches highlighted lines of code: they are drawn every frame
	// while anything animates.
	painted map[paintKey]string
}

// paintKey is a line of a function's code, with the callee whose call sites
// are marked on it.
type paintKey struct {
	f      *fn
	marked *fn
	line   int
}

// newStyles makes the example's styles from tint t. bubbletint's roles
// derive what each color is for, such as muted text or the selected row, from
// the tint, so the example reads on any tint, dark or light. The code alone
// is colored by the tint's own palette, through chromatint.
func newStyles(t *tint.Tint) styles {
	f := roles.Derive(t).Fit(roles.Target{Profile: colorprofile.TrueColor})
	raised := f.On(roles.Raised) // the roles for drawing on a dialog
	c := f.Color

	// A function in the tree past the selected one is drawn on a fill half
	// way from the screen's background to a panel's.
	p := f.Palette()
	preview := f.Fit(p.Color(roles.Bg).Mix(p.Color(roles.Surface), 0.5), roles.Surface)

	codeStyle, err := chroma.NewStyle("golens", chromatint.StyleEntry(t, false))
	if err != nil {
		panic(err)
	}

	onSel := f.On(roles.Selection).Color(roles.Fg) // text on the selected row
	item := f.Fill(roles.Surface).PaddingRight(1)
	panel := f.Fill(roles.Surface).Padding(0, 1)
	row := f.Text(roles.Fg).Padding(0, 1)
	rowSel := f.Fill(roles.Selection).Padding(0, 1)
	rowSelBlur := row.Background(c(trail))
	return styles{
		fit: f,

		text:   f.Text(roles.Fg),
		muted:  f.Text(roles.Muted),
		faint:  f.Text(roles.Subtle),
		bright: f.Text(roles.Fg).Bold(true),
		accent: f.Text(roles.Accent),

		item:         item,
		itemPath:     item.Background(c(trail)),
		itemSel:      item.Background(c(roles.Selection)),
		itemPreview:  item.Background(preview),
		itemHover:    item.Background(c(roles.Hover)),
		bar:          f.Text(roles.Accent),
		name:         f.Text(roles.Fg),
		nameSel:      lipgloss.NewStyle().Foreground(onSel).Bold(true),
		nameExternal: f.Text(roles.Muted).Italic(true),
		meta:         f.Text(roles.Muted),
		iconFunc:     f.Text(roles.Yellow),
		iconMethod:   f.Text(roles.Blue),
		iconIface:    f.Text(roles.Cyan),
		iconExternal: f.Text(roles.Muted),
		wire:         f.Text(roles.Border),
		wireHot:      f.Text(roles.Accent),

		screen:     f.Fill(roles.Bg),
		panel:      panel,
		dialog:     raised.Fill(roles.Raised).Border(lipgloss.RoundedBorder()).BorderForeground(raised.Color(roles.Border)).BorderBackground(c(roles.Raised)).Padding(0, 1),
		row:        row,
		rowSel:     rowSel,
		rowSelBlur: rowSelBlur,

		code:         make(map[chroma.TokenType]lipgloss.Style),
		mark:         f.Fill(roles.Accent.Soft()).Bold(true),
		lineNo:       f.Text(roles.Subtle),
		lineNoMarked: f.Text(roles.Accent),

		callsPanel:      panel.Padding(0),
		rowHover:        row.Background(c(roles.Hover)),
		rowSelHover:     rowSel.Background(c(roles.Hover)),
		rowSelBlurHover: rowSelBlur.Background(c(roles.Hover)),
		result:          row.Padding(0),
		resultSel:       rowSel.Padding(0),
		resultHover:     row.Padding(0).Background(c(roles.Hover)),
		resultSelHover:  rowSel.Padding(0).Background(c(roles.Hover)),

		codeStyle: codeStyle,
		painted:   make(map[paintKey]string),
	}
}

// icon returns f's icon and its style.
func (st *styles) icon(f *fn) (string, lipgloss.Style) {
	switch f.kind {
	case kindMethod:
		return "ƒ", st.iconMethod
	case kindInterface:
		return "◇", st.iconIface
	case kindExternal:
		return "ƒ", st.iconExternal
	case kindFunc:
	}
	return "ƒ", st.iconFunc
}

// line returns line i of f's code, highlighted, with the call sites of
// marked on it marked.
func (st *styles) line(f *fn, i int, marked *fn) string {
	sites := f.sites[marked]
	var on []site
	for _, s := range sites {
		if s.line == i {
			on = append(on, s)
		}
	}
	if len(on) == 0 {
		marked = nil
	}
	key := paintKey{f: f, marked: marked, line: i}
	if s, ok := st.painted[key]; ok {
		return s
	}
	var mark func(int) bool
	if len(on) > 0 {
		mark = func(col int) bool {
			for _, s := range on {
				if s.col == col {
					return true
				}
			}
			return false
		}
	}
	s := st.paint(f.src[i], f.toks[i], mark)
	st.painted[key] = s
	return s
}

// signature returns f's signature, highlighted.
func (st *styles) signature(f *fn) string {
	key := paintKey{f: f, line: -1}
	if s, ok := st.painted[key]; ok {
		return s
	}
	lines := strings.Split(f.sig, "\n")
	toks := highlight(lines)
	for i := range lines {
		lines[i] = st.paint(lines[i], toks[i], nil)
	}
	s := strings.Join(lines, "\n")
	st.painted[key] = s
	return s
}
