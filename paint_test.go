// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
)

func TestCursorHiddenByEffects(t *testing.T) {
	t.Parallel()

	tree := Transition(0, Text("abc").Cursor(tea.NewCursor(0, 0)), fx.Wipe{From: fx.Left})
	r := New()
	for range 2 { // the second frame is not composited again
		if v := r.View(5, 1, tree); v.Cursor != nil {
			t.Fatalf("cursor on a cell an effect hid: %+v", v.Cursor)
		}
	}
}

func TestPointerEffectsAreDrawnAgain(t *testing.T) {
	t.Parallel()

	dim := &fx.Dim{Amount: 0}
	for _, e := range []fx.Effect{dim, fx.Delay{Effect: dim}, fx.Reverse{Effect: fx.Reverse{Effect: dim}}} {
		dim.Amount = 0
		tree := Overlay(Text("base").Style(lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))), Layer(Text("x")).Anchor(End, End).Backdrop(e))
		r := New()
		first := r.Render(8, 1, tree)
		dim.Amount = 1
		if second := r.Render(8, 1, tree); second == first {
			t.Fatalf("a changed pointer effect in %T was not drawn", e)
		}
	}
}

// uncomparable is an effect that cannot be compared with ==, though it has
// no field that says so at a glance.
type uncomparable struct{ _ [0]func() }

func (uncomparable) Apply(*canvas.Surface, float64) {}

func TestUncomparableEffectsAreDrawnAgain(t *testing.T) {
	t.Parallel()

	for _, e := range []fx.Effect{uncomparable{}, fx.Delay{Effect: uncomparable{}}} {
		r := New()
		for range 2 {
			r.Render(8, 1, Transition(0.5, Text("x"), e))
		}
	}
}

func TestEffectsAreSnapshotted(t *testing.T) {
	t.Parallel()

	// A caller reusing a slice of effects must not make the next frame look
	// the same as the last.
	effects := []fx.Effect{fx.Recolor{Fg: lipgloss.Color("#111111")}}
	tree := Overlay(Text("base").Style(lipgloss.NewStyle()), Layer(Text("x")).Anchor(End, End).Backdrop(effects...))
	r := New()
	first := r.Render(10, 2, tree)
	effects[0] = fx.Recolor{Fg: lipgloss.Color("#eeeeee")}
	if second := r.Render(10, 2, tree); second == first {
		t.Fatal("a changed effect in a reused slice was not drawn")
	}
}

func TestBorderFx(t *testing.T) {
	t.Parallel()

	red := color.RGBA{R: 0xff, A: 0xff}
	var given image.Rectangle
	spy := fx.Func(func(s *canvas.Surface, _ float64) { given = s.Rect() })
	box := Box(rounded.Margin(1), Text("in")).
		Edge(Top, Start, Text("t")).
		BorderFx(1, spy, fx.Recolor{Fg: red})
	out := Render(box, 8, 6)

	// The effects are given the box less its margins, whose edge is the
	// border.
	if want := image.Rect(1, 1, 7, 5); given != want {
		t.Fatalf("effects were given %v, want %v", given, want)
	}
	c := canvas.Parse(out, ansi.GraphemeWidth)
	if got := c.CellAt(1, 1); got.Content != "╭" || got.Style.Fg != red {
		t.Fatalf("the border was not recolored: %+v", got)
	}
	for _, p := range []image.Point{{2, 2}, {3, 1}} {
		if got := c.CellAt(p.X, p.Y); got.Style.Fg != nil {
			t.Fatalf("what is inside or on an edge was recolored at %v: %+v", p, got)
		}
	}
}

func TestBorderFxUnchangedFrame(t *testing.T) {
	t.Parallel()

	tree := func(t float64) Child {
		return Box(rounded, Text("x")).BorderFx(t, fx.Chase{To: color.RGBA{R: 0xff, A: 0xff}})
	}
	r := New()
	first := r.Render(10, 4, tree(0.25))
	if again := r.Render(10, 4, tree(0.25)); unsafe.StringData(first) != unsafe.StringData(again) {
		t.Fatal("an unchanged frame with border effects was drawn again")
	}
	if changed := r.Render(10, 4, tree(0.5)); changed == first {
		t.Fatal("a frame whose border effects moved on was not drawn")
	}
}

func TestBorderFxNoBorder(t *testing.T) {
	t.Parallel()

	// A box that is all margin has no frame for its effects to change.
	box := Box(lipgloss.NewStyle().Margin(1), nil).BorderFx(1, fx.Recolor{Bg: color.RGBA{R: 0xff, A: 0xff}})
	if got := Render(box, 2, 2); strings.Contains(got, "\x1b[") {
		t.Fatalf("effects changed the margins: %q", got)
	}
}

func TestSameStyle(t *testing.T) {
	t.Parallel()

	a := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	b := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	c := a.Bold(true)
	upper := a.Transform(strings.ToUpper)
	switch {
	case !sameStyle(&a, &a):
		t.Fatal("a style differs from itself")
	case !sameStyle(&a, &b):
		t.Fatal("equal styles built apart differ")
	case sameStyle(&a, &c):
		t.Fatal("different styles match")
	case sameStyle(&upper, &upper):
		t.Fatal("a style with a transform matches")
	}
}

func TestFrameFollowsStyleChanges(t *testing.T) {
	t.Parallel()

	// The frame of a box's style is kept from the last frame only while the
	// style is the same.
	r := New()
	tree := func(pad int) Child {
		return Row(Box(rounded.Padding(0, pad), Text("x").ID("x")), Space())
	}
	r.Render(20, 3, tree(0))
	r.Render(20, 3, tree(3))
	expectRect(t, r.Frame(), "x", image.Rect(4, 1, 5, 2))
	// Frames no box used are dropped, along with their styles.
	r.Render(20, 3, Text("plain"))
	for i, m := range r.r.frameMemos {
		if m.ok {
			t.Fatalf("the frame of box %d is still kept", i)
		}
	}
}

// BenchmarkBorderFrame draws a 200×60 box with a title, a counter, and a
// scrollbar that can be dragged on its border, whose border chases round a
// little more each frame.
func BenchmarkBorderFrame(b *testing.B) {
	lines := strings.TrimSuffix(strings.Repeat("a line of the list\n", 400), "\n")
	r := New()
	tree := func(t float64) Child {
		return Box(rounded, Scroll(Text(lines).ID("body"), 100)).
			Edge(Top, Start, Text(" title ")).
			Edge(Top, End, Text("101-158 of 400")).
			Edge(Right, Stretch, ScrollbarOf("body").OnScroll(func(int) tea.Cmd { return nil })).
			BorderFx(t, fx.Chase{To: color.RGBA{R: 0xff, A: 0xff}})
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.Render(benchW, benchH, tree(float64(i%100)/100))
	}
}
