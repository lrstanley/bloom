// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// progress records the progress it was last applied with, and -1 until it
// is, as an effect.
type progress struct{ t *float64 }

func newProgress() progress { t := -1.0; return progress{t: &t} }

func (p progress) Apply(_ *canvas.Surface, t float64) { *p.t = t }

func (p progress) take() float64 { t := *p.t; *p.t = -1; return t }

// presenceRoot returns a root on a clock that the returned func moves on.
func presenceRoot() (r *Root, wait func(time.Duration)) {
	now := time.Unix(1, 0)
	r = New(WithClock(func() time.Time { return now }))
	return r, func(d time.Duration) { now = now.Add(d) }
}

func TestLayerEnterExit(t *testing.T) {
	setMotion(t, anim.Full)

	in, out := newProgress(), newProgress()
	r, wait := presenceRoot()
	view := func(open bool, layers ...LayerNode) string {
		if open {
			layers = append(layers, Layer(Text("dialog")).ID("d").Enter(linear, in).Exit(linear, out))
		}
		r.Update("msg") // the frame that follows is on its way
		return r.Render(20, 3, Overlay(Text("base"), layers...))
	}

	// On the first frame it is shown at once.
	view(true)
	if got := in.take(); got != -1 {
		t.Fatalf("entered on the first frame, at %v", got)
	}
	// Gone, it is drawn as it exits, until it is out.
	if s := view(false); !strings.Contains(s, "dialog") {
		t.Fatalf("not drawn as it exits:\n%s", s)
	}
	if got := out.take(); got != 1 {
		t.Fatalf("exiting at %v, want 1", got)
	}
	wait(time.Second / 2)
	view(false)
	if got := out.take(); got != 0.5 {
		t.Fatalf("exiting at %v, want 0.5", got)
	}
	wait(time.Second)
	if s := view(false); strings.Contains(s, "dialog") || out.take() != -1 {
		t.Fatalf("still exiting once out:\n%s", s)
	}
	if r.r.presence.running || r.Update(layoutFrameMsg{root: r}) != nil {
		t.Fatal("asked for a frame once out")
	}

	// Drawn again, it enters, with frames to follow.
	view(true)
	if got := in.take(); got != 0 {
		t.Fatalf("entering at %v, want 0", got)
	}
	wait(time.Second / 4)
	view(true)
	if got := in.take(); got != 0.25 {
		t.Fatalf("entering at %v, want 0.25", got)
	}
	if !r.r.presence.running || r.Update(layoutFrameMsg{root: r}) == nil {
		t.Fatal("no frame asked for while entering")
	}

	// Gone partway in, it exits from there, and drawn again partway out,
	// it comes back from there.
	view(false)
	if got := out.take(); got != 0.25 {
		t.Fatalf("exiting from %v, want 0.25", got)
	}
	wait(time.Second / 8)
	view(false)
	left := out.take()
	if left >= 0.25 || left <= 0 {
		t.Fatalf("exiting at %v", left)
	}
	view(true)
	if got := in.take(); got != left {
		t.Fatalf("came back from %v, want %v", got, left)
	}
	wait(time.Second)
	view(true)
	view(true)
	if in.take() != -1 || r.r.presence.running {
		t.Fatal("still entering once in")
	}
}

func TestLayerExitPlace(t *testing.T) {
	setMotion(t, anim.Full)

	r, wait := presenceRoot()
	top := Layer(Text("top")).ID("top").Anchor(Start, Start)
	view := func(open bool) string {
		layers := []LayerNode{Layer(Text("under")).Anchor(Start, Start)}
		if open {
			layers = append(layers, Layer(Text("dialog")).ID("d").Anchor(Start, Start).Exit(linear, fx.Fade{}))
		}
		layers = append(layers, top)
		r.Update("msg")
		return r.Render(10, 1, Overlay(Text(""), layers...))
	}
	view(true)
	view(false)
	wait(time.Second / 10)
	// It exits among the layers where it was, beneath the one after it.
	if s := view(false); !strings.HasPrefix(s, "top") {
		t.Fatalf("exiting over the layer after it:\n%s", s)
	}
	if _, ok := r.Frame().Rect("d"); ok {
		t.Fatal("exiting layer is in the frame, to take input")
	}
}

// draws counts how many times it is drawn, as a drawable.
type draws struct{ n *int }

func (d draws) Draw(scr uv.Screen, area image.Rectangle) {
	*d.n++
	uv.NewStyledString("dialog").Draw(scr, area)
}

func TestLayerExitKeepsCells(t *testing.T) {
	setMotion(t, anim.Full)

	n := 0
	r, wait := presenceRoot()
	view := func(open bool) string {
		var layers []LayerNode
		if open {
			layers = append(layers, Layer(Drawable(draws{&n})).Place(image.Rect(2, 0, 8, 1)).ID("d").Exit(linear, fx.Fade{}))
		}
		r.Update("msg")
		return r.Render(10, 1, Overlay(Text("base......"), layers...))
	}
	view(true)
	drawn := n
	// What it is made of is not drawn again once it is gone: it shows what
	// it last showed.
	if s := view(false); !strings.Contains(ansi.Strip(s), "dialog") {
		t.Fatalf("not drawn as it exits:\n%s", s)
	}
	wait(time.Second / 10)
	view(false)
	if n != drawn {
		t.Fatalf("its child drawn %d more times as it exits", n-drawn)
	}
}

func TestLayerEnterStill(t *testing.T) {
	in := newProgress()
	r, _ := presenceRoot()
	tree := func(open bool) Child {
		var layers []LayerNode
		if open {
			layers = append(layers, Layer(Text("dialog")).ID("d").Enter(linear, in).Exit(linear, in))
		}
		return Overlay(Text("base"), layers...)
	}

	// With no frame on its way, it is shown at once, rather than waiting
	// for one.
	r.Render(20, 3, tree(false))
	r.Render(20, 3, tree(true))
	if got := in.take(); got != -1 {
		t.Fatalf("entered at %v with no frame to follow", got)
	}

	// Nor does it enter or exit with motion off.
	setMotion(t, anim.Off)
	r.Update("msg")
	r.Render(20, 3, tree(false))
	r.Render(20, 3, tree(true))
	if got := in.take(); got != -1 {
		t.Fatalf("entered at %v with motion off", got)
	}
	r.Render(20, 3, tree(false))
	if got := in.take(); got != -1 {
		t.Fatalf("exited at %v with motion off", got)
	}
}

func TestLayerExitAfterPresence(t *testing.T) {
	setMotion(t, anim.Full)

	out := newProgress()
	r, _ := presenceRoot()
	p := anim.NewPresence()
	p.Show()
	p.Hide() // hiding, by its own presence
	view := func(layers ...LayerNode) {
		r.Update("msg")
		r.Render(20, 3, Overlay(Text("base"), layers...))
	}
	view(Layer(Text("dialog")).ID("d").Presence(p).Exit(linear, out))
	view()
	if got := out.take(); got != -1 {
		t.Fatalf("exited again at %v once its presence hid it", got)
	}
}

func TestReducedMotion(t *testing.T) {
	setMotion(t, anim.Reduced)

	// Effects that move cells fade the node instead.
	r, _ := presenceRoot()
	at := newProgress()
	slid := r.Render(10, 1, Overlay(Text("base"), Layer(Text("dialog")).Anchor(Start, Start).Transition(0, fx.Slide{From: Right}, at)))
	if got := at.take(); got != 0 {
		t.Fatalf("the rest of its effects ran at %v", got)
	}
	faded := r.Render(10, 1, Overlay(Text("base"), Layer(Text("dialog")).Anchor(Start, Start).Transition(0, fx.Fade{})))
	if slid != faded {
		t.Fatalf("slid rather than faded:\n%q\nwant:\n%q", slid, faded)
	}

	// Layout snaps rather than gliding.
	now := time.Unix(1, 0)
	r = New(WithClock(func() time.Time { return now }))
	glide := func(x int) Child { return Row(Fixed(x, nil), Fixed(5, Text("x")).ID("x").Animate(linear...)) }
	r.View(100, 10, glide(0))
	r.View(100, 10, glide(20))
	expectRect(t, r.Frame(), "x", image.Rect(20, 0, 25, 10))
}
