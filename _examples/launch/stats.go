// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strconv"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	tea "charm.land/bubbletea/v2"
)

// cards describe the statistics shown, with their values at the start, and
// next, their values after n updates.
var cards = [...]struct {
	title string
	unit  string
	start int
	next  func(n int) int
}{
	{
		title: "Requests", unit: "per minute", start: 1021,
		next: func(n int) int { return 1021 + n*37 + (n*n*11)%53 },
	},
	{
		title: "p95 latency", unit: "milliseconds", start: 148,
		next: func(n int) int { return 120 + (n*37)%61 },
	},
	{
		title: "Errors", unit: "per hour", start: 7,
		next: func(n int) int { return 3 + (n*5)%9 },
	},
}

const (
	shimmerPeriod = 1400 * time.Millisecond
	loadDelay     = 1500 * time.Millisecond
)

type (
	loadedMsg struct{}
	statsMsg  int // the generation of the ticker that sent it.
)

// stats are the cards, side by side, and the numbers they show.
type stats struct {
	st styles

	loaded  bool
	shimmer anim.Sequence // loops until the data arrives.

	// every is how often the numbers change. Each change rolls from the
	// previous values to the new ones.
	every   time.Duration
	paused  bool
	gen     int
	updates int
	vals    [len(cards)]int
	prev    [len(cards)]int
	roll    anim.Value
}

// load starts the shimmer, and the wait for the data.
func (s *stats) load() tea.Cmd {
	return tea.Batch(
		s.shimmer.Loop(),
		tea.Tick(loadDelay, func(time.Time) tea.Msg { return loadedMsg{} }),
	)
}

// schedule returns the command for the next change of the numbers, which
// replaces any earlier one still waiting.
func (s *stats) schedule() tea.Cmd {
	s.gen++
	gen := s.gen
	return tea.Tick(s.every, func(time.Time) tea.Msg { return statsMsg(gen) })
}

// Update handles the arrival of the data, and the changes of the numbers.
func (s *stats) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case loadedMsg:
		s.loaded = true
		s.shimmer.Stop()
		return s.schedule()
	case statsMsg:
		if int(msg) == s.gen && !s.paused {
			s.updates++
			s.prev = s.vals
			for i, c := range cards {
				s.vals[i] = c.next(s.updates)
			}
			return tea.Batch(s.roll.Play(0, 1), s.schedule())
		}
	}
	return nil
}

// pause pauses the numbers, or starts them again.
func (s *stats) pause() tea.Cmd {
	s.paused = !s.paused
	if !s.paused && s.loaded {
		return s.schedule()
	}
	return nil
}

func (s *stats) Layout(bloom.Context) bloom.Child {
	slots := make([]bloom.Child, len(cards))
	for i := range cards {
		slots[i] = bloom.Fill(s.card(i)).ID(cardID.Sub(i))
	}
	return bloom.Row(slots...).Gap(2)
}

// card is the card for statistic i: a placeholder that shimmers until the
// data arrives, then its number, rolling from the previous one while it
// changes.
func (s *stats) card(i int) bloom.Child {
	if !s.loaded {
		// A line for each of the card's below, so nothing moves when the
		// data arrives.
		return bloom.Box(s.st.card, bloom.Skeleton(8, 0, 5, 11).Shimmer(s.shimmer.Float()))
	}

	c := cards[i]
	num := bloom.Child(bloom.Text(number(s.vals[i])).Style(s.st.big))
	if s.roll.Animating() {
		num = bloom.Transition(s.roll.Float(), num, fx.Roll{From: number(s.prev[i])})
	}
	return bloom.Box(s.st.card, bloom.Column(
		bloom.Text(c.title).Style(s.st.heading),
		bloom.Fixed(1, nil),
		num,
		bloom.Text(c.unit).Style(s.st.label),
	))
}

// number formats n with a comma between each group of thousands.
func number(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
