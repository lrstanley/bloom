// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/fx"
)

func TestMoveWith(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
	pane := func(id, text string, move bool) SlotNode {
		s := Fixed(6, Text(text)).ID(id).Animate(opts...)
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
	requireRect(t, r, "b", image.Rect(0, 0, 6, 2))
	requireRect(t, r, "a", image.Rect(6, 0, 12, 2))
	if !r.r.motion.running {
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
	if r.r.motion.running {
		t.Fatal("the animation is still running after the move")
	}
}

func TestMoveWithModes(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
	base := func(at int, s SlotNode) Child { return Row(Fixed(at, nil), s, Space()) }

	// A dragged layer gliding with an ID, whose slot then takes over, as
	// when it is dropped, finishes its glide rather than moving with effects.
	layer := func(x int) Child {
		return Overlay(Row(Space()), Layer(Text("x")).Anchor(Start, Start).Offset(x, 0).Size(4, 1).ID("p").Animate(opts...))
	}
	r.Render(30, 1, layer(0))
	r.Render(30, 1, layer(20))
	now = now.Add(500 * time.Millisecond)
	r.Render(30, 1, base(0, Fixed(4, Text("x")).ID("p").Animate(opts...).MoveWith(fx.Fly{})))
	requireRect(t, r, "p", image.Rect(10, 0, 14, 1))

	// Moving with effects once it has landed, it moves from there.
	now = now.Add(2 * time.Second)
	slot := func(at int, move bool) Child {
		s := Fixed(4, Text("x")).ID("p").Animate(opts...)
		if move {
			s = s.MoveWith(fx.Fly{})
		}
		return base(at, s)
	}
	r.Render(30, 1, slot(0, true))
	requireRect(t, r, "p", image.Rect(0, 0, 4, 1))
	r.Render(30, 1, slot(10, true))
	requireRect(t, r, "p", image.Rect(10, 0, 14, 1))
	if !r.r.motion.running {
		t.Fatal("did not move with effects")
	}

	// Gliding once more, it glides from where it was laid out.
	now = now.Add(2 * time.Second)
	r.Render(30, 1, slot(10, true))
	r.Render(30, 1, slot(20, false))
	requireRect(t, r, "p", image.Rect(10, 0, 14, 1))
	now = now.Add(500 * time.Millisecond)
	r.Render(30, 1, slot(20, false))
	requireRect(t, r, "p", image.Rect(15, 0, 19, 1))
}

func TestLayerMoveWith(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
	tree := func(x int, move bool) Child {
		l := Layer(Text("ab\ncd")).Anchor(Start, Start).Offset(x, 0).Size(2, 2).ID("l").Animate(opts...)
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
	requireRect(t, r, "l", image.Rect(8, 0, 10, 2))
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
	opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
	tree := func(swapped bool) Child {
		a := Fixed(4, Text("aaaa")).ID("a").Animate(opts...).MoveWith(fx.Fly{})
		b := Fixed(4, Text("bbbb")).ID("b").Animate(opts...).MoveWith(fx.Fly{})
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
			opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
			tree := func(x int) Child {
				l := Layer(Text("ab")).Anchor(Start, Start).Offset(x, 0).Size(2, 1).ID("l").Animate(opts...).MoveWith(fx.Fly{})
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
