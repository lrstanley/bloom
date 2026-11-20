// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fx

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	"bloomfx.dev/bloom/canvas"
	uv "github.com/charmbracelet/ultraviolet"
)

// The effects in this file are ambient: they draw a moving texture of glyphs
// into the empty cells of their surface, the spaces on the terminal's
// background, leaving everything else as it is, so they show behind what is
// drawn. Their Time is a clock, such as anim.Ticker's Elapsed, and t is
// their strength, so they suit a layer's backdrop, which fades them with the
// layer, as well as a transition over a whole screen at t=1:
//
//	bloom.Transition(1, screen, fx.Drift{Time: m.clock.Elapsed()})
//
// A clock that changes on every frame keeps such a frame from being
// composited again only when nothing changed, so ambient effects cost a
// composite per frame while their clock runs.

// alnum is the default set of glyphs for ambient effects: letters and digits,
// which read as text from afar without spelling anything.
const alnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// glyphSet picks from a set of single-width glyphs, given as a string,
// without allocating for ASCII ones.
type glyphSet struct {
	ascii string
	runes []string
}

// newGlyphSet returns the set of glyphs, or of [alnum] if empty.
func newGlyphSet(glyphs string) glyphSet {
	if glyphs == "" {
		glyphs = alnum
	}
	for i := range len(glyphs) {
		if glyphs[i] >= utf8.RuneSelf {
			// Each glyph slices the string, rather than allocating its own.
			g := glyphSet{runes: make([]string, 0, utf8.RuneCountInString(glyphs))}
			for rest := glyphs; rest != ""; {
				r, n := utf8.DecodeRuneInString(rest)
				if r == utf8.RuneError && n == 1 {
					g.runes = append(g.runes, string(utf8.RuneError))
				} else {
					g.runes = append(g.runes, rest[:n])
				}
				rest = rest[n:]
			}
			return g
		}
	}
	return glyphSet{ascii: glyphs}
}

// pick returns the glyph at h, from 0 up to 1, of the set.
func (g glyphSet) pick(h float64) string {
	if g.runes != nil {
		return g.runes[int(h*float64(len(g.runes)))]
	}
	i := int(h * float64(len(g.ascii)))
	return g.ascii[i : i+1]
}

// open reports whether an ambient effect may draw into c: a single-width
// space showing the terminal's background, with nothing else drawn in it.
func open(s *canvas.Surface, c *uv.Cell) bool {
	if c.Width != 1 || (c.Content != " " && c.Content != "") ||
		c.Style.Underline != 0 || c.Style.Attrs&(uv.AttrReverse|uv.AttrStrikethrough) != 0 {
		return false
	}
	return c.Style.Bg == nil || sameRGBA(c.Style.Bg, s.Env.Bg)
}

// rooms is scratch for [room].
var rooms = sync.Pool{New: func() any { return new([]bool) }}

// room returns, for each cell of s, row by row, whether an ambient effect
// may draw into it: an [open] cell whose neighbors on the left and right
// are open too, so that glyphs keep a cell away from what is drawn, and the
// spaces between words stay empty. Put it back in rooms when done.
func room(s *canvas.Surface) *[]bool {
	w, h := s.Width(), s.Height()
	m := rooms.Get().(*[]bool) //nolint:errcheck,forcetypeassert // the pool only holds these.
	if cap(*m) < w*h {
		*m = make([]bool, w*h)
	}
	*m = (*m)[:w*h]
	free := *m
	for y := range h {
		row := free[y*w : (y+1)*w]
		for x := range w {
			row[x] = s.Visible(x, y) && open(s, s.Cell(x, y))
		}
		left := true
		for x := range w {
			here := row[x]
			row[x] = here && left && (x == w-1 || row[x+1])
			left = here
		}
	}
	return m
}

func sameRGBA(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// ink draws glyph into the empty cell c, k of the way from its background
// toward to, keeping its background.
func ink(s *canvas.Surface, c *uv.Cell, glyph string, to color.Color, k float64) {
	c.Content = glyph
	c.Style.Fg = s.Mix(s.Bg(c), to, min(k, 1))
	c.Style.Attrs &^= uv.AttrBold | uv.AttrFaint
}

// inkMin is the brightness below which an ambient effect leaves a cell
// empty, as a glyph so faint does not show.
const inkMin = 0.03

// noise returns smooth value noise at x, y, z, from 0 to 1.
func noise(x, y, z float64, seed uint32) float64 {
	xf, yf, zf := math.Floor(x), math.Floor(y), math.Floor(z)
	xi, yi, zi := int(xf), int(yf), int(zf)
	u, v, w := smoothstep(x-xf), smoothstep(y-yf), smoothstep(z-zf)
	plane := func(z int) float64 {
		sd := stepSeed(seed, uint32(z)) //nolint:gosec // wrapping is fine for a hash.
		a := hash(xi, yi, sd) + (hash(xi+1, yi, sd)-hash(xi, yi, sd))*u
		b := hash(xi, yi+1, sd) + (hash(xi+1, yi+1, sd)-hash(xi, yi+1, sd))*u
		return a + (b-a)*v
	}
	a, b := plane(zi), plane(zi+1)
	return a + (b-a)*w
}

// fbm returns two octaves of [noise], from 0 to 1, with more contrast than
// one octave has.
func fbm(x, y, z float64, seed uint32) float64 {
	n := (noise(x, y, z, seed) + 0.5*noise(2*x+17.3, 2*y-9.1, 1.7*z, seed+1)) / 1.5
	// Sums of noise bunch up around the middle; spread them back out.
	return min(max((n-0.5)*1.7+0.5, 0), 1)
}

// Drop is a disturbance that sends a ring out across a [Ripple], such as
// where the pointer was clicked.
type Drop struct {
	// At is where the drop falls, in frame coordinates, as mouse events
	// report them.
	At image.Point
	// Time is when it fell, on the clock of the Ripple's Time.
	Time time.Duration
	// Size scales how far the ring travels, how long it lasts, and how
	// bright it is; zero is 1. A drop of 0.3 suits one left by the pointer
	// moving.
	Size float64
}

// maxDrops is how many drops [Drops] keeps.
const maxDrops = 32

// Drops keeps the 32 most recent drops for a [Ripple], dropping the oldest
// as more are added. It is a value, so a Ripple holding it compares equal
// while nothing changes, and a frame need not be drawn again.
type Drops struct {
	list  [maxDrops]Drop
	added int
}

// Add adds a drop.
func (d *Drops) Add(drop Drop) {
	d.list[d.added%maxDrops] = drop
	d.added++
}

// Ripple sends rings of glyphs out across the empty cells of its surface
// from each of its drops, like a stone dropped in still water: a ring of
// glyphs that scramble as it travels, bright at its front, leaving a faint
// wake that fades behind it. Rings that cross add up. With Rain, drops also
// fall on their own, at random, so it moves while nothing happens.
//
// Feed it drops from mouse events. See the comment at the top of this file
// for Time and t.
type Ripple struct {
	// Time is the clock the drops fell on, such as anim.Ticker's Elapsed.
	Time time.Duration
	// Drops are the drops whose rings are drawn, until each has faded: a
	// drop of size 1 after Life, smaller ones sooner, and larger ones later.
	Drops Drops
	// Rain is how many drops fall on their own each second, at random
	// places; zero is none.
	Rain float64
	// Color is what the glyphs are drawn in, at their brightest; nil is the
	// terminal's foreground.
	Color color.Color
	// Speed is how fast rings travel, in columns per second; zero is 26.
	Speed float64
	// Life is how long a ring of a drop of size 1 lasts; zero is 1.8s.
	Life time.Duration
	// Glyphs are the glyphs drawn, each a single cell wide; empty is
	// letters and digits.
	Glyphs string
	// Seed varies the glyphs, and where rain falls.
	Seed uint32
}

// rippleRainMax bounds how many drops of rain a ripple draws at once.
const rippleRainMax = 64

// ring is the ring of one drop, as it is now.
type ring struct {
	at        image.Point // in the surface's coordinates
	age, life float64     // in seconds
	size      float64
	id        uint32 // varies the ring's hashes
}

// swell is how much the fronts and the wakes of rings add up to in a cell.
type swell struct{ front, wake float32 }

// swells is scratch for [Ripple].
var swells = sync.Pool{New: func() any { return new([]swell) }}

// Apply implements [Effect].
func (e Ripple) Apply(s *canvas.Surface, t float64) {
	w, h := s.Width(), s.Height()
	if t <= 0 || w <= 0 || h <= 0 {
		return
	}
	speed, life := e.Speed, e.Life.Seconds()
	if speed <= 0 {
		speed = 26
	}
	if life <= 0 {
		life = 1.8
	}
	now := e.Time.Seconds()

	// The rings still spreading, of the drops and of the rain.
	var buf [maxDrops + rippleRainMax + 1]ring
	rings := buf[:0]
	add := func(at image.Point, fell, size float64, id uint32) {
		if size <= 0 {
			size = 1
		}
		r := ring{at: at, age: now - fell, life: life * (0.6 + 0.4*size), size: size, id: id}
		if r.age >= 0 && r.age < r.life {
			rings = append(rings, r)
		}
	}
	origin := s.Rect().Min
	for i, d := range e.Drops.list[:min(e.Drops.added, maxDrops)] {
		add(d.At.Sub(origin), d.Time.Seconds(), d.Size, uint32(i)) //nolint:gosec // only varies a hash.
	}
	if e.Rain > 0 {
		every := 1 / e.Rain
		last := int(math.Floor(now / every))
		first := max(int(math.Floor((now-life*1.6)/every)), last-rippleRainMax)
		for k := first; k <= last; k++ {
			at := image.Pt(int(hash(k, 1, e.Seed)*float64(w)), int(hash(k, 2, e.Seed)*float64(h)))
			fell := (float64(k) + hash(k, 3, e.Seed)) * every
			add(at, fell, 0.45+0.75*hash(k, 4, e.Seed), uint32(k)) //nolint:gosec // only varies a hash.
		}
	}
	if len(rings) == 0 {
		return
	}

	field := swells.Get().(*[]swell) //nolint:errcheck,forcetypeassert // the pool only holds these.
	defer swells.Put(field)
	if cap(*field) < w*h {
		*field = make([]swell, w*h)
	}
	*field = (*field)[:w*h]
	cells := *field
	clear(cells)
	for _, r := range rings {
		r.draw(cells, w, h, speed)
	}

	to := e.Color
	if to == nil {
		to = s.Env.Fg
	}
	glyphs := newGlyphSet(e.Glyphs)
	free := room(s)
	defer rooms.Put(free)
	// Glyphs at a front scramble quickly; those of a wake turn over slowly.
	fast, slow := uint32(now*22), now*3
	for y := range h {
		for x := range w {
			i := y*w + x
			front, wake := float64(cells[i].front), float64(cells[i].wake)
			k := min(front, 1) + min(wake, 1)*0.3
			if k*t < inkMin || !(*free)[i] {
				continue
			}
			var glyph string
			if front > wake {
				glyph = glyphs.pick(hash(x, y, stepSeed(e.Seed, fast)))
			} else {
				step := uint32(slow + hash(x, y, e.Seed^0x5bd1e995)*4) //nolint:gosec // only varies a hash.
				glyph = glyphs.pick(hash(x, y, stepSeed(e.Seed, step)))
			}
			ink(s, s.Cell(x, y), glyph, to, k*t)
		}
	}
}

// draw adds the ring to the cells of a w×h surface, row by row.
func (r ring) draw(cells []swell, w, h int, speed float64) {
	p := r.age / r.life
	fade := (1 - p) * (1 - p)
	radius := speed * r.age * (0.7 + 0.3*r.size)
	band := 2.2 + 1.6*r.size // how wide the front is, in columns
	bright := min(r.size, 1) * fade

	// Only the cells the ring can reach: columns count once, rows twice.
	reach := radius + band
	x0, x1 := max(r.at.X-int(reach)-1, 0), min(r.at.X+int(reach)+2, w)
	y0, y1 := max(r.at.Y-int(reach/2)-1, 0), min(r.at.Y+int(reach/2)+2, h)
	for y := y0; y < y1; y++ {
		dy := 2 * float64(y-r.at.Y)
		for x := x0; x < x1; x++ {
			dx := float64(x - r.at.X)
			d := radius - math.Sqrt(dx*dx+dy*dy) // how far inside the front the cell is
			if d < -1 {
				continue
			}
			c := &cells[y*w+x]
			if d < band {
				// The front: brightest just behind its leading edge, with a
				// ragged edge as some cells light before others.
				f := 1 - math.Abs(d-band*0.25)/band
				if f > 0 {
					f *= 0.75 + 0.25*hash(x, y, r.id)
					c.front += float32(f * f * bright)
				}
				continue
			}
			// The wake, thinning out toward the middle.
			if hash(x, y, r.id^0x27d4eb2f) < 0.55*(1-d/(radius+1)) {
				c.wake += float32(0.9 * bright * (1 - d/(radius+1)))
			}
		}
	}
}

// Drift fills the empty cells of its surface with faint patches of glyphs
// that drift, swell, and fade, each glyph scrambling as it fades in before
// it settles. With Radius, the patches also gather and brighten around
// Focus, such as where the pointer is.
//
// See the comment at the top of this file for Time and t.
type Drift struct {
	// Time is the clock the patches move on, such as anim.Ticker's Elapsed.
	Time time.Duration
	// Color is what the glyphs are drawn in, at their brightest; nil is the
	// terminal's foreground.
	Color color.Color
	// Amount is how bright the glyphs are, from 0 to 1; zero is 0.16.
	Amount float64
	// Cover is how much of the surface the patches cover, from 0 to 1; zero
	// is 0.3.
	Cover float64
	// Scale is how large the patches are, in columns; zero is 16.
	Scale float64
	// Speed is how fast the patches change, where 1 is about once every
	// few seconds; zero is 1.
	Speed float64
	// Focus is where the patches gather, in frame coordinates, within
	// Radius columns of it; a zero Radius leaves them alone.
	Focus  image.Point
	Radius int
	// Glyphs are the glyphs drawn, each a single cell wide; empty is
	// letters and digits.
	Glyphs string
	Seed   uint32
}

// Apply implements [Effect].
func (e Drift) Apply(s *canvas.Surface, t float64) {
	if t <= 0 {
		return
	}
	amount, cover, scale, speed := e.Amount, e.Cover, e.Scale, e.Speed
	if amount <= 0 {
		amount = 0.16
	}
	if cover <= 0 {
		cover = 0.3
	}
	if scale <= 0 {
		scale = 16
	}
	if speed <= 0 {
		speed = 1
	}
	to := e.Color
	if to == nil {
		to = s.Env.Fg
	}
	glyphs := newGlyphSet(e.Glyphs)
	now := e.Time.Seconds()
	z := now * 0.22 * speed
	// The patches drift slowly up and to the right as they change.
	ox, oy := now*0.35*speed, -now*0.12*speed
	edge := 1 - cover
	focus := e.Focus.Sub(s.Rect().Min)
	radius := float64(e.Radius)
	w := s.Width()
	free := room(s)
	defer rooms.Put(free)

	for y := range s.Height() {
		for x := range w {
			if !(*free)[y*w+x] {
				continue
			}
			n := fbm((float64(x)-ox)/scale, (2*float64(y)-oy)/scale, z, e.Seed)
			v := (n - edge) / 0.14
			if radius > 0 {
				dx, dy := float64(x-focus.X), 2*float64(y-focus.Y)
				if lens := 1 - math.Sqrt(dx*dx+dy*dy)/radius; lens > 0 {
					v = max(v, 0) + 1.4*smoothstep(lens)*(0.2+n)
				}
			}
			if v <= 0 {
				continue
			}
			c := s.Cell(x, y)
			v = min(v, 1.8)
			// A cell fading in scrambles quickly and settles as it brightens.
			jitter := hash(x, y, e.Seed^0x68e31da4)
			rate := 0.4 + 9*max(1-v, 0)
			step := uint32(now*rate + jitter*7) //nolint:gosec // only varies a hash.
			glyph := glyphs.pick(hash(x, y, stepSeed(e.Seed, step)))
			k := amount * smoothstep(min(v, 1)) * (0.35 + 0.65*jitter)
			if v > 1 {
				k += amount * (v - 1)
			}
			if k*t < inkMin {
				continue
			}
			ink(s, c, glyph, to, k*t)
		}
	}
}

// Rain sends streams of glyphs falling down the empty cells of its surface,
// each led by a bright glyph that scrambles as it falls and trailed by glyphs
// that settle and fade behind it.
//
// See the comment at the top of this file for Time and t.
type Rain struct {
	// Time is the clock the rain falls on, such as anim.Ticker's Elapsed.
	Time time.Duration
	// Color is what the trails are drawn in; nil is the terminal's
	// foreground. Head is what the glyph leading each stream is; nil is
	// Color.
	Color, Head color.Color
	// Amount is how bright the trails are, from 0 to 1; zero is 0.5.
	Amount float64
	// Density is how many of the columns have a stream falling at once,
	// from 0 to 1; zero is 0.3.
	Density float64
	// Speed is how fast streams fall, in rows per second; zero is 12.
	Speed float64
	// Length is how many rows a trail fades over; zero is 14.
	Length int
	// Glyphs are the glyphs drawn, each a single cell wide; empty is
	// letters and digits.
	Glyphs string
	Seed   uint32
}

// Apply implements [Effect].
func (e Rain) Apply(s *canvas.Surface, t float64) {
	if t <= 0 {
		return
	}
	amount, density, speed, length := e.Amount, e.Density, e.Speed, float64(e.Length)
	if amount <= 0 {
		amount = 0.5
	}
	if density <= 0 {
		density = 0.3
	}
	if speed <= 0 {
		speed = 12
	}
	if length <= 0 {
		length = 14
	}
	trail := e.Color
	if trail == nil {
		trail = s.Env.Fg
	}
	head := e.Head
	if head == nil {
		head = trail
	}
	glyphs := newGlyphSet(e.Glyphs)
	now := e.Time.Seconds()
	w, h := s.Width(), s.Height()
	fast := uint32(now * 18)
	free := room(s)
	defer rooms.Put(free)

	for x := range w {
		// Each column falls at its own pace, one stream after another, with a
		// gap between them of up to a screen.
		v := speed * (0.55 + 0.9*hash(x, 0, e.Seed))
		span := float64(h) + length + float64(h)*hash(x, 1, e.Seed)
		at := now*v + span*hash(x, 2, e.Seed)
		cycle := int(math.Floor(at / span))
		if hash(x, cycle, e.Seed^0x2545f491) >= density {
			continue
		}
		top := at - float64(cycle)*span // where the head is, from above the top
		headY := int(top)
		for y := max(headY-int(length), 0); y <= min(headY, h-1); y++ {
			if !(*free)[y*w+x] {
				continue
			}
			c := s.Cell(x, y)
			behind := float64(headY - y)
			if behind == 0 {
				ink(s, c, glyphs.pick(hash(x, y, stepSeed(e.Seed, fast))), head, min(amount*1.8, 1)*t)
				continue
			}
			k := 1 - behind/length
			if amount*k*k*t < inkMin {
				continue
			}
			// Trailing glyphs settle, turning over now and then.
			step := uint32(now*1.5 + hash(x, y, e.Seed)*5) //nolint:gosec // only varies a hash.
			ink(s, c, glyphs.pick(hash(x, y^cycle, stepSeed(e.Seed, step))), trail, amount*k*k*t)
		}
	}
}
