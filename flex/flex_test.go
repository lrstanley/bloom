// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package flex

import (
	"slices"
	"testing"
)

func TestSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		budget int
		gap    int
		tracks []Track
		want   []int
	}{
		{"fixed and fill", 80, 0, []Track{Fixed(20), Fill(1)}, []int{20, 60}},
		{"gap", 80, 1, []Track{Fixed(20), Fill(1)}, []int{20, 59}},
		{"weights", 9, 0, []Track{Fill(1), Fill(2)}, []int{3, 6}},
		{"equal remainder goes last", 10, 0, []Track{Fill(1), Fill(1), Fill(1)}, []int{3, 3, 4}},
		{"fill min", 10, 0, []Track{{Grow: 1, Min: 8, Shrink: 1}, Fill(1)}, []int{8, 2}},
		{"fill max redistributes", 20, 0, []Track{{Grow: 3, Max: 6, Shrink: 1}, Fill(1)}, []int{6, 14}},
		{"auto keeps size", 80, 0, []Track{Auto(10), Fill(1)}, []int{10, 70}},
		{"auto shrinks", 10, 0, []Track{Auto(10), Auto(10)}, []int{5, 5}},
		{"auto shrinks by basis", 12, 0, []Track{Auto(8), Auto(16)}, []int{4, 8}},
		{"auto shrink stops at min", 10, 0, []Track{{Basis: 10, Min: 8, Shrink: 1}, Auto(10)}, []int{8, 2}},
		{"fixed never shrinks", 10, 0, []Track{Fixed(8), Auto(8)}, []int{8, 2}},
		{
			"collapse lowest priority first", 50, 0,
			[]Track{{Basis: 20, Min: 20, Max: 20, Collapse: 2}, {Grow: 1, Min: 30, Shrink: 1}, {Basis: 20, Min: 20, Max: 20, Collapse: 1}},
			[]int{20, 30, Collapsed},
		},
		{
			"collapse drops its gap", 31, 1,
			[]Track{{Basis: 20, Min: 20, Max: 20, Collapse: 1}, {Grow: 1, Min: 30, Shrink: 1}},
			[]int{Collapsed, 31},
		},
		{"empty takes no gap", 10, 1, []Track{Fixed(0), Fill(1), Auto(0)}, []int{0, 10, 0}},
		{"overflow without collapse", 10, 0, []Track{Fixed(8), Fixed(8)}, []int{8, 8}},
		{"no grow leaves space", 10, 0, []Track{Auto(3)}, []int{3}},
		{"empty", 10, 0, nil, []int{}},
		// A track keeps a cell while they all can, rather than being
		// rounded away while those after it keep theirs, and when they
		// cannot, the last give up theirs first.
		{"a small track keeps a cell", 20, 0, []Track{Auto(40), Auto(1), Auto(40)}, []int{10, 1, 9}},
		{"a cell each while they fit", 12, 0, []Track{Auto(4), Auto(3), Auto(6), Auto(3), Auto(18)}, []int{2, 1, 2, 1, 6}},
		{
			"from the end when a cell each does not fit", 6, 0,
			[]Track{Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1)},
			[]int{1, 1, 1, 1, 1, 1, 0, 0},
		},
		{"a cell each, from the end", 3, 0, []Track{Auto(4), Auto(1), Auto(4), Auto(1)}, []int{1, 1, 1, 0}},
		// A growing track left with nothing takes no gap, which the others
		// share.
		{"no gap for a fill left empty", 5, 2, []Track{Auto(4), Auto(4), Fill(1)}, []int{2, 1, 0}},
		{"a cell each, at their min", 5, 0, []Track{Auto(4), {Basis: 4, Min: 3, Shrink: 1}, Auto(4)}, []int{1, 3, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Split(nil, tt.budget, tt.gap, tt.tracks, ShrinkByBasis)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Split(%d, %d) = %v, want %v", tt.budget, tt.gap, got, tt.want)
			}
		})
	}
}

// TestSplitFromEnd checks that a deficit taken with [ShrinkFromEnd] comes
// from the last tracks that can shrink, each down to its Min, before any
// before them gives up space, that what does not shrink works as it does
// when shrinking by basis, and that collapsible tracks are dropped while
// the tracks at their basis do not fit, before any is cut.
func TestSplitFromEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		budget int
		gap    int
		tracks []Track
		want   []int
	}{
		{"last first", 6, 0, []Track{Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1), Auto(1)}, []int{1, 1, 1, 1, 1, 1, 0, 0}},
		{"partly", 12, 0, []Track{Auto(8), Auto(16)}, []int{8, 4}},
		{"down to min", 10, 0, []Track{Auto(10), {Basis: 10, Min: 3, Shrink: 1}}, []int{7, 3}},
		{"fixed kept", 10, 0, []Track{Auto(4), Auto(4), Fixed(4)}, []int{4, 2, 4}},
		{"fill gets none when the rest fill it", 3, 0, []Track{Auto(1), Fill(1), Auto(1), Auto(1)}, []int{1, 0, 1, 1}},
		{"fill takes what is left", 5, 0, []Track{Auto(1), Fill(1), Auto(1)}, []int{1, 3, 1}},
		{"gaps", 7, 1, []Track{Auto(2), Auto(2), Auto(2)}, []int{2, 2, 1}},
		// A track cut to nothing takes no gap, which the tracks before it
		// keep.
		{"a gap given back", 6, 2, []Track{Auto(2), Auto(2), Auto(2)}, []int{2, 2, 0}},
		{"gaps given back", 11, 1, []Track{Auto(5), Auto(5), Auto(5), Auto(5)}, []int{5, 5, 0, 0}},
		// Nothing collapses while the tracks fit at their bases with the
		// gaps between those with cells.
		{
			"no collapse for a fill's gap", 4, 1,
			[]Track{{Basis: 1, Min: 1, Max: 1, Collapse: 1}, Auto(2), Fill(1)},
			[]int{1, 2, 0},
		},
		{
			"no collapse for a fill's gap, between", 6, 2,
			[]Track{Auto(2), {Basis: 2, Min: 2, Max: 2, Collapse: 1}, Fill(1)},
			[]int{2, 2, 0},
		},
		{
			"collapse before shrinking", 6, 0,
			[]Track{{Basis: 3, Min: 3, Max: 3, Collapse: 1}, Fixed(3), Auto(2), Auto(2)},
			[]int{Collapsed, 3, 2, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Split(nil, tt.budget, tt.gap, tt.tracks, ShrinkFromEnd)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Split(%d, %d) = %v, want %v", tt.budget, tt.gap, got, tt.want)
			}
		})
	}
}

func TestFit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tracks []Track
		wants  []int
		want   int
	}{
		{"one", []Track{Fill(1)}, []int{7}, 7},
		{"equal weights", []Track{Fill(1), Fill(1)}, []int{3, 10}, 19},
		{"by weight", []Track{Fill(1), Fill(3)}, []int{2, 9}, 11},
		{"rounding favors earlier tracks", []Track{Fill(1), Fill(1), Fill(2)}, []int{2, 6, 4}, 22},
		{"max leaves the rest", []Track{{Grow: 1, Max: 1, Shrink: 1}, Fill(1)}, []int{1, 10}, 11},
		{"min leaves the rest", []Track{{Grow: 1, Min: 20, Shrink: 1}, Fill(1)}, []int{1, 10}, 30},
		{"wants are clamped", []Track{{Grow: 1, Max: 4, Shrink: 1}}, []int{9}, 4},
		{"not growing", []Track{Auto(3), Fill(1)}, []int{3, 5}, 8},
		{"nothing wanted", []Track{Fill(1), Fill(1)}, []int{0, 0}, 0},
		{
			"shrinking a cell each",
			[]Track{{Basis: 4, Grow: 1, Shrink: 1}, {Basis: 2, Grow: 2, Shrink: 1}, {Basis: 3, Grow: 1, Shrink: 1}},
			[]int{0, 1, 0},
			2,
		},
		{"empty", nil, nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Fit(nil, tt.tracks, tt.wants)
			if got != tt.want {
				t.Fatalf("Fit(%v) = %d, want %d", tt.wants, got, tt.want)
			}
			// It is enough, and one cell less is not.
			fits := func(budget int) bool {
				sizes := Split(nil, budget, 0, tt.tracks, ShrinkByBasis)
				for i, tr := range tt.tracks {
					if sizes[i] < tr.clamp(tt.wants[i]) {
						return false
					}
				}
				return true
			}
			if !fits(got) || got > 0 && fits(got-1) {
				t.Fatalf("Fit(%v) = %d is not the least that fits", tt.wants, got)
			}
		})
	}
}

func TestPlace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sizes   []int
		budget  int
		gap     int
		justify Justify
		want    []int
	}{
		{"start", []int{2, 2}, 10, 1, Start, []int{0, 3}},
		{"center", []int{2, 2}, 10, 0, Center, []int{3, 5}},
		{"end", []int{2, 2}, 10, 0, End, []int{6, 8}},
		{"between", []int{2, 2, 2}, 10, 0, SpaceBetween, []int{0, 4, 8}},
		{"around", []int{2, 2}, 12, 0, SpaceAround, []int{2, 8}},
		{"evenly", []int{2, 2}, 10, 0, SpaceEvenly, []int{2, 6}},
		{"collapsed takes no gap", []int{2, Collapsed, 2}, 10, 1, Start, []int{0, 3, 3}},
		{"empty takes no gap", []int{0, 2, 0, 2, 0}, 10, 1, Start, []int{0, 0, 3, 3, 5}},
		{"overflow packs from start", []int{8, 8}, 10, 0, End, []int{0, 8}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Place(nil, tt.sizes, tt.budget, tt.gap, tt.justify)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Place(%v) = %v, want %v", tt.sizes, got, tt.want)
			}
		})
	}
}

func FuzzSplit(f *testing.F) {
	f.Add(80, 1, 20, 0, 10, 2, 30, 1)
	f.Add(10, 0, 8, 8, 0, 0, 8, 0)
	f.Add(3, 2, 0, 1, 5, 3, 0, 1)

	f.Fuzz(func(t *testing.T, budget, gap, basisA, growA, minA, basisB, growB, collapseB int) {
		budget, gap = clampFuzz(budget, 500), clampFuzz(gap, 5)
		tracks := []Track{
			{Basis: clampFuzz(basisA, 200), Grow: clampFuzz(growA, 5), Min: clampFuzz(minA, 100), Shrink: 1},
			{Basis: clampFuzz(basisB, 200), Grow: clampFuzz(growB, 5), Shrink: 1, Collapse: clampFuzz(collapseB, 3)},
			Fill(1),
		}
		for _, shrink := range []Shrinking{ShrinkByBasis, ShrinkFromEnd} {
			checkSplit(t, budget, gap, tracks, shrink)
			// With none collapsed, no track is given less for more space,
			// which Fit's search counts on.
			less, more := Split(nil, budget, gap, tracks, shrink), Split(nil, budget+1, gap, tracks, shrink)
			if slices.Contains(less, Collapsed) || slices.Contains(more, Collapsed) {
				continue
			}
			for i := range less {
				if more[i] < less[i] {
					t.Fatalf("track %d given %d in %d, %d in %d: %v, %v", i, less[i], budget, more[i], budget+1, less, more)
				}
			}
		}
	})
}

// checkSplit checks what [Split] gives tracks in budget, with gap between
// them, a Fill track among them.
func checkSplit(t *testing.T, budget, gap int, tracks []Track, shrink Shrinking) {
	t.Helper()
	sizes := Split(nil, budget, gap, tracks, shrink)
	visible, sum, floors, bases, keeps, shown := 0, 0, 0, 0, 0, 0
	for i, s := range sizes {
		if s == Collapsed {
			if tracks[i].Collapse == 0 {
				t.Fatalf("track %d collapsed without Collapse: %v", i, sizes)
			}
			continue
		}
		if s < tracks[i].floor() {
			t.Fatalf("track %d = %d below its floor %d: %v", i, s, tracks[i].floor(), sizes)
		}
		if !tracks[i].Empty() {
			visible++
		}
		if s > 0 {
			shown++
		}
		sum += s
		floors += tracks[i].floor()
		bases += tracks[i].clamp(tracks[i].Basis)
		keeps += tracks[i].kept()
	}
	avail := budget - gap*max(visible-1, 0)
	// A Fill track is always present, so space never goes unused when the
	// tracks fit at their bases; shrinking by basis, while each keeps a
	// cell, the sizes and the gaps between those given cells fill the
	// budget, unless they fit at their bases; and those only overflow when
	// even the floors do not fit.
	if bases <= avail && sum != avail {
		t.Fatalf("sizes %v add up to %d, want %d", sizes, sum, avail)
	}
	used := sum + gap*max(shown-1, 0)
	if shrink == ShrinkByBasis && bases > avail && keeps <= avail && used != budget && !atBases(sizes, tracks) {
		t.Fatalf("sizes %v and their gaps take %d, want %d", sizes, used, budget)
	}
	if floors <= avail && used > budget {
		t.Fatalf("sizes %v and their gaps take %d, past %d", sizes, used, budget)
	}
	// While a track after it that can shrink still has more than its
	// floor, no track gives up space when shrinking from the end, and none
	// that had a cell is left with none when shrinking by basis.
	later := -1
	for i, s := range slices.Backward(sizes) {
		if s == Collapsed {
			continue
		}
		basis := tracks[i].clamp(tracks[i].Basis)
		shrunk := s < basis
		if shrink == ShrinkByBasis {
			shrunk = s == 0 && basis > 0 && tracks[i].Shrink > 0
		}
		if later >= 0 && shrunk {
			t.Fatalf("track %d shrunk to %d before track %d (%d): %v", i, s, later, sizes[later], sizes)
		}
		if tracks[i].Shrink > 0 && s > tracks[i].floor() {
			later = i
		}
	}

	offsets := Place(nil, sizes, budget, gap, SpaceEvenly)
	prev := 0
	for i, s := range sizes {
		if offsets[i] < prev {
			t.Fatalf("offsets go backwards: %v for %v", offsets, sizes)
		}
		if s != Collapsed {
			prev = offsets[i] + s
		}
	}
}

// atBases reports whether every track not collapsed is at its basis.
func atBases(sizes []int, tracks []Track) bool {
	for i, s := range sizes {
		if s != Collapsed && s != tracks[i].clamp(tracks[i].Basis) {
			return false
		}
	}
	return true
}

func clampFuzz(v, hi int) int {
	if v < 0 {
		v = -v
	}
	if v < 0 { // math.MinInt
		return 0
	}
	return v % (hi + 1)
}

func BenchmarkSplit(b *testing.B) {
	tracks := []Track{
		Fixed(20),
		{Grow: 2, Min: 30, Shrink: 1},
		Auto(18),
		{Basis: 24, Min: 24, Max: 24, Collapse: 1},
		Fill(1),
		Auto(6),
		Fill(3),
		Fixed(1),
	}
	sizes := make([]int, len(tracks))
	offsets := make([]int, len(tracks))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		sizes = Split(sizes, 120+i%80, 1, tracks, ShrinkByBasis)
		offsets = Place(offsets, sizes, 120+i%80, 1, SpaceBetween)
	}
}
