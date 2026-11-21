// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"strings"

	"bloomfx.dev/bloom/internal/escape"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Measure returns the size of s, a string with ANSI styles: the width of its
// widest line and its number of lines. It does not parse s into cells.
func Measure(s string, method ansi.Method) (w, h int) {
	if s == "" {
		return 0, 0
	}
	for line := range strings.Lines(s) {
		w = max(w, method.StringWidth(strings.TrimRight(line, "\r\n")))
		h++
	}
	if strings.HasSuffix(s, "\n") {
		h++
	}
	return w, h
}

// Parse returns a canvas holding s, a string with ANSI styles and
// hyperlinks, sized as [Measure] reports.
func Parse(s string, method ansi.Method) *Canvas {
	w, h := Measure(s, method)
	return parseInto(&Canvas{method: method}, s, w, h)
}

// parseInto parses s into c, sized w×h, reusing its storage when it is big
// enough, and returns c.
func parseInto(c *Canvas, s string, w, h int) *Canvas {
	c.Resize(w, h)
	// The parser panics on a sequence with too many parameters, which no
	// style needs, but text from outside the program could hold.
	uv.NewStyledString(escape.DropOverflows(s)).Draw(c, c.Bounds())
	return c
}

// Cache keeps strings measured and parsed between frames, so a component
// whose view did not change is not measured or parsed again. Canvases it
// returns are shared and must not be changed.
//
// A canvas it returns is kept for as long as its string is asked for, and
// until the [Cache.EndFrame] after the last frame it was asked for in; it is
// then reused for another string, so text that changes every frame, such
// as a live table, parses into the same storage rather than allocating
// more. A caller that keeps a canvas longer, without asking for it again,
// copies it.
type Cache struct {
	method  ansi.Method
	entries map[string]*cacheEntry
	frame   uint64
	cells   int       // cells held by all parsed entries
	free    []*Canvas // canvases of dropped entries, to parse into
	freeLen int       // cells their storage holds
}

// maxFree is how many dropped canvases a cache keeps to parse into.
const maxFree = 16

type cacheEntry struct {
	w, h   int
	tab    bool    // it holds a tab
	canvas *Canvas // nil until parsed
	safe   *Canvas // parsed sanitized, nil until then
	used   uint64
}

// cacheIdleFrames is how many frames an unused entry is kept for, so content
// that comes and goes, like a spinner's frames, stays parsed.
const cacheIdleFrames = 64

// NewCache returns an empty cache that measures and parses with method.
func NewCache(method ansi.Method) *Cache {
	return &Cache{method: method, entries: make(map[string]*cacheEntry)}
}

func (c *Cache) entry(s string) *cacheEntry {
	e, ok := c.entries[s]
	if !ok {
		e = &cacheEntry{tab: strings.IndexByte(s, '\t') >= 0}
		e.w, e.h = Measure(s, c.method)
		c.entries[s] = e
	}
	e.used = c.frame
	return e
}

// Size returns the size of s, as [Measure] does.
func (c *Cache) Size(s string) (w, h int) {
	e := c.entry(s)
	return e.w, e.h
}

// SizeTab returns the size of s, as [Size] does, and whether it holds a
// tab, which a Lip Gloss style draws as spaces, so that a caller measuring
// text it styles need not look for one each time.
func (c *Cache) SizeTab(s string) (w, h int, tab bool) {
	e := c.entry(s)
	return e.w, e.h, e.tab
}

// Get returns s parsed, as [Parse] does.
func (c *Cache) Get(s string) *Canvas {
	e := c.entry(s)
	if e.canvas == nil {
		e.canvas = parseInto(c.take(e.w*e.h), s, e.w, e.h)
		c.cells += e.w * e.h
	}
	return e.canvas
}

// Sanitized returns s parsed after [Sanitize], sized as [Measure] reports
// for s, for text that the program did not make all of.
func (c *Cache) Sanitized(s string) *Canvas {
	e := c.entry(s)
	if e.safe == nil {
		if t := Sanitize(s); t == s {
			e.safe = c.Get(s)
		} else {
			e.safe = parseInto(c.take(e.w*e.h), t, e.w, e.h)
			c.cells += e.w * e.h
		}
	}
	return e.safe
}

// cellsOf returns how many cells the canvases of e hold.
func (e *cacheEntry) cellsOf() int {
	n := 0
	if e.canvas != nil {
		n += e.w * e.h
	}
	if e.safe != nil && e.safe != e.canvas {
		n += e.w * e.h
	}
	return n
}

// take returns a canvas to parse a string of n cells into: the smallest
// dropped one whose storage holds them without being much bigger, which a
// larger string would want, or a new one.
func (c *Cache) take(n int) *Canvas {
	best := -1
	if n == 0 {
		// Of no cells, but as many rows as it has lines, which a dropped
		// canvas would keep holding.
		return &Canvas{method: c.method}
	}
	for i, f := range c.free {
		if k := cap(f.cells); k >= n && k <= 2*n+64 && (best < 0 || k < cap(c.free[best].cells)) {
			best = i
		}
	}
	if best < 0 {
		return &Canvas{method: c.method}
	}
	return c.unfree(best)
}

// drop keeps canvas f of a dropped entry to parse into, while the canvases
// kept hold no more than limit cells and number no more than maxFree,
// making room by letting go of the smallest.
func (c *Cache) drop(f *Canvas, limit int) {
	if f == nil || cap(f.cells) == 0 || cap(f.cells) > limit {
		return
	}
	for len(c.free) > 0 && (len(c.free) == maxFree || c.freeLen+cap(f.cells) > limit) {
		small := 0
		for i, g := range c.free {
			if cap(g.cells) < cap(c.free[small].cells) {
				small = i
			}
		}
		c.unfree(small)
	}
	// What its cells held, such as an image, is not kept for as long as it
	// waits.
	clear(f.cells[:cap(f.cells)])
	c.free = append(c.free, f)
	c.freeLen += cap(f.cells)
}

// unfree removes canvas i from the dropped canvases, returning it.
func (c *Cache) unfree(i int) *Canvas {
	f := c.free[i]
	c.freeLen -= cap(f.cells)
	last := len(c.free) - 1
	c.free[i], c.free[last] = c.free[last], nil
	c.free = c.free[:last]
	return f
}

// EndFrame marks the end of a frame of frameCells cells. Entries unused for a
// while are dropped, and when the cache holds much more than a few frames'
// worth of cells, such as when a large view changes on every frame, every
// entry not used in this frame is dropped. The canvases of dropped entries
// are parsed into again, up to a frame's worth of cells or two.
func (c *Cache) EndFrame(frameCells int) {
	frameCells = max(frameCells, 0) // a natural size given as a negative one
	limit, free := 4*frameCells+1<<16, 2*frameCells+1<<14
	if c.cells > limit || c.frame%cacheIdleFrames == 0 {
		for s, e := range c.entries {
			if e.used < c.frame && (c.cells > limit || e.used+cacheIdleFrames < c.frame) {
				c.cells -= e.cellsOf()
				if e.safe != e.canvas {
					c.drop(e.safe, free)
				}
				c.drop(e.canvas, free)
				delete(c.entries, s)
			}
		}
	}
	// Those kept for a larger frame, such as before the terminal shrank.
	for c.freeLen > free && len(c.free) > 0 {
		c.unfree(0)
	}
	c.frame++
}
