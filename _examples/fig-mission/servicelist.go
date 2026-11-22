// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
)

type service struct {
	name    string
	up      bool
	latency int // ms
}

var services = []service{
	{"api-gateway", true, 42},
	{"auth", true, 18},
	{"billing", false, 0},
	{"search", true, 97},
	{"notifications", true, 31},
	{"media-transcode", true, 260},
	{"queue", true, 9},
	{"cdn-edge", true, 64},
	{"recommendations", true, 143},
}

// servicesUp returns how many of the services are up.
func servicesUp() int {
	up := 0
	for _, s := range services {
		if s.up {
			up++
		}
	}
	return up
}

// serviceList is a row for each service: its status, name, and latency,
// with a bar for the latency.
type serviceList struct{ st *styles }

func (l serviceList) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(services)+1)
	for _, s := range services {
		hue, lat := l.st.good, fmt.Sprintf("%4dms", s.latency)
		switch {
		case !s.up:
			hue, lat = l.st.bad, "  down"
		case s.latency > 200:
			hue = l.st.warn
		}
		rows = append(rows, bloom.Row(
			bloom.Text("● ").Style(hue),
			bloom.Fill(bloom.Text(s.name).Style(l.st.text).Truncate()),
			bloom.Text(" "+meter(s.latency, 300, 6)).Style(hue),
			bloom.Text(lat).Style(l.st.dim),
		))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}

// meter is a bar w cells wide, filled v of the way to most, in eighths.
func meter(v, most, w int) string {
	const eighths = " ▏▎▍▌▋▊▉█"
	levels := []rune(eighths)
	fill := min(v, most) * w * 8 / most
	var b strings.Builder
	for range w {
		b.WriteRune(levels[min(max(fill, 0), 8)])
		fill -= 8
	}
	return b.String()
}
