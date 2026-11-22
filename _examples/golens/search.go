// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"slices"
	"strconv"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// openSearch opens the search dialog, with the last query.
func (m *model) openSearch() tea.Cmd {
	m.runSearch()
	return m.search.Show()
}

func (m *model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return m.search.Hide()
	case "enter":
		if m.resultSel < len(m.results) {
			m.swapFrom = bloom.Top
			return tea.Batch(m.search.Hide(), m.jump(m.results[m.resultSel]), m.swap.Play(0, 1))
		}
		return nil
	case "up", "ctrl+p", "shift+tab":
		m.resultSel = max(m.resultSel-1, 0)
	case "down", "ctrl+n", "tab":
		m.resultSel = min(m.resultSel+1, max(len(m.results)-1, 0))
	case "backspace":
		if q := []rune(m.query); len(q) > 0 {
			m.query = string(q[:len(q)-1])
			m.runSearch()
		}
	case "ctrl+u":
		m.query = ""
		m.runSearch()
	default:
		if msg.Text != "" {
			m.query += msg.Text
			m.runSearch()
		}
	}
	return nil
}

// runSearch finds the functions matching the query: names containing it
// first, then names with its letters in order.
func (m *model) runSearch() {
	m.resultSel = 0
	q := strings.ToLower(strings.TrimSpace(m.query))
	type hit struct {
		f     *fn
		score int
	}
	var hits []hit
	for _, f := range m.prog.all {
		name := strings.ToLower(f.qualified())
		score := 0
		switch i := strings.Index(name, q); {
		case q == "":
		case i >= 0:
			score = i
			if !strings.Contains(strings.ToLower(f.name), q) {
				score += 100 // matched only with the package
			}
		case fuzzy(name, q):
			score = 1000
		default:
			continue
		}
		if !f.local() {
			score += 10000
		}
		hits = append(hits, hit{f, score})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.score - b.score })
	m.results = m.results[:0]
	for _, h := range hits {
		m.results = append(m.results, h.f)
	}
}

// fuzzy reports whether the letters of q appear in s in order.
func fuzzy(s, q string) bool {
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
}

// searchView is the search dialog: a query and the functions matching it.
func (m *model) searchView() bloom.Child {
	rows := min(max(len(m.results), 1), 12)
	w := min(72, m.w-4) // as the layer is
	q := m.query
	input := bloom.Row(
		bloom.Text("/ ").Style(m.st.accent),
		bloom.Fill(bloom.Text(q).Style(m.st.bright).Cursor(tea.NewCursor(ansi.StringWidth(q), 0))),
		bloom.Text(strconv.Itoa(len(m.results))).Style(m.st.muted),
	)
	var list bloom.Child = bloom.Text("no matches").Style(m.st.muted)
	if len(m.results) > 0 {
		list = bloom.List(len(m.results), func(i int) bloom.Child {
			f := m.results[i]
			st, hover := m.st.result, m.st.resultHover
			if i == m.resultSel {
				st, hover = m.st.resultSel, m.st.resultSelHover
			}
			icon, iconSt := m.st.icon(f)
			return bloom.Box(st, bloom.Row(
				bloom.Text(" "+icon+" ").Style(iconSt),
				bloom.Fill(bloom.Text(f.name).Truncate()),
				bloom.Text(" "+f.pkgPath+" ").Style(m.st.muted),
			)).HoverStyle(hover).On(on.Click(func() tea.Cmd {
				return tea.Batch(m.search.Hide(), m.jump(f))
			}))
		}).Keep(m.resultSel)
	}
	return bloom.Box(m.st.dialog, bloom.Column(
		bloom.Fixed(1, input),
		// On a terminal narrower than the dialog's frame there is no room for
		// the rule, rather than a negative count, which would panic.
		bloom.Fixed(1, bloom.Text(strings.Repeat("─", max(w-m.st.dialog.GetHorizontalFrameSize(), 0))).Style(m.st.faint)),
		bloom.Fixed(rows, list),
	))
}
