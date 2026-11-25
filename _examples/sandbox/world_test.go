// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import "testing"

// BenchmarkStep steps the world of a 200×120 cell screen, 200×240
// pixels: in a rain that has fallen long enough to pile up, where most of
// it has settled, and in a sand fall, where all of it is falling: a row of
// sand is poured across the top each step, and what reaches the floor is
// taken away.
func BenchmarkStep(b *testing.B) {
	b.Run("rain", func(b *testing.B) {
		wd := newWorld(1)
		wd.resize(200, 240)
		wd.scene()
		run(b, wd, 600, func(i int) { wd.rain(i) })
	})
	b.Run("falling", func(b *testing.B) {
		wd := newWorld(1)
		wd.resize(200, 240)
		run(b, wd, 600, func(i int) {
			for x := range wd.w {
				wd.set(x, 0, sand, i)
				wd.set(x, wd.h-1, empty, 0)
			}
		})
	})
}

// run steps wd warm times, then for the benchmark, calling each before
// each step, and reports the time per pixel that isn't empty.
func run(b *testing.B, wd *world, warm int, each func(i int)) {
	b.Helper()
	for i := range warm {
		each(i)
		wd.update()
	}
	var grains int
	for i := 0; b.Loop(); i++ {
		each(i)
		wd.update()
		grains += wd.grains
	}
	b.ReportMetric(float64(b.Elapsed())/float64(grains), "ns/grain")
	b.ReportMetric(float64(grains)/float64(b.N), "grains")
}

// TestGrains checks the count of pixels that aren't empty against the
// pixels, after liquids have flowed, steam has risen and vanished, and
// pixels have been painted and erased.
func TestGrains(t *testing.T) {
	t.Parallel()
	wd := newWorld(1)
	wd.resize(80, 60)
	wd.scene()
	for i := range 400 {
		wd.rain(i)
		if i%50 == 0 {
			wd.paint(40, 30, 6, empty, 0)
			wd.paint(20, 10, 4, water, 0)
		}
		wd.update()
	}
	want := 0
	for _, p := range wd.px {
		if p.mat != empty {
			want++
		}
	}
	if wd.grains != want {
		t.Fatalf("grains = %d, want %d", wd.grains, want)
	}
}
