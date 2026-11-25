// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"time"
)

// window is how many samples a meter keeps.
const window = 120

// meter keeps the last samples of a duration.
type meter struct {
	buf  [window]time.Duration
	n    int // samples kept, up to window
	next int // where the next one goes
}

func (m *meter) add(d time.Duration) {
	m.buf[m.next] = d
	m.next = (m.next + 1) % window
	m.n = min(m.n+1, window)
}

// avg returns the mean of the samples kept.
func (m *meter) avg() time.Duration {
	if m.n == 0 {
		return 0
	}
	var sum time.Duration
	for i := range m.n {
		sum += m.buf[i]
	}
	return sum / time.Duration(m.n)
}

// samples appends the samples kept to dst, oldest first.
func (m *meter) samples(dst []time.Duration) []time.Duration {
	for i := range m.n {
		dst = append(dst, m.buf[(m.next-m.n+i+window)%window])
	}
	return dst
}

// perf is what the sidebar shows of how long things take: a step of the
// simulation, the world's Draw, and a whole frame, which is laying out,
// drawing, and compositing the tree, and encoding it for the terminal.
type perf struct {
	sim, draw, frame meter

	drew   time.Duration // the last Draw, which the world's view sets
	second time.Time     // when the current second of frames began
	count  int           // frames in it so far
	// views is the frames made in the last whole second. Bubble Tea calls
	// View after every message, such as each move of the mouse, and writes
	// at most fps of them a second to the terminal, so it can be more.
	views int
}

// framed notes a frame that took d.
func (p *perf) framed(d time.Duration) {
	p.frame.add(d)
	p.draw.add(p.drew)
	now := time.Now()
	if p.second.IsZero() {
		p.second = now
	}
	p.count++
	if now.Sub(p.second) >= time.Second {
		p.views, p.count, p.second = p.count, 0, now
	}
}

// ms formats d in milliseconds.
func ms(d time.Duration) string {
	return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
}
