// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"time"

	"bloomfx.dev/bloom"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// logview is the log in its pane: the buffer it is kept in, where the
// viewport showing it is scrolled to and how it draws its lines, and the
// search that marks it. Its handlers change it, so it is placed by
// pointer.
type logview struct {
	st styles

	buf     *bloom.Buffer
	lines   int // how many lines were written, to number the next
	pos     bloom.ViewportPos
	wrap    bool
	numbers bool

	searching bool   // the query is being typed
	query     string // what was typed
	re        *regexp.Regexp
	found     bloom.Match
	hasFound  bool
}

// newLogview returns an empty log, at its end.
func newLogview() logview {
	return logview{
		buf:     &bloom.Buffer{},
		pos:     bloom.ViewportEnd,
		wrap:    true,
		numbers: true,
	}
}

// setStyles sets the styles the log is drawn with, of which it keeps a copy.
func (l *logview) setStyles(st styles) *logview {
	l.st = st
	return l
}

// preload adds n lines to the log at once. Its lines are colored with its
// styles, so they must be set first.
func (l *logview) preload(n int) {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = l.logLine(l.lines + i)
	}
	l.buf.Append(lines...)
	l.lines += n
}

// add adds the next line to the log.
func (l *logview) add() {
	l.buf.Append(l.logLine(l.lines))
	l.lines++
}

func (l *logview) Layout(bloom.Context) bloom.Child {
	log := bloom.Viewport(l.buf).ID(paneLog).At(l.pos).Follow().Selectable().
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { l.pos = p; return nil })
	if l.wrap {
		log = log.Wrap(bloom.WrapWords)
	}
	if l.numbers {
		log = log.LineNumbers(l.st.numbers)
	}
	if l.re != nil {
		log = log.Marks(l.marks).LineStyle(func(line int) lipgloss.Style {
			if l.hasFound && line == l.found.Line {
				return l.st.focused
			}
			return lipgloss.Style{}
		})
	}

	pane := bloom.Box(l.st.panel, log).
		Edge(bloom.Top, bloom.Start, bloom.Text("server.log").Style(l.st.title)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf("%d lines", l.buf.Len())).Style(l.st.counter)).
		Edge(bloom.Right, bloom.Stretch, bloom.ScrollbarOf(paneLog).
			Style(l.st.bar).HoverStyle(l.st.barHover))
	if !l.wrap {
		pane = pane.Edge(bloom.Bottom, bloom.Stretch, bloom.ScrollbarOf(paneLog).Horizontal().
			Style(l.st.bar).HoverStyle(l.st.barHover))
	}
	return pane
}

// typeQuery handles a key pressed while typing the query, and returns what
// the status line says of it, if anything.
func (l *logview) typeQuery(msg tea.KeyPressMsg, v bloom.ViewportView) string {
	switch msg.String() {
	case "enter":
		l.searching = false
		re, err := regexp.Compile("(?i)" + l.query)
		if err != nil || l.query == "" {
			return "not a search"
		}
		l.re, l.hasFound = re, false
		return l.find(v, true)
	case "esc":
		l.searching = false
	case "backspace":
		if r := []rune(l.query); len(r) > 0 {
			l.query = string(r[:len(r)-1])
		}
	default:
		l.query += msg.Text
	}
	return ""
}

// find moves to the next match of the search, or the one before, from the
// last one found, or from the top of v, wrapping around the log. It
// returns what the status line says of it, or nothing without a search.
func (l *logview) find(v bloom.ViewportView, forward bool) string {
	if l.re == nil {
		return ""
	}
	line, at := v.First, 0
	if l.hasFound {
		line, at = l.found.Line, l.found.End
		if !forward {
			at = l.found.Start
		}
	}
	var found bloom.Match
	var ok bool
	wrapped := ""
	if forward {
		if found, ok = l.buf.Find(l.re, line, at); !ok {
			found, ok = l.buf.Find(l.re, 0, 0)
			wrapped = ", from the top"
		}
	} else if found, ok = l.buf.FindBefore(l.re, line, at); !ok {
		found, ok = l.buf.FindBefore(l.re, l.buf.Len()-1, 1<<30)
		wrapped = ", from the bottom"
	}
	if !ok {
		return fmt.Sprintf("no match for %q", l.query)
	}
	l.found, l.hasFound = found, true
	l.pos = v.Reveal(found.Line)
	return fmt.Sprintf("%q on line %d%s", l.query, found.Line+1, wrapped)
}

// marks marks every match of the search in a line, and the one found in
// another style.
func (l *logview) marks(line int, text string) []bloom.Mark {
	locs := l.re.FindAllStringIndex(text, -1)
	marks := make([]bloom.Mark, len(locs))
	for i, loc := range locs {
		marks[i] = bloom.Mark{Start: loc[0], End: loc[1], Style: l.st.match}
		if l.hasFound && line == l.found.Line && loc[0] == l.found.Start {
			marks[i].Style = l.st.current
		}
	}
	return marks
}

// The levels of a log line.
const (
	info  = "INFO"
	warn  = "WARN"
	fail  = "ERROR"
	debug = "DEBUG"
)

// reset ends the style a log line's stamp or level starts.
const reset = "\x1b[m"

// order is the levels of the log's lines, in turn.
var order = []string{info, info, debug, info, warn, info, info, fail, debug, info, info, info, warn}

// logLine is line i of a made-up server log.
func (l *logview) logLine(i int) string {
	at := time.Date(2026, 11, 8, 14, 0, 0, 0, time.UTC).Add(time.Duration(i) * 137 * time.Millisecond)
	level := order[i%len(order)]
	var msg string
	switch level {
	case fail:
		msg = fmt.Sprintf("POST /api/v1/orders/%d/refund 502 in %dms: payments: dial tcp 10.0.4.%d:443: i/o timeout "+
			"(attempt %d of 3, retrying with the same idempotency key rfnd_%x)", 1000+i%9000, 900+i%400, i%250, 1+i%3, i*7919)
	case warn:
		msg = fmt.Sprintf("upstream payments slow: %dms for order %d", 700+i%600, 1000+i%9000)
	case debug:
		msg = fmt.Sprintf("pool: %d idle, %d busy", i%8, i%5)
	default:
		msg = fmt.Sprintf("GET /api/v1/orders/%d 200 in %dms", 1000+i%9000, 3+i%40)
	}
	return l.st.stamp + at.Format("15:04:05.000") + reset + " " + l.st.levels[level] + fmt.Sprintf("%-5s", level) + reset + " " + msg
}
