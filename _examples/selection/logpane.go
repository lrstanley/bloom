// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// The levels of a line of the log.
const (
	info  = "INFO"
	warn  = "WARN"
	fail  = "ERROR"
	debug = "DEBUG"
)

// requests is how many requests the list holds, far more than it shows.
const requests = 10_000

// logpane is the pane that shows the log, or the requests in its place.
// Its handlers scroll it, so it is placed by pointer.
type logpane struct {
	st     styles
	log    *bloom.Buffer
	at     bloom.ViewportPos // where the log is scrolled to
	listed bool              // the requests are shown in the log's place
	first  int               // the first request shown
}

// newLogpane returns the pane, showing the log.
func newLogpane() logpane { return logpane{log: &bloom.Buffer{}} }

// setStyles sets the styles the pane is drawn with, of which it keeps a copy,
// and writes the log, whose levels are colored with them.
func (p *logpane) setStyles(st styles) *logpane {
	p.st = st
	p.log.SetContent(logLines(&p.st))
	return p
}

func (p *logpane) Layout(ctx bloom.Context) bloom.Child {
	if p.listed {
		// Only the rows in sight are built for a frame, and a selection
		// builds those it covers out of sight when its text is read. The
		// wheel scrolls from where the list was, three rows a notch, and so
		// does a drag held past its edge.
		return p.st.pane("requests", bloom.List(requests, p.requestRow).ID(requestsList).Offset(p.first).Selectable().
			On(on.Scroll(func(_, dy int) tea.Cmd {
				v, _ := ctx.Frame().List(requestsList)
				p.first = min(max(v.First+dy*3, 0), requests-v.Visible)
				return nil
			})))
	}
	// All of the log is selectable, so a selection takes what is scrolled
	// out of sight too.
	return p.st.pane("server.log", bloom.Viewport(p.log).ID(logText).At(p.at).Selectable().
		OnScroll(func(pos bloom.ViewportPos) tea.Cmd { p.at = pos; return nil }))
}

// requestRow is request i of the list: a row of cells, each a node of its
// own, so that a selection joins them with a space. The path is cut to fit,
// but copied whole.
func (p *logpane) requestRow(i int) bloom.Child {
	method, code := "GET", "200"
	switch i % 5 {
	case 2:
		method, code = "POST", "201"
	case 3:
		code = "404"
	case 4:
		method, code = "DELETE", "502"
	}
	at := time.Date(2026, 11, 8, 14, 2, 0, 0, time.UTC).Add(time.Duration(i) * 350 * time.Millisecond)
	st := p.st.value
	if code[0] != '2' {
		st = p.st.levels[fail]
	}
	return bloom.Row(
		bloom.Fixed(12, bloom.Text(at.Format("15:04:05.000")).Style(p.st.label)),
		bloom.Fixed(6, bloom.Text(method).Style(p.st.levels[info])),
		bloom.Fill(bloom.Text(fmt.Sprintf("/api/v1/orders/%d/items", 1000+i)).Style(p.st.value).Truncate()),
		bloom.Fixed(3, bloom.Text(code).Style(st)),
	).Gap(1)
}

// logLines is a made-up server log.
func logLines(st *styles) string {
	msgs := []struct {
		level string
		msg   string
	}{
		{info, "listening on :8080"},
		{debug, "pool: 4 idle, 0 busy"},
		{info, "GET /api/v1/orders 200 12ms"},
		{info, "GET /api/v1/orders/1042 200 8ms"},
		{warn, "upstream payments slow: 950ms"},
		{info, "POST /api/v1/orders/1042/refund started"},
		{fail, "payments: dial tcp 10.0.4.12:443: i/o timeout"},
		{fail, "POST /api/v1/orders/1042/refund 502 1284ms"},
		{info, "retrying refund 1042 in 2s"},
		{debug, "pool: 3 idle, 1 busy"},
	}
	start := time.Date(2026, 11, 8, 14, 2, 0, 0, time.UTC)
	var b strings.Builder
	for i := range 36 {
		m := msgs[i%len(msgs)]
		if i > 0 {
			b.WriteByte('\n')
		}
		at := start.Add(time.Duration(i) * 1700 * time.Millisecond).Format("15:04:05.000")
		fmt.Fprintf(&b, "%s %s %s", st.label.Render(at), st.levels[m.level].Render(fmt.Sprintf("%-5s", m.level)), st.value.Render(m.msg))
	}
	return b.String()
}
