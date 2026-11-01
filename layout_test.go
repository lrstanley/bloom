// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"
	"strings"
	"testing"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/canvas"
	"github.com/lrstanley/bloom/fx"
)

func TestSolveMatchesRoot(t *testing.T) {
	t.Parallel()

	for _, size := range []image.Point{{200, 60}, {120, 30}, {70, 20}} {
		tree := benchTree(4, 1, 0.5)
		r := New()
		r.Render(size.X, size.Y, tree)
		solved := Solve(tree, size.X, size.Y)
		for _, id := range []any{"sidebar", "log", 4, "dialog"} {
			want, wantOK := r.Frame().Rect(id)
			got, gotOK := solved.Rect(id)
			if id == "dialog" {
				// Solve does not run effects, so a slid layer is where it
				// was laid out.
				continue
			}
			if got != want || gotOK != wantOK {
				t.Errorf("%v at %v: Solve %v %v, Root %v %v", id, size, got, gotOK, want, wantOK)
			}
		}
	}
}

func TestUnchangedFrame(t *testing.T) {
	t.Parallel()

	r := New()
	first := r.Render(80, 24, benchTree(2, 0, -1))
	frame := r.Frame()
	again := r.Render(80, 24, benchTree(2, 0, -1))
	if unsafe.StringData(first) != unsafe.StringData(again) || r.Frame() != frame {
		t.Fatal("an unchanged frame was drawn again")
	}
	if changed := r.Render(80, 24, benchTree(3, 0, -1)); changed == first {
		t.Fatal("a changed frame was not drawn")
	}
}

func TestCursor(t *testing.T) {
	t.Parallel()

	input := Text("> abc").Cursor(tea.NewCursor(3, 0))
	tree := Row(Fixed(10, Text("label")), input)

	v := New().View(30, 3, tree)
	if v.Cursor == nil || v.Cursor.X != 13 || v.Cursor.Y != 0 {
		t.Fatalf("cursor = %+v, want 13,0", v.Cursor)
	}

	covered := Overlay(tree, Layer(Text("dialog over the input")).Anchor(End, Start))
	if cur := New().View(30, 3, covered).Cursor; cur != nil {
		t.Fatalf("cursor under a layer is shown at %+v", cur)
	}
}

type itemID int

func mouseTree(modal bool) Child {
	items := Column(
		Text("zero").ID(itemID(0)),
		Text("one").ID(itemID(1)),
		Text("two").ID(itemID(2)),
	)
	base := Row(Fixed(10, items).ID("list"), Fill(Text("main")).ID("main"))
	if !modal {
		return base
	}
	return Overlay(base, Layer(Text("modal")).ID("modal").Modal().Anchor(End, End))
}

func TestHit(t *testing.T) {
	t.Parallel()

	f := Solve(mouseTree(false), 20, 4)
	if got := f.Hit(2, 1); !slices.Equal(got, []any{"list", itemID(1)}) {
		t.Fatalf("Hit on item 1 = %v", got)
	}
	if got := f.Hit(2, 3); !slices.Equal(got, []any{"list"}) {
		t.Fatalf("Hit on the list below its items = %v", got)
	}

	f = Solve(mouseTree(true), 20, 4)
	if got := f.Hit(2, 1); got != nil {
		t.Fatalf("Hit outside a modal = %v, want nil", got)
	}
	if got := f.Hit(18, 3); !slices.Equal(got, []any{"modal"}) {
		t.Fatalf("Hit on a modal = %v", got)
	}
}

// topHit returns the ID of the topmost node at x, y, or nil.
func topHit(f *Frame, x, y int) any {
	if path := f.Hit(x, y); len(path) > 0 {
		return path[len(path)-1]
	}
	return nil
}

func TestSlideMovesHits(t *testing.T) {
	t.Parallel()

	tree := Overlay(Text("base").ID("base"), Layer(Text("up").Cursor(tea.NewCursor(1, 0))).ID("up").Transition(0.5, fx.Slide{From: fx.Bottom}))
	r := New()
	v := r.View(10, 10, tree)
	got, _ := r.Frame().Rect("up")
	laidOut, _ := Solve(tree, 10, 10).Rect("up")
	if got.Min.Y <= laidOut.Min.Y || got.Size() != laidOut.Size() {
		t.Fatalf("slid layer at %v, laid out at %v", got, laidOut)
	}
	if v.Cursor == nil || v.Cursor.Y != got.Min.Y || v.Cursor.X != got.Min.X+1 {
		t.Fatalf("cursor at %+v, want it in the slid layer at %v", v.Cursor, got)
	}
	if hit := topHit(r.Frame(), got.Min.X, got.Min.Y); hit != "up" {
		t.Fatalf("a click on the slid layer went to %v", hit)
	}

	// Drawing the same frame again is skipped, but must not lose the slide.
	v = r.View(10, 10, tree)
	if again, _ := r.Frame().Rect("up"); again != got || v.Cursor == nil || v.Cursor.Y != got.Min.Y {
		t.Fatalf("redrawn slid layer at %v with cursor %+v, want %v", again, v.Cursor, got)
	}
}

func TestSlidingLayerHits(t *testing.T) {
	t.Parallel()

	// The layer starts partly off the overlay; sliding brings cells that
	// were clipped into view, and they must be hit as the layer's.
	tree := Overlay(Text("base").ID("base"),
		Layer(Text("ABCD")).ID("layer").Anchor(Start, Start).Offset(-2, 0).Transition(0.5, fx.Slide{From: fx.Right}))
	r := New()
	r.Render(8, 1, tree)
	col := strings.Index(r.r.canvas.String(), "A")
	if col < 0 {
		t.Fatalf("layer not drawn: %q", r.r.canvas.String())
	}
	if hit := topHit(r.Frame(), col, 0); hit != "layer" {
		t.Fatalf("a click on the layer's A at %d went to %v", col, hit)
	}
}

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

// meteredLeaf counts how often it is measured.
type meteredLeaf struct{ n *int }

func (meteredLeaf) View() string { return "abcdefghij" }

func (l meteredLeaf) Measure(avail Size) Size {
	*l.n++
	return Size{W: min(10, avail.W), H: 1}
}

func nested(depth int, leaf Child) Child {
	c := leaf
	for d := range depth {
		if d%2 == 0 {
			c = Row(c, Text("x"))
		} else {
			c = Column(c, Text("x"))
		}
	}
	return c
}

func TestNestedMeasureIsLinear(t *testing.T) {
	t.Parallel()

	var n int
	Solve(nested(16, meteredLeaf{n: &n}), 200, 60)
	if n > 200 {
		t.Fatalf("the leaf was measured %d times at depth 16", n)
	}
}

func TestEnvWithUncomparableColors(t *testing.T) {
	t.Parallel()

	r := New(WithColors(sliceColor{0xff}, sliceColor{0}))
	r.Render(4, 1, Text("a"))
	r.Render(4, 1, Text("a"))
}

// sliceColor is a valid [color.Color] that cannot be compared with ==.
type sliceColor []uint8

func (c sliceColor) RGBA() (r, g, b, a uint32) {
	v := uint32(c[0]) * 0x101
	return v, v, v, 0xffff
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

func focusTree(dialog bool) Child {
	base := Column(
		Text("a").ID("a").Focusable(),
		Text("b").ID("b").Focusable(),
		Text("not focusable").ID("c"),
	)
	if !dialog {
		return base
	}
	return Overlay(base, Layer(Row(Text("ok").ID("ok").Focusable(), Text("cancel").ID("cancel").Focusable()).Gap(1)).ID("dialog").Modal())
}

func TestFocus(t *testing.T) {
	t.Parallel()

	var f Focus[string]
	r := New(WithFocus(&f))
	r.Render(20, 5, focusTree(false))
	for _, want := range []string{"a", "b", "a"} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next focused %v, want %v", focused(&f), want)
		}
	}
	f.Prev(r.Frame())
	if !f.Is("b") {
		t.Fatalf("Prev focused %v", focused(&f))
	}

	r.Render(20, 5, focusTree(true))
	f.Push("dialog")
	if _, ok := f.Current(); ok || f.Scope() != "dialog" {
		t.Fatal("Push did not clear the focus")
	}
	for _, want := range []string{"ok", "cancel", "ok"} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next inside the dialog focused %v, want %v", focused(&f), want)
		}
	}
	if !f.Within(r.Frame(), "dialog") {
		t.Fatal("focus is not within the dialog")
	}

	r.Render(20, 5, focusTree(true))
	r.ClickAt(0, 0)
	if !f.Is("ok") {
		t.Fatalf("a click outside the dialog focused %v", focused(&f))
	}

	f.Pop()
	if !f.Is("b") || f.Within(r.Frame(), "dialog") {
		t.Fatalf("Pop focused %v, want b", focused(&f))
	}

	r.Render(20, 5, focusTree(false))
	r.ClickAt(0, 0)
	if !f.Is("a") {
		t.Fatalf("clicking a focused %v", focused(&f))
	}
}

// TestFocusType checks that focus only moves between nodes with IDs of its
// type, and that untyped constants convert to it.
func TestFocusType(t *testing.T) {
	t.Parallel()

	type field int
	type button string
	tree := Row(
		Text("name").ID(field(0)).Focusable(),
		Text("x").ID("other").Focusable(),
		Text("ok").ID(button("ok")).Focusable(),
		Text("email").ID(field(1)).Focusable(),
	)
	r := New()
	r.Render(30, 1, tree)

	var f Focus[field]
	if id, ok := f.Current(); ok || f.Is(0) {
		t.Fatalf("a zero Focus has %v focused", id)
	}
	for _, want := range []field{0, 1, 0} {
		f.Next(r.Frame())
		if !f.Is(want) {
			t.Fatalf("Next focused %v, want %v", focused(&f), want)
		}
	}

	var buttons Focus[button]
	buttons.Set("ok")
	if id, ok := buttons.Current(); !ok || id != "ok" {
		t.Fatalf("Set an untyped constant: %v, %v", id, ok)
	}

	var all Focus[any]
	for _, want := range []any{field(0), "other", button("ok"), field(1)} {
		all.Next(r.Frame())
		if !all.Is(want) {
			t.Fatalf("Focus[any] focused %v, want %v", focused(&all), want)
		}
	}
}
