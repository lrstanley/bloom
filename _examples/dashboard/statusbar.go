// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"time"

	"bloomfx.dev/bloom"
)

// statusbar is the key hints, and how long the last frame took to draw.
type statusbar struct {
	st    *styles
	drawn time.Duration // time the last frame took to lay out and draw
}

// Layout lays the hints and the timing out in one row.
func (s statusbar) Layout(bloom.Context) bloom.Child {
	// The key hints give way (and are cut off) before the timing.
	hints := "↑↓ select · enter restart · tab focus · ctrl+b sidebar · 1-6 effects · q quit"
	timing := fmt.Sprintf("frame %.2fms · %.1f%% of 120fps", ms(s.drawn), 100*ms(s.drawn)/8.33)
	return bloom.Row(
		bloom.Fill(bloom.Text(hints).Style(s.st.status).Truncate()),
		bloom.Text(timing).Style(s.st.status),
	)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
