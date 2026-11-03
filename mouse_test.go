// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

func down(x, y int) tea.MouseMsg { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }

func release(x, y int) tea.MouseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func move(x, y int) tea.MouseMsg { return tea.MouseMotionMsg{X: x, Y: y} }

func dragTo(x, y int) tea.MouseMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// log records what handlers ran, in order.
type log []string

func (l *log) add(s string) func() tea.Cmd {
	return func() tea.Cmd { *l = append(*l, s); return nil }
}

func (l *log) take() []string {
	got := *l
	*l = nil
	return got
}

func expect(t *testing.T, l *log, want ...string) {
	t.Helper()
	if got := l.take(); !slices.Equal(got, want) {
		t.Fatalf("ran %q, want %q", got, want)
	}
}

func TestClickRunsOnRelease(t *testing.T) {
	t.Parallel()

	var l log
	tree := Row(
		Fixed(5, Text("a").On(on.Click(l.add("a")))),
		Fixed(5, Text("b").On(on.Click(l.add("b")), on.RightClick(l.add("b right")), on.MiddleClick(l.add("b middle")))),
	)
	r := New()
	r.Render(10, 1, tree)

	r.Update(down(1, 0))
	expect(t, &l)
	r.Update(release(1, 0))
	expect(t, &l, "a")

	// Released over another node: neither is clicked.
	r.Update(down(1, 0))
	r.Update(release(6, 0))
	expect(t, &l)

	// Motion with no button down means the release was lost, such as
	// outside the window, so a late release does not click.
	r.Update(down(1, 0))
	r.Update(move(1, 0))
	r.Update(release(1, 0))
	expect(t, &l)

	r.Update(tea.MouseClickMsg{X: 6, Y: 0, Button: tea.MouseRight})
	r.Update(tea.MouseReleaseMsg{X: 6, Y: 0, Button: tea.MouseRight})
	r.Update(tea.MouseClickMsg{X: 6, Y: 0, Button: tea.MouseMiddle})
	r.Update(tea.MouseReleaseMsg{X: 6, Y: 0}) // terminals that do not say which button
	expect(t, &l, "b right", "b middle")
}

func TestInnermostOfEachKind(t *testing.T) {
	t.Parallel()

	var l log
	var scrolled []int
	row := func(name string) Child { return Text(name).On(on.Click(l.add(name))) }
	pane := Column(row("one"), row("two"), Text("no handlers")).
		On(on.Click(l.add("pane")), on.Scroll(func(_, dy int) tea.Cmd { scrolled = append(scrolled, dy); return nil }))
	r := New()
	r.Render(20, 4, pane)

	r.ClickAt(0, 1)
	expect(t, &l, "two")
	r.ClickAt(0, 2)
	expect(t, &l, "pane")

	r.Update(tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelDown})
	r.Update(tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelUp})
	if !slices.Equal(scrolled, []int{1, -1}) {
		t.Fatalf("scrolled %v over a row inside the pane", scrolled)
	}
	expect(t, &l)
}

func TestEventIsRelativeToHandlerNode(t *testing.T) {
	t.Parallel()

	var got on.Event
	// The text inside has an ID, which used to make positions relative to it.
	bar := Box(lipgloss.NewStyle().Border(lipgloss.NormalBorder()), Text("seek").ID("label")).
		On(on.ClickAt(func(e on.Event) tea.Cmd { got = e; return nil }))
	r := New()
	r.Render(30, 5, Row(Fixed(4, nil), Fixed(10, bar)))

	r.ClickAt(6, 1)
	if got.X != 2 || got.Y != 1 || got.W != 10 || got.H != 5 || got.Screen != image.Pt(6, 1) || !got.Inside {
		t.Fatalf("ClickAt got %+v, want 2,1 in a 10×5 box", got)
	}
}

func TestRightClickAt(t *testing.T) {
	t.Parallel()

	var got []on.Event
	box := Text("menu").On(on.RightClickAt(func(e on.Event) tea.Cmd { got = append(got, e); return nil }))
	r := New()
	r.Render(30, 5, Row(Fixed(4, nil), Fixed(10, box)))

	r.ClickAt(6, 0)
	r.Update(tea.MouseClickMsg{X: 5, Y: 0, Button: tea.MouseRight})
	r.Update(tea.MouseReleaseMsg{X: 7, Y: 0, Button: tea.MouseRight})
	if len(got) != 1 || got[0].X != 3 || got[0].Screen != image.Pt(7, 0) || !got[0].Inside {
		t.Fatalf("RightClickAt got %+v, want one event at 3,0 where the button came up", got)
	}
}

func TestDoubleClick(t *testing.T) {
	t.Parallel()

	var l log
	now := time.Unix(0, 0)
	r := New(WithClock(func() time.Time { return now }))
	r.Render(10, 3, Column(
		Text("a").On(on.Click(l.add("click")), on.DoubleClick(l.add("double"))),
		Text("b").On(on.DoubleClick(l.add("b double"))),
	))

	r.ClickAt(0, 0)
	now = now.Add(300 * time.Millisecond)
	r.ClickAt(0, 0)
	expect(t, &l, "click", "click", "double")

	// A third click starts again, as does one too late.
	now = now.Add(100 * time.Millisecond)
	r.ClickAt(0, 0)
	now = now.Add(500 * time.Millisecond)
	r.ClickAt(0, 0)
	expect(t, &l, "click", "click")

	// Clicks on two nodes are not a double click, nor are two clicks with
	// one on something else between them.
	r.ClickAt(0, 1)
	r.ClickAt(0, 0)
	r.ClickAt(0, 1)
	r.ClickAt(0, 2) // below both
	r.ClickAt(0, 1)
	expect(t, &l, "click")
}

func TestHover(t *testing.T) {
	t.Parallel()

	var l log
	hover := func(name string) on.Handler {
		return on.Hover(func(over bool) tea.Cmd {
			if over {
				l = append(l, "+"+name)
			} else {
				l = append(l, "-"+name)
			}
			return nil
		})
	}
	tree := func(dialog bool) Child {
		list := Column(Text("row").On(hover("row")), Text("other")).On(hover("list"))
		base := Row(Fixed(10, list), Fill(Text("beside")))
		var layers []LayerNode
		if dialog {
			layers = append(layers, Layer(Text("dialog")).Modal().Anchor(End, End))
		}
		return Overlay(base, layers...)
	}
	r := New()
	r.Render(20, 4, tree(false))

	r.Update(move(0, 0))
	expect(t, &l, "+list", "+row") // every node under the pointer
	r.Update(move(1, 0))
	expect(t, &l)
	r.Update(move(0, 1))
	expect(t, &l, "-row")
	r.Update(move(15, 0))
	expect(t, &l, "-list")
	r.Update(move(0, 0))
	expect(t, &l, "+list", "+row")

	// A dialog opening over the pointer ends the hover, without it moving.
	r.Render(20, 4, tree(true))
	r.Update(nil)
	expect(t, &l, "-row", "-list")
	r.Render(20, 4, tree(false))
	r.Update(nil)
	expect(t, &l, "+list", "+row")

	// Leaving runs the handler of the frame drawn last, not of the frame the
	// pointer arrived in.
	r.Render(20, 4, Overlay(Row(Fixed(10, Column(Text("row").On(hover("new row")), Text("other")).On(hover("list"))))))
	r.Update(move(0, 1))
	expect(t, &l, "-new row")
	r.Render(20, 4, tree(false))
	r.Update(move(0, 0))
	expect(t, &l, "+row")

	// The window losing focus ends it too, until the pointer moves, with
	// the handlers of the frame drawn last.
	r.Render(20, 4, Overlay(Row(Fixed(10, Column(Text("row").On(hover("new row")), Text("other")).On(hover("list"))))))
	r.Update(tea.BlurMsg{})
	expect(t, &l, "-new row", "-list")
	r.Render(20, 4, tree(false))
	r.Update(nil)
	expect(t, &l)
}

func TestHoverStyle(t *testing.T) {
	t.Parallel()

	plain := lipgloss.NewStyle()
	lit := plain.Reverse(true)
	tree := Row(Text("a").Style(plain).HoverStyle(lit), Box(plain.Padding(0, 1), Text("b")).HoverStyle(lit.Padding(0, 1)))
	r := New()
	idle := r.Render(10, 1, tree)
	r.Update(move(0, 0))
	overText := r.Render(10, 1, tree)
	r.Update(move(3, 0))
	overBox := r.Render(10, 1, tree)
	r.Update(move(9, 0))
	if overText == idle || overBox == idle || overText == overBox || r.Render(10, 1, tree) != idle {
		t.Fatalf("hover styles not drawn:\nidle %q\ntext %q\nbox  %q", idle, overText, overBox)
	}
}

func TestDrag(t *testing.T) {
	t.Parallel()

	var events []on.Event
	split := 5
	tree := func() Child {
		divider := Text("|").On(on.Drag(func(e on.Event) tea.Cmd {
			events = append(events, e)
			split = e.Screen.X
			return nil
		}))
		return Row(Fixed(split, nil), Fixed(1, divider), Fill(Text("rest").On(on.Click(func() tea.Cmd {
			t.Error("the click at the end of a drag reached the node released on")
			return nil
		}))))
	}
	r := New()
	r.Render(20, 1, tree())

	r.Update(down(5, 0))
	r.Render(20, 1, tree())
	r.Update(dragTo(9, 0)) // far outside the one-cell divider
	r.Render(20, 1, tree())
	r.Update(dragTo(12, 0))
	r.Render(20, 1, tree())
	r.Update(release(12, 0))
	if split != 12 || len(events) != 4 {
		t.Fatalf("split at %d after %d events, want 12 after 4", split, len(events))
	}
	start, mid, end := events[0], events[1], events[3]
	if start.Start != image.Pt(5, 0) || !start.Inside || start.Done {
		t.Fatalf("drag start %+v", start)
	}
	// The divider moved to 9 with the drag before the next event.
	if mid.X != 4 || mid.Inside || mid.Start != image.Pt(5, 0) {
		t.Fatalf("drag motion %+v, want 4 cells right of the divider", mid)
	}
	if !end.Done || end.Msg == nil {
		t.Fatalf("drag end %+v", end)
	}

	// Other buttons going down and up leave the drag going.
	events = nil
	r.Update(down(12, 0))
	r.Update(tea.MouseClickMsg{X: 12, Y: 0, Button: tea.MouseRight})
	r.Update(tea.MouseReleaseMsg{X: 12, Y: 0, Button: tea.MouseRight})
	r.Update(release(12, 0))
	if len(events) != 2 || events[0].Done || !events[1].Done {
		t.Fatalf("a right click ended a left drag: %+v", events)
	}

	// No more drag events once released.
	events = nil
	r.Update(dragTo(3, 0))
	if len(events) != 0 {
		t.Fatalf("drag events after the release: %+v", events)
	}
}

func TestDragEnds(t *testing.T) {
	t.Parallel()

	var done int
	handle := on.Drag(func(e on.Event) tea.Cmd {
		if e.Done {
			done++
		}
		return nil
	})
	tree := func(shown, dialog bool) Child {
		var kids []Child
		if shown {
			kids = append(kids, Text("handle").On(handle))
		}
		var layers []LayerNode
		if dialog {
			layers = append(layers, Layer(Text("dialog")).Modal())
		}
		return Overlay(Column(kids...), layers...)
	}
	r := New()
	for _, tt := range []struct {
		name string
		end  func()
	}{
		{"the node leaves the frame", func() { r.Render(20, 3, tree(false, false)); r.Update(nil) }},
		{"a dialog opens over it", func() { r.Render(20, 3, tree(true, true)); r.Update(dragTo(2, 0)) }},
		{"the release is lost", func() { r.Update(move(2, 0)) }},
		{"another press comes", func() { r.Update(down(10, 2)) }},
	} {
		done = 0
		r.Render(20, 3, tree(true, false))
		r.Update(down(0, 0))
		tt.end()
		r.Update(release(0, 0))
		if done != 1 {
			t.Fatalf("%s: drag ended %d times", tt.name, done)
		}
	}
}

func TestDragFollowsNodeByTreePosition(t *testing.T) {
	t.Parallel()

	var moves int
	handle := Text("handle").On(on.Drag(func(e on.Event) tea.Cmd {
		if !e.Done {
			moves++
		}
		return nil
	}))
	tree := func(banner bool) Child {
		b := Child(Text("banner"))
		if !banner {
			b = absentNode{}
		}
		return Column(b, handle)
	}
	r := New()
	r.Render(20, 2, tree(false))
	r.Update(down(0, 0))
	// A sibling before it appearing moves it on screen, not in the tree.
	r.Render(20, 2, tree(true))
	r.Update(dragTo(2, 1))
	if moves != 2 {
		t.Fatalf("drag lost when a sibling appeared: %d moves", moves)
	}
}

func TestClickOutside(t *testing.T) {
	t.Parallel()

	var l log
	tree := func(modal bool) Child {
		base := Row(Fixed(10, Text("field").On(on.Click(l.add("field")))), Fill(Text("rest")))
		menu := Layer(Text("menu").On(on.Click(l.add("menu")))).Anchor(End, Start).On(on.ClickOutside(l.add("close menu")))
		layers := []LayerNode{menu}
		if modal {
			layers = append(layers, Layer(Text("dialog")).Modal().Anchor(Start, End).On(on.ClickOutside(l.add("backdrop"))))
		}
		return Overlay(base, layers...)
	}
	r := New()
	r.Render(20, 3, tree(false))

	// The click reaches what it is on as well.
	r.ClickAt(1, 0)
	expect(t, &l, "close menu", "field")
	r.ClickAt(18, 0)
	expect(t, &l, "menu")

	// A modal layer takes the click on its backdrop from what is beneath it,
	// the menu included.
	r.Render(20, 3, tree(true))
	r.ClickAt(1, 0)
	expect(t, &l, "backdrop")
	r.ClickAt(1, 2)
	expect(t, &l)
}

func TestMouseEscapeHatch(t *testing.T) {
	t.Parallel()

	var got []string
	r := New()
	r.Render(10, 1, Text("x").On(on.Mouse(func(e on.Event) tea.Cmd {
		got = append(got, reflect.TypeOf(e.Msg).Name())
		return nil
	})))
	for _, msg := range []tea.MouseMsg{down(0, 0), dragTo(1, 0), release(1, 0), tea.MouseWheelMsg{}, move(0, 0)} {
		r.Update(msg)
	}
	want := []string{"MouseClickMsg", "MouseMotionMsg", "MouseReleaseMsg", "MouseWheelMsg", "MouseMotionMsg"}
	if !slices.Equal(got, want) {
		t.Fatalf("on.Mouse got %v", got)
	}
}

func TestContainersTakeHandlers(t *testing.T) {
	t.Parallel()

	var l log
	style := lipgloss.NewStyle()
	tree := Column(
		Row(Text("a"), Text("b")).Gap(2).On(on.Click(l.add("row"))), // the gap is the row's
		Grid(2, Text("c"), Text("d")).On(on.Click(l.add("grid"))),
		Wrap(Text("e")).On(on.Click(l.add("wrap"))),
		Box(style.Padding(0, 1), Text("f")).On(on.Click(l.add("box"))),
		Fixed(1, Text("g")).On(on.Click(l.add("slot"))),
	)
	r := New()
	r.Render(10, 5, tree)
	for y := range 5 {
		r.ClickAt(2, y)
	}
	expect(t, &l, "row", "grid", "wrap", "box", "slot")
}

func TestOnMouseRunsHandlersOnce(t *testing.T) {
	t.Parallel()

	var l log
	r := New()
	v := r.View(10, 1, Text("x").On(on.Click(func() tea.Cmd {
		l = append(l, "click")
		return func() tea.Msg { return "sent" }
	})))
	if v.OnMouse(down(0, 0)) != nil {
		t.Fatal("a press with no handler returned a command")
	}
	r.Update(down(0, 0))
	cmd := v.OnMouse(release(0, 0))
	expect(t, &l, "click")

	// The handler's command is not run on the event loop, but batched for
	// Bubble Tea to run in the background.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 1 || batch[0]() != "sent" {
		t.Fatalf("OnMouse command sent %#v", cmd())
	}

	// Update skips the event OnMouse ran handlers for, but runs them for
	// one it did not see.
	if r.Update(release(0, 0)) != nil {
		t.Fatal("Update ran handlers again")
	}
	expect(t, &l)
	r.Update(down(0, 0))
	if cmd = r.Update(release(0, 0)); cmd == nil || cmd() != "sent" {
		t.Fatal("Update did not run handlers for an event OnMouse did not see")
	}
	expect(t, &l, "click")
}

func TestViewRoutesItsOwnFrame(t *testing.T) {
	t.Parallel()

	var l log
	r := New()
	shown := r.View(10, 1, Text("old").On(on.Click(l.add("old"))))
	// A newer view, not yet on screen, has another node there.
	r.View(10, 1, Text("new").On(on.Click(l.add("new"))))
	shown.OnMouse(down(0, 0))
	shown.OnMouse(release(0, 0))
	expect(t, &l, "old")
}

func TestFocusMovesBeforeHandlers(t *testing.T) {
	t.Parallel()

	var f Focus[string]
	r := New(WithFocus(&f))
	r.Render(20, 1, Row(
		Text("field").ID("field").Focusable(),
		Text("next").ID("next").Focusable().On(on.Press(func() tea.Cmd {
			if !f.Is("next") {
				t.Error("the button was not focused before its handler ran")
			}
			f.Set("field")
			return nil
		})),
	))
	if _, ok := r.Click("next"); !ok || !f.Is("field") {
		t.Fatalf("focus is %v, want field, where the handler moved it", focused(&f))
	}
}

func TestHandlersAreFreshEachFrame(t *testing.T) {
	t.Parallel()

	var got int
	tree := func(n int) Child {
		return Text("x").ID("x").On(on.Click(func() tea.Cmd { got = n; return nil }))
	}
	r := New()
	r.Render(5, 1, tree(1))
	r.Render(5, 1, tree(2)) // the same geometry, with a new handler
	r.Click("x")
	if got != 2 {
		t.Fatalf("ran the handler of frame %d, want 2", got)
	}
	if _, ok := r.Click("missing"); ok {
		t.Fatal("clicked a node not in the frame")
	}
}

func TestMsg(t *testing.T) {
	t.Parallel()

	r := New()
	r.Render(5, 1, Text("x").On(on.Click(on.Msg("hello"))))
	if cmd := r.ClickAt(0, 0); cmd == nil || cmd() != "hello" {
		t.Fatal("on.Msg did not send its message")
	}
}

func TestThrottleMouse(t *testing.T) {
	t.Parallel()

	filter := ThrottleMouse(time.Hour)
	for i, tt := range []struct {
		msg  tea.Msg
		kept bool
	}{
		{move(0, 0), true},
		{move(1, 0), false},
		{tea.MouseWheelMsg{}, true},
		{tea.MouseWheelMsg{}, false},
		{down(0, 0), true},
		{release(0, 0), true},
		{tea.KeyPressMsg{}, true},
	} {
		if got := filter(nil, tt.msg); (got != nil) != tt.kept {
			t.Fatalf("%d: %T kept %v, want %v", i, tt.msg, got != nil, tt.kept)
		}
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
