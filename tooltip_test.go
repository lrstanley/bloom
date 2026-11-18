// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// tipRig drives a Root with tooltips on, drawing tree at w×h after every
// message as Bubble Tea does, with a clock and timer the test moves: the
// ticks the Root asks for are sent once the clock passes them.
type tipRig struct {
	t     *testing.T
	r     *Root
	w, h  int
	tree  func() Child
	now   time.Time
	ticks []tipTick
	out   string
}

type tipTick struct {
	at time.Time
	fn func(time.Time) tea.Msg
}

func newTipRig(t *testing.T, w, h int, tree func() Child, opts ...Option) *tipRig {
	t.Helper()
	g := &tipRig{t: t, w: w, h: h, tree: tree, now: time.Unix(1000, 0)}
	opts = append([]Option{WithClock(func() time.Time { return g.now }), WithTooltips(Tooltips{})}, opts...)
	g.r = New(opts...)
	g.r.after = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		g.ticks = append(g.ticks, tipTick{at: g.now.Add(d), fn: fn})
		return nil
	}
	g.draw()
	return g
}

// draw renders the tree.
func (g *tipRig) draw() { g.out = g.r.Render(g.w, g.h, g.tree()) }

// send passes msg to the root, and draws.
func (g *tipRig) send(msg tea.Msg) {
	g.r.Update(msg)
	g.draw()
}

func (g *tipRig) move(x, y int) { g.send(move(x, y)) }

func (g *tipRig) key(s string) { g.send(tea.KeyPressMsg{Code: rune(s[0]), Text: s}) }

// wait moves the clock on by d, sending the ticks that come due on the way,
// in order.
func (g *tipRig) wait(d time.Duration) {
	end := g.now.Add(d)
	for {
		next := -1
		for i, tick := range g.ticks {
			if !tick.at.After(end) && (next < 0 || tick.at.Before(g.ticks[next].at)) {
				next = i
			}
		}
		if next < 0 {
			break
		}
		tick := g.ticks[next]
		g.ticks = slices.Delete(g.ticks, next, next+1)
		if tick.at.After(g.now) {
			g.now = tick.at
		}
		g.send(tick.fn(g.now))
	}
	g.now = end
}

// showing returns the text of the tooltip showing, as Root.Tooltip reports
// it, or "".
func (g *tipRig) showing() string {
	text, _ := g.r.Tooltip()
	return text
}

// want fails unless the tooltip showing is the one of text, or none for "",
// and it is drawn exactly when it shows.
func (g *tipRig) want(text string) {
	g.t.Helper()
	if got := g.showing(); got != text {
		g.t.Fatalf("showing %q, want %q", got, text)
	}
	if text != "" && !strings.Contains(ansi.Strip(g.out), text) {
		g.t.Fatalf("%q shows but is not drawn:\n%s", text, ansi.Strip(g.out))
	}
}

// rows returns the frame drawn last as plain text, by row.
func (g *tipRig) rows() []string { return strings.Split(ansi.Strip(g.out), "\n") }

// at returns where s is drawn, or -1, -1.
func (g *tipRig) at(s string) (x, y int) {
	for y, row := range g.rows() {
		if before, _, ok := strings.Cut(row, s); ok {
			return ansi.StringWidth(before), y
		}
	}
	return -1, -1
}

// twoTips is a blank row, then alpha and beta side by side, each with a
// tooltip, then two more blank rows, at 30×4.
func twoTips() Child {
	return Column(
		Text(" "),
		Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fixed(6, Text("beta").Tooltip("tip b")), Fill(Text(" "))),
		Text(" "),
		Text(" "),
	)
}

// shown has the tooltip of alpha show, as the pointer rests on it.
func (g *tipRig) shown() {
	g.t.Helper()
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
}

func TestTooltipArmsOnMotion(t *testing.T) {
	t.Parallel()

	g := newTipRig(t, 30, 4, twoTips)
	g.wait(time.Second)
	g.want("")
	if len(g.ticks) != 0 {
		t.Fatal("a tooltip was armed without the pointer moving")
	}
	// The pointer rests for 500ms by default.
	g.move(1, 1)
	g.wait(499 * time.Millisecond)
	g.want("")
	g.wait(time.Millisecond)
	g.want("tip a")
	// Above alpha, on the next row, lined up with it, with a cell of
	// padding either side.
	if x, y := g.at(" tip a "); x != 0 || y != 0 {
		t.Fatalf("tooltip drawn at %d,%d, want 0,0:\n%s", x, y, ansi.Strip(g.out))
	}
}

func TestTooltipNotArmedByNewFrame(t *testing.T) {
	t.Parallel()

	moved := false
	tree := func() Child {
		at := 0
		if moved {
			at = 20
		}
		return Column(Text(" "), Row(Fixed(at, nil), Fixed(6, Text("alpha").Tooltip("tip a")), Fill(Text(" "))))
	}
	g := newTipRig(t, 30, 2, tree)
	g.move(21, 1)
	// The node moves under the pointer, which stays still: the next
	// message brings hover up to date with the frame, and arms nothing.
	moved = true
	g.draw()
	g.send(struct{}{})
	g.wait(time.Second)
	g.want("")
	// Moving now arms it.
	g.move(22, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
}

func TestTooltipRestTimer(t *testing.T) {
	t.Parallel()

	// Motion inside the node does not start the wait again.
	g := newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.wait(300 * time.Millisecond)
	g.move(3, 1)
	g.wait(200 * time.Millisecond)
	g.want("tip a")

	// Motion onto another node waits for it from the start.
	g = newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.wait(300 * time.Millisecond)
	g.move(7, 1)
	g.wait(200 * time.Millisecond)
	g.want("")
	g.wait(300 * time.Millisecond)
	g.want("tip b")

	// Moving off every node stops the wait.
	g = newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.move(25, 3)
	g.wait(time.Second)
	g.want("")

	// A tooltip's own delay replaces the root's.
	g = newTipRig(t, 30, 2, func() Child {
		return Column(Text(" "), Text("alpha").Tooltip("quick", TipDelay(100*time.Millisecond)))
	})
	g.move(1, 1)
	g.wait(100 * time.Millisecond)
	g.want("quick")

	// A negative Delay shows it at once.
	g = newTipRig(t, 30, 4, twoTips, WithTooltips(Tooltips{Delay: -1}))
	g.move(1, 1)
	g.want("tip a")
}

// TestTooltipRecheckedWhenDue checks that a tooltip shows only if its node
// is still under the pointer in the frame on screen when the wait ends,
// as the motion that left it may not have been reported, and not that of
// another node that is under it now.
func TestTooltipRecheckedWhenDue(t *testing.T) {
	t.Parallel()

	moved := false
	tree := func() Child {
		alpha := Fixed(6, Text("alpha").ID("alpha").Tooltip("tip a"))
		if moved {
			return Column(Text(" "), Row(Fixed(6, Text("beta").ID("beta").Tooltip("tip b")), Fixed(14, nil), alpha))
		}
		return Column(Text(" "), Row(alpha, Fill(Text(" "))))
	}
	g := newTipRig(t, 30, 2, tree)
	g.move(1, 1)
	moved = true
	g.draw()
	g.wait(defaultTipDelay)
	g.want("")
}

func TestTooltipStaysOverNodeAndItself(t *testing.T) {
	t.Parallel()

	g := newTipRig(t, 30, 4, twoTips)
	g.shown()
	g.move(4, 1)
	g.want("tip a")
	// Onto the tooltip's own cells, above alpha.
	g.move(3, 0)
	g.want("tip a")
	g.move(6, 0)
	g.want("tip a")
	// It never closes on its own.
	g.wait(time.Hour)
	g.want("tip a")
	// Off both.
	g.move(10, 0)
	g.want("")
}

func TestTooltipDismissed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		do   func(g *tipRig, tree *string)
	}{
		{"key press", func(g *tipRig, _ *string) { g.key("x") }},
		{"click", func(g *tipRig, _ *string) { g.send(down(1, 1)) }},
		{"right click", func(g *tipRig, _ *string) {
			g.send(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseRight})
		}},
		{"wheel", func(g *tipRig, _ *string) { g.send(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown}) }},
		{"blur", func(g *tipRig, _ *string) { g.send(tea.BlurMsg{}) }},
		{"pointer leaves", func(g *tipRig, _ *string) { g.move(25, 3) }},
		{"node leaves the frame", func(g *tipRig, tree *string) { *tree = "gone"; g.draw() }},
		{"node moves", func(g *tipRig, tree *string) { *tree = "moved"; g.draw() }},
		{"node resized", func(g *tipRig, tree *string) { *tree = "narrow"; g.draw() }},
		{"tooltip removed", func(g *tipRig, tree *string) { *tree = "untipped"; g.draw() }},
		{"modal layer over it", func(g *tipRig, tree *string) { *tree = "modal"; g.draw() }},
		{"tooltips disabled", func(g *tipRig, _ *string) {
			g.r.EnableTooltips(false)
			g.draw()
			g.r.EnableTooltips(true)
			g.draw()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var state string
			tree := func() Child {
				alpha := Text("alpha").Tooltip("tip a")
				var row Child
				switch state {
				case "gone":
					row = Text(" ")
				case "moved":
					row = Row(Fixed(1, nil), Fixed(6, alpha))
				case "narrow":
					row = Row(Fixed(4, alpha))
				case "untipped":
					row = Row(Fixed(6, Text("alpha")))
				default:
					row = Row(Fixed(6, alpha))
				}
				var layers []LayerNode
				if state == "modal" {
					layers = append(layers, Layer(Text("dialog")).Anchor(End, End).Modal())
				}
				return Overlay(Column(Text(" "), row, Text(" "), Text(" ")), layers...)
			}
			g := newTipRig(t, 30, 4, tree)
			g.shown()
			tt.do(g, &state)
			g.want("")
			// Drawn as with tooltips off: it leaves nothing behind.
			if off := New().Render(30, 4, tree()); g.out != off {
				t.Fatalf("drawn as\n%s\nwant\n%s", ansi.Strip(g.out), ansi.Strip(off))
			}
		})
	}
}

// TestTooltipNotRearmedAfterDismissal checks that a key press, click, or
// wheel event over a node keeps its tooltip away until the pointer has
// left the node.
func TestTooltipNotRearmedAfterDismissal(t *testing.T) {
	t.Parallel()

	dismiss := map[string]tea.Msg{
		"key":   tea.KeyPressMsg{Code: 'x', Text: "x"},
		"click": down(1, 1),
		"wheel": tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown},
	}
	for name, msg := range dismiss {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newTipRig(t, 30, 4, twoTips)
			g.shown()
			g.send(msg)
			g.send(release(1, 1))
			g.move(2, 1)
			g.wait(time.Second)
			g.want("")
			// Onto another node and back.
			g.move(7, 1)
			g.wait(defaultTipDelay)
			g.want("tip b")
			g.move(1, 1)
			g.wait(defaultTipDelay)
			g.want("tip a")
			// Off every node and back.
			g.send(msg)
			g.send(release(1, 1))
			g.move(25, 3)
			g.move(1, 1)
			g.wait(defaultTipDelay)
			g.want("tip a")
		})
	}

	// The node the pointer rests on is kept away while its tooltip waits,
	// too, as when typing with the pointer left over a button.
	g := newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.key("x")
	g.wait(time.Second)
	g.move(2, 1)
	g.wait(time.Second)
	g.want("")
}

func TestTooltipWarmWindow(t *testing.T) {
	t.Parallel()

	// After a tooltip closes as the pointer left, the next shows at once,
	// for 300ms by default.
	g := newTipRig(t, 30, 4, twoTips)
	g.shown()
	g.move(25, 3)
	g.want("")
	g.wait(299 * time.Millisecond)
	g.move(7, 1)
	g.want("tip b")
	// Straight from one node onto the next.
	g.move(1, 1)
	g.want("tip a")

	// Once the window has passed, it waits again.
	g = newTipRig(t, 30, 4, twoTips)
	g.shown()
	g.move(25, 3)
	g.wait(300 * time.Millisecond)
	g.move(7, 1)
	g.want("")
	g.wait(defaultTipDelay)
	g.want("tip b")

	// A key press closing it starts no window.
	g = newTipRig(t, 30, 4, twoTips)
	g.shown()
	g.key("x")
	g.move(7, 1)
	g.want("")
	g.wait(defaultTipDelay)
	g.want("tip b")

	// And ends one already started.
	g = newTipRig(t, 30, 4, twoTips)
	g.shown()
	g.move(7, 1)
	g.want("tip b")
	g.key("x")
	g.move(1, 1)
	g.want("")

	// Nor does a negative Warm.
	g = newTipRig(t, 30, 4, twoTips, WithTooltips(Tooltips{Warm: -1}))
	g.shown()
	g.move(7, 1)
	g.want("")
}

func TestTooltipNotArmedWhileBusy(t *testing.T) {
	t.Parallel()

	// A button held down, as for a drag.
	g := newTipRig(t, 30, 4, twoTips)
	g.send(down(25, 3))
	g.send(dragTo(1, 1))
	g.wait(time.Second)
	g.want("")
	// Released over the node, the pointer has to move again.
	g.send(release(1, 1))
	g.wait(time.Second)
	g.want("")
	g.move(2, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")

	// A button pressed while the tooltip waits.
	g = newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.send(down(1, 1))
	g.wait(time.Second)
	g.want("")

	// Selecting text.
	g = newTipRig(t, 30, 4, func() Child { return Column(twoTips()).Selectable() })
	g.send(down(25, 3))
	g.send(dragTo(1, 1))
	g.wait(time.Second)
	g.want("")

	// A blurred terminal.
	g = newTipRig(t, 30, 4, twoTips)
	g.send(tea.BlurMsg{})
	g.move(1, 1)
	g.wait(time.Second)
	g.want("")
	g.send(tea.FocusMsg{})
	g.move(2, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")

	// The terminal blurred while the tooltip waits.
	g = newTipRig(t, 30, 4, twoTips)
	g.move(1, 1)
	g.send(tea.BlurMsg{})
	g.wait(time.Second)
	g.want("")
}

func TestTooltipNearestAncestor(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Column(
			Text(" "),
			Row(
				Fixed(10, Column(Text("inner").ID("inner").On(on.Click(func() tea.Cmd { return nil })).Tooltip(""), Text("own").Tooltip("own tip")).
					Tooltip("outer tip")),
				Fixed(10, Text("plain")),
			),
			Text(" "),
		)
	}
	g := newTipRig(t, 20, 4, tree)
	// A node with an entry of its own but no tooltip, as empty text gives
	// none, shows the one around it.
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("outer tip")
	g.move(1, 2)
	g.wait(defaultTipDelay)
	g.want("own tip")
	g.move(11, 1)
	g.wait(defaultTipDelay)
	g.want("")
}

func TestTooltipUnderModalLayer(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		base := Column(Text(" "), Text("alpha").Tooltip("tip a"), Text(" "))
		return Overlay(base, Layer(Text("dialog").Tooltip("tip dialog")).Anchor(End, End).Modal())
	}
	g := newTipRig(t, 20, 3, tree)
	g.move(1, 1)
	g.wait(time.Second)
	g.want("")
	g.move(15, 2)
	g.wait(defaultTipDelay)
	g.want("tip dialog")
}

// focusTips is a model with focus over three nodes with tooltips and a field
// without one, which moves the focus with tab.
type focusTips struct {
	f Focus[string]
	g *tipRig
}

func newFocusTips(t *testing.T) *focusTips {
	t.Helper()
	m := &focusTips{}
	tree := func() Child {
		return Column(
			Text(" "),
			Row(
				Fixed(6, Text("one").ID("one").Focusable().Tooltip("tip one")),
				Fixed(6, Text("two").ID("two").Focusable().Tooltip("tip two")),
				Fixed(6, Text("three").Tooltip("tip three")),
				Fill(Text("field").ID("field").Focusable()),
			),
			Text(" "),
		)
	}
	m.g = newTipRig(t, 40, 3, tree, WithFocus(&m.f))
	return m
}

// press sends key s to the root and then has the model handle it, as a
// model that passes every message to the root first does, and draws.
func (m *focusTips) press(s string) {
	key := tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	if s == "tab" {
		key = tea.KeyPressMsg{Code: tea.KeyTab}
	}
	m.g.r.Update(key)
	m.f.Update(key, m.g.r.Frame())
	m.g.draw()
}

func TestTooltipFocus(t *testing.T) {
	t.Parallel()

	m := newFocusTips(t)
	g := m.g
	g.move(2, 0)
	m.press("tab")
	g.wait(defaultTipDelay - time.Millisecond)
	g.want("")
	g.wait(time.Millisecond)
	g.want("tip one")
	// Placed against the node, below it, as above it is the cell under the
	// pointer.
	if x, y := g.at(" tip one "); x != 0 || y != 2 {
		t.Fatalf("focus tooltip at %d,%d, want 0,2:\n%s", x, y, ansi.Strip(g.out))
	}
	// Motion over nothing leaves it.
	g.move(30, 2)
	g.want("tip one")

	// Typing in the focused node closes it, and never brings it back.
	m.press("x")
	g.want("")
	g.wait(time.Second)
	g.want("")
	m.press("y")
	g.wait(time.Second)
	g.want("")

	// Moving on shows the next one.
	m.press("tab")
	g.wait(defaultTipDelay)
	g.want("tip two")
	// A node without a tooltip shows none.
	m.press("tab")
	g.wait(time.Second)
	g.want("")

	// A model that moves the focus before passing the key to the root.
	m.f.Next(g.r.Frame())
	g.r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	g.draw()
	g.wait(defaultTipDelay)
	g.want("tip one")

	// The focus moved away without a key closes it.
	m.f.Set("field")
	g.draw()
	g.want("")
	// And moved on without a key while it waits shows none.
	m.press("tab")
	g.wait(time.Millisecond)
	m.f.Set("two")
	g.draw()
	g.wait(time.Second)
	g.want("")
}

func TestTooltipFocusNotFromClicks(t *testing.T) {
	t.Parallel()

	m := newFocusTips(t)
	g := m.g
	g.send(down(7, 1))
	g.send(release(7, 1))
	if !m.f.Is("two") {
		t.Fatal("the click did not focus two")
	}
	g.wait(time.Second)
	g.want("")
	m.press("x")
	g.wait(time.Second)
	g.want("")
}

func TestTooltipPointerAndFocusReplaceEachOther(t *testing.T) {
	t.Parallel()

	m := newFocusTips(t)
	g := m.g
	m.press("tab")
	g.wait(defaultTipDelay)
	g.want("tip one")
	// The pointer resting on another node replaces it, after a wait that
	// starts again once it has been to the focused node and back.
	g.move(13, 1)
	g.wait(defaultTipDelay - time.Millisecond)
	g.move(1, 1)
	g.move(13, 1)
	g.wait(time.Millisecond)
	g.want("tip one")
	g.wait(defaultTipDelay)
	g.want("tip three")
	// And a key press moving the focus replaces that.
	m.press("tab")
	g.want("")
	g.wait(defaultTipDelay)
	g.want("tip two")
	// The pointer moving inside its own node does not close the focus's.
	g.move(14, 1)
	g.want("tip two")
	// A key press while the pointer waits stops the wait.
	g.move(1, 1)
	m.press("x")
	g.wait(time.Second)
	g.want("")
}

func TestPlaceSide(t *testing.T) {
	t.Parallel()

	screen := image.Rect(0, 0, 20, 5)
	tests := []struct {
		name   string
		target image.Rectangle
		size   image.Point
		side   Direction
		at     image.Point
		fits   bool
	}{
		{"top", image.Rect(5, 2, 9, 3), image.Pt(6, 1), Top, image.Pt(5, 1), true},
		{"bottom", image.Rect(5, 2, 9, 3), image.Pt(6, 1), Bottom, image.Pt(5, 3), true},
		{"left", image.Rect(5, 2, 9, 3), image.Pt(3, 1), Left, image.Pt(1, 2), true},
		{"right", image.Rect(5, 2, 9, 3), image.Pt(3, 1), Right, image.Pt(10, 2), true},
		{"top flips at the top edge", image.Rect(5, 0, 9, 1), image.Pt(6, 1), Top, image.Pt(5, 1), true},
		{"bottom flips at the bottom edge", image.Rect(5, 4, 9, 5), image.Pt(6, 1), Bottom, image.Pt(5, 3), true},
		{"left flips at the left edge", image.Rect(0, 2, 4, 3), image.Pt(3, 1), Left, image.Pt(5, 2), true},
		{"right flips at the right edge", image.Rect(16, 2, 20, 3), image.Pt(3, 1), Right, image.Pt(12, 2), true},
		{"shifted left of the right edge", image.Rect(17, 2, 20, 3), image.Pt(6, 1), Top, image.Pt(14, 1), true},
		{"fits neither above nor below", image.Rect(0, 1, 20, 4), image.Pt(6, 2), Top, image.Pt(0, 0), false},
		{"fits neither left nor right", image.Rect(2, 2, 18, 3), image.Pt(4, 1), Left, image.Pt(0, 2), false},
		{"wider than the screen", image.Rect(2, 2, 18, 3), image.Pt(25, 1), Top, image.Pt(-5, 1), false},
		{"over the cursor", image.Rect(5, 2, 9, 3), image.Pt(6, 1), Top, image.Pt(5, 3), true},
		{"over the cursor either way", image.Rect(5, 2, 9, 3), image.Pt(3, 1), Left, image.Pt(1, 2), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var avoid []image.Rectangle
			switch tt.name {
			case "over the cursor":
				avoid = []image.Rectangle{image.Rect(6, 1, 7, 2)}
			case "over the cursor either way":
				avoid = []image.Rectangle{image.Rect(1, 2, 2, 3), image.Rect(11, 2, 12, 3)}
			}
			got, fits := placeSide(tt.target, screen, tt.size, tt.side, avoid)
			if got.Min != tt.at || got.Size() != tt.size || fits != tt.fits {
				t.Fatalf("placed at %v (fits %t), want at %v with size %v (fits %t)", got, fits, tt.at, tt.size, tt.fits)
			}
		})
	}
}

func TestTooltipSides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		side Direction
		row  int // the row the target is on
		x, y int
	}{
		{"top", Top, 2, 8, 1},
		{"bottom", Bottom, 2, 8, 3},
		{"left", Left, 2, 2, 2},
		{"right", Right, 2, 13, 2},
		{"top flipped below", Top, 0, 8, 1},
		{"bottom flipped above", Bottom, 4, 8, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Over the text above and below it, where it would not go without
			// TipSide.
			tree := func() Child {
				x := Text(strings.Repeat("x", 20))
				rows := []Child{x, x, x, x, x}
				rows[tt.row] = Row(Fixed(8, nil), Fixed(4, Text("node").Tooltip("tip", TipSide(tt.side))), Fill(Text(" ")))
				return Column(rows...)
			}
			g := newTipRig(t, 20, 5, tree)
			g.move(9, tt.row)
			g.wait(defaultTipDelay)
			g.want("tip")
			if x, y := g.at(" tip "); x != tt.x || y != tt.y {
				t.Fatalf("drawn at %d,%d, want %d,%d:\n%s", x, y, tt.x, tt.y, ansi.Strip(g.out))
			}
		})
	}
}

// TestTooltipOnPointerRow checks that the tooltip of a node taller than a
// row goes beside the node's cells on the row the pointer rests on.
func TestTooltipOnPointerRow(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Row(Fixed(6, Column(Text("a"), Text("b"), Text("c"), Text("d")).Tooltip("tip", TipSide(Right))), Fill(Text(" ")))
	}
	g := newTipRig(t, 20, 4, tree)
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if x, y := g.at(" tip "); x != 7 || y != 2 {
		t.Fatalf("drawn at %d,%d, want 7,2", x, y)
	}
}

func TestTooltipChip(t *testing.T) {
	t.Parallel()

	tip := func(text string, opts ...TipOption) func() Child {
		return func() Child { return Column(Text(" "), Text(" "), Text("alpha").Tooltip(text, opts...)) }
	}

	// A tooltip's style replaces the root's, which replaces the default.
	g := newTipRig(t, 30, 3, tip("tip a", TipStyle(lipgloss.NewStyle())))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if g.rows()[1] != "tip a"+strings.Repeat(" ", 25) {
		t.Fatalf("unstyled chip drawn as %q", g.rows()[1])
	}
	g = newTipRig(t, 30, 3, tip("tip a"), WithTooltips(Tooltips{Style: lipgloss.NewStyle().Padding(0, 2)}))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if !strings.HasPrefix(g.rows()[1], "  tip a  ") {
		t.Fatalf("root's style drawn as %q", g.rows()[1])
	}

	// The default has a background, between the terminal's colors.
	g = newTipRig(t, 30, 3, tip("tip a"))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if !strings.Contains(g.out, "48;2;") {
		t.Fatalf("default chip has no background: %q", g.out)
	}

	// Wrapped at MaxWidth, and at the screen's width.
	g = newTipRig(t, 30, 3, tip("one two three"), WithTooltips(Tooltips{MaxWidth: 11}))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if rows := g.rows(); !strings.HasPrefix(rows[0], " one two    ") || !strings.HasPrefix(rows[1], " three      ") {
		t.Fatalf("wrapped at 11 as:\n%s", strings.Join(rows, "\n"))
	}
	g = newTipRig(t, 11, 3, tip("one two three"))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if rows := g.rows(); rows[0] != " one two   " || rows[1] != " three     " {
		t.Fatalf("wrapped at the screen's width as:\n%s", strings.Join(rows, "\n"))
	}
	// And at 60 cells by default.
	g = newTipRig(t, 80, 3, tip(strings.Repeat("abcd ", 13)+"abcd"))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if rows := g.rows(); strings.Count(rows[0], "abcd") != 11 || strings.Count(rows[1], "abcd") != 3 {
		t.Fatalf("wrapped by default as:\n%s", strings.Join(rows, "\n"))
	}

	// Escape sequences meant for the terminal are dropped.
	g = newTipRig(t, 30, 3, tip("ab\x1b_Ga=d,d=A\x1b\\cd"))
	g.move(1, 2)
	g.wait(defaultTipDelay)
	if strings.Contains(g.out, "\x1b_") || !strings.Contains(ansi.Strip(g.out), "abcd") {
		t.Fatalf("tooltip text not sanitized: %q", g.out)
	}
}

func TestTooltipClickGoesThrough(t *testing.T) {
	t.Parallel()

	var l log
	tree := func() Child {
		return Column(Text("under the tooltip").On(on.Click(l.add("under"))), Text("alpha").Tooltip("tip a"))
	}
	g := newTipRig(t, 30, 2, tree)
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
	g.send(down(2, 0))
	g.send(release(2, 0))
	expect(t, &l, "under")
	g.want("")
}

func TestTooltipUnchangedFrame(t *testing.T) {
	t.Parallel()

	// While none shows, drawn as with tooltips off.
	g := newTipRig(t, 30, 4, twoTips)
	if off := New().Render(30, 4, twoTips()); g.out != off {
		t.Fatalf("drawn with tooltips on as\n%q\nwant\n%q", g.out, off)
	}
	first, frame := g.out, g.r.Frame()
	g.draw()
	if unsafe.StringData(first) != unsafe.StringData(g.out) || g.r.Frame() != frame {
		t.Fatal("an unchanged frame with tooltips on was drawn again")
	}
	g.shown()
	first, frame = g.out, g.r.Frame()
	g.draw()
	if unsafe.StringData(first) != unsafe.StringData(g.out) || g.r.Frame() != frame {
		t.Fatal("an unchanged frame with a tooltip was drawn again")
	}
}

func TestTooltipAndCursor(t *testing.T) {
	t.Parallel()

	// A bubble goes where it does not cover the cursor's cell: below, as
	// the blank row above has the cursor.
	tree := Column(Text(" ").Cursor(tea.NewCursor(2, 0)), Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fill(Text(" "))), Text(" "))
	g := newTipRig(t, 30, 3, func() Child { return tree })
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
	if x, y := g.at(" tip a "); x != 0 || y != 2 {
		t.Fatalf("tooltip at %d,%d, want 0,2:\n%s", x, y, ansi.Strip(g.out))
	}
	if v := g.r.View(30, 3, tree); v.Cursor == nil {
		t.Fatal("the cursor is hidden")
	}

	// Text unfolded over the cursor hides it while it shows.
	tree = Column(Row(Fixed(6, Text("> abcdefgh").Truncate().Cursor(tea.NewCursor(2, 0))), Fill(Text(" "))), Text(" "))
	g = newTipRig(t, 30, 2, func() Child { return tree })
	g.move(1, 0)
	g.wait(defaultTipDelay)
	g.want("> abcdefgh")
	if v := g.r.View(30, 2, tree); v.Cursor != nil {
		t.Fatalf("cursor at %d,%d shown under the tooltip", v.Cursor.X, v.Cursor.Y)
	}
	g.key("x")
	if v := g.r.View(30, 2, tree); v.Cursor == nil {
		t.Fatal("no cursor after the tooltip closed")
	}
}

func TestTooltipOff(t *testing.T) {
	t.Parallel()

	r := New()
	r.after = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd {
		t.Fatal("a tooltip was armed with tooltips off")
		return nil
	}
	r.Render(30, 4, twoTips())
	r.Update(move(1, 1))
	r.Render(30, 4, twoTips())
	if len(r.Frame().entries) != 0 {
		t.Fatal("nodes with tooltips have entries with tooltips off")
	}

	// Turned on later, with the defaults.
	r.EnableTooltips(true)
	r.Render(30, 4, twoTips())
	if len(r.Frame().tips) != 2 {
		t.Fatalf("%d tooltips in the frame, want 2", len(r.Frame().tips))
	}
}

func TestTruncatedRecorded(t *testing.T) {
	t.Parallel()

	framed := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder())
	bang := lipgloss.NewStyle().Transform(func(s string) string { return s + "!!" })
	tests := []struct {
		name string
		node TextNode
		w    int
		full string // the line it was cut from, or "" when it fits
	}{
		{"cut", Text("hello world").Truncate(), 5, "hello world"},
		{"a cell short", Text("hello world").Truncate(), 10, "hello world"},
		{"fits exactly", Text("hello world").Truncate(), 11, ""},
		{"fits", Text("hello world").Truncate(), 20, ""},
		{"inside padding and a border", Text("abcdef").Truncate().Style(framed), 10, ""},
		{"cut by padding and a border", Text("abcdef").Truncate().Style(framed), 9, "abcdef"},
		{"cut once transformed", Text("abc").Truncate().Style(bang), 4, "abc!!"},
		{"fits once transformed", Text("abc").Truncate().Style(bang), 5, ""},
		{"line breaks", Text("one\ntwo three").Truncate(), 5, "one two three"},
		{"wrapped", Text("hello world"), 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := func(n TextNode) Child { return Column(Row(Fixed(tt.w, n), Fill(Text(" ")))) }
			cut := tt.full != ""

			// With an ID, the frame reports it, and so does Solve.
			r := New(WithTooltips(Tooltips{}))
			r.Render(30, 3, tree(tt.node.ID("t")))
			f := r.Frame()
			if got := f.Truncated("t"); got != cut {
				t.Fatalf("Truncated reports %t, want %t", got, cut)
			}
			if got := Solve(30, 3, tree(tt.node.ID("t"))).Truncated("t"); got != cut {
				t.Fatalf("Truncated reports %t for Solve, want %t", got, cut)
			}

			// Without one, it has an entry only for its tooltip, which is
			// its full text.
			r.Render(30, 3, tree(tt.node))
			f = r.Frame()
			switch {
			case !cut && (len(f.entries) != 0 || len(f.tips) != 0):
				t.Fatalf("text that fits has %d entries and %d tooltips", len(f.entries), len(f.tips))
			case cut && (len(f.entries) != 1 || len(f.tips) != 1):
				t.Fatalf("text cut has %d entries and %d tooltips, want 1 of each", len(f.entries), len(f.tips))
			case cut && (f.tips[0].tip != nil || f.tips[0].full != tt.full || !f.entries[0].cut):
				t.Fatalf("text cut has the tooltip %q, want %q", f.tips[0].text(), tt.full)
			}
		})
	}

	if New().Frame().Truncated("none") || Solve(10, 1, Text("abc")).Truncated("none") {
		t.Fatal("Truncated reports a node not in the frame")
	}
}

// TestTruncatedNotRecorded checks that with tooltips off, or with Truncated
// set to TipOff, text cut to fit is given no entry and no tooltip, and
// needs nothing of the mouse, though a node with an ID still reports it.
func TestTruncatedNotRecorded(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Row(Fixed(5, Text("hello world").Truncate()), Fixed(5, Text("hello world").Truncate().ID("id")))
	}
	tests := map[string][]Option{
		"off":    nil,
		"TipOff": {WithTooltips(Tooltips{Truncated: TipOff})},
	}
	for name, opts := range tests {
		r := New(opts...)
		v := r.View(30, 1, tree())
		f := r.Frame()
		if len(f.entries) != 1 || len(f.tips) != 0 {
			t.Errorf("%s: %d entries and %d tooltips, want the ID's entry alone", name, len(f.entries), len(f.tips))
		}
		if !f.Truncated("id") {
			t.Errorf("%s: Truncated does not report the node with an ID", name)
		}
		if v.MouseMode != tea.MouseModeNone {
			t.Errorf("%s: mouse mode %v, want none", name, v.MouseMode)
		}
	}

	// Turned on, and off again, at runtime.
	r := New()
	r.EnableTooltips(true)
	if v := r.View(30, 1, tree()); len(r.Frame().tips) != 2 || v.MouseMode != tea.MouseModeAllMotion {
		t.Fatalf("turned on: %d tooltips, mouse mode %v", len(r.Frame().tips), v.MouseMode)
	}
	r.SetTooltips(Tooltips{Truncated: TipOff})
	r.Render(30, 1, tree())
	if len(r.Frame().tips) != 0 {
		t.Fatalf("set to TipOff: %d tooltips", len(r.Frame().tips))
	}
}

func TestNoTooltip(t *testing.T) {
	t.Parallel()

	long := func(s string) TextNode { return Text(s + " and then some more").Truncate() }
	tree := func() Child {
		return Column(
			Text(" "),
			Row(Fixed(6, long("own").NoTooltip().ID("own")), Fill(Text(" "))),
			Row(
				Fixed(6, Column(long("inner"), long("explicit").Tooltip("explicit tip"), Row(long("deep")))).NoTooltip(),
				Fixed(6, long("outside")),
				Fill(Text(" ")),
			),
			Text(" "),
			Text(" "),
		)
	}
	g := newTipRig(t, 30, 7, tree)
	f := g.r.Frame()
	var texts []string
	for i := range f.tips {
		texts = append(texts, f.tips[i].text())
	}
	if want := []string{"explicit tip", "outside and then some more"}; !slices.Equal(texts, want) {
		t.Fatalf("tooltips %q, want %q", texts, want)
	}
	if !f.Truncated("own") {
		t.Fatal("NoTooltip keeps Truncated from reporting the node")
	}

	// None shows on the node, or inside the slot, but the tooltip given
	// inside it does.
	for _, p := range []image.Point{{1, 1}, {1, 2}, {1, 4}} {
		g.move(p.X, p.Y)
		g.wait(time.Second)
		g.want("")
	}
	g.move(1, 3)
	g.wait(defaultTipDelay)
	g.want("explicit tip")
	g.move(7, 2)
	g.wait(defaultTipDelay)
	g.want("outside and then some more")
}

func TestTruncatedExplicitWins(t *testing.T) {
	t.Parallel()

	g := newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(Fixed(5, Text("hello world").Truncate().Tooltip("greeting")), Fill(Text(" "))), Text(" "))
	})
	if len(g.r.Frame().tips) != 1 {
		t.Fatalf("%d tooltips, want 1", len(g.r.Frame().tips))
	}
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("greeting")
	// A bubble, not the full text unfolded.
	if strings.Contains(ansi.Strip(g.out), "hello world") || !strings.HasPrefix(g.rows()[1], "hell…") {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
}

// TestUnfold checks that the full text of text cut to fit is drawn over it
// with its first glyph in the cell where the first glyph of the cut text
// was.
func TestUnfold(t *testing.T) {
	t.Parallel()

	const full = "abcdefghijkl"
	cell := func(st lipgloss.Style) TextNode { return Text(full).Truncate().Style(st) }
	plain := lipgloss.NewStyle()
	tests := []struct {
		name string
		row  Child
		h    int
		x, y int // where the cut text's first glyph is
	}{
		{"plain", Row(Fixed(3, nil), Fixed(6, Text(full).Truncate()), Fill(Text(" "))), 2, 3, 0},
		{"padded", Row(Fixed(3, nil), Fixed(8, cell(plain.Padding(0, 2))), Fill(Text(" "))), 2, 5, 0},
		{"bordered", Row(Fixed(3, nil), Fixed(9, cell(plain.Border(lipgloss.NormalBorder()).Padding(0, 1))), Fill(Text(" "))), 3, 5, 1},
		{"right aligned", Row(Fixed(3, nil), Fixed(8, cell(plain.MaxWidth(6).Align(lipgloss.Right))), Fill(Text(" "))), 2, 5, 0},
		{"at column zero", Row(Fixed(6, Text(full).Truncate()), Fill(Text(" "))), 2, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := newTipRig(t, 30, tt.h, func() Child { return tt.row })
			if x, y := g.at("abc"); x != tt.x || y != tt.y {
				t.Fatalf("the cut text starts at %d,%d, want %d,%d:\n%s", x, y, tt.x, tt.y, ansi.Strip(g.out))
			}
			g.move(tt.x, tt.y)
			g.wait(defaultTipDelay)
			g.want(full)
			if x, y := g.at(full); x != tt.x || y != tt.y {
				t.Fatalf("unfolded at %d,%d, want %d,%d:\n%s", x, y, tt.x, tt.y, ansi.Strip(g.out))
			}
		})
	}
}

func TestUnfoldAtScreenEdges(t *testing.T) {
	t.Parallel()

	// At the right edge, shifted left just enough to stay on screen.
	const full = "abcdefghijkl"
	g := newTipRig(t, 20, 2, func() Child {
		return Column(Row(Fill(Text(" ")), Fixed(6, Text(full).Truncate())), Text(" "))
	})
	g.move(15, 0)
	g.wait(defaultTipDelay)
	g.want(full)
	if row := g.rows()[0]; !strings.HasSuffix(row, " "+full+" ") || g.r.tip.chip != image.Rect(6, 0, 20, 1) {
		t.Fatalf("drawn at %v as %q", g.r.tip.chip, row)
	}

	// Wider than the screen, a bubble instead, wrapped to fit, beside it.
	const wide = "abcdefghijklmnop"
	g = newTipRig(t, 10, 4, func() Child {
		return Column(Text(" "), Row(Fixed(5, Text(wide).Truncate()), Fill(Text(" "))), Text(" "), Text(" "))
	})
	g.move(1, 1)
	g.wait(defaultTipDelay)
	if g.showing() != wide {
		t.Fatalf("showing %q", g.showing())
	}
	rows := g.rows()
	if !strings.HasPrefix(rows[1], "abcd…") || g.r.tip.chip.Overlaps(image.Rect(0, 1, 5, 2)) || g.r.tip.chip.Dx() != 10 {
		t.Fatalf("bubble at %v:\n%s", g.r.tip.chip, strings.Join(rows, "\n"))
	}
}

func TestTruncatedAsBubble(t *testing.T) {
	t.Parallel()

	const full = "abcdefghijkl"
	g := newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(Fixed(6, Text(full).Truncate()), Fill(Text(" "))), Text(" "))
	}, WithTooltips(Tooltips{Truncated: TipBubble}))
	g.move(2, 1)
	g.wait(defaultTipDelay)
	g.want(full)
	if x, y := g.at(full); y == 1 || !strings.HasPrefix(g.rows()[1], "abcde…") {
		t.Fatalf("drawn at %d,%d, over the text:\n%s", x, y, ansi.Strip(g.out))
	}
}

// TestUnfoldInlineStyles checks that text with styles of its own keeps its
// colors when unfolded, on the chip's background all the way across, past a
// reset in the text.
func TestUnfoldInlineStyles(t *testing.T) {
	t.Parallel()

	const full = "\x1b[31mred\x1b[0m and plain text"
	g := newTipRig(t, 30, 2, func() Child {
		return Column(Row(Fixed(3, nil), Fixed(6, Text(full).Truncate()), Fill(Text(" "))), Text(" "))
	})
	g.move(4, 0)
	g.wait(defaultTipDelay)
	if g.showing() != full || !strings.HasPrefix(g.rows()[0], "   red and plain text ") {
		t.Fatalf("showing %q, drawn as:\n%s", g.showing(), ansi.Strip(g.out))
	}
	// The chip is the padding at 2, the text from 3, and the padding after.
	drawn := canvas.Parse(g.out, ansi.GraphemeWidth)
	if c := drawn.CellAt(2, 0); c.Style.Fg == nil {
		t.Fatal("no foreground on the padding")
	}
	bg := drawn.CellAt(2, 0).Style.Bg
	for x := 2; x < 3+len("red and plain text")+1; x++ {
		if c := drawn.CellAt(x, 0); c.Style.Bg == nil || !sameRGBA(c.Style.Bg, bg) {
			t.Fatalf("cell %d of the chip has background %v, want %v", x, c.Style.Bg, bg)
		}
	}
	if c := drawn.CellAt(3, 0); c.Content != "r" || !sameRGBA(c.Style.Fg, ansi.Red) {
		t.Fatalf("the text's own color is lost: %q in %v", c.Content, c.Style.Fg)
	}
}

func sameRGBA(a, b interface{ RGBA() (r, g, b, a uint32) }) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// TestGlyphAt checks where the first glyph of a line cut to fit is found to
// be against where Lip Gloss draws it.
func TestGlyphAt(t *testing.T) {
	t.Parallel()

	plain := lipgloss.NewStyle()
	tests := []struct {
		name  string
		style lipgloss.Style
	}{
		{"plain", plain},
		{"padding", plain.Padding(1, 2)},
		{"border and padding", plain.Border(lipgloss.RoundedBorder()).Padding(0, 1)},
		{"margin", plain.Margin(1, 1).Padding(0, 1)},
		{"centered", plain.MaxWidth(9).Align(lipgloss.Center)},
		{"right", plain.MaxWidth(9).Align(lipgloss.Right)},
		{"width", plain.Width(8).Align(lipgloss.Right)},
		{"middle row", plain.AlignVertical(lipgloss.Center)},
		{"bottom row", plain.AlignVertical(lipgloss.Bottom).Padding(0, 1)},
	}
	rect := image.Rect(2, 1, 14, 6)
	for _, tt := range tests {
		n := Text("Qabcdefghijklmnopqrstuvwxyz").Truncate().Style(tt.style)
		out, full := textOf(n, rect.Dx(), rect.Dy(), ansi.GraphemeWidth)
		if full == "" {
			t.Fatalf("%s: not cut", tt.name)
		}
		want := image.Pt(-1, -1)
		for y, row := range strings.Split(ansi.Strip(out), "\n") {
			if before, _, ok := strings.Cut(row, "Q"); ok {
				want = rect.Min.Add(image.Pt(ansi.StringWidth(before), y))
				break
			}
		}
		if got := glyphAt(tt.style, rect); got != want {
			t.Errorf("%s: found at %v, drawn at %v:\n%s", tt.name, got, want, ansi.Strip(out))
		}
	}
}

// panes is a list of entries in a pane on the left, the second with a
// tooltip, and a pane on the right that is blank but for what is in right,
// row by row, at 40×5.
func panes(right []string) Child {
	rows := make([]Child, 5)
	for y := range rows {
		s := " "
		if y < len(right) {
			s = right[y]
		}
		rows[y] = Text(s).Truncate()
	}
	left := Column(
		Text("alpha entry one").Truncate(),
		Text("beta").Tooltip("about beta"),
		Text("gamma entry three").Truncate(),
		Text("delta entry four").Truncate(),
		Text("epsilon entry five").Truncate(),
	)
	return Row(Fixed(16, left), Fill(Column(rows...)))
}

func TestTooltipPlacedOnBlank(t *testing.T) {
	t.Parallel()

	var right []string
	g := newTipRig(t, 40, 5, func() Child { return panes(right) })
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("about beta")
	// Not above or below, over the entries, but right of the pane's row,
	// with a cell between, on the blank pane.
	if x, y := g.at(" about beta "); x != 17 || y != 1 {
		t.Fatalf("drawn at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}

	// It stays where it is as what is beneath it changes.
	right = []string{" ", "something new is written here"}
	g.draw()
	g.want("about beta")
	if x, y := g.at(" about beta "); x != 17 || y != 1 {
		t.Fatalf("moved to %d,%d as what is beneath changed:\n%s", x, y, ansi.Strip(g.out))
	}

	// Once shown again, it goes where it now hides least.
	g.move(30, 4)
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("about beta")
	if x, y := g.at(" about beta "); x == 17 && y == 1 {
		t.Fatalf("drawn over the text:\n%s", ansi.Strip(g.out))
	}
}

func TestTooltipPlacedAgainOnResize(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		line := Text(strings.Repeat("x", 40)).Truncate()
		return Column(line, Row(Fixed(6, Text("target").Tooltip("tip")), Fill(Text(" "))), line)
	}
	g := newTipRig(t, 30, 3, tree)
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip")
	if x, y := g.at(" tip "); x != 7 || y != 1 {
		t.Fatalf("drawn at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	// Too narrow for it right of the node: above it, over what is there.
	g.w = 10
	g.draw()
	g.want("tip")
	if x, y := g.at(" tip "); x != 0 || y != 0 {
		t.Fatalf("drawn at %d,%d once resized:\n%s", x, y, ansi.Strip(g.out))
	}
}

// TestTooltipPlacedOverContent checks that a bubble is placed over what the
// frame draws, not over the tooltip drawn before it, which the canvas still
// holds when the frame is unchanged.
func TestTooltipPlacedOverContent(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Column(
			Text(" "),
			Row(Fixed(4, Text("aaaa").Tooltip("tip a")), Fixed(4, Text("bbbb").Tooltip("tip b")), Fill(Text(" "))),
			Text(strings.Repeat("x", 30)),
		)
	}
	g := newTipRig(t, 30, 3, tree)
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
	if x, y := g.at(" tip a "); x != 0 || y != 0 {
		t.Fatalf("drawn at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	// Straight onto b, at once, as the pointer left a: above b is blank once
	// the tooltip of a is gone, and the middle is nearest the pointer.
	g.move(5, 1)
	g.want("tip b")
	if x, y := g.at(" tip b "); x != 3 || y != 0 {
		t.Fatalf("drawn at %d,%d, avoiding the tooltip it replaced:\n%s", x, y, ansi.Strip(g.out))
	}
	if strings.Contains(ansi.Strip(g.out), "tip a") {
		t.Fatalf("the tooltip of a is still drawn:\n%s", ansi.Strip(g.out))
	}
}

// TestUnfoldOnFocus checks that the full text of a focusable node cut to
// fit shows when a key press moves the focus onto it.
func TestUnfoldOnFocus(t *testing.T) {
	t.Parallel()

	var f Focus[string]
	const full = "abcdefghijkl"
	tree := func() Child {
		return Column(Text(" "), Row(Fixed(6, Text(full).Truncate().ID("cut").Focusable()), Fill(Text(" "))))
	}
	g := newTipRig(t, 30, 2, tree, WithFocus(&f))
	key := tea.KeyPressMsg{Code: tea.KeyTab}
	g.r.Update(key)
	f.Update(key, g.r.Frame())
	g.draw()
	g.wait(defaultTipDelay)
	g.want(full)
	if x, y := g.at(full); x != 0 || y != 1 {
		t.Fatalf("unfolded at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
}

// count returns how many times s is drawn in the frame drawn last.
func (g *tipRig) count(s string) int { return strings.Count(ansi.Strip(g.out), s) }

// chips returns where the tooltips of the frame drawn last are drawn, in the
// order they are.
func (g *tipRig) chips() []image.Rectangle {
	var rects []image.Rectangle
	for _, o := range g.r.r.topOps {
		rects = append(rects, o.rect)
	}
	return rects
}

// mouseMode returns the mouse mode of the view of the tree, drawn again.
func (g *tipRig) mouseMode() tea.MouseMode { return g.r.View(g.w, g.h, g.tree()).MouseMode }

// noOverlaps fails if any two of rects overlap, or one of them covers one of
// targets that is not the one at the same index.
func noOverlaps(t *testing.T, rects, targets []image.Rectangle) {
	t.Helper()
	for i, a := range rects {
		for j, b := range rects {
			if i < j && a.Overlaps(b) {
				t.Fatalf("tooltips %v and %v overlap", a, b)
			}
		}
		for j, b := range targets {
			if i != j && a.Overlaps(b) {
				t.Fatalf("tooltip %v covers the node at %v", a, b)
			}
		}
	}
}

// key returns the message of a key press of s, or of tab.
func keyMsg(s string) tea.KeyPressMsg {
	if s == "tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestTooltipPinned(t *testing.T) {
	t.Parallel()

	pinned := true
	tree := func() Child {
		return Column(Text(" "), Row(Fixed(6, Text("alpha").Tooltip("tip a").TooltipOpen(pinned)), Fill(Text(" "))), Text(" "))
	}
	g := newTipRig(t, 30, 3, tree)
	// At once, with nothing armed, and without the mouse.
	if g.count("tip a") != 1 || len(g.ticks) != 0 {
		t.Fatalf("pinned tooltip not drawn at once (%d ticks):\n%s", len(g.ticks), ansi.Strip(g.out))
	}
	if x, y := g.at(" tip a "); x != 0 || y != 0 {
		t.Fatalf("drawn at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	if m := g.mouseMode(); m != tea.MouseModeNone {
		t.Fatalf("a frame with only a pinned tooltip asks for mouse mode %v", m)
	}

	// Nothing the user does closes it.
	for _, msg := range []tea.Msg{
		keyMsg("x"),
		down(1, 1),
		release(1, 1),
		tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown},
		move(20, 2),
		tea.BlurMsg{},
		tea.FocusMsg{},
	} {
		g.send(msg)
		g.wait(time.Second)
		if g.count("tip a") != 1 {
			t.Fatalf("pinned tooltip gone after %T:\n%s", msg, ansi.Strip(g.out))
		}
	}
	// The pointer resting on its node shows it once.
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
	if g.count("tip a") != 1 {
		t.Fatalf("pinned tooltip drawn %d times with the pointer on it:\n%s", g.count("tip a"), ansi.Strip(g.out))
	}
	g.move(20, 2)
	if g.count("tip a") != 1 {
		t.Fatalf("pinned tooltip gone as the pointer left:\n%s", ansi.Strip(g.out))
	}

	pinned = false
	g.draw()
	if g.count("tip a") != 0 {
		t.Fatalf("still drawn once unpinned:\n%s", ansi.Strip(g.out))
	}
	if m := g.mouseMode(); m != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode %v once unpinned, want all motion", m)
	}
}

func TestTooltipPinnedFollowsNode(t *testing.T) {
	t.Parallel()

	at := 0
	tree := func() Child {
		return Column(Text(" "), Row(Fixed(at, nil), Fixed(6, Text("alpha").Tooltip("tip a").TooltipOpen(true)), Fill(Text(" "))), Text(" "))
	}
	g := newTipRig(t, 30, 3, tree)
	if x, y := g.at(" tip a "); x != 0 || y != 0 {
		t.Fatalf("drawn at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	// Lined up with the end of the node, its middle nearest the node's.
	at = 10
	g.draw()
	if x, y := g.at(" tip a "); x != 9 || y != 0 {
		t.Fatalf("drawn at %d,%d once its node moved:\n%s", x, y, ansi.Strip(g.out))
	}
	// It stays where it is as what is beneath it changes.
	g.tree = func() Child {
		return Column(Text(strings.Repeat("x", 30)), Row(Fixed(at, nil), Fixed(6, Text("alpha").Tooltip("tip a").TooltipOpen(true)), Fill(Text(" "))), Text(" "))
	}
	g.draw()
	if x, y := g.at(" tip a "); x != 9 || y != 0 {
		t.Fatalf("moved to %d,%d as what is beneath changed:\n%s", x, y, ansi.Strip(g.out))
	}
}

func TestTooltipPinnedTogether(t *testing.T) {
	t.Parallel()

	// Three nodes side by side, each pinned: none covers another, or
	// another's node.
	node := func(s string) Child { return Fixed(6, Text(s).Tooltip("tip "+s).TooltipOpen(true)) }
	g := newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(node("a"), node("b"), node("c"), Fill(Text(" "))), Text(" "))
	})
	for _, s := range []string{"tip a", "tip b", "tip c"} {
		if g.count(s) != 1 {
			t.Fatalf("%q not drawn:\n%s", s, ansi.Strip(g.out))
		}
	}
	targets := []image.Rectangle{image.Rect(0, 1, 6, 2), image.Rect(6, 1, 12, 2), image.Rect(12, 1, 18, 2)}
	noOverlaps(t, g.chips(), targets)

	// On one row with one below, the one in the middle fits nowhere, and is
	// left out, until those either side are unpinned: the one that was
	// placed keeps its place.
	first, last := true, true
	g = newTipRig(t, 18, 2, func() Child {
		return Column(Row(
			Fixed(6, Text("a").Tooltip("tip a").TooltipOpen(first)), node("b"), Fixed(6, Text("c").Tooltip("tip c").TooltipOpen(last)),
		), Text(" "))
	})
	if g.count("tip a") != 1 || g.count("tip b") != 0 || g.count("tip c") != 1 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	noOverlaps(t, g.chips(), nil)
	again := g.out
	g.draw()
	if unsafe.StringData(again) != unsafe.StringData(g.out) {
		t.Fatal("an unchanged frame with a tooltip left out was drawn again")
	}
	first = false
	g.draw()
	if g.count("tip a") != 0 || g.count("tip b") != 0 || g.count("tip c") != 1 {
		t.Fatalf("drawn once a is unpinned as:\n%s", ansi.Strip(g.out))
	}
	last = false
	g.draw()
	if g.count("tip b") != 1 || g.count("tip c") != 0 {
		t.Fatalf("drawn once c is unpinned too as:\n%s", ansi.Strip(g.out))
	}
}

// TestTooltipPinnedSide checks that tooltips pinned open together go on
// the side each asks for when that is free, and where the scorer puts them
// otherwise.
func TestTooltipPinnedSide(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Column(
			Text(" "),
			Row(
				Fixed(6, Text("right").Tooltip("tip r", TipSide(Right)).TooltipOpen(true)),
				Fixed(8, nil),
				Fixed(6, Text("left").Tooltip("tip l", TipSide(Left)).TooltipOpen(true)),
				Fixed(6, Text("next").Tooltip("tip n").TooltipOpen(true)),
				Fill(Text(" ")),
			),
			Text(" "),
		)
	}
	g := newTipRig(t, 40, 3, tree)
	if x, y := g.at(" tip r "); x != 7 || y != 1 {
		t.Fatalf("tip r at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	// Left of left is the right one's tooltip, and right of it the next
	// node, so it goes where the scorer puts it: above, lined up with its
	// end, whose middle is nearest the middle of the node.
	if x, y := g.at(" tip l "); x != 13 || y != 0 {
		t.Fatalf("tip l at %d,%d:\n%s", x, y, ansi.Strip(g.out))
	}
	noOverlaps(t, g.chips(), nil)
}

// TestTooltipSideChanged checks that a tooltip left open goes to the side
// it is then asked to, pinned open or of the pointer.
func TestTooltipSideChanged(t *testing.T) {
	t.Parallel()

	for _, pinned := range []bool{true, false} {
		side := Top
		g := newTipRig(t, 20, 3, func() Child {
			return Column(Text(" "), Row(Fixed(6, Text("alpha").Tooltip("tip a", TipSide(side)).TooltipOpen(pinned)), Fill(Text(" "))), Text(" "))
		})
		if !pinned {
			g.shown()
		}
		for _, want := range []int{0, 2} {
			g.draw()
			if _, y := g.at(" tip a "); y != want {
				t.Fatalf("pinned %t: tooltip on row %d, want %d:\n%s", pinned, y, want, ansi.Strip(g.out))
			}
			side = Bottom
		}
	}
}

// TestTooltipMovesForPinned checks that the bubble of the pointer's tooltip
// moves once one pinned open is placed where it is.
func TestTooltipMovesForPinned(t *testing.T) {
	t.Parallel()

	pinned := false
	g := newTipRig(t, 30, 3, func() Child {
		return Column(
			Text(" "),
			Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fixed(6, Text("beta").Tooltip("tip b").TooltipOpen(pinned)), Fill(Text(" "))),
			Text(" "),
		)
	})
	g.move(5, 1)
	g.wait(defaultTipDelay)
	g.want("tip a")
	pinned = true
	g.draw()
	if g.count("tip a") != 1 || g.count("tip b") != 1 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	noOverlaps(t, g.chips(), nil)
}

// TestTooltipMovedOffScreen checks that an effect moving a node off screen
// takes what it shows with it: a tooltip pinned open is not drawn, and a
// sink no longer takes the tooltips, which float again.
func TestTooltipMovedOffScreen(t *testing.T) {
	t.Parallel()

	away := func(c Child) Child { return Transition(0, c, fx.Slide{From: fx.Right}) }
	g := newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), away(Text("alpha").Tooltip("tip a").TooltipOpen(true)), Text(" "))
	})
	if g.count("tip a") != 0 {
		t.Fatalf("a tooltip is pinned open on a node off screen:\n%s", ansi.Strip(g.out))
	}

	g = newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Text("alpha").Tooltip("tip a"), away(TooltipSink()))
	}, WithTooltips(Tooltips{Explicit: TipSink}))
	g.shown()
	if g.count("tip a") != 1 {
		t.Fatalf("the tooltip went to a sink off screen:\n%s", ansi.Strip(g.out))
	}
}

func TestTooltipPinnedCut(t *testing.T) {
	t.Parallel()

	const full = "abcdefghijkl"
	tests := []struct {
		name  string
		node  Child
		opts  []Option
		shown string // the text pinned open, or ""
	}{
		{"cut", Text(full).Truncate().TooltipOpen(true), nil, full},
		{"fits", Text("short").Truncate().TooltipOpen(true), nil, ""},
		{"no tooltip", Text("plain").TooltipOpen(true), nil, ""},
		{"inside NoTooltip", Column(Text(full).Truncate().TooltipOpen(true)).NoTooltip(), nil, full},
		{"with NoTooltip", Text(full).Truncate().NoTooltip().TooltipOpen(true), nil, full},
		{"truncated off", Text(full).Truncate().TooltipOpen(true), []Option{WithTooltips(Tooltips{Truncated: TipOff})}, ""},
		{"truncated hidden", Text(full).Truncate().TooltipOpen(true), []Option{WithTooltips(Tooltips{Truncated: TipHidden})}, ""},
		{"explicit hidden", Text("alpha").Tooltip("tip a").TooltipOpen(true), []Option{WithTooltips(Tooltips{Explicit: TipHidden})}, ""},
		{"explicit on cut text", Text(full).Truncate().Tooltip("tip a").TooltipOpen(true), nil, "tip a"},
		{"unpinned", Text(full).Truncate().TooltipOpen(false), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := func() Child {
				return Column(Text(" "), Row(Fixed(3, nil), Fixed(6, tt.node), Fill(Text(" "))), Text(" "))
			}
			g := newTipRig(t, 30, 3, tree, tt.opts...)
			if tt.shown == "" {
				if len(g.chips()) != 0 {
					t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
				}
				return
			}
			if g.count(tt.shown) != 1 {
				t.Fatalf("%q not drawn:\n%s", tt.shown, ansi.Strip(g.out))
			}
			// Cut text unfolds over itself.
			if x, y := g.at(tt.shown); tt.shown == full && (x != 3 || y != 1) {
				t.Fatalf("unfolded at %d,%d:\n%s", x, y, ansi.Strip(g.out))
			}
		})
	}

	// Inside NoTooltip, it shows only as it is pinned: the pointer passes
	// over it, and the frame needs nothing of the mouse.
	g := newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(Fixed(6, Column(Text(full).Truncate().TooltipOpen(true)).NoTooltip()), Fill(Text(" "))))
	})
	g.move(1, 1)
	if len(g.ticks) != 0 || g.r.tip.shown {
		t.Fatal("the pointer armed a tooltip shown only as it is pinned")
	}
	if m := g.mouseMode(); m != tea.MouseModeNone {
		t.Fatalf("mouse mode %v, want none", m)
	}
	if _, ok := g.r.Tooltip(); ok {
		t.Fatal("Root.Tooltip reports a pinned tooltip")
	}
}

// sinkTree is a row with a node with a tooltip and text cut to fit, and a
// status line with a sink, at 40×4.
func sinkTree() Child {
	return Column(
		Text(" "),
		Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fixed(6, Text("abcdefghijkl").Truncate()), Fill(Text(" "))),
		Text(" "),
		Row(Fixed(8, Text("status:")), Fill(TooltipSink())),
	)
}

// status returns what the sink of sinkTree shows.
func (g *tipRig) status() string {
	g.t.Helper()
	return strings.TrimSpace(strings.TrimPrefix(g.rows()[3], "status:"))
}

func TestTooltipSink(t *testing.T) {
	t.Parallel()

	g := newTipRig(t, 40, 4, sinkTree, WithTooltips(Tooltips{Explicit: TipSink}))
	if g.status() != "" {
		t.Fatalf("sink shows %q with no tooltip", g.status())
	}
	g.move(1, 1)
	g.wait(defaultTipDelay)
	// In the frame it opens in, in the sink alone.
	if g.status() != "tip a" || g.count("tip a") != 1 || len(g.chips()) != 0 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	// It stays while the pointer is over its node, and has no cells of its
	// own, where its bubble would be.
	g.move(4, 1)
	if g.status() != "tip a" {
		t.Fatalf("sink shows %q", g.status())
	}
	g.move(1, 0)
	if g.status() != "" || g.showing() != "" {
		t.Fatalf("sink shows %q once the pointer left the node", g.status())
	}

	// A kind not shown in the sink floats, and leaves the sink alone.
	g.move(30, 2)
	g.wait(time.Second)
	g.move(7, 1)
	g.wait(defaultTipDelay)
	if g.status() != "" || g.count("abcdefghijkl") != 1 {
		t.Fatalf("cut text not unfolded:\n%s", ansi.Strip(g.out))
	}

	// The truncated kind in the sink too.
	g = newTipRig(t, 40, 4, sinkTree, WithTooltips(Tooltips{Truncated: TipSink}))
	g.move(7, 1)
	g.wait(defaultTipDelay)
	if g.status() != "abcdefghijkl" || len(g.chips()) != 0 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	g.move(1, 1)
	g.wait(defaultTipDelay)
	if g.status() != "" || g.count("tip a") != 1 || len(g.chips()) != 1 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
}

// TestTooltipSinkClosesInFrame checks that a sink stops showing a tooltip in
// the frame its node moves in, which closes it once the frame is drawn.
func TestTooltipSinkClosesInFrame(t *testing.T) {
	t.Parallel()

	moved := false
	tree := func() Child {
		at := 0
		if moved {
			at = 1
		}
		return Column(
			Text(" "),
			Row(Fixed(at, nil), Fixed(6, Text("alpha").Tooltip("tip a")), Fill(Text(" "))),
			Text(" "),
			Row(Fixed(8, Text("status:")), Fill(TooltipSink())),
		)
	}
	g := newTipRig(t, 40, 4, tree, WithTooltips(Tooltips{Explicit: TipSink}))
	g.move(2, 1)
	g.wait(defaultTipDelay)
	if g.status() != "tip a" {
		t.Fatalf("sink shows %q", g.status())
	}
	moved = true
	g.draw()
	if g.showing() != "" || g.status() != "" {
		t.Fatalf("sink shows %q once its node moved", g.status())
	}
}

func TestTooltipSinkPinned(t *testing.T) {
	t.Parallel()

	sel := 1
	tree := func() Child {
		return Column(
			Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fill(Text(" "))),
			Row(Fixed(6, Text("first row").Truncate().TooltipOpen(sel == 1)), Fill(Text(" "))),
			Row(Fixed(6, Text("second row").Truncate().TooltipOpen(sel >= 2)), Fill(Text(" "))),
			Row(Fixed(6, Text("third row").Truncate().TooltipOpen(sel >= 2)), Fill(Text(" "))),
			Row(Fixed(8, Text("status:")), Fill(TooltipSink())),
		)
	}
	g := newTipRig(t, 30, 5, tree, WithTooltips(Tooltips{Explicit: TipSink, Truncated: TipSink}))
	status := func() string { return strings.TrimSpace(strings.TrimPrefix(g.rows()[4], "status:")) }
	if status() != "first row" || len(g.chips()) != 0 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	// The first pinned, in the order of the tree, and nothing floats.
	sel = 2
	g.draw()
	if status() != "second row" || len(g.chips()) != 0 || g.count("third row") != 0 {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	// The pointer's comes first, and the pinned one is back once it closes.
	g.move(1, 0)
	g.wait(defaultTipDelay)
	if status() != "tip a" {
		t.Fatalf("sink shows %q", status())
	}
	g.key("x")
	if status() != "second row" {
		t.Fatalf("sink shows %q", status())
	}
	// With no sink on screen, pinned tooltips float.
	g.h = 4
	g.draw()
	if g.count("second row") != 1 || g.count("third row") != 1 {
		t.Fatalf("drawn without the sink as:\n%s", ansi.Strip(g.out))
	}
}

func TestTooltipSinkFallback(t *testing.T) {
	t.Parallel()

	tests := map[string]func(sink Child) Child{
		"absent": func(Child) Child { return Text(" ") },
		"clipped": func(sink Child) Child {
			return Row(Fixed(8, Text("status:")), Fixed(0, sink), Fill(Text(" ")))
		},
		"scrolled away": func(sink Child) Child {
			return Scroll(Column(sink, Text("status")), 1)
		},
		"below the screen": func(sink Child) Child { return Column(Text(" "), sink) },
	}
	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := func() Child {
				return Column(
					Text(" "),
					Row(Fixed(6, Text("alpha").Tooltip("tip a")), Fixed(6, Text("abcdefghijkl").Truncate()), Fill(Text(" "))),
					Text(" "),
					Fixed(1, status(TooltipSink())),
				)
			}
			g := newTipRig(t, 40, 4, tree, WithTooltips(Tooltips{Explicit: TipSink, Truncated: TipSink}))
			if len(g.r.r.sinks) != 0 {
				t.Fatalf("%d sinks on screen", len(g.r.r.sinks))
			}
			// A bubble, and the cut text unfolded.
			g.move(1, 1)
			g.wait(defaultTipDelay)
			if g.count("tip a") != 1 || len(g.chips()) != 1 {
				t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
			}
			g.move(7, 1)
			g.wait(defaultTipDelay)
			if x, y := g.at("abcdefghijkl"); x != 6 || y != 1 {
				t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
			}
		})
	}
}

func TestTooltipSinkNode(t *testing.T) {
	t.Parallel()

	// A row as wide as it is given, inside its style's frame, blank while
	// there is no tooltip.
	padded := lipgloss.NewStyle().Padding(0, 2).Background(lipgloss.Color("4"))
	if got := Solve(0, 0, TooltipSink()).entries; len(got) != 0 {
		t.Fatalf("a sink has %d entries", len(got))
	}
	if out := Render(12, 0, TooltipSink()); out != strings.Repeat(" ", 12) {
		t.Fatalf("blank sink drawn as %q", out)
	}
	if out := Render(0, 0, TooltipSink().Style(padded)); ansi.StringWidth(out) != 4 || strings.Contains(out, "\n") {
		t.Fatalf("sink at its natural size drawn as %q", out)
	}
	if out := Render(12, 0, TooltipSink().Style(padded)); !strings.Contains(out, "48;5;4m") && !strings.Contains(out, "44m") {
		t.Fatalf("blank sink drawn without its background: %q", out)
	}

	// Text too long for it is cut, and is not a tooltip of its own.
	tree := func() Child {
		return Column(
			Row(Fixed(6, Text("alpha").Tooltip("a tooltip much too long for the sink")), Fill(Text(" "))),
			Row(Fixed(8, Text("status:")), Fixed(12, TooltipSink()), Fill(Text(" "))),
			Row(Fixed(8, Text("again:")), Fill(TooltipSink())),
		)
	}
	g := newTipRig(t, 40, 3, tree, WithTooltips(Tooltips{Explicit: TipSink}))
	g.move(1, 0)
	g.wait(defaultTipDelay)
	rows := g.rows()
	if rows[1] != "status: a tooltip m…"+strings.Repeat(" ", 20) || rows[2] != "again:  a tooltip much too long for the…" {
		t.Fatalf("drawn as:\n%s", ansi.Strip(g.out))
	}
	if f := g.r.Frame(); len(f.tips) != 1 || f.Truncated("status") {
		t.Fatalf("%d tooltips in the frame, want 1", len(f.tips))
	}
	if len(g.chips()) != 0 {
		t.Fatalf("a chip floats beside the sinks:\n%s", ansi.Strip(g.out))
	}
}

// TestTooltipSinkFrames checks that a sink showing what it did is not drawn
// again, and one showing something else is.
func TestTooltipSinkFrames(t *testing.T) {
	t.Parallel()

	g := newTipRig(t, 40, 4, sinkTree, WithTooltips(Tooltips{Explicit: TipSink}))
	first := g.out
	g.draw()
	if unsafe.StringData(first) != unsafe.StringData(g.out) {
		t.Fatal("an unchanged frame with a blank sink was drawn again")
	}
	g.move(1, 1)
	g.wait(defaultTipDelay)
	if g.out == first {
		t.Fatal("the sink was not drawn again for the tooltip")
	}
	first = g.out
	g.draw()
	if unsafe.StringData(first) != unsafe.StringData(g.out) {
		t.Fatal("an unchanged frame with a sink showing a tooltip was drawn again")
	}
	g.key("x")
	if g.status() != "" {
		t.Fatalf("sink shows %q once closed", g.status())
	}
}

func TestRootTooltip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []Option
		at   image.Point
		want string
	}{
		{"bubble", nil, image.Pt(1, 1), "tip a"},
		{"unfold", nil, image.Pt(7, 1), "abcdefghijkl"},
		{"bubble of cut text", []Option{WithTooltips(Tooltips{Truncated: TipBubble})}, image.Pt(7, 1), "abcdefghijkl"},
		{"sink", []Option{WithTooltips(Tooltips{Explicit: TipSink})}, image.Pt(1, 1), "tip a"},
		{"hidden", []Option{WithTooltips(Tooltips{Explicit: TipHidden})}, image.Pt(1, 1), "tip a"},
		{"hidden cut text", []Option{WithTooltips(Tooltips{Truncated: TipHidden})}, image.Pt(7, 1), "abcdefghijkl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := newTipRig(t, 40, 4, sinkTree, tt.opts...)
			blank := g.out
			if _, ok := g.r.Tooltip(); ok {
				t.Fatal("Tooltip reports one before any shows")
			}
			g.move(tt.at.X, tt.at.Y)
			g.wait(defaultTipDelay)
			if text, ok := g.r.Tooltip(); !ok || text != tt.want {
				t.Fatalf("Tooltip reports %q, %t, want %q", text, ok, tt.want)
			}
			// Hidden, the root draws nothing for it.
			if strings.HasPrefix(tt.name, "hidden") && (g.out != blank || len(g.chips()) != 0) {
				t.Fatalf("a hidden tooltip is drawn:\n%s", ansi.Strip(g.out))
			}
			g.key("x")
			if _, ok := g.r.Tooltip(); ok {
				t.Fatal("Tooltip reports one once it closed")
			}
		})
	}

	// The text of the one showing changes with its node's.
	text := "tip a"
	g := newTipRig(t, 30, 3, func() Child { return Column(Text(" "), Row(Fixed(6, Text("alpha").Tooltip(text)), Fill(Text(" ")))) })
	g.move(1, 1)
	g.wait(defaultTipDelay)
	text = "tip a, again"
	g.draw()
	g.want("tip a, again")

	// Not one that is pinned.
	g = newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(Fixed(6, Text("alpha").Tooltip("tip a").TooltipOpen(true)), Fixed(6, Text("beta").Tooltip("tip b"))))
	})
	if _, ok := g.r.Tooltip(); ok {
		t.Fatal("Tooltip reports a pinned tooltip")
	}
	if _, ok := New().Tooltip(); ok {
		t.Fatal("Tooltip reports one with tooltips off")
	}
}

// TestTooltipWideGlyphs checks that wide glyphs, emoji, and combining
// characters are drawn whole in a chip of every kind, in the default style,
// which has colors: in a bubble, wrapped or not, unfolded, and in a sink.
func TestTooltipWideGlyphs(t *testing.T) {
	t.Parallel()

	texts := []string{"abc界d", "界界界界", "a🙂b🙂c", "cafe\u0301 ole\u0301"}
	for _, s := range texts {
		t.Run("bubble "+s, func(t *testing.T) {
			t.Parallel()

			g := newTipRig(t, 30, 4, func() Child {
				return Column(Text(" "), Text(" "), Row(Fixed(3, nil), Fixed(5, Text("x").Tooltip(s).TooltipOpen(true)), Fill(Text(" "))), Text(" "))
			})
			if g.count(s) != 1 {
				t.Fatalf("%q not drawn whole:\n%s", s, ansi.Strip(g.out))
			}
		})
	}

	// Wrapped to four cells inside the padding, a line for each word.
	for _, word := range []string{"界界", "🙂🙂"} {
		t.Run("wrapped "+word, func(t *testing.T) {
			t.Parallel()

			s := strings.Repeat(word+" ", 2) + word
			g := newTipRig(t, 30, 6, func() Child {
				return Column(Text(" "), Text(" "), Text(" "), Text(" "), Row(Fixed(3, nil), Fixed(5, Text("x").Tooltip(s).TooltipOpen(true)), Fill(Text(" "))), Text(" "))
			}, WithTooltips(Tooltips{MaxWidth: 6}))
			if g.count(word) != 3 {
				t.Fatalf("%q not drawn whole:\n%s", s, ansi.Strip(g.out))
			}
		})
	}

	// Unfolded, the first glyph of the full text is in the cell of the first
	// glyph of the cut text, wherever the cut lands among wide glyphs.
	right := lipgloss.NewStyle().MaxWidth(6).Align(lipgloss.Right)
	tests := []struct {
		name  string
		full  string
		width int
		style lipgloss.Style
		x     int // where the cut text's first glyph is
	}{
		{"cut before a wide glyph", "abc界defgh", 5, lipgloss.NewStyle(), 3},
		{"cut after a wide glyph", "ab界cdefgh", 5, lipgloss.NewStyle(), 3},
		{"cut inside a wide glyph", "abcd界efgh", 5, lipgloss.NewStyle(), 3},
		{"wide glyphs only", "界界界界界界", 5, lipgloss.NewStyle(), 3},
		{"cut inside the first glyph", "界abcdef", 1, lipgloss.NewStyle(), 3},
		{"emoji", "🙂🙂🙂🙂🙂", 5, lipgloss.NewStyle(), 3},
		{"combining", "cafe\u0301 au lait", 5, lipgloss.NewStyle(), 3},
		{"right aligned", "ab界cdefgh", 8, right, 5},
	}
	for _, tt := range tests {
		t.Run("unfold "+tt.name, func(t *testing.T) {
			t.Parallel()

			tree := func() Child {
				return Column(Text(" "), Row(Fixed(3, nil), Fixed(tt.width, Text(tt.full).Truncate().Style(tt.style).TooltipOpen(true)), Fill(Text(" "))), Text(" "))
			}
			row := strings.Split(ansi.Strip(New().Render(30, 3, tree())), "\n")[1]
			if x := ansi.StringWidth(row) - ansi.StringWidth(strings.TrimLeft(row, " ")); x != tt.x {
				t.Fatalf("the cut text starts at %d, want %d: %q", x, tt.x, row)
			}
			g := newTipRig(t, 30, 3, tree)
			if x, y := g.at(tt.full); x != tt.x || y != 1 {
				t.Fatalf("unfolded at %d,%d, want %d,1:\n%q", x, y, tt.x, g.rows()[1])
			}
		})
	}

	// In a sink, whole, and cut to fit it with "…".
	t.Run("sink", func(t *testing.T) {
		t.Parallel()

		const s = "界 and 🙂"
		tree := func() Child {
			return Column(
				Text(" "),
				Row(Fixed(6, Text("alpha").Tooltip(s)), Fill(Text(" "))),
				Row(Fixed(8, Text("status:")), Fill(TooltipSink())),
				Row(Fixed(8, Text("short:")), Fixed(4, TooltipSink()), Fill(Text(" "))),
			)
		}
		g := newTipRig(t, 30, 4, tree, WithTooltips(Tooltips{Explicit: TipSink}))
		g.move(1, 1)
		g.wait(defaultTipDelay)
		rows := g.rows()
		if got := strings.TrimSpace(strings.TrimPrefix(rows[2], "status:")); got != s {
			t.Fatalf("sink shows %q, want %q", got, s)
		}
		if got := strings.TrimSpace(strings.TrimPrefix(rows[3], "short:")); got != "界 …" {
			t.Fatalf("short sink shows %q", got)
		}
	})
}

// TestUnfoldMovedByEffect checks that cut text pinned open is unfolded where
// its text is drawn while an effect moves it, not where it was laid out.
func TestUnfoldMovedByEffect(t *testing.T) {
	t.Parallel()

	const full = "abcdefghijkl"
	tree := func() Child {
		return Column(
			Text(" "),
			Transition(0.5, Row(Fixed(6, Text(full).Truncate().TooltipOpen(true)), Fill(Text(" "))), fx.Slide{From: fx.Right}),
			Text(" "),
		)
	}
	g := newTipRig(t, 30, 3, tree)
	ref := New(WithClock(func() time.Time { return g.now }))
	moved := false
	for range 8 {
		row := strings.Split(ansi.Strip(ref.Render(30, 3, tree())), "\n")[1]
		x := strings.Index(row, "abc")
		moved = moved || x > 0
		if gx, gy := g.at(full); gx != x || gy != 1 {
			t.Fatalf("the cut text is drawn at %d,1, unfolded at %d,%d:\n%s", x, gx, gy, ansi.Strip(g.out))
		}
		g.wait(100 * time.Millisecond)
		g.draw()
	}
	if !moved {
		t.Fatal("the effect never moved the text")
	}
}

// TestUnfoldWiderThanScreenKept checks that the bubble shown for cut text
// wider than the screen, of the pointer or pinned open, is kept from frame to
// frame, so an unchanged frame is not drawn again.
func TestUnfoldWiderThanScreenKept(t *testing.T) {
	const wide = "abcdefghijklmnopqrstuvwxyz"
	for _, pinned := range []bool{false, true} {
		g := newTipRig(t, 20, 4, func() Child {
			return Column(Text(" "), Row(Fixed(5, Text(wide).Truncate().TooltipOpen(pinned)), Fill(Text(" "))), Text(" "), Text(" "))
		})
		if !pinned {
			g.move(1, 1)
			g.wait(defaultTipDelay)
			if g.showing() != wide {
				t.Fatalf("showing %q", g.showing())
			}
		}
		if g.count("abcdefghijklmnop") != 1 {
			t.Fatalf("pinned %t: drawn as:\n%s", pinned, ansi.Strip(g.out))
		}
		first := g.out
		g.draw()
		if unsafe.StringData(first) != unsafe.StringData(g.out) {
			t.Fatalf("pinned %t: an unchanged frame was drawn again", pinned)
		}
		if n := testing.AllocsPerRun(20, g.draw); n > 20 {
			t.Fatalf("pinned %t: %v allocations for an unchanged frame", pinned, n)
		}
	}
}

// TestUnfoldClosesPastTarget checks that cut text unfolded stays open only
// while the pointer is over the cut text itself: moving onto what the chip
// covers past it closes it, as leaving any node does, so the tooltip of the
// node beneath shows at once.
func TestUnfoldClosesPastTarget(t *testing.T) {
	t.Parallel()

	const full = "abcdefghijkl"
	tree := func() Child {
		return Column(
			Text(" "),
			Row(Fixed(6, Text(full).Truncate()), Fixed(8, Text("ok").Tooltip("status ok")), Fill(Text(" "))),
			Text(" "),
			Text(" "),
		)
	}
	g := newTipRig(t, 30, 4, tree)
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want(full)
	g.move(5, 1)
	g.want(full)
	// Onto the status cell, which the chip covers.
	g.move(8, 1)
	g.want("status ok")

	// Past the target onto the chip over nothing with a tooltip.
	g = newTipRig(t, 30, 3, func() Child {
		return Column(Text(" "), Row(Fixed(6, Text(full).Truncate()), Fill(Text(" "))), Text(" "))
	})
	g.move(1, 1)
	g.wait(defaultTipDelay)
	g.want(full)
	g.move(9, 1)
	g.want("")
	if g.count(full) != 0 {
		t.Fatalf("still drawn:\n%s", ansi.Strip(g.out))
	}
}
