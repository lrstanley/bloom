// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package flex shares a fixed number of terminal cells between tracks, the
// rows or columns of a container, using integer arithmetic only.
//
// [Split] decides sizes and [Place] decides positions. Neither allocates when
// given a destination slice with enough capacity, so both are safe to call on
// every frame.
//
// See the [documentation] for guides and examples. It is also available for
// LLMs and agents as [llms.txt].
//
// [documentation]: https://bloomfx.dev/
// [llms.txt]: https://bloomfx.dev/llms.txt
package flex

import "slices"

// Collapsed is the size [Split] reports for a track it dropped because the
// minimum sizes of all tracks did not fit the budget.
const Collapsed = -1

// Track describes how one child takes part in sharing the main axis.
type Track struct {
	// Basis is the size before growing or shrinking.
	Basis int
	// Min and Max clamp the final size. A Max of zero or less is unbounded.
	Min, Max int
	// Grow is this track's share of leftover space, relative to the Grow of
	// the other tracks.
	Grow int
	// Shrink is this track's share of any deficit, weighted by its basis. A
	// track with zero Shrink never goes below its basis.
	Shrink int
	// Collapse makes the track collapsible. When even the minimum sizes do
	// not fit (or the sizes before shrinking, with [ShrinkFromEnd]; see
	// [Split]), collapsible tracks are dropped lowest Collapse first, and the
	// last one when tied.
	Collapse int
}

// Fixed is a track of exactly n cells.
func Fixed(n int) Track { return Track{Basis: n, Min: n, Max: n} }

// Fill is a track that starts empty and takes a weighted share of the
// leftover space.
func Fill(weight int) Track { return Track{Grow: weight, Shrink: 1} }

// Auto is a track that starts at its content size n and can shrink.
func Auto(n int) Track { return Track{Basis: n, Shrink: 1} }

// floor is the smallest size the track can be given.
func (t Track) floor() int {
	if t.Shrink <= 0 {
		return t.clamp(t.Basis)
	}
	return max(t.Min, 0)
}

// least is the size the track counts at when [Split] decides whether to
// collapse tracks: its floor, or with [ShrinkFromEnd], its basis.
func (t Track) least(shrink Shrinking) int {
	if shrink == ShrinkFromEnd {
		return t.clamp(t.Basis)
	}
	return t.floor()
}

// spaced reports whether the track, not empty, takes a gap when [Split]
// decides whether to collapse tracks: with [ShrinkFromEnd], only when it
// has cells at its basis, as a track left with none takes no gap (see
// [Place]).
func (t Track) spaced(shrink Shrinking) bool {
	return shrink != ShrinkFromEnd || t.least(shrink) > 0
}

// kept is the smallest size shrinking by basis gives the track while it
// can: its floor, and for one that can shrink and has a cell, at least that.
func (t Track) kept() int {
	if t.Shrink <= 0 {
		return t.floor()
	}
	return max(t.floor(), min(t.clamp(t.Basis), 1))
}

// fit clamps n to the track's Min and Max, and when shrinking, to what it
// keeps (see [Track.kept]).
func (t Track) fit(n int, growing bool) int {
	n = t.clamp(n)
	if !growing {
		n = max(n, t.kept())
	}
	return n
}

// Empty reports whether the track can never be given a cell, such as a
// fixed track of zero. Like a collapsed track, it takes no gap.
func (t Track) Empty() bool { return t.Grow <= 0 && t.clamp(t.Basis) == 0 }

func (t Track) clamp(n int) int {
	if t.Max > 0 && n > t.Max {
		n = t.Max
	}
	return max(n, t.Min, 0)
}

// Shrinking is how [Split] takes a deficit from the tracks that can shrink.
type Shrinking uint8

const (
	// ShrinkByBasis takes it from every shrinking track at once, in
	// proportion to its Shrink weight times its basis, so that each keeps
	// some of its size for as long as it can: as a row shares its width
	// between text that is cut short to fit. While it can, it leaves each
	// track at least one cell, rather than rounding a small one away while
	// the tracks after it keep theirs; when the tracks cannot each keep one,
	// each keeps no more than that, and the last give up even that first.
	ShrinkByBasis Shrinking = iota
	// ShrinkFromEnd takes it from the last shrinking track back, each down
	// to its Min before the one before it gives up any, so that the tracks
	// before keep their size and the last are cut off: as a column too short
	// for its rows shows the first of them. Shrink weights only say which
	// tracks can shrink. As cutting a track short loses some of it,
	// collapsible tracks are dropped first, while the tracks at their basis
	// do not fit, rather than only once their minimum sizes do not.
	ShrinkFromEnd
)

// Split writes the size of each track into dst, reusing its storage, and
// returns it. Sizes add up to budget, minus the gaps between tracks that are
// neither collapsed nor empty (see [Place]),
// whenever the tracks allow: leftover space goes to growing tracks in
// proportion to their weights, and a deficit is taken from shrinking ones
// as shrink says. Shares are rounded to whole cells so that they add up,
// which spreads any remainder across the tracks. If the minimum sizes
// still do not fit (or with [ShrinkFromEnd], the sizes before shrinking),
// collapsible tracks are dropped and reported as [Collapsed]; if the
// minimum sizes still do not fit, they overflow the budget.
func Split(dst []int, budget, gap int, tracks []Track, shrink Shrinking) []int {
	n := len(tracks)
	dst = slices.Grow(dst[:0], n)[:n]
	clear(dst)

	// need is the least the tracks not collapsed take, with the gaps
	// between those that take one.
	visible, spaced, need := 0, 0, 0
	for _, t := range tracks {
		if !t.Empty() {
			visible++
			need += t.least(shrink)
			if t.spaced(shrink) {
				spaced++
			}
		}
	}
	need += gap * max(spaced-1, 0)
	// Collapsible tracks are dropped until the rest fit: the lowest
	// priority first, and of those, the last first.
	for lowest := 0; need > budget; {
		next := 0
		for _, t := range tracks {
			if !t.Empty() && t.Collapse > lowest && (next == 0 || t.Collapse < next) {
				next = t.Collapse
			}
		}
		if next == 0 {
			break
		}
		lowest = next
		for i := n - 1; i >= 0 && need > budget; i-- {
			if t := tracks[i]; t.Collapse == lowest && !t.Empty() {
				dst[i] = Collapsed
				need -= t.least(shrink)
				if t.spaced(shrink) {
					if spaced > 1 {
						need -= gap
					}
					spaced--
				}
				visible--
			}
		}
	}

	avail := budget - gap*max(visible-1, 0)
	switch sum := bases(dst, tracks); {
	case sum == avail:
	case sum < avail:
		resolve(dst, tracks, avail, true)
	case shrink == ShrinkFromEnd:
		cutFromEnd(dst, tracks, budget, gap)
	default:
		shrinkByBasis(dst, tracks, sum, budget, gap)
	}
	return dst
}

// shrinkByBasis shrinks the tracks, at their [bases] in dst, which add up
// to sum, to fit budget with gap between them for [ShrinkByBasis]: by basis
// when each can keep what it keeps (see [Track.kept]), and otherwise from
// what each keeps, cut from the end, so that sizes do not jump from cut
// short to shared by basis as the budget grows.
func shrinkByBasis(dst []int, tracks []Track, sum, budget, gap int) {
	// Shrinking leaves a track that keeps nothing with none, and so without
	// its gap, which the others share.
	kept, shown := 0, 0
	for i, t := range tracks {
		if k := t.kept(); dst[i] != Collapsed && k > 0 {
			kept += k
			shown++
		}
	}
	room := budget - gap*max(shown-1, 0)
	switch {
	case sum <= room:
		return // they fit at their bases, without those gaps
	case kept <= room:
		resolve(dst, tracks, room, false)
		return
	}
	for i, t := range tracks {
		if dst[i] != Collapsed {
			dst[i] = t.kept()
		}
	}
	cutFromEnd(dst, tracks, budget, gap)
}

// bases sets the size in dst of each track not collapsed to its basis,
// clamped to its Min and Max, and returns their sum.
func bases(dst []int, tracks []Track) int {
	sum := 0
	for i, t := range tracks {
		if dst[i] != Collapsed {
			dst[i] = t.clamp(t.Basis)
			sum += dst[i]
		}
	}
	return sum
}

// Fit returns the least space in which [Split], given no gap, sizes every
// track at least as big as its want, which is clamped to the track's Min and
// Max: how much room growing tracks need for each to fit its content. dst is
// scratch storage for Split, as for its own dst.
func Fit(dst []int, tracks []Track, wants []int) int {
	// Shared by weight alone, the most a track wants for each unit of its
	// weight, times all their weights, is enough. Clamped tracks freeze and
	// leave the others more, which can make less enough, down to the wants
	// added up.
	least, most := 0, 0
	need, per, weights := 0, 1, 0
	for i, t := range tracks {
		want := t.clamp(wants[i])
		least += want
		if t.Grow <= 0 {
			most += want
			continue
		}
		if want*per > need*t.Grow {
			need, per = want, t.Grow
		}
		weights += t.Grow
	}
	most += (need*weights + per - 1) / per

	fits := func(budget int) bool {
		dst = Split(dst, budget, 0, tracks, ShrinkByBasis)
		for i, t := range tracks {
			if dst[i] < t.clamp(wants[i]) {
				return false
			}
		}
		return true
	}
	for least < most {
		if mid := least + (most-least)/2; fits(mid) {
			most = mid
		} else {
			least = mid + 1
		}
	}
	return least
}

// resolve sizes the visible tracks, at their [bases] in dst, to fill avail,
// growing them when there is space left or shrinking them when there is a
// deficit, the way CSS resolves flexible lengths: free space (or a deficit)
// is shared by weight from each track's basis, then sizes are clamped to
// Min and Max, and when shrinking, to a cell for each that has one. If
// clamping moved the total, the tracks that were clamped are frozen and the
// rest share again.
func resolve(dst []int, tracks []Track, avail int, growing bool) {
	// weight is a track's share of the free space; zero means it keeps its
	// clamped basis.
	weight := func(t Track) int {
		if growing {
			return t.Grow
		}
		return t.Shrink * t.Basis
	}

	// state is 0 for a track still sharing, frozen for one that keeps its
	// size, or the direction a clamp moved it in this round.
	const (
		sharing int8 = iota
		frozen
		raised
		lowered
	)
	var buf [64]int8
	state := buf[:0]
	if len(tracks) > len(buf) {
		state = make([]int8, 0, len(tracks))
	}
	for i, t := range tracks {
		if dst[i] == Collapsed || weight(t) <= 0 {
			state = append(state, frozen)
		} else {
			state = append(state, sharing)
		}
	}

	for range len(tracks) + 1 {
		free, weights := avail, 0
		for i, t := range tracks {
			switch {
			case dst[i] == Collapsed:
			case state[i] == frozen:
				free -= dst[i]
			default:
				free -= t.Basis
				weights += weight(t)
			}
		}
		if weights == 0 {
			return
		}

		// Shares come from rounding cumulative edges, so they add up to
		// exactly free without a remainder pass.
		sign := 1
		if free < 0 {
			sign, free = -1, -free
		}
		violation, cum, edge := 0, 0, 0
		for i, t := range tracks {
			if state[i] == frozen {
				continue
			}
			cum += weight(t)
			next := free * cum / weights
			target := t.Basis + sign*(next-edge)
			edge = next
			dst[i] = t.fit(target, growing)
			violation += dst[i] - target
			switch {
			case dst[i] > target:
				state[i] = raised
			case dst[i] < target:
				state[i] = lowered
			default:
				state[i] = sharing
			}
		}
		if violation == 0 {
			return
		}

		// Freeze the tracks clamped in the direction of the total violation,
		// as CSS does, so the others make up the difference.
		for i := range state {
			if (violation > 0 && state[i] == raised) || (violation < 0 && state[i] == lowered) {
				state[i] = frozen
			}
		}
	}
}

// cutFromEnd shrinks the tracks that can shrink, at their sizes in dst,
// from the last back, each down to its floor, until they and the gaps
// between them fit budget, for [ShrinkFromEnd]. A track cut to nothing
// gives back its gap too, as [Place] gives it none, so the tracks before it
// keep that much more.
func cutFromEnd(dst []int, tracks []Track, budget, gap int) {
	shown, used := occupied(dst, gap)
	for i := len(tracks) - 1; i >= 0 && used > budget; i-- {
		if dst[i] <= 0 || tracks[i].Shrink <= 0 {
			continue
		}
		give := min(used-budget, dst[i]-tracks[i].floor())
		dst[i] -= give
		used -= give
		if dst[i] == 0 && shown > 1 {
			used -= gap
			shown--
		}
	}
}

// occupied returns how many of sizes have cells, and the cells they and the
// gaps between them take, as [Place] places them.
func occupied(sizes []int, gap int) (shown, used int) {
	for _, s := range sizes {
		if s > 0 {
			shown++
			used += s
		}
	}
	return shown, used + gap*max(shown-1, 0)
}

// Justify is where tracks go on the main axis when they do not fill it.
type Justify uint8

const (
	// Start packs tracks at the start.
	Start Justify = iota
	// Center packs tracks in the middle.
	Center
	// End packs tracks at the end.
	End
	// SpaceBetween puts the free space between tracks, none at the edges.
	SpaceBetween
	// SpaceAround puts equal space around each track, so the edges get half
	// as much as the space between tracks.
	SpaceAround
	// SpaceEvenly puts equal space between tracks and at both edges.
	SpaceEvenly
)

// Place writes the offset of each track into dst, reusing its storage, and
// returns it. sizes are the result of [Split]; tracks that are collapsed or
// have no cells get the offset of the track after them and take no gap, so
// a track animating to zero leaves no gap behind.
func Place(dst, sizes []int, budget, gap int, justify Justify) []int {
	n := len(sizes)
	dst = slices.Grow(dst[:0], n)[:n]

	visible, used := occupied(sizes, gap)
	if visible == 0 {
		clear(dst)
		return dst
	}
	free := max(budget-used, 0)

	// lead is the space before the first track, and extra the space between
	// tracks on top of gap, handed out over the gaps by cumulative edges.
	lead, spaces, between := 0, 0, 0
	switch justify {
	case Start:
	case Center:
		lead = free / 2
	case End:
		lead = free
	case SpaceBetween:
		spaces, between = visible-1, free
	case SpaceAround:
		lead = free / (2 * visible)
		spaces, between = visible-1, free-2*lead
	case SpaceEvenly:
		lead = free / (visible + 1)
		spaces, between = visible-1, free-2*lead
	}

	pos, seen, edge := lead, 0, 0
	for i, s := range sizes {
		dst[i] = pos
		if s <= 0 {
			continue
		}
		seen++
		pos += s
		if seen < visible {
			pos += gap
			if spaces > 0 {
				next := between * seen / spaces
				pos += next - edge
				edge = next
			}
		}
	}
	return dst
}
