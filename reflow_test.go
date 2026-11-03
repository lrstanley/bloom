// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fx"
)

// linear is a one second linear layout animation, which moves a box
// predictably for tests to check where it is.
var linear = []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}

func TestLayoutAnimationTween(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(offset, width int) Child {
		return Row(Fixed(offset, nil), Fixed(width, Row(Fill(Text("a")).ID("a"), Fill(Text("b")).ID("b"))).ID("pane").Animate(linear...))
	}
	r.View(100, 10, tree(0, 20))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 20, 10))
	if r.Update("change") == nil {
		t.Fatal("animated tree did not request a discovery tick")
	}
	r.View(100, 10, tree(20, 40))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 20, 10))
	now = now.Add(500 * time.Millisecond)
	r.View(100, 10, tree(20, 40))
	expectRect(t, r.Frame(), "pane", image.Rect(10, 0, 40, 10))
	expectRect(t, r.Frame(), "a", image.Rect(10, 0, 25, 10))
	expectRect(t, r.Frame(), "b", image.Rect(25, 0, 40, 10))
	i, _ := r.Frame().hit(image.Pt(11, 1))
	if i < 0 || r.Frame().entries[i].id != "a" {
		t.Fatal("hit test did not use visual position")
	}
	if old, _ := r.Frame().hit(image.Pt(1, 1)); old >= 0 {
		t.Fatal("hit test used old position")
	}
	// Retarget from the last drawn rectangle without a discontinuity.
	r.View(100, 10, tree(0, 10))
	expectRect(t, r.Frame(), "pane", image.Rect(10, 0, 40, 10))
	now = now.Add(500 * time.Millisecond)
	r.View(100, 10, tree(0, 10))
	expectRect(t, r.Frame(), "pane", image.Rect(5, 0, 25, 10))
	now = now.Add(500 * time.Millisecond)
	r.View(100, 10, tree(0, 10))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 10, 10))
	if r.r.motion.Running() {
		t.Fatal("tween never settled")
	}
}

func TestLayoutAnimationLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tree  Child
		count int
	}{
		{"plain", Fixed(10, Text("plain")).ID("plain"), 0},
		{"no id", Fixed(10, Text("anonymous")).Animate(), 0},
		{"animated", Fixed(10, Text("animated")).ID("pane").Animate(), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New()
			r.View(80, 10, tc.tree)
			if r.r.motion.Len() != tc.count {
				t.Fatalf("states = %d", r.r.motion.Len())
			}
			if (r.Update("change") != nil) != (tc.count > 0) {
				t.Fatal("unexpected discovery tick")
			}
			r.View(80, 10, nil)
			if r.r.motion.Len() != 0 {
				t.Fatal("removed node retained")
			}
		})
	}
}

func TestLayoutAnimationResizeAndZero(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(w int) Child {
		return Row(Fixed(w, Text("pane")).ID("pane").Animate(anim.Duration(time.Second), anim.Ease(anim.Linear)), Space())
	}
	r.View(80, 10, tree(20))
	r.View(80, 10, tree(0))
	now = now.Add(500 * time.Millisecond)
	r.View(80, 10, tree(0))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 10, 10))
	now = now.Add(500 * time.Millisecond)
	r.View(80, 10, tree(0))
	if _, ok := r.Frame().Rect("pane"); ok {
		t.Fatal("zero size still visible")
	}
	if r.r.motion.Len() != 1 {
		t.Fatal("zero size lost animation state")
	}
	r.View(80, 10, tree(30))
	r.View(90, 10, tree(30))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 30, 10))
	if r.r.motion.Running() {
		t.Fatal("resize did not stop animation")
	}
	r.View(90, 10, Row(Fixed(30, Text("pane")).ID("pane").Animate().Min(100).Collapsible(1), Fill(Text("other")).Min(80)))
	if r.r.motion.Len() != 0 {
		t.Fatal("collapsed slot retained")
	}
	r.View(90, 10, tree(15))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 15, 10))
}

func TestLayoutAnimationSpring(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(x int) Child {
		return Overlay(Text("base"), Layer(Text("spring")).Size(10, 2).Anchor(Start, Start).Offset(x, 0).ID("spring").Animate(anim.Spring(15, 0.8)))
	}
	r.View(100, 10, tree(0))
	r.View(100, 10, tree(40))
	now = now.Add(100 * time.Millisecond)
	r.View(100, 10, tree(40))
	rect, _ := r.Frame().Rect("spring")
	if rect.Min.X <= 0 || rect.Min.X >= 40 {
		t.Fatalf("spring did not advance: %v", rect)
	}
	velocity := r.r.motion.Values("spring")[0]
	r.View(100, 10, tree(20))
	if got := r.r.motion.Values("spring")[0]; got != velocity {
		t.Fatal("retarget jumped")
	}
	for range 200 {
		now = now.Add(16 * time.Millisecond)
		r.View(100, 10, tree(20))
	}
	expectRect(t, r.Frame(), "spring", image.Rect(20, 0, 30, 2))
	if r.r.motion.Running() {
		t.Fatal("spring never settled")
	}
}

func TestLayoutAnimationTicks(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }), WithFPS(1000))
	tree := func(x int) Child {
		return Row(Fixed(x, nil), Fixed(5, Text("x")).ID("x").Animate(anim.Duration(time.Second)))
	}
	r.View(80, 10, tree(0))
	cmd := r.Update("change")
	if cmd == nil {
		t.Fatal("missing discovery tick")
	}
	if r.Update("duplicate") != nil {
		t.Fatal("duplicate pending tick")
	}
	r.View(80, 10, tree(20))
	msg := cmd()
	cmd = r.Update(msg)
	if cmd == nil {
		t.Fatal("running animation stopped ticking")
	}
	now = now.Add(time.Second)
	r.View(80, 10, tree(20))
	if r.Update(cmd()) != nil {
		t.Fatal("settled animation kept ticking")
	}
	if r.Update(layoutFrameMsg{root: New()}) != nil {
		t.Fatal("accepted foreign root tick")
	}
}

func TestLayoutAnimationStateless(t *testing.T) {
	tree := Row(Fixed(20, nil), Fixed(10, Text("target")).ID("x").Animate())
	got, ok := Solve(tree, 80, 10).Rect("x")
	if !ok || got != image.Rect(20, 0, 30, 10) {
		t.Fatalf("Solve = %v, %v", got, ok)
	}
	if Render(tree, 80, 10) != Render(Row(Fixed(20, nil), Fixed(10, Text("target"))), 80, 10) {
		t.Fatal("stateless render animated")
	}
}

func TestLayoutAnimationAnchoredLayer(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(x int) Child {
		return Overlay(Row(Fixed(x, nil), Fixed(10, Text("anchor")).ID("anchor")),
			Layer(Text("follow")).Size(6, 2).AnchorTo("anchor", Bottom).ID("follower").Animate(anim.Duration(time.Second), anim.Ease(anim.Linear)))
	}
	r.View(100, 10, tree(0))
	r.View(100, 10, tree(40))
	now = now.Add(500 * time.Millisecond)
	r.View(100, 10, tree(40))
	expectRect(t, r.Frame(), "follower", image.Rect(20, 8, 26, 10))
}

func TestLayoutAnimationMovingTarget(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(x int) Child {
		return Row(Fixed(x, nil), Fixed(5, Text("moving")).ID("moving").Animate(anim.Duration(time.Second), anim.Ease(anim.Linear)))
	}
	r.View(100, 10, tree(0))
	r.View(100, 10, tree(20))
	for i := 1; i <= 5; i++ {
		now = now.Add(100 * time.Millisecond)
		r.View(100, 10, tree(20+i))
		got, _ := r.Frame().Rect("moving")
		if got.Min.X < i {
			t.Fatalf("changing target stalled: %v", got)
		}
	}
}

func TestLayoutAnimationHeight(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(h int) Child {
		return Column(
			Fixed(h, Column(Fill(Text("top")).ID("top"), Fill(Text("bottom")).ID("bottom"))).
				ID("pane").Animate(anim.Duration(time.Second), anim.Ease(anim.Linear)),
			Space(),
		)
	}
	r.View(40, 20, tree(4))
	r.View(40, 20, tree(12))
	now = now.Add(500 * time.Millisecond)
	r.View(40, 20, tree(12))
	expectRect(t, r.Frame(), "pane", image.Rect(0, 0, 40, 8))
	expectRect(t, r.Frame(), "top", image.Rect(0, 0, 40, 4))
	expectRect(t, r.Frame(), "bottom", image.Rect(0, 4, 40, 8))
}

func TestLayoutAnimationIdleClock(t *testing.T) {
	r := New(WithClock(func() time.Time { t.Fatal("clock called for plain layout"); return time.Time{} }))
	for range 3 {
		r.View(40, 10, Row(Fill(Text("plain")).ID("plain"), Space()))
		if r.Update("message") != nil {
			t.Fatal("plain layout scheduled a tick")
		}
	}
}

func TestLayoutAnimationNestedMovesWithParent(t *testing.T) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(offset int) Child {
		return Row(Fixed(offset, nil), Fixed(20, Column(
			Fixed(1, Text("x")),
			Fixed(1, Text("item")).ID("item").Animate(anim.Duration(4*time.Second)),
		)).ID("pane").Animate(linear...))
	}
	r.View(100, 10, tree(0))
	r.View(100, 10, tree(40))
	now = now.Add(500 * time.Millisecond)
	r.View(100, 10, tree(40))
	// Halfway, the item is where it always is in the pane, rather than an
	// eighth of the way, by its own slower animation, to where it is going.
	expectRect(t, r.Frame(), "pane", image.Rect(20, 0, 40, 10))
	expectRect(t, r.Frame(), "item", image.Rect(20, 1, 40, 2))
}

func TestLayoutAnimationUnfocusedFPS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Unix(1, 0)
		r := New(WithClock(func() time.Time { return now }), WithFPS(1000), WithUnfocusedFPS(4))
		tree := func(x int) Child {
			return Row(Fixed(x, nil), Fixed(5, Text("x")).ID("x").Animate(anim.Duration(time.Hour)))
		}
		r.View(80, 10, tree(0))
		r.View(80, 10, tree(20))

		delay := func(cmd tea.Cmd) time.Duration {
			start := time.Now()
			cmd()
			return time.Since(start)
		}
		if d := delay(r.Update(tea.BlurMsg{})); d != 250*time.Millisecond {
			t.Fatalf("blurred tick waited %v", d)
		}
		r.Update(tea.FocusMsg{})
		if d := delay(r.Update(layoutFrameMsg{root: r})); d != time.Millisecond {
			t.Fatalf("refocused tick waited %v", d)
		}
	})
	if slow := New(WithFPS(4), WithUnfocusedFPS(1000)); slow.unfocused != slow.interval {
		t.Fatalf("unfocused interval %v is faster than the focused %v", slow.unfocused, slow.interval)
	}
}

func TestMoveWith(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	pane := func(id, text string, move bool) SlotNode {
		s := Fixed(6, Text(text)).ID(id).Animate(linear...)
		if move {
			s = s.MoveWith(fx.Fly{Seed: 1})
		}
		return s
	}
	tree := func(swapped, move bool) Child {
		a, b := pane("a", "aaaaaa\naaaaaa", move), pane("b", "bbbbbb\nbbbbbb", move)
		if swapped {
			a, b = b, a
		}
		return Row(a, b, Text("|c"))
	}

	// At rest, a slot that moves with effects draws as any other.
	before := r.Render(16, 2, tree(false, true))
	if want := Render(tree(false, false), 16, 2); before != want {
		t.Fatalf("at rest:\n%s\nwant:\n%s", before, want)
	}

	// When the panes trade places, they are laid out where they go, and
	// start out drawn where they were.
	if got := r.Render(16, 2, tree(true, true)); got != before {
		t.Fatalf("at the start of the move:\n%s\nwant it as before:\n%s", got, before)
	}
	expectRect(t, r.Frame(), "b", image.Rect(0, 0, 6, 2))
	expectRect(t, r.Frame(), "a", image.Rect(6, 0, 12, 2))
	if !r.r.motion.Running() {
		t.Fatal("moving panes did not keep the animation running")
	}

	now = now.Add(500 * time.Millisecond)
	after := Render(tree(true, false), 16, 2)
	if mid := r.Render(16, 2, tree(true, true)); mid == before || mid == after {
		t.Fatalf("half way, the panes are at one end:\n%s", mid)
	}

	now = now.Add(600 * time.Millisecond)
	if got := r.Render(16, 2, tree(true, true)); got != after {
		t.Fatalf("after the move:\n%s\nwant:\n%s", got, after)
	}
	if r.r.motion.Running() {
		t.Fatal("the animation is still running after the move")
	}
}

func TestMoveWithModes(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	base := func(at int, s SlotNode) Child { return Row(Fixed(at, nil), s, Space()) }

	// A dragged layer gliding with an ID, whose slot then takes over, as
	// when it is dropped, finishes its glide rather than moving with effects.
	layer := func(x int) Child {
		return Overlay(Row(Space()), Layer(Text("x")).Anchor(Start, Start).Offset(x, 0).Size(4, 1).ID("p").Animate(linear...))
	}
	r.Render(30, 1, layer(0))
	r.Render(30, 1, layer(20))
	now = now.Add(500 * time.Millisecond)
	r.Render(30, 1, base(0, Fixed(4, Text("x")).ID("p").Animate(linear...).MoveWith(fx.Fly{})))
	expectRect(t, r.Frame(), "p", image.Rect(10, 0, 14, 1))

	// Moving with effects once it has landed, it moves from there.
	now = now.Add(2 * time.Second)
	slot := func(at int, move bool) Child {
		s := Fixed(4, Text("x")).ID("p").Animate(linear...)
		if move {
			s = s.MoveWith(fx.Fly{})
		}
		return base(at, s)
	}
	r.Render(30, 1, slot(0, true))
	expectRect(t, r.Frame(), "p", image.Rect(0, 0, 4, 1))
	r.Render(30, 1, slot(10, true))
	expectRect(t, r.Frame(), "p", image.Rect(10, 0, 14, 1))
	if !r.r.motion.Running() {
		t.Fatal("did not move with effects")
	}

	// Gliding once more, it glides from where it was laid out.
	now = now.Add(2 * time.Second)
	r.Render(30, 1, slot(10, true))
	r.Render(30, 1, slot(20, false))
	expectRect(t, r.Frame(), "p", image.Rect(10, 0, 14, 1))
	now = now.Add(500 * time.Millisecond)
	r.Render(30, 1, slot(20, false))
	expectRect(t, r.Frame(), "p", image.Rect(15, 0, 19, 1))
}

func TestLayerMoveWith(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(x int, move bool) Child {
		l := Layer(Text("ab\ncd")).Anchor(Start, Start).Offset(x, 0).Size(2, 2).ID("l").Animate(linear...)
		if move {
			l = l.MoveWith(fx.Fly{Seed: 1})
		}
		// A layer after it stays on top as its cells fly by.
		top := Layer(Text("|")).Anchor(Start, Start).Offset(4, 1).Size(1, 1)
		return Overlay(Text("..........\n.........."), l, top)
	}

	before := r.Render(10, 2, tree(0, true))
	if want := Render(tree(0, false), 10, 2); before != want {
		t.Fatalf("at rest:\n%s\nwant:\n%s", before, want)
	}
	if got := r.Render(10, 2, tree(8, true)); got != before {
		t.Fatalf("at the start of the move:\n%s\nwant it as before:\n%s", got, before)
	}
	expectRect(t, r.Frame(), "l", image.Rect(8, 0, 10, 2))
	for range 9 {
		now = now.Add(100 * time.Millisecond)
		lines := strings.Split(ansi.Strip(r.Render(10, 2, tree(8, true))), "\n")
		if lines[1][4] != '|' {
			t.Fatalf("a flying cell went over the layer after it:\n%s", strings.Join(lines, "\n"))
		}
	}
	now = now.Add(200 * time.Millisecond)
	if got, want := r.Render(10, 2, tree(8, true)), Render(tree(8, false), 10, 2); got != want {
		t.Fatalf("after the move:\n%s\nwant:\n%s", got, want)
	}
}

func TestMoveWithOutOfSight(t *testing.T) {
	t.Parallel()

	// In a row as wide as one slot, the one in sight moves out of it, and is
	// drawn on its way out, as the other moves in.
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(swapped bool) Child {
		a := Fixed(4, Text("aaaa")).ID("a").Animate(linear...).MoveWith(fx.Fly{})
		b := Fixed(4, Text("bbbb")).ID("b").Animate(linear...).MoveWith(fx.Fly{})
		if swapped {
			a, b = b, a
		}
		return Row(a, b)
	}
	before := r.Render(4, 1, tree(false))
	if got := r.Render(4, 1, tree(true)); got != before {
		t.Fatalf("at the start of the move: %q, want it as before, %q", got, before)
	}
	if _, ok := r.Frame().Rect("a"); ok {
		t.Fatal("the slot moving out of sight can still be hit")
	}
	now = now.Add(2 * time.Second)
	if got, want := r.Render(4, 1, tree(true)), Render(tree(true), 4, 1); got != want {
		t.Fatalf("after the move: %q, want %q", got, want)
	}
}

func TestLayerMoveWithOutOfSight(t *testing.T) {
	t.Parallel()

	// A layer moving partly or wholly out of sight is drawn whole on its way
	// out, with its own effects or without.
	for _, to := range []int{9, 12} {
		for _, own := range []bool{false, true} {
			now := time.Unix(1, 0)
			r := New(WithClock(func() time.Time { return now }))
			tree := func(x int) Child {
				l := Layer(Text("ab")).Anchor(Start, Start).Offset(x, 0).Size(2, 1).ID("l").Animate(linear...).MoveWith(fx.Fly{})
				if own {
					l = l.Transition(1, fx.Fade{})
				}
				return Overlay(Text(".........."), l)
			}
			before := r.Render(10, 1, tree(0))
			if got := r.Render(10, 1, tree(to)); got != before {
				t.Fatalf("to %d, own effects %v, at the start of the move: %q, want it as before, %q", to, own, got, before)
			}
		}
	}
}
