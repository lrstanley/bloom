// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"cmp"
	"image"
	"reflect"
	"slices"
	"time"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/internal/anchor"
	"bloomfx.dev/bloom/internal/same"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultTipDelay = 500 * time.Millisecond
	defaultTipWarm  = 300 * time.Millisecond
	defaultTipWidth = 60
	// tipTint is how far the default chip's background is from the
	// terminal's background toward its foreground.
	tipTint = 0.25
)

// Tooltips configures the tooltips of a [Root], given with [WithTooltips] or
// [Root.SetTooltips]. A field left at its zero value takes its default.
//
// A tooltip is short text about one node, given with the node's Tooltip
// method, or the full text of a [TextNode.Truncate] node that was cut to
// fit, unless the node or one it is inside has NoTooltip. It shows when the
// pointer rests on the node, or when a key press moves the focus onto it
// (see [WithFocus]), and goes away on any key press, click, or wheel event,
// or when the pointer leaves the node, unless onto the tooltip's bubble.
// Root draws it over everything in the layout. It takes no space in the
// layout and no mouse events: a click on it reaches what is beneath, and
// the pointer over it is over what is beneath.
//
// A tooltip given with Tooltip is a bubble beside its node, on the side
// given with [TipSide], or without one, on whichever side hides the least of
// what is on screen, preferring above, then below, right, and left. It is
// never drawn over its node, the cell under the pointer, or the cursor,
// unless it fits nowhere else, but for the pointer on a node taller than a
// row, it goes beside the node's cells on the pointer's row, to stay near
// the pointer, and may cover the node's other rows. Cut text is unfolded:
// its full text is drawn over it, in place, on its row, and stays while the
// pointer is over the cut text, not over the rest of it past the node,
// which covers what is beside the node, such as the next cell of a table's
// row. Either kind can be shown in a [TooltipSink] instead, such as in a
// status line, or drawn by the app from [Root.Tooltip].
//
// A node's TooltipOpen pins its tooltip open.
type Tooltips struct {
	// Style is the style of the chip a tooltip is drawn in. Without one, the
	// chip has a cell of padding on either side and a background between the
	// terminal's default background and foreground.
	Style lipgloss.Style
	// Delay is how long the pointer rests on a node, or the focus stays on
	// it, before its tooltip shows: 500ms by default. A negative delay shows
	// it at once. A tooltip's [TipDelay] replaces it.
	Delay time.Duration
	// Warm is how long, after a tooltip closes because the pointer left it,
	// the next one the pointer moves onto shows at once, without waiting for
	// its delay: 300ms by default. A negative one never does.
	Warm time.Duration
	// MaxWidth is the widest a tooltip beside its node gets, its padding and
	// border included, before its text wraps: 60 cells by default. It is
	// never wider than the screen.
	MaxWidth int
	// Truncated is how the full text of a [TextNode.Truncate] node that was
	// cut is shown: [TipUnfold] for [TipDefault], [TipBubble], [TipSink],
	// [TipHidden], or [TipOff] for not at all. Cut text has a tooltip of
	// its own only while it is not TipOff, which costs a frame with
	// tooltips on a little for each node that was cut.
	Truncated TipMode
	// Explicit is how the text given with Tooltip is shown: [TipBubble] for
	// [TipDefault], [TipSink], or [TipHidden]. Other modes are taken as
	// TipBubble.
	Explicit TipMode
}

// TipMode is how the tooltips of a kind are shown, set for each kind in
// [Tooltips]. The zero value, [TipDefault], is the kind's default.
type TipMode uint8

const (
	// TipDefault shows the tooltips of a kind as the kind does by default:
	// TipUnfold for truncated text, and TipBubble for the text given with
	// Tooltip.
	TipDefault TipMode = iota
	// TipUnfold draws the full text of text that was cut over it, in place,
	// as if the rest of it were revealed: the first glyph of the full text
	// is in the cell of the first glyph of the cut text, and the chip's
	// padding goes before it, shifted left only to stay on screen. Text
	// wider than the screen is shown as a bubble instead. The one of the
	// pointer stays open only while the pointer is over the cut text. It is
	// the default for truncated text.
	TipUnfold
	// TipBubble draws a tooltip in a chip beside its node. It is the default
	// for the text given with Tooltip.
	TipBubble
	// TipSink shows tooltips in a [TooltipSink] that the layout places, such
	// as in a status line, and none beside their nodes while a sink has a
	// cell on screen. While none does, they are shown as they are by
	// default.
	TipSink
	// TipHidden draws no tooltips of the kind, but has them open and close
	// as they otherwise would, for an app that draws the one of the
	// pointer or the focus itself, from [Root.Tooltip].
	TipHidden
	// TipOff shows no tooltips of the kind. Unlike TipHidden, the kind has
	// no tooltips at all: none opens.
	TipOff
)

// WithTooltips turns tooltips on, configured with t. Without it, they are
// off: a node's Tooltip does nothing, and drawing a frame costs nothing
// more. [Root.EnableTooltips] turns them on and off again.
func WithTooltips(t Tooltips) Option {
	return func(c *config) { c.tips, c.tipsOn = t, true }
}

// SetTooltips replaces the configuration of tooltips, such as for a new
// theme, without turning them on or off.
func (r *Root) SetTooltips(t Tooltips) {
	r.tip.configure(t)
	r.r.autoTips = r.tip.on && r.tip.cfg.Truncated != TipOff
}

// EnableTooltips turns tooltips on or off, such as for a setting of the
// user's. Turning them off closes the one showing.
func (r *Root) EnableTooltips(on bool) {
	r.tip.on, r.r.tipsOn, r.r.top, r.r.sinkText = on, on, nil, nil
	r.r.autoTips = on && r.tip.cfg.Truncated != TipOff
	if on {
		r.r.top, r.r.sinkText = r.paintTips, r.sinkText
		return
	}
	s := &r.tip
	s.disarm()
	s.close(time.Time{}, false)
	s.blockedOK, s.warmUntil = false, time.Time{}
	clear(s.pins)
	s.pins = s.pins[:0]
}

// Tooltip returns the text of the tooltip of the pointer or the focus
// showing now, however it is shown, and reports false while none is. It is
// not one pinned open. It is what the root last drew, or opened
// since, so read it in View, before the frame is drawn.
//
// It is for an app that shows the tooltip itself, such as in a status line
// of its own. To have the root draw nothing for it, set the [Tooltips]
// mode of its kind to [TipHidden], which still has it open and close as it
// otherwise would. [TooltipSink] is a node that shows it in the frame it
// opens in, for an app that only needs it placed.
func (r *Root) Tooltip() (string, bool) {
	s := &r.tip
	f := r.r.frame
	if !s.on || !s.shown || f == nil {
		return "", false
	}
	if t := tipIn(f.tips, f.find(s.shownKey)); t != nil && !t.pinOnly {
		return t.text(), true
	}
	return "", false
}

// tooltip is the tooltip a node was given with Tooltip.
type tooltip struct {
	text    string
	side    Direction // where it goes, or 0 to place it automatically
	style   lipgloss.Style
	styled  bool
	delay   time.Duration
	delayed bool
}

// TipOption configures the tooltip given with a node's Tooltip method.
type TipOption func(*tooltip)

// TipSide puts the tooltip against side of its node: above or below it on
// the next row, or left or right of it with a blank cell between, shifted
// along the side to stay on screen, even when it covers what is there. It
// goes on the opposite side when it does not fit on side, or would cover
// the cell under the pointer or the cursor, and where it would without
// TipSide when it fits on neither. Without it, a tooltip goes on whichever
// side hides the least of what is on screen.
func TipSide(side Direction) TipOption { return func(t *tooltip) { t.side = side } }

// TipStyle draws the tooltip with style, in place of the [Tooltips] Style.
func TipStyle(style lipgloss.Style) TipOption {
	return func(t *tooltip) { t.style, t.styled = style, true }
}

// TipDelay shows the tooltip once the pointer has rested on its node for d,
// or the focus has stayed on it for d, in place of the [Tooltips] Delay. A
// delay of zero or less shows it at once.
func TipDelay(d time.Duration) TipOption {
	return func(t *tooltip) { t.delay, t.delayed = max(d, 0), true }
}

// newTooltip returns the tooltip of text with opts, or nil for no text.
func newTooltip(text string, opts []TipOption) *tooltip {
	if text == "" {
		return nil
	}
	t := &tooltip{text: text}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// equal reports whether t and u show the same way.
func (t *tooltip) equal(u *tooltip) bool {
	return t == u || (t.text == u.text && t.side == u.side && t.styled == u.styled &&
		t.delay == u.delay && t.delayed == u.delayed && (!t.styled || same.Style(&t.style, &u.style)))
}

// tipAt is the tooltip of the node of a frame's entry: the one it was given,
// or for a truncated text node cut to fit without one, its full text.
type tipAt struct {
	entry int
	tip   *tooltip // nil for the full text of cut text
	// pinned is set for a node with TooltipOpen, and pinOnly for one whose
	// tooltip is the full text of cut text inside a node with NoTooltip,
	// which shows only as it is pinned: the pointer and the focus pass
	// over it.
	pinned, pinOnly bool
	// For cut text, the line it was cut from, and its box in the frame
	// being drawn, which is only good while it is drawn.
	full string
	box  int
}

// tipIn returns the tooltip of entry i in tips, which are in the order of
// their entries, or nil.
func tipIn(tips []tipAt, i int) *tipAt {
	if i < 0 {
		return nil
	}
	if j, ok := slices.BinarySearchFunc(tips, i, func(t tipAt, i int) int { return cmp.Compare(t.entry, i) }); ok {
		return &tips[j]
	}
	return nil
}

func sameTips(a, b []tipAt) bool {
	return slices.EqualFunc(a, b, func(x, y tipAt) bool {
		return x.entry == y.entry && x.full == y.full && x.pinned == y.pinned && x.pinOnly == y.pinOnly &&
			(x.tip == nil) == (y.tip == nil) && (x.tip == nil || x.tip.equal(y.tip))
	})
}

// text returns the text the tooltip shows.
func (t *tipAt) text() string {
	if t.tip != nil {
		return t.tip.text
	}
	return t.full
}

// side returns the side of its node the tooltip asks to go on, or 0.
func (t *tipAt) side() Direction {
	if t.tip != nil {
		return t.tip.side
	}
	return 0
}

// tipTarget returns the entry of chain, innermost first, whose tooltip the
// pointer shows: the innermost with one. A node without a tooltip of its own
// shows that of the nearest node it is inside that has one.
func (f *Frame) tipTarget(chain []int) int {
	for _, i := range chain {
		if t := tipIn(f.tips, i); t != nil && !t.pinOnly {
			return i
		}
	}
	return -1
}

// tipSource is what showed a tooltip.
type tipSource uint8

const (
	tipPointer tipSource = iota + 1 // the pointer resting on the node
	tipFocus                        // a key press moving the focus onto it
)

// tipMsg is the tick of the wait before a tooltip shows, the seq'th armed.
type tipMsg struct {
	root *Root
	seq  int
}

// tipState is what a Root keeps of tooltips: the one waiting to show, the
// one showing, and how it was configured.
type tipState struct {
	on     bool
	cfg    Tooltips // with the defaults filled in
	styled bool     // cfg.Style was given
	seq    int      // counts tooltips armed, for the tick of one to know it

	// The tooltip waiting to show, from src: for the pointer, that of the
	// node with key; for the focus, that of the node the focus goes to, once
	// keyOK, which was not focused when the key was pressed: prev, if
	// prevOK.
	armed  bool
	src    tipSource
	key    nodeKey
	keyOK  bool
	prev   any
	prevOK bool

	// The tooltip showing, of the node with shownKey, from shownSrc, which
	// was on screen at vis, and is shown while it stays there, or for the
	// focus, while the node with ID shownID is focused. For the pointer, it
	// is placed beside the cells of the node on row, where the pointer was.
	// It was drawn last at chip, unfolded over its node if unfolded.
	shown    bool
	shownSrc tipSource
	shownKey nodeKey
	shownID  any
	vis      image.Rectangle
	row      int
	chip     image.Rectangle
	unfolded bool

	// Where the bubble showing went, once placed, where it stays rather
	// than move as what is under it changes.
	place  placement
	busy   []image.Rectangle // scratch for placing tooltips; see busyAreas
	busyOK bool
	avoid  []image.Rectangle

	// sunkActive is set while the sinks of the frame being drawn show the
	// tooltip showing.
	sunkActive bool

	// The tooltips pinned open in the frame drawn last, and those placed
	// in the frame being drawn: what the bubbles of tooltips shown with
	// others avoid.
	pins   []pin
	taken  []image.Rectangle
	others []image.Rectangle // scratch

	// The node under the pointer when a key, click, or wheel event closed a
	// tooltip, whose tooltip does not show until the pointer has left it.
	blocked   nodeKey
	blockedOK bool
	// Until when the next tooltip the pointer moves onto shows at once.
	warmUntil time.Time

	// The focus in the frame drawn last, before a key press moves it.
	lastFocus   any
	lastFocusOK bool

	memo tipChips
}

// pin is a tooltip pinned open, found in the frame by the key of its node:
// where it goes while what it is placed against stays the same, and its
// chip.
type pin struct {
	key      nodeKey
	seen     bool // in the frame being drawn
	unfolded bool
	keep     bool // its bubble keeps the place it has
	place    placement
	memo     tipChips
}

// placement is where a bubble of size went, beside anchor on a screen of
// screen, asked to go on side, if it fit.
type placement struct {
	ok     bool
	anchor image.Rectangle
	size   image.Point
	screen image.Point
	side   Direction
	at     image.Rectangle
}

// keeps reports whether the bubble p placed, if any, stays where it is,
// being of size beside target on screen and asked to go on side as it was,
// and covering none of avoid.
func (p *placement) keeps(target, screen image.Rectangle, size image.Point, side Direction, avoid []image.Rectangle) bool {
	return p.ok && p.anchor == target && p.size == size && p.screen == screen.Size() && p.side == side &&
		!slices.ContainsFunc(avoid, p.at.Overlaps)
}

// tipChips are the chips of a tooltip drawn last, unfolded and as a bubble,
// each kept apart, so cut text too wide to unfold, tried unfolded and then
// shown as a bubble in every frame, draws neither again.
type tipChips struct{ fold, bubble chipMemo }

// chipMemo is the chip drawn last, kept as rendering with Lip Gloss is slow,
// wrapped to limit, or not at all for 0.
type chipMemo struct {
	ok    bool
	text  string
	style lipgloss.Style
	limit int
	out   *canvas.Canvas
}

func (s *tipState) configure(t Tooltips) {
	switch {
	case t.Delay == 0:
		t.Delay = defaultTipDelay
	case t.Delay < 0:
		t.Delay = 0
	}
	if t.Warm == 0 {
		t.Warm = defaultTipWarm
	}
	if t.MaxWidth <= 0 {
		t.MaxWidth = defaultTipWidth
	}
	t.Truncated = cmp.Or(t.Truncated, TipUnfold)
	if t.Explicit != TipSink && t.Explicit != TipHidden {
		t.Explicit = TipBubble
	}
	s.cfg, s.styled = t, !reflect.ValueOf(t.Style).IsZero()
}

// modeOf returns how tooltip t is shown: as its kind is.
func (s *tipState) modeOf(t *tipAt) TipMode {
	if t.tip != nil {
		return s.cfg.Explicit
	}
	return s.cfg.Truncated
}

// floats reports whether tooltip t is drawn over the frame, beside its node
// or unfolded, rather than not at all or in a sink, which is on screen when
// sinkOn.
func (s *tipState) floats(t *tipAt, sinkOn bool) bool {
	m := s.modeOf(t)
	return m != TipHidden && (m != TipSink || !sinkOn)
}

// disarm stops the tooltip waiting to show, if any.
func (s *tipState) disarm() { s.armed, s.keyOK = false, false }

// close closes the tooltip showing, if any, starting the warm window when
// warm, as the pointer left it.
func (s *tipState) close(now time.Time, warm bool) {
	if s.shown && warm && s.shownSrc == tipPointer && s.cfg.Warm > 0 {
		s.warmUntil = now.Add(s.cfg.Warm)
	}
	s.shown, s.chip, s.unfolded, s.place = false, image.Rectangle{}, false, placement{}
}

// delayOf returns how long t waits to show.
func (s *tipState) delayOf(t *tipAt) time.Duration {
	if t.tip != nil && t.tip.delayed {
		return t.tip.delay
	}
	return s.cfg.Delay
}

// tipUpdate follows the messages [Root.Update] gets, besides the mouse's,
// for tooltips: a key press closes the one showing and may show the one of
// the node it focuses, the terminal losing focus closes it, and the tick of
// the one waiting may show it.
func (r *Root) tipUpdate(msg tea.Msg) tea.Cmd {
	s := &r.tip
	if !s.on {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		r.dismissTip(r.r.frame, r.pointerChain(r.r.frame))
		return r.armFocus()
	case tea.BlurMsg:
		s.disarm()
		s.close(r.now(), false)
	case tipMsg:
		if msg.root == r && s.armed && msg.seq == s.seq {
			return r.fireTip()
		}
	}
	return nil
}

// pointerChain returns the entries of f under the pointer, innermost first,
// or nil when where the pointer is is not known.
func (r *Root) pointerChain(f *Frame) []int {
	if f == nil || !r.mouse.atOK {
		return nil
	}
	chain, _ := f.under(r.mouse.at, r.mouse.chain[:0])
	r.mouse.chain = chain
	return chain
}

// tipMouse follows mouse event msg at p in frame f, where the entries under
// the pointer are chain, for tooltips: motion with no button down arms the
// tooltip of the node it moves onto, or keeps the one showing, and any
// click or wheel event closes it. The one of the pointer showing stays open
// while the pointer is over its node, or over its bubble: an unfold covers
// the cells past the cut text, such as the next cell of a table row, which
// the pointer moving onto closes it for, as leaving its node does.
func (r *Root) tipMouse(f *Frame, msg tea.MouseMsg, p image.Point, chain []int) tea.Cmd {
	s := &r.tip
	if !s.on {
		return nil
	}
	switch msg.(type) {
	case tea.MouseClickMsg, tea.MouseWheelMsg:
		r.dismissTip(f, chain)
		return nil
	case tea.MouseMotionMsg:
	default:
		return nil
	}
	if msg.Mouse().Button != tea.MouseNone || !r.canPoint() {
		if s.armed && s.src == tipPointer {
			s.disarm()
		}
		return nil
	}
	i := f.tipTarget(chain)
	var k nodeKey
	if i >= 0 {
		k = f.entries[i].key
	}
	if s.blockedOK && (i < 0 || k != s.blocked) {
		s.blockedOK = false
	}
	if s.shown && s.shownSrc == tipPointer {
		if (i >= 0 && k == s.shownKey) || (!s.unfolded && p.In(s.chip)) {
			return nil
		}
		s.close(r.now(), true)
	}
	switch {
	case i < 0, s.blockedOK:
		if s.armed && s.src == tipPointer {
			s.disarm()
		}
		return nil
	case s.armed && s.src == tipPointer && s.key == k:
		// Motion inside the node does not start the wait again.
		return nil
	case s.shown && s.shownKey == k:
		// Its tooltip shows already, for the focus. The pointer has left
		// the node it was waiting on.
		if s.armed && s.src == tipPointer {
			s.disarm()
		}
		return nil
	}
	return r.armPointer(f, i)
}

// canPoint reports whether the pointer can arm a tooltip: it is somewhere
// known in a focused terminal, with no button down, and nothing being
// dragged or selected.
func (r *Root) canPoint() bool {
	m := &r.mouse
	return !r.blurred && m.atOK && m.press.button == tea.MouseNone && m.drag == nil && !r.sel.dragging
}

// armPointer arms the tooltip of entry i of f, which the pointer moved
// onto, returning the command for the tick of its wait. Inside the warm
// window it shows at once.
func (r *Root) armPointer(f *Frame, i int) tea.Cmd {
	s := &r.tip
	t := tipIn(f.tips, i)
	d := s.delayOf(t)
	if d <= 0 || r.now().Before(s.warmUntil) {
		r.showTip(f, i, tipPointer)
		return nil
	}
	s.seq++
	s.armed, s.src, s.key, s.keyOK = true, tipPointer, f.entries[i].key, true
	return r.tipTick(d)
}

// armFocus arms the tooltip of the node a key press focuses, if the root
// has a focus, returning the command for the tick that finds out which
// node that is, once the model has moved the focus.
func (r *Root) armFocus() tea.Cmd {
	if r.r.focus == nil {
		return nil
	}
	s := &r.tip
	s.seq++
	s.armed, s.src, s.keyOK = true, tipFocus, false
	s.prev, s.prevOK = s.lastFocus, s.lastFocusOK
	return r.tipTick(0)
}

// tipTick returns the command for the tick of the tooltip armed last, after
// d.
func (r *Root) tipTick(d time.Duration) tea.Cmd {
	seq := r.tip.seq
	return r.after(d, func(time.Time) tea.Msg { return tipMsg{root: r, seq: seq} })
}

// fireTip handles the tick of the tooltip armed, checked against the frame
// on screen: the tooltip of the node the pointer rested on shows if the
// pointer is still on it, as the last motion may not have been reported.
// For the focus, the first tick finds the node the key press focused, which
// shows its tooltip after its delay if the focus is still on it then.
func (r *Root) fireTip() tea.Cmd {
	s := &r.tip
	f := r.r.frame
	if f == nil {
		s.disarm()
		return nil
	}
	if s.src == tipPointer {
		i := f.tipTarget(r.pointerChain(f))
		if !r.canPoint() || i < 0 || f.entries[i].key != s.key {
			s.disarm()
			return nil
		}
		r.showTip(f, i, tipPointer)
		return nil
	}

	i := r.focusedTip(f)
	if i < 0 || (s.keyOK && f.entries[i].key != s.key) {
		s.disarm()
		return nil
	}
	if s.keyOK {
		r.showTip(f, i, tipFocus)
		return nil
	}
	// The focus stayed where it was, such as while typing in a field,
	// which does not bring its tooltip back.
	if id := f.entries[i].id; s.prevOK && id == s.prev {
		s.disarm()
		return nil
	}
	if d := s.delayOf(tipIn(f.tips, i)); d > 0 {
		s.seq++
		s.key, s.keyOK = f.entries[i].key, true
		return r.tipTick(d)
	}
	r.showTip(f, i, tipFocus)
	return nil
}

// focusedTip returns the entry of f of the focused node, if it has a
// tooltip and is on screen, or -1.
func (r *Root) focusedTip(f *Frame) int {
	id, ok := r.r.focus.focusedID()
	if !ok {
		return -1
	}
	if i := f.index(id); i >= 0 && !f.entries[i].vis.Empty() {
		if t := tipIn(f.tips, i); t != nil && !t.pinOnly {
			return i
		}
	}
	return -1
}

// showTip shows the tooltip of entry i of f, from src, in place of any
// other.
func (r *Root) showTip(f *Frame, i int, src tipSource) {
	s := &r.tip
	e := &f.entries[i]
	s.disarm()
	s.shown, s.shownSrc, s.shownKey, s.shownID, s.vis = true, src, e.key, e.id, e.vis
	s.row, s.chip, s.unfolded, s.place = r.mouse.at.Y, image.Rectangle{}, false, placement{}
}

// dismissTip closes the tooltip showing and stops the one waiting, for a key,
// click, or wheel event, while the entries of f under the pointer are
// chain. The tooltip of the node under the pointer does not show again
// until the pointer has left it.
func (r *Root) dismissTip(f *Frame, chain []int) {
	s := &r.tip
	s.disarm()
	s.close(r.now(), false)
	s.blockedOK, s.warmUntil = false, time.Time{}
	if f != nil {
		if i := f.tipTarget(chain); i >= 0 {
			s.blocked, s.blockedOK = f.entries[i].key, true
		}
	}
}

// noteFocus notes the focus of the frame being drawn, for a key press to
// tell whether it moved the focus.
func (r *Root) noteFocus() {
	if r.tip.on && r.r.focus != nil {
		r.tip.lastFocus, r.tip.lastFocusOK = r.r.focus.focusedID()
	}
}

// paintTips draws the tooltips over the frame just painted, which is w×h,
// as the renderer's top layer: those pinned open, and then the one of the
// pointer or the focus, if any, over them. It closes that one if its node
// has left the frame, moved, lost its tooltip, or been covered by a modal
// layer, or for the focus, if the focus has left it.
func (r *Root) paintTips(w, h int) {
	s, rr := &r.tip, r.r
	s.busyOK = false
	i, t := r.activeTip()
	if s.shown && (t == nil || rr.entries[i].vis != s.vis) {
		s.close(r.now(), false)
		t = nil
		if s.sunkActive {
			// The sinks were given its text before it was found gone.
			rr.refillSinks()
		}
	}
	screen := image.Rect(0, 0, w, h)
	if screen.Empty() {
		return
	}
	sinkOn := slices.ContainsFunc(rr.sinks, func(s sinkAt) bool { return !s.vis.Empty() })
	s.taken = s.taken[:0]
	r.paintPins(screen, sinkOn)
	if t != nil {
		r.paintActive(t, screen, sinkOn)
	}
}

// activeTip returns the entry of the frame being drawn of the tooltip of the
// pointer or the focus showing, and the tooltip, or -1 and nil when none
// shows: when its node is gone, has no tooltip, is covered by a modal
// layer, or for the focus, is no longer focused. [Root.paintTips] closes it
// too when its node moved.
func (r *Root) activeTip() (int, *tipAt) {
	s, rr := &r.tip, r.r
	if !s.shown {
		return -1, nil
	}
	i := findKey(rr.entries, s.shownKey)
	t := tipIn(rr.tips, i)
	if t == nil || t.pinOnly || covered(rr.entries, i, true) || (s.shownSrc == tipFocus && !r.focusIs(s.shownID)) {
		return -1, nil
	}
	return i, t
}

// paintActive draws tooltip t of the pointer or the focus on screen,
// unless it shows some other way: pinned open, in a sink, which is on
// screen when sinkOn, or by the app. Shown some other way, it has no cells
// of its own to keep it open.
func (r *Root) paintActive(t *tipAt, screen image.Rectangle, sinkOn bool) {
	s, rr := &r.tip, r.r
	s.chip, s.unfolded = image.Rectangle{}, false
	if t.pinned || !s.floats(t, sinkOn) {
		return
	}
	src, rect, ok := r.unfold(t, screen, &s.memo.fold)
	s.unfolded = ok
	if !ok {
		src = r.chip(&s.memo.bubble, t, min(s.cfg.MaxWidth, screen.Dx()))
		size := image.Pt(src.Width(), src.Height())
		if size.X <= 0 || size.Y <= 0 {
			return
		}
		rect = r.placeBubble(t, screen, size)
	}
	s.chip = rect.Intersect(screen)
	rr.topOps = append(rr.topOps, op{kind: opBlit, src: src, rect: rect, clip: screen})
}

// focusIs reports whether the node with ID id is focused.
func (r *Root) focusIs(id any) bool {
	_, focused := r.r.focus.focuses(id)
	return focused
}

// unfold returns the full text of cut text t drawn as a chip over it, in
// place, with the first glyph of the text in the cell where the first
// glyph of what was drawn of it is, and where the chip goes on screen: its
// padding goes before that cell, and it is shifted left to stay on screen.
// The chip is kept in m. It reports false for a tooltip shown as a bubble
// instead: one given with Tooltip, one of text wider than screen, or any
// with [Tooltips] Truncated set to TipBubble.
func (r *Root) unfold(t *tipAt, screen image.Rectangle, m *chipMemo) (*canvas.Canvas, image.Rectangle, bool) {
	if t.tip != nil || r.tip.cfg.Truncated == TipBubble {
		return nil, image.Rectangle{}, false
	}
	// A line too wide to unfold is not drawn to find that out.
	if w, _ := r.r.cache.Size(t.text()); w > screen.Dx() {
		return nil, image.Rectangle{}, false
	}
	st := r.tipStyle(nil)
	src := r.chip(m, t, 0)
	if src.Width()-st.GetHorizontalFrameSize() > screen.Dx() || src.Width() <= 0 {
		return nil, image.Rectangle{}, false
	}
	lead := st.GetMarginLeft() + st.GetBorderLeftSize() + st.GetPaddingLeft()
	top := st.GetMarginTop() + st.GetBorderTopSize() + st.GetPaddingTop()
	// Shifted left just enough to stay on screen, but never so far that the
	// text starts off it, which only its padding may.
	glyph := r.r.firstGlyph(t)
	x := max(min(glyph.X-lead, screen.Max.X-src.Width()), screen.Min.X-lead)
	at := image.Pt(x, glyph.Y-top)
	return src, image.Rectangle{Min: at, Max: at.Add(image.Pt(src.Width(), src.Height()))}, true
}

// anchorOf returns what the bubble of the tooltip showing goes beside: the
// cells of its node on screen, or for the pointer, those on the row the
// pointer was on, so that on a large node it is near the pointer.
func (s *tipState) anchorOf() image.Rectangle {
	target := s.vis
	if s.shownSrc == tipPointer && s.row >= target.Min.Y && s.row < target.Max.Y {
		target.Min.Y, target.Max.Y = s.row, s.row+1
	}
	return target
}

// placeBubble returns where the bubble of tooltip t showing, a chip of size,
// goes on screen: against the side it asks for, as [placeSide] puts it, and
// where it hides the least of the frame, as [Root.fitTip] finds, without
// one or when it fits on neither that side nor the opposite one. Either
// way, it does not cover the cell under the pointer, the cursor's, or the
// tooltips pinned open. It is placed once, and again only once what it
// goes beside, its size, its side, or the screen's size changes, or a
// tooltip pinned open is over it, so it does not move as what is beneath it
// changes.
func (r *Root) placeBubble(t *tipAt, screen image.Rectangle, size image.Point) image.Rectangle {
	s := &r.tip
	target, side := s.anchorOf(), t.side()
	if s.place.keeps(target, screen, size, side, s.taken) {
		return s.place.at
	}
	s.avoid = append(r.pointerCells(s.avoid[:0]), s.taken...)
	var rect image.Rectangle
	ok := false
	if side != 0 {
		rect, ok = placeSide(target, screen, size, side, s.avoid)
	}
	if !ok {
		rect = r.fitTip(target, screen, size)
	}
	s.place = placement{ok: true, anchor: target, size: size, screen: screen.Size(), side: side, at: rect}
	return rect
}

// pointerCells appends to cells those that no tooltip covers when placed:
// the cell under the pointer and the cursor's.
func (r *Root) pointerCells(cells []image.Rectangle) []image.Rectangle {
	if r.mouse.atOK {
		cells = append(cells, image.Rectangle{Min: r.mouse.at, Max: r.mouse.at.Add(image.Pt(1, 1))})
	}
	if c := r.r.cursor; c != nil {
		cells = append(cells, image.Rect(c.X, c.Y, c.X+1, c.Y+1))
	}
	return cells
}

// fitTip returns where the bubble of the tooltip showing, of size, goes
// beside target on screen, as [Root.fit] finds, never over the areas it
// avoids unless it fits nowhere else. Ties go to the place nearest the
// pointer, for a tooltip of the pointer, and to the middle of target
// otherwise.
func (r *Root) fitTip(target, screen image.Rectangle, size image.Point) image.Rectangle {
	s := &r.tip
	near := middle(target)
	if r.mouse.atOK && s.shownSrc == tipPointer {
		near = r.mouse.at
	}
	s.busyOK = false
	rect, _ := r.fit(target, screen, size, near, s.avoid)
	return rect
}

// middle returns the cell in the middle of rect, the upper left of the
// middle ones.
func middle(rect image.Rectangle) image.Point {
	return image.Pt(rect.Min.X+(rect.Dx()-1)/2, rect.Min.Y+(rect.Dy()-1)/2)
}

// fit returns where a bubble of size goes beside target on screen, as
// [anchor.Scene.Fit] finds: where it hides the least of the frame drawn
// beneath it, counting the nodes that take clicks, focus, or the pointer
// double, never over an area of avoid, with ties going to the place nearest
// near. It reports false when it covers one of avoid wherever it goes.
func (r *Root) fit(target, screen image.Rectangle, size, near image.Point, avoid []image.Rectangle) (image.Rectangle, bool) {
	cells := r.r.contentCanvas()
	scene := anchor.Scene{
		Screen: screen, Busy: r.busyAreas(), Avoid: avoid,
		Cell: func(p image.Point) anchor.Cell { return cellKind(cells.CellAt(p.X, p.Y)) },
	}
	return scene.Fit(target, size, near)
}

// busyAreas returns where the nodes of the frame being drawn that take
// clicks, focus, or the pointer are, found once for the frame.
func (r *Root) busyAreas() []image.Rectangle {
	s, rr := &r.tip, r.r
	if s.busyOK {
		return s.busy
	}
	s.busy = s.busy[:0]
	for j := range rr.entries {
		if e := &rr.entries[j]; e.focusable || e.hoverable || len(rr.handlers[j]) > 0 {
			s.busy = append(s.busy, e.vis)
		}
	}
	s.busyOK = true
	return s.busy
}

// placeAmong returns where the bubble of tooltip t, of size, goes beside
// target on screen, shown with other tooltips, which it must not cover:
// none of avoid, the cell under the pointer, or the cursor's. That is on
// the side it asks for, as [placeSide] puts it, or where it hides the least
// of the frame, as [Root.fit] finds, or nowhere, reporting false. It is
// kept where p holds it while target, size, its side, and the screen stay
// the same and it covers none of avoid, even once the pointer or the cursor
// is over it, so it does not move, and p notes where it goes. Whether it
// fits anywhere is found before what is on screen is read, which takes
// drawing the frame again when a tooltip was drawn over it, so one that
// fits nowhere costs little.
func (r *Root) placeAmong(p *placement, t *tipAt, target, screen image.Rectangle, size image.Point, avoid []image.Rectangle) (image.Rectangle, bool) {
	side := t.side()
	if p.keeps(target, screen, size, side, avoid) {
		return p.at, true
	}
	s := &r.tip
	s.others = r.pointerCells(append(s.others[:0], avoid...))
	var rect image.Rectangle
	ok := false
	if side != 0 {
		rect, ok = placeSide(target, screen, size, side, s.others)
	}
	if !ok {
		blank := anchor.Scene{Screen: screen, Avoid: s.others}
		if _, ok = blank.Fit(target, size, middle(target)); ok {
			rect, ok = r.fit(target, screen, size, middle(target), s.others)
		}
	}
	*p = placement{ok: ok, anchor: target, size: size, screen: screen.Size(), side: side, at: rect}
	return rect, ok
}

// paintPins draws the tooltips pinned open in the frame being drawn on
// screen, other than those under a modal layer, and those of a kind shown
// in a sink, which is on screen when sinkOn, or by the app. Cut text is
// unfolded over itself, wherever it is. Bubbles go in the order of the
// tree, each where it covers none of the unfolds, the other bubbles, or
// the nodes of the others; one that fits nowhere is left out of the frame.
// A bubble still where it was placed keeps its place before any other is
// placed, so none moves for another.
func (r *Root) paintPins(screen image.Rectangle, sinkOn bool) {
	s, rr := &r.tip, r.r
	for k := range s.pins {
		s.pins[k].seen = false
	}
	// The unfolds first, and the nodes of all of them, which the bubbles
	// then avoid.
	s.avoid = s.avoid[:0]
	bubbles := false
	for j := range rr.tips {
		t := &rr.tips[j]
		if !r.pinShown(t, sinkOn) {
			continue
		}
		p := s.pinOf(rr.entries[t.entry].key)
		s.avoid = append(s.avoid, rr.entries[t.entry].vis)
		var src *canvas.Canvas
		var rect image.Rectangle
		src, rect, p.unfolded = r.unfold(t, screen, &p.memo.fold)
		if p.unfolded {
			rr.topOps = append(rr.topOps, op{kind: opBlit, src: src, rect: rect, clip: screen})
			s.taken = append(s.taken, rect.Intersect(screen))
		} else {
			bubbles = true
		}
	}
	if bubbles {
		s.avoid = append(s.avoid, s.taken...)
		for j := range rr.tips {
			if t := &rr.tips[j]; r.pinShown(t, sinkOn) {
				p := s.pinOf(rr.entries[t.entry].key)
				src := r.chip(&p.memo.bubble, t, min(s.cfg.MaxWidth, screen.Dx()))
				p.keep = !p.unfolded && p.place.keeps(rr.entries[t.entry].vis, screen, image.Pt(src.Width(), src.Height()), t.side(), s.avoid)
			}
		}
		// Those that keep their place first, then the others.
		for _, kept := range [...]bool{true, false} {
			for j := range rr.tips {
				t := &rr.tips[j]
				if !r.pinShown(t, sinkOn) {
					continue
				}
				p := s.pinOf(rr.entries[t.entry].key)
				if p.unfolded || p.keep != kept {
					continue
				}
				src := r.chip(&p.memo.bubble, t, min(s.cfg.MaxWidth, screen.Dx()))
				size := image.Pt(src.Width(), src.Height())
				if size.X <= 0 || size.Y <= 0 {
					continue
				}
				rect, ok := r.placeAmong(&p.place, t, rr.entries[t.entry].vis, screen, size, s.avoid)
				if !ok {
					continue
				}
				rr.topOps = append(rr.topOps, op{kind: opBlit, src: src, rect: rect, clip: screen})
				s.taken = append(s.taken, rect)
				s.avoid = append(s.avoid, rect)
			}
		}
	}
	s.pins = slices.DeleteFunc(s.pins, func(p pin) bool { return !p.seen })
}

// pinShown reports whether tooltip t of the frame being drawn is pinned
// open and drawn over the frame, as [Root.paintPins] has it. One whose
// node an effect moved off screen is not.
func (r *Root) pinShown(t *tipAt, sinkOn bool) bool {
	return t.pinned && r.tip.floats(t, sinkOn) && !r.r.entries[t.entry].vis.Empty() && !covered(r.r.entries, t.entry, true)
}

// pinOf returns what is kept of the tooltip pinned open of the node with
// key k, noting that it is in the frame being drawn.
func (s *tipState) pinOf(k nodeKey) *pin {
	for i := range s.pins {
		if s.pins[i].key == k {
			s.pins[i].seen = true
			return &s.pins[i]
		}
	}
	s.pins = append(s.pins, pin{key: k, seen: true})
	return &s.pins[len(s.pins)-1]
}

// sinkText returns the text the tooltip sinks of the frame being drawn
// show, as the renderer's sinkText: that of the tooltip of the pointer or
// the focus, or else of the first pinned open, in the order of the tree,
// that is of a kind shown in a sink, or "" for none. It is called once the
// tree is painted, before the frame is drawn, so that a sink shows a
// tooltip in the frame it opens in.
func (r *Root) sinkText() string {
	s, rr := &r.tip, r.r
	s.sunkActive = false
	if _, t := r.activeTip(); t != nil && s.modeOf(t) == TipSink {
		s.sunkActive = true
		return t.text()
	}
	for j := range rr.tips {
		if t := &rr.tips[j]; t.pinned && s.modeOf(t) == TipSink && !covered(rr.entries, t.entry, true) {
			return t.text()
		}
	}
	return ""
}

// cellKind returns what cell c of a frame shows, for placing a tooltip
// over it.
func cellKind(c *uv.Cell) anchor.Cell {
	switch {
	case c == nil || blankCell(c):
		return anchor.Blank
	case c.Width == 1 && (c.Content == " " || c.Content == ""):
		return anchor.Painted
	}
	// A glyph, or the cells a wide one covers past its first.
	return anchor.Glyph
}

// chip returns tooltip t drawn as a chip, wrapped to limit cells, its
// padding and border included, or kept on one line for a limit of 0, kept
// in m.
func (r *Root) chip(m *chipMemo, t *tipAt, limit int) *canvas.Canvas {
	// Compared in place, as a style of its own would be allocated for it.
	r.r.style = r.tipStyle(t.tip)
	text := t.text()
	if m.ok && m.limit == limit && m.text == text && same.Style(&m.style, &r.r.style) {
		return m.out
	}
	st := r.r.style
	out := st
	if tw, _ := canvas.Measure(text, r.r.method); limit > 0 && out.GetWidth() == 0 && tw+out.GetHorizontalFrameSize() > limit {
		out = out.Width(max(limit-out.GetHorizontalMargins(), 0))
	}
	src := fillChip(r.r.cache.Sanitized(out.Render(text)), st, r.r.method)
	*m = chipMemo{ok: true, text: text, style: st, limit: limit, out: src}
	return src
}

// fillChip returns chip src, drawn with style st, with the colors of st
// given to the cells inside its border that the text left without them, as
// a reset inside styled text does, so the chip has one background across
// and it is clear where it ends. The text's own colors are kept. src itself
// is returned when no cell needs them. The cells a wide glyph covers past
// its first are left as they are: they draw nothing, and setting one would
// blank the glyph.
func fillChip(src *canvas.Canvas, st lipgloss.Style, method ansi.Method) *canvas.Canvas {
	fg, bg := st.GetForeground(), st.GetBackground()
	_, noFg := fg.(lipgloss.NoColor)
	_, noBg := bg.(lipgloss.NoColor)
	if noFg && noBg {
		return src
	}
	inner := src.Bounds()
	inner.Min.X += st.GetMarginLeft() + st.GetBorderLeftSize()
	inner.Min.Y += st.GetMarginTop() + st.GetBorderTopSize()
	inner.Max.X -= st.GetMarginRight() + st.GetBorderRightSize()
	inner.Max.Y -= st.GetMarginBottom() + st.GetBorderBottomSize()
	var out *canvas.Canvas
	for y := inner.Min.Y; y < inner.Max.Y; y++ {
		for x := inner.Min.X; x < inner.Max.X; x++ {
			c := src.CellAt(x, y)
			if c == nil || c.Width == 0 || ((noFg || c.Style.Fg != nil) && (noBg || c.Style.Bg != nil)) {
				continue
			}
			if out == nil {
				out = canvas.New(src.Width(), src.Height(), method)
				out.Blit(src, out.Bounds(), out.Bounds())
			}
			cell := *c
			if !noFg && cell.Style.Fg == nil {
				cell.Style.Fg = fg
			}
			if !noBg && cell.Style.Bg == nil {
				cell.Style.Bg = bg
			}
			out.SetCell(x, y, &cell)
		}
	}
	if out == nil {
		return src
	}
	return out
}

// tipStyle returns the style tooltip t is drawn with: its own, the root's,
// or the default, which is worked out from the terminal's colors. A nil t is
// the full text of cut text, which has no style of its own.
func (r *Root) tipStyle(t *tooltip) lipgloss.Style {
	switch {
	case t != nil && t.styled:
		return t.style
	case r.tip.styled:
		return r.tip.cfg.Style
	}
	st := lipgloss.NewStyle().Padding(0, 1)
	if env := &r.r.env; env.Fg != nil && env.Bg != nil {
		st = st.Foreground(env.Fg).Background(r.r.mixer.Mix(env.Bg, env.Fg, tipTint))
	}
	return st
}

// placeSide returns where a chip of size goes against side of target inside
// screen: on the next row above or below it, or a cell away to its left or
// right, shifted along the side to stay inside screen, or on the opposite
// side when it does not fit on side or would cover an area of avoid. It
// reports false when it fits on neither side.
func placeSide(target, screen image.Rectangle, size image.Point, side Direction, avoid []image.Rectangle) (image.Rectangle, bool) {
	var gap image.Point
	if dx, dy := side.Vector(); dy == 0 {
		gap.X = -dx
	}
	var first image.Rectangle
	for i, d := range [...]Direction{side, anchor.Opposite(side)} {
		at := anchor.On(target, screen, size, side, d, gap)
		rect := image.Rectangle{Min: at, Max: at.Add(size)}
		if rect.In(screen) && !rect.Overlaps(target) && !slices.ContainsFunc(avoid, rect.Overlaps) {
			return rect, true
		}
		if i == 0 {
			first = rect
		}
	}
	return first, false
}

// firstGlyph returns the cell where the first glyph of cut text t is drawn
// in the frame being drawn: in its entry's rectangle, which is where its
// box was laid out, moved as the effects it is inside move its cells.
func (r *renderer) firstGlyph(t *tipAt) image.Point {
	b := &r.boxes[t.box]
	n, _ := b.child.(TextNode)
	if n.hover != nil && r.isHovered(b) {
		n.style = *n.hover
	}
	return glyphAt(n.style, r.entries[t.entry].rect)
}

// glyphAt returns the cell where the first glyph of a truncated line cut to
// fit is drawn, for a text node with style at rect: inside the style's
// margin, border, and padding, where its alignment puts the line, which is
// as wide as the room [lineWidth] gives it.
func glyphAt(st lipgloss.Style, rect image.Rectangle) image.Point {
	// As textOf renders it, at the node's size unless the style sets one.
	w, h := st.GetWidth(), st.GetHeight()
	if w == 0 {
		w = max(rect.Dx()-st.GetHorizontalMargins(), 0)
	}
	if h == 0 {
		h = max(rect.Dy()-st.GetVerticalMargins(), 0)
	}
	inW := w - st.GetHorizontalBorderSize() - st.GetHorizontalPadding()
	inH := h - st.GetVerticalBorderSize() - st.GetVerticalPadding()
	room := lineWidth(st, rect.Dx())
	return image.Pt(
		rect.Min.X+st.GetMarginLeft()+st.GetBorderLeftSize()+st.GetPaddingLeft()+alignedAt(st.GetAlignHorizontal(), inW-room),
		rect.Min.Y+st.GetMarginTop()+st.GetBorderTopSize()+st.GetPaddingTop()+alignedAt(st.GetAlignVertical(), inH-1),
	)
}

// alignedAt returns where Lip Gloss puts a line short of the room it is
// aligned in at pos by gap cells, from the start of that room: it aligns
// only to the start, middle, and end.
func alignedAt(pos lipgloss.Position, gap int) int {
	switch {
	case gap <= 0:
		return 0
	case pos == lipgloss.Right: // and Bottom
		return gap
	case pos == lipgloss.Center:
		return gap / 2
	}
	return 0
}

// TooltipSinkNode shows the text of a tooltip in the layout, such as in a
// status line. Build one with [TooltipSink].
type TooltipSinkNode struct {
	meta

	style  lipgloss.Style
	styled bool
}

// TooltipSink returns a node that shows the text of the tooltip of the
// pointer or the focus, or else of the first one pinned open in the order of
// the tree, for the kinds of tooltip whose [Tooltips] mode is [TipSink],
// and is blank while there is none, such as in a status line. While a sink
// has a cell on screen, tooltips of those kinds are not drawn beside their
// nodes; one laid out but clipped or scrolled out of sight is not on
// screen, and they are shown as by default. Every sink on screen shows the
// same text. It shows a tooltip in the same frame the tooltip opens in.
//
// The tooltip of the pointer stays open while the pointer is over its
// node: the sink is not part of it. A sink is a row tall, inside its
// style's frame, and as wide as the space it is given. Text too long for it
// is cut with "…", and shows no tooltip of its own.
func TooltipSink() TooltipSinkNode { return TooltipSinkNode{} }

// Style draws the sink's text with style, whose background fills the sink,
// also while it is blank.
func (n TooltipSinkNode) Style(style lipgloss.Style) TooltipSinkNode {
	n.style, n.styled = style, true
	return n
}

// View implements [Child], drawing the node at its natural size, blank.
func (n TooltipSinkNode) View() string { return Render(0, 0, n) }

// line returns the text node sink n draws to show text.
func (n TooltipSinkNode) line(text string) TextNode {
	return TextNode{s: lineBreaks.Replace(text), style: n.style, styled: n.styled, truncate: true}
}

// sinkAt is a tooltip sink of the frame being drawn: its box, the op that
// draws it, and its cells on screen, once [renderer.placeEntries] has moved
// them as the effects it is inside did.
type sinkAt struct {
	box, op int
	vis     image.Rectangle
}

// measureSink returns the natural size of sink n, box i, within avail: a
// row inside its style's frame, as wide as avail, or as its frame when the
// width is unbounded.
func (e *engine) measureSink(i int, n TooltipSinkNode, avail Size) Size {
	w := avail.W
	if w >= unbounded {
		w = 0
	}
	s := e.rendered(i, n.line(""), w).size
	if w > 0 {
		s.W = w
	}
	s.H = max(s.H, 1)
	return s
}

// paintSink adds the op for tooltip sink box i, visible at vis, which
// [renderer.fillSinks] gives its text once the tree is painted.
func (r *renderer) paintSink(i int, vis image.Rectangle) {
	if !r.parse || vis.Empty() {
		return
	}
	r.sinks = append(r.sinks, sinkAt{box: i, op: len(r.ops), vis: vis})
	r.ops = append(r.ops, op{kind: opBlit, rect: r.boxes[i].rect, clip: vis})
}

// fillSinks gives the ops of the tooltip sinks of the frame being drawn the
// text sinkText returns, cut to fit each, before the frame is compared with
// the last one, so a sink showing other text draws the frame again, and one
// showing the same does not.
func (r *renderer) fillSinks() {
	var text string
	if r.sinkText != nil {
		text = r.sinkText()
	}
	for _, s := range r.sinks {
		b := &r.boxes[s.box]
		n, _ := b.child.(TooltipSinkNode)
		out, _ := r.textAt(s.box, n.line(text), b.rect.Dx(), b.rect.Dy())
		r.ops[s.op].src = r.cache.Sanitized(out)
	}
}

// refillSinks gives the tooltip sinks the text sinkText now returns, once
// the top found that the tooltip they were given closed after the frame
// was drawn, and draws the frame again with it.
func (r *renderer) refillSinks() {
	r.fillSinks()
	r.redraw()
	r.topRedrawn = true
}
