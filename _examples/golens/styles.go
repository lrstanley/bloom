// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// theme is the bubbletint tint the colors come from.
var theme = tint.TintKanagawaDragon

// palette is the colors the styles are made from.
type palette struct {
	fg, bright, muted, faint, accent color.Color
	call, method, iface, external    color.Color
}

var colors = palette{
	fg:       theme.Fg,
	bright:   lipgloss.Lighten(theme.Fg, 0.25),
	muted:    lipgloss.Darken(theme.BrightBlack, 0.3),
	faint:    lipgloss.Lighten(theme.Bg, 0.22),
	accent:   theme.BrightRed,
	call:     theme.BrightYellow,
	method:   theme.BrightBlue,
	iface:    theme.BrightCyan,
	external: lipgloss.Darken(theme.BrightBlack, 0.2),
}

// The backgrounds, from the screen's up.
var (
	bgPanel   = lipgloss.Lighten(theme.Bg, 0.03)
	bgItem    = lipgloss.Lighten(theme.Bg, 0.05)
	bgPath    = lipgloss.Lighten(theme.Bg, 0.1)
	bgHover   = lipgloss.Lighten(theme.Bg, 0.13)
	bgSel     = lipgloss.Lighten(theme.Bg, 0.16)
	bgMarked  = lipgloss.Darken(theme.BrightRed, 0.55)
	bgDialog  = lipgloss.Lighten(theme.Bg, 0.06)
	bgPreview = lipgloss.Lighten(theme.Bg, 0.025)
)

type styles struct {
	text, muted, faint, bright, accent lipgloss.Style

	item, itemPath, itemSel, itemPreview, itemHover lipgloss.Style
	bar                                             lipgloss.Style
	name, nameSel, nameExternal, meta               lipgloss.Style
	iconFunc, iconMethod, iconIface, iconExternal   lipgloss.Style
	wire, wireHot                                   lipgloss.Style

	panel, dialog, row, rowSel, rowSelBlur lipgloss.Style
	code                                   map[chroma.TokenType]lipgloss.Style
	mark, lineNo, lineNoMarked             lipgloss.Style

	// painted caches highlighted lines of code: they are drawn every frame
	// while anything animates.
	painted map[paintKey]string
}

// paintKey is a line of a function's code, with the callee whose call sites
// are marked on it.
type paintKey struct {
	f, marked *fn
	line      int
}

func newStyles() *styles {
	s := lipgloss.NewStyle()
	item := s.Background(bgItem).PaddingRight(1)
	row := s.Foreground(colors.fg).Padding(0, 1)
	return &styles{
		text:   s.Foreground(colors.fg),
		muted:  s.Foreground(colors.muted),
		faint:  s.Foreground(colors.faint),
		bright: s.Foreground(colors.bright).Bold(true),
		accent: s.Foreground(colors.accent),

		item:         item,
		itemPath:     item.Background(bgPath),
		itemSel:      item.Background(bgSel),
		itemPreview:  item.Background(bgPreview),
		itemHover:    item.Background(bgHover),
		bar:          s.Foreground(colors.accent),
		name:         s.Foreground(colors.fg),
		nameSel:      s.Foreground(colors.bright).Bold(true),
		nameExternal: s.Foreground(colors.external).Italic(true),
		meta:         s.Foreground(colors.muted),
		iconFunc:     s.Foreground(colors.call),
		iconMethod:   s.Foreground(colors.method),
		iconIface:    s.Foreground(colors.iface),
		iconExternal: s.Foreground(colors.external),
		wire:         s.Foreground(colors.faint),
		wireHot:      s.Foreground(colors.accent),

		panel:      s.Background(bgPanel).Padding(0, 1),
		dialog:     s.Background(bgDialog).Border(lipgloss.RoundedBorder()).BorderForeground(colors.faint).BorderBackground(bgDialog).Padding(0, 1),
		row:        row,
		rowSel:     row.Background(bgSel).Foreground(colors.bright),
		rowSelBlur: row.Background(bgPath),

		code:         make(map[chroma.TokenType]lipgloss.Style),
		mark:         s.Foreground(colors.bright).Background(bgMarked).Bold(true),
		lineNo:       s.Foreground(colors.faint),
		lineNoMarked: s.Foreground(colors.accent),

		painted: make(map[paintKey]string),
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
