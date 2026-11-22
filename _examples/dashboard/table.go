// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
	"github.com/charmbracelet/x/ansi"
)

// table is the scrolling table of requests to each service, at a frame,
// colored with the escape sequences of st.
func table(st *styles, frame int) bloom.Child {
	dim, green, reset := st.tableDim, st.tableGood, ansi.ResetStyle

	var b strings.Builder
	b.Grow(120 * 96) // about the length of a row, so it is not grown row by row
	for i := range 120 {
		svc := services[i%len(services)]
		rps := 900 + (i*37+frame)%400
		latency := 5 + (i*13+frame)%90
		load := strings.Repeat("▪", 1+(i+frame/8)%12)

		fmt.Fprintf(&b, "%s%04d%s  %-8s  ", dim, i, reset, svc)
		fmt.Fprintf(&b, "%s%5d rps%s  %3dms  %s\n", green, rps, reset, latency, load)
	}
	return bloom.Text(strings.TrimSuffix(b.String(), "\n"))
}
