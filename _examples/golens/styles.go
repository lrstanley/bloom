// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/lrstanley/bubbletint/chromatint/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// styles are the example's colors and styles, all made from one bubbletint
// tint by newStyles, so that nothing is a package global. The model keeps
// them, and gives a copy to each component it keeps. A component built
// afresh each frame borrows the styles of whatever builds it, by pointer,
// rather than copying them into every frame.
type styles struct {
	theme *tint.Tint // the bubbletint tint the colors come from

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

	// codeStyle is the chroma style made from the theme by chromatint,
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

// newStyles makes the example's styles from tint t.
func newStyles(t *tint.Tint) styles {
	// The colors the styles are made from.
	var (
		fg       = t.Fg
		bright   = lipgloss.Lighten(t.Fg, 0.25)
		muted    = lipgloss.Darken(t.BrightBlack, 0.3)
		faint    = lipgloss.Lighten(t.Bg, 0.22)
		accent   = t.BrightRed
		call     = t.BrightYellow
		method   = t.BrightBlue
		iface    = t.BrightCyan
		external = lipgloss.Darken(t.BrightBlack, 0.2)
	)

	// The backgrounds, from the screen's up.
	var (
		bgPanel   = lipgloss.Lighten(t.Bg, 0.03)
		bgItem    = lipgloss.Lighten(t.Bg, 0.05)
		bgPath    = lipgloss.Lighten(t.Bg, 0.1)
		bgHover   = lipgloss.Lighten(t.Bg, 0.13)
		bgSel     = lipgloss.Lighten(t.Bg, 0.16)
		bgMarked  = lipgloss.Darken(t.BrightRed, 0.55)
		bgDialog  = lipgloss.Lighten(t.Bg, 0.06)
		bgPreview = lipgloss.Lighten(t.Bg, 0.025)
	)

	codeStyle, err := chroma.NewStyle("golens", chromatint.StyleEntry(t, false))
	if err != nil {
		panic(err)
	}

	s := lipgloss.NewStyle()
	item := s.Background(bgItem).PaddingRight(1)
	panel := s.Background(bgPanel).Padding(0, 1)
	row := s.Foreground(fg).Padding(0, 1)
	rowSel := row.Background(bgSel).Foreground(bright)
	rowSelBlur := row.Background(bgPath)
	return styles{
		theme: t,

		text:   s.Foreground(fg),
		muted:  s.Foreground(muted),
		faint:  s.Foreground(faint),
		bright: s.Foreground(bright).Bold(true),
		accent: s.Foreground(accent),

		item:         item,
		itemPath:     item.Background(bgPath),
		itemSel:      item.Background(bgSel),
		itemPreview:  item.Background(bgPreview),
		itemHover:    item.Background(bgHover),
		bar:          s.Foreground(accent),
		name:         s.Foreground(fg),
		nameSel:      s.Foreground(bright).Bold(true),
		nameExternal: s.Foreground(external).Italic(true),
		meta:         s.Foreground(muted),
		iconFunc:     s.Foreground(call),
		iconMethod:   s.Foreground(method),
		iconIface:    s.Foreground(iface),
		iconExternal: s.Foreground(external),
		wire:         s.Foreground(faint),
		wireHot:      s.Foreground(accent),

		screen:     s.Background(t.Bg),
		panel:      panel,
		dialog:     s.Background(bgDialog).Border(lipgloss.RoundedBorder()).BorderForeground(faint).BorderBackground(bgDialog).Padding(0, 1),
		row:        row,
		rowSel:     rowSel,
		rowSelBlur: rowSelBlur,

		code:         make(map[chroma.TokenType]lipgloss.Style),
		mark:         s.Foreground(bright).Background(bgMarked).Bold(true),
		lineNo:       s.Foreground(faint),
		lineNoMarked: s.Foreground(accent),

		callsPanel:      panel.Padding(0),
		rowHover:        row.Background(bgHover),
		rowSelHover:     rowSel.Background(bgHover),
		rowSelBlurHover: rowSelBlur.Background(bgHover),
		result:          row.Padding(0),
		resultSel:       rowSel.Padding(0),
		resultHover:     row.Padding(0).Background(bgHover),
		resultSelHover:  rowSel.Padding(0).Background(bgHover),

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
