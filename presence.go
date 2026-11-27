// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"
	"time"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// layerPresence is how far in a layer with [LayerNode.Enter] or
// [LayerNode.Exit] is, kept by its ID from frame to frame.
type layerPresence struct {
	p anim.Value
	// last is the layer as it was last declared, and kept and at what it
	// last showed and where, in its overlay, or on the screen for one that
	// floats, to draw as it exits: what it is made of may not draw once it
	// is gone.
	last LayerNode
	kept *canvas.Canvas
	at   image.Rectangle
	// overlay is the path of the overlay the layer is in, and index where
	// it is among the overlay's layers.
	overlay uint64
	index   int
	exiting bool
	// live is whether the layer was drawn this frame, as declared, wasLive
	// whether it was on the last, and seen whether it was drawn at all,
	// live or exiting.
	live, wasLive, seen bool
}

// layerPresences tracks the layers that enter or exit, by ID, from frame to
// frame, for a [Root].
type layerPresences struct {
	now    func() time.Time
	method ansi.Method
	m      map[ID]*layerPresence
	at     time.Time
	size   image.Point
	snap   bool
	// follow is whether a frame follows this one, so a layer can start
	// entering or exiting: one is already on its way.
	follow bool
	// running is whether a layer is still entering or exiting, and
	// overlays whether an overlay was drawn, so a layer could appear in it
	// on the next frame.
	running, overlays bool
}

func newLayerPresences(now func() time.Time, method ansi.Method) *layerPresences {
	return &layerPresences{now: now, method: method, m: make(map[ID]*layerPresence)}
}

// armed reports whether a layer could enter or exit on the next frame, for
// [Root] to have one follow it.
func (ps *layerPresences) armed() bool { return ps.overlays || len(ps.m) > 0 }

// begin starts a frame laid out in w×h. When the size differs from the last
// frame's, layers are shown or gone at once rather than entering or exiting.
func (ps *layerPresences) begin(w, h int) {
	size := image.Pt(w, h)
	ps.snap, ps.size = size != ps.size, size
	ps.at, ps.running, ps.overlays = time.Time{}, false, false
	for _, s := range ps.m {
		s.wasLive, s.live, s.seen = s.live, false, false
	}
}

// end forgets the layers not drawn this frame.
func (ps *layerPresences) end() {
	for id, s := range ps.m {
		if !s.seen {
			delete(ps.m, id)
		}
	}
}

// clock reads the time of the frame once, when the first layer that enters
// or exits is drawn.
func (ps *layerPresences) clock() time.Time {
	if ps.at.IsZero() {
		ps.at = ps.now()
	}
	return ps.at
}

// still reports whether layers appear and go at once this frame.
func (ps *layerPresences) still() bool { return ps.snap || !ps.follow || !anim.Animates() }

// present returns the layers of the overlay at path to draw this frame:
// those declared, each drawn as far in as it has entered, with those that
// are exiting among them, where they were.
func (ps *layerPresences) present(path uint64, layers []LayerNode) []LayerNode {
	ps.overlays = true
	var out []LayerNode
	for i, n := range layers {
		if n.id.IsZero() || (n.enter == nil && n.exit == nil) || n.absent {
			continue
		}
		if out == nil {
			out = slices.Clone(layers)
		}
		out[i] = ps.live(n, path, i)
	}
	if out == nil {
		out = layers
	}
	// The layers drawn last frame and gone now exit, and those exiting are
	// drawn where they were, in the order they were.
	var exits []*layerPresence
	for id, s := range ps.m {
		if s.overlay != path || s.live {
			continue
		}
		if !s.exiting && !ps.startExit(s) {
			continue
		}
		if done := !ps.advance(s); done && s.p.Float() <= 0 {
			delete(ps.m, id)
			continue
		}
		s.seen = true
		exits = append(exits, s)
	}
	if len(exits) == 0 {
		return out
	}
	slices.SortFunc(exits, func(a, b *layerPresence) int { return a.index - b.index })
	out = slices.Clone(out)
	for _, s := range exits {
		out = slices.Insert(out, min(s.index, len(out)), s.ghost())
	}
	return out
}

// ghost returns the layer of s as it exits: what it last showed, where it
// was, with its backdrop and shadow. It takes no input.
func (s *layerPresence) ghost() LayerNode {
	l := s.last
	n := Layer(Drawable(keptCells{s.kept})).Place(s.at)
	n.float, n.backdrop, n.shadowX, n.shadowY, n.shadow, n.clear = l.float, l.backdrop, l.shadowX, l.shadowY, l.shadow, l.clear
	n = n.Transition(s.p.Float(), l.exit.effects...)
	n.leaving = true
	return n
}

// placed notes where the layer with ID id was laid out this frame, in its
// overlay, or on the screen for one that floats, for it to exit from.
func (ps *layerPresences) placed(id ID, r image.Rectangle) {
	if s := ps.m[id]; s != nil && s.live {
		s.at = r
	}
}

// live notes layer n, at index i of the overlay at path, drawn this frame,
// and returns it as far in as it has entered.
func (ps *layerPresences) live(n LayerNode, path uint64, i int) LayerNode {
	s := ps.m[n.id]
	switch {
	case s == nil:
		s = &layerPresence{p: anim.NewValue(1)}
		ps.m[n.id] = s
		if n.enter != nil && !ps.still() {
			s.p = anim.NewValue(0, n.enter.opts...)
			s.p.SetTarget(1, ps.clock())
		}
	case s.exiting:
		// Drawn again, it comes back from as far as it went.
		s.exiting = false
		s.p = anim.NewValue(s.p.Float(), enterOpts(n)...)
		if n.enter == nil || ps.still() {
			s.p.Set(1)
		} else {
			s.p.SetTarget(1, ps.clock())
		}
	}
	// Where it is drawn is noted as it is laid out, and what it shows as it
	// is drawn, before its own effects.
	s.live, s.seen, s.last, s.overlay, s.index, s.at = true, true, n, path, i, image.Rectangle{}
	var keep []fx.Effect
	if n.exit != nil {
		if s.kept == nil {
			s.kept = canvas.New(0, 0, ps.method)
		}
		keep = []fx.Effect{keepCells{s.kept}}
	}
	t, effects := n.t, n.effects
	if ps.advance(s) || s.p.Float() < 1 {
		t, effects = s.p.Float(), nil
		if n.enter != nil {
			effects = n.enter.effects
		} else if n.exit != nil {
			effects = n.exit.effects
		}
	}
	if keep == nil && t == n.t && len(effects) == len(n.effects) {
		return n
	}
	return n.Transition(t, slices.Concat(keep, effects)...)
}

// startExit has the layer of s, drawn last frame and gone now, exit,
// reporting whether it does: it needs Exit, and to have been drawn as
// declared rather than hiding by its own presence.
func (ps *layerPresences) startExit(s *layerPresence) bool {
	if !s.wasLive || s.last.exit == nil || s.last.leaving || s.at.Empty() || ps.still() {
		return false
	}
	s.exiting = true
	s.p = anim.NewValue(s.p.Float(), s.last.exit.opts...)
	s.p.SetTarget(0, ps.clock())
	return true
}

// advance moves the progress of s on to this frame, reporting whether it is
// still running. On a frame of a new size, it is done.
func (ps *layerPresences) advance(s *layerPresence) bool {
	if !s.p.Animating() {
		return false
	}
	if ps.snap {
		s.p.Set(s.p.Target())
		return false
	}
	if s.p.Advance(ps.clock()) {
		ps.running = true
		return true
	}
	return false
}

// enterOpts returns the options of n's entrance, or its exit's for a layer
// that only exits.
func enterOpts(n LayerNode) []anim.Option {
	if n.enter != nil {
		return n.enter.opts
	}
	if n.exit != nil {
		return n.exit.opts
	}
	return nil
}

// keepCells copies the cells of a layer that exits into kept, as it is
// drawn, before its own effects, for it to be drawn as it was once gone.
type keepCells struct{ kept *canvas.Canvas }

// Apply implements [fx.Effect].
func (k keepCells) Apply(s *canvas.Surface, _ float64) {
	k.kept.Resize(s.Width(), s.Height())
	k.kept.Clear()
	for y := range s.Height() {
		for x := range s.Width() {
			if c := s.Cell(x, y); c != nil && c.Width > 0 && s.Visible(x, y) {
				k.kept.SetCell(x, y, c)
			}
		}
	}
}

// keptCells draws what a layer that exits last showed.
type keptCells struct{ kept *canvas.Canvas }

// Draw implements [uv.Drawable].
func (k keptCells) Draw(scr uv.Screen, area image.Rectangle) {
	for y := range min(k.kept.Height(), area.Dy()) {
		for x := range min(k.kept.Width(), area.Dx()) {
			if c := k.kept.CellAt(x, y); c != nil && c.Width > 0 {
				scr.SetCell(area.Min.X+x, area.Min.Y+y, c)
			}
		}
	}
}
