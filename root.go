// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"cmp"
	"image/color"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/anim"
	"github.com/lrstanley/bloom/internal/motion"
	"github.com/lrstanley/bloom/on"
)

// Root renders a layout as a model's [tea.View], keeping what it can between
// frames. Keep one in the root model, by pointer, and pass it the model's
// layout from View. Pass every message to [Root.Update] and return its command.
type Root struct {
	r         *renderer
	interval  time.Duration
	unfocused time.Duration // the interval while the terminal is blurred
	blurred   bool
	pending   bool
	now       func() time.Time
	focus     focusTarget
	mouse     mouse
}

// layoutFrameMsg is the tick of a layout animation frame for root.
type layoutFrameMsg struct{ root *Root }

// Option configures a [Root].
type Option func(*config)

type config struct {
	method       ansi.Method
	fg, bg       color.Color
	fps          int
	unfocusedFPS int
	now          func() time.Time
	focus        focusTarget
}

// WithWidthMethod sets how the width of graphemes is measured. It must match
// the terminal's, which Bubble Tea measures with [ansi.GraphemeWidth] when
// the terminal supports it. The default is [ansi.GraphemeWidth], as Lip
// Gloss uses.
func WithWidthMethod(method ansi.Method) Option {
	return func(c *config) { c.method = method }
}

// WithColors sets the terminal's default colors, for effects that blend
// colors, until the terminal reports its own (see [Root.Init]).
func WithColors(fg, bg color.Color) Option {
	return func(c *config) { c.fg, c.bg = fg, bg }
}

// WithFPS sets the layout animation tick rate, defaulting to 60.
// Values below one use one frame per second.
func WithFPS(n int) Option { return func(c *config) { c.fps = max(n, 1) } }

// WithUnfocusedFPS sets the layout animation tick rate while the terminal is
// not focused, defaulting to 20, and never above [WithFPS]. Terminals report
// focus only while [tea.View] ReportFocus is set, as on the view from
// [Root.View]; until they do, the root counts the terminal as focused.
func WithUnfocusedFPS(n int) Option { return func(c *config) { c.unfocusedFPS = max(n, 1) } }

// WithClock sets the clock for layout animations and for telling double
// clicks apart, for tests. It must not be nil.
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// WithFocus lets clicks move the model's focus: a click focuses the
// innermost node under the pointer that f can focus, as [Focus.Next]
// would, before any handler for the click runs, so a handler can move the
// focus on from there. f must stay where it is, as a field of a model used
// by pointer does.
func WithFocus[K comparable](f *Focus[K]) Option { return func(c *config) { c.focus = f } }

// New returns a Root.
func New(opts ...Option) *Root {
	c := config{method: ansi.GraphemeWidth, fps: 60, unfocusedFPS: 20, now: time.Now}
	for _, opt := range opts {
		opt(&c)
	}
	r := &Root{
		r:         newRenderer(c.method),
		interval:  time.Second / time.Duration(c.fps),
		unfocused: time.Second / time.Duration(min(c.unfocusedFPS, c.fps)),
		now:       c.now,
		focus:     c.focus,
	}
	r.r.motion = motion.New(c.now)
	r.r.env.Fg = cmp.Or(c.fg, r.r.env.Fg)
	r.r.env.Bg = cmp.Or(c.bg, r.r.env.Bg)
	return r
}

// Init asks the terminal for its default colors, which effects that blend
// colors use. Return it from the model's Init. Pass every message to
// [Root.Update] and return its command from the model's Update.
func (r *Root) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestForegroundColor)
}

// Update notes what the terminal reports about itself: its default colors,
// color profile, and focus. Pass every message to Update and return its
// command: it schedules frames while layout animations run, fewer while the
// terminal is not focused (see [WithUnfocusedFPS]), and returns the commands
// of mouse handlers it runs. It runs them for mouse events that the OnMouse
// handler of [Root.View] did not see, such as when the view is built some
// other way, and runs [on.Hover] handlers as nodes move under the pointer or
// the window loses focus.
func (r *Root) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		r.r.env.Bg = msg.Color
	case tea.ForegroundColorMsg:
		r.r.env.Fg = msg.Color
	case tea.ColorProfileMsg:
		r.r.env.Profile = msg.Profile
	case tea.FocusMsg:
		r.blurred = false
	case tea.BlurMsg:
		r.blurred = true
	}
	return tea.Batch(r.input(msg), r.tick(msg))
}

// tick returns the command for the next layout animation frame, if one is
// due.
func (r *Root) tick(msg tea.Msg) tea.Cmd {
	if tick, ok := msg.(layoutFrameMsg); ok {
		if tick.root != r {
			return nil
		}
		r.pending = false
		if !r.r.motion.Running() {
			return nil
		}
	}
	// With motion off, boxes reach their targets as they are laid out.
	if r.pending || r.r.motion.Len() == 0 || !anim.Motion() {
		return nil
	}
	r.pending = true
	d := r.interval
	if r.blurred {
		d = r.unfocused
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return layoutFrameMsg{root: r} })
}

// Render lays out and draws c in a w×h frame and returns it as a string. A
// height of zero or less is c's natural height, for programs that do not use
// the whole screen. Nothing is drawn while the width is zero or less, as it is
// before Bubble Tea reports the window's size.
func (r *Root) Render(w, h int, c Child) string {
	if w <= 0 {
		c = nil
	}
	out := r.r.render(c, w, h)
	r.r.updateFrame()
	return out
}

// View lays out and draws c in a w×h frame, and returns a view showing it,
// with the cursor of the [TextNode] that placed one, and an OnMouse handler
// that runs the mouse handlers of the nodes under the pointer in this
// frame, which is the one on screen when Bubble Tea calls it. It runs them
// on the event loop, before Update gets the event, so they can change the
// model, and returns their commands to run in the background.
func (r *Root) View(w, h int, c Child) tea.View {
	v := tea.NewView(r.Render(w, h, c))
	f := r.r.frame
	v.OnMouse = func(msg tea.MouseMsg) tea.Cmd { return r.mouseEvent(f, msg) }
	v.Cursor = f.cursor
	return v
}

// Frame returns the geometry of the last frame rendered, for moving focus
// with [Focus] and for finding where nodes are on screen.
func (r *Root) Frame() *Frame {
	return r.r.frame
}

// updateFrame makes the Frame for the frame just drawn, keeping the last one
// when nothing about it changed. Handlers are closures made by the latest
// View, so a frame with any is made again, sharing what did not change.
func (r *renderer) updateFrame() {
	f := r.frame
	same := f != nil && slices.Equal(f.entries, r.entries) && sameCursor(f.cursor, r.cursor)
	if same && !hasHandlers(f.handlers) && !hasHandlers(r.handlers) {
		return
	}
	entries, cursor := r.entries, r.cursor
	if same {
		entries, cursor = f.entries, f.cursor
	} else {
		entries = slices.Clone(entries)
	}
	r.frame = &Frame{entries: entries, handlers: slices.Clone(r.handlers), cursor: cursor}
}

func hasHandlers(handlers [][]on.Handler) bool {
	for _, h := range handlers {
		if len(h) > 0 {
			return true
		}
	}
	return false
}

func sameCursor(a, b *tea.Cursor) bool {
	return a == b || (a != nil && b != nil && *a == *b)
}

var renderers = sync.Pool{New: func() any { return newRenderer(ansi.GraphemeWidth) }}

// Render lays out and draws c in a w×h frame and returns it as a string,
// for using a layout where a string is wanted. A w or h of zero or less is
// c's natural size on that axis. A model's View should use [Root] instead,
// which keeps more between frames. Render is stateless and draws animation
// targets immediately.
func Render(c Child, w, h int) string {
	r, _ := renderers.Get().(*renderer)
	defer renderers.Put(r)
	return r.render(c, w, h)
}

// Solve lays out c in a w×h frame without drawing it, and returns its
// geometry at animation targets. Solve is stateless; retained layout
// animations may draw intermediate rectangles. Components that need their
// target size in Update can use Solve there. Effects are not run, so a node a
// transition is moving, such as with [fx.Slide], is where it was laid out:
//
//	case tea.WindowSizeMsg:
//		m.width, m.height = msg.Width, msg.Height
//		if r, ok := bloom.Solve(m.layout(), m.width, m.height).Rect(paneReader); ok {
//			m.reader.SetWidth(r.Dx())
//			m.reader.SetHeight(r.Dy())
//		}
func Solve(c Child, w, h int) *Frame {
	r, _ := renderers.Get().(*renderer)
	defer renderers.Put(r)
	r.parse = false
	defer func() { r.parse = true }()
	r.paintTree(c, w, h)
	r.placeEntries()
	r.cache.EndFrame(w * h)
	return &Frame{entries: slices.Clone(r.entries), handlers: slices.Clone(r.handlers), cursor: r.cursor}
}
