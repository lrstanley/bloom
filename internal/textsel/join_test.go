// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package textsel

import (
	"image"
	"image/color"
	"testing"
	"time"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/internal/linebuf"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func TestJoiner(t *testing.T) {
	t.Parallel()

	var j Joiner
	j.Add("key", 0, 0)
	j.Add("value", 0, 0) // beside it on the same row
	j.Add("two\nrows", 1, 2)
	j.Apart = true
	j.Add("item", 2, 2) // on the row the last ended on, but apart
	j.Add("", 3, 3)
	if got := j.String(); got != "key value\ntwo\nrows\nitem" {
		t.Fatalf("got %q", got)
	}
}

func TestSpanAndRectText(t *testing.T) {
	t.Parallel()

	c := canvas.Parse("hello\nworld\n", ansi.GraphemeWidth)
	if got := SpanText(c.CellAt, 5, image.Pt(1, 0), image.Pt(2, 1), nil); got != "ello\nwor" {
		t.Errorf("SpanText = %q", got)
	}
	if got := SpanText(c.CellAt, 5, image.Pt(1, 0), image.Pt(2, 1), []Join{JoinSpace}); got != "ello wor" {
		t.Errorf("SpanText joined with a space = %q", got)
	}
	if got := RectText(c.CellAt, image.Rect(1, 0, 3, 3)); got != "el\nor" {
		t.Errorf("RectText = %q", got)
	}
	if got := Corners(image.Pt(3, 1), image.Pt(1, 4)); got != image.Rect(1, 1, 4, 5) {
		t.Errorf("Corners = %v", got)
	}
}

func TestClicks(t *testing.T) {
	t.Parallel()

	var c Clicks
	now := time.Unix(1, 0)
	const slop, within = 1, 400 * time.Millisecond
	soon := now.Add(within / 2)
	if c.Next(image.Pt(5, 2), now, slop, within) {
		t.Fatal("a first press continues nothing")
	}
	want := []Unit{UnitCell, UnitWord, UnitLine}
	for i, u := range want {
		next := c.Next(image.Pt(5+i%2, 2), soon, slop, within)
		if got := c.Press(next, image.Pt(5+i%2, 2), soon); got != u {
			t.Fatalf("press %d selects %v, want %v", i, got, u)
		}
	}
	if c.Next(image.Pt(5, 2), soon, slop, within) {
		t.Fatal("a fourth click continued a triple click")
	}
	c.Press(false, image.Pt(5, 2), now)
	for _, tc := range []struct {
		name string
		p    image.Point
		at   time.Time
	}{
		{"another row", image.Pt(5, 3), soon},
		{"too far along the row", image.Pt(7, 2), soon},
		{"too late", image.Pt(5, 2), now.Add(within * 2)},
	} {
		if c.Next(tc.p, tc.at, slop, within) {
			t.Errorf("%s continued the click", tc.name)
		}
	}
	c.Reset()
	if c.Next(image.Pt(5, 2), soon, slop, within) || !c.Time().Equal(now) {
		t.Fatal("a press after a reset continued the click")
	}
}

func TestPainter(t *testing.T) {
	t.Parallel()

	env := &canvas.Env{Fg: color.White, Bg: color.Black}
	var mixer canvas.Mixer
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	// The default selection is a background between the terminal's two
	// colors, keeping a foreground that reads on it.
	var p Painter
	c := uv.Cell{Content: "a", Style: uv.Style{Fg: white, Attrs: uv.AttrReverse}}
	p.Highlight(&c, env, &mixer)
	if c.Style.Bg == nil || c.Style.Fg != white || c.Style.Attrs&uv.AttrReverse != 0 {
		t.Fatalf("highlighted %+v", c.Style)
	}

	// A foreground hard to read on the selection's background takes the
	// terminal color that reads better.
	p = Painter{Style: NewStyle(uv.Style{Bg: white, Attrs: uv.AttrBold}, true)}
	c = uv.Cell{Content: "a", Style: uv.Style{Fg: white}}
	p.Highlight(&c, env, &mixer)
	if c.Style.Bg != white || c.Style.Fg != color.Black || c.Style.Attrs&uv.AttrBold == 0 {
		t.Fatalf("highlighted on white %+v", c.Style)
	}

	// A style of reverse alone reverses the cell.
	p = Painter{Style: NewStyle(uv.Style{Attrs: uv.AttrReverse}, true)}
	c = uv.Cell{Content: "a"}
	p.Highlight(&c, env, &mixer)
	if c.Style.Attrs&uv.AttrReverse == 0 {
		t.Fatalf("reversed %+v", c.Style)
	}

	// The band of the flash is drawn as selected at its middle, and not
	// far from it.
	p = Painter{}
	area := image.Rect(0, 0, 40, 1)
	mid, far := uv.Cell{Content: "a"}, uv.Cell{Content: "a"}
	p.Flash(&mid, image.Pt(20, 0), 0.5, area, env, &mixer)
	p.Flash(&far, image.Pt(39, 0), 0.5, area, env, &mixer)
	want := uv.Cell{Content: "a"}
	p.Highlight(&want, env, &mixer)
	if mid.Style.Bg != want.Style.Bg || far.Style.Bg != nil {
		t.Fatalf("the band is %v at its middle and %v far from it", mid.Style.Bg, far.Style.Bg)
	}
}

func TestMemo(t *testing.T) {
	t.Parallel()

	var a, b linebuf.Store
	a.AppendText("a")
	b.AppendText("b")
	var m Memo[int]
	revs := m.Read()
	revs = NoteStore(revs, &a)
	revs = NoteStore(revs, &a) // the store noted last, once
	revs = NoteStore(revs, &b)
	if len(revs) != 2 {
		t.Fatalf("noted %d stores", len(revs))
	}
	m.Done(1, "text", revs, true)
	if got, ok := m.Get(1); !ok || got != "text" {
		t.Fatalf("got %q, %v", got, ok)
	}
	if _, ok := m.Get(2); ok {
		t.Fatal("kept for another key")
	}
	b.AppendText("more")
	if _, ok := m.Get(1); ok {
		t.Fatal("kept after a store it was read from changed")
	}

	// A read not kept, or a reset, keeps nothing.
	m.Done(1, "text", m.Read(), false)
	if _, ok := m.Get(1); ok {
		t.Fatal("kept a read not to keep")
	}
	m.Done(1, "text", m.Read(), true)
	m.Reset()
	if _, ok := m.Get(1); ok {
		t.Fatal("kept after a reset")
	}
}
