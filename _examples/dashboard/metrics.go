// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"math"
	"strings"

	"bloomfx.dev/bloom"
)

// metrics are the cards along the top, each with a live sparkline.
type metrics struct {
	st    *styles
	frame int
}

// Layout lays the cards out in a row.
func (m metrics) Layout(bloom.Context) bloom.Child {
	names := []string{"requests/s", "p99 latency", "errors", "saturation"}
	cards := make([]bloom.Child, len(names))
	for i, name := range names {
		cards[i] = bloom.Fill(bloom.Box(m.st.panel, bloom.Column(
			bloom.Text(name).Style(m.st.label),
			bloom.Text(sparkline(m.frame, i, 40)).Style(m.st.spark),
		)))
	}
	return bloom.Row(cards...).Gap(1)
}

var bars = []rune("▁▂▃▄▅▆▇█")

// sparkline is n bars of a made-up series at a frame.
func sparkline(frame, series, n int) string {
	var b strings.Builder
	for i := range n {
		slow := math.Sin(float64(frame+i)/(4+float64(series)) + float64(series))
		fast := math.Sin(float64(frame+i*3) / 7)
		b.WriteRune(bars[int((slow+fast+2)/4*float64(len(bars)-1))])
	}
	return b.String()
}
