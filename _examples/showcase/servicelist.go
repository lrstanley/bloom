// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// services are the services listed, with their status: 0 is healthy, 1
// slow, and 2 down.
var services = []struct {
	name   string
	status int
	ms     int
}{
	{"api", 0, 24},
	{"auth", 0, 18},
	{"billing", 1, 212},
	{"gateway", 0, 31},
	{"search", 2, 0},
	{"storage", 0, 47},
	{"worker", 1, 164},
}

var statusNames = [...]string{"healthy", "slow", "down"}

// serviceList is a row for each service, which is selected when clicked.
type serviceList struct {
	st       styles
	selected int // the selected service.
}

// Update takes the new styles of a themeMsg. The list has no animations,
// so it returns no command.
func (l *serviceList) Update(msg tea.Msg) {
	if msg, ok := msg.(themeMsg); ok {
		l.st = *msg.st
	}
}

// Layout lists the services, with the selected one's status below them.
func (l *serviceList) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(services)+2)
	for i, s := range services {
		st, hover := l.st.item, l.st.hovered
		if i == l.selected {
			st, hover = l.st.selected, l.st.selHover
		}
		latency := fmt.Sprintf("%dms", s.ms)
		if s.status == 2 {
			latency = "—"
		}
		row := bloom.Row(
			bloom.Text("●").Style(l.st.status[s.status]),
			bloom.Text(s.name).Style(l.st.text),
			bloom.Space(),
			bloom.Text(latency).Style(l.st.muted),
		).Gap(1)
		rows = append(rows, bloom.Fixed(1, bloom.Box(st, row).HoverStyle(hover).On(on.Click(func() tea.Cmd {
			l.selected = i
			return nil
		}))))
	}
	s := services[l.selected]
	rows = append(
		rows,
		bloom.Space(),
		bloom.Text(s.name+" is "+statusNames[s.status]).Style(l.st.status[s.status]).Truncate(),
	)
	return bloom.Column(rows...)
}
