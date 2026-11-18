// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// lines returns a list item function of one-line items, counting the items
// it is asked for.
func lines(calls *int) func(i int) Child {
	return func(i int) Child {
		*calls++
		return Text(fmt.Sprintf("item %d", i))
	}
}

func TestListWindowsItems(t *testing.T) {
	t.Parallel()

	var calls int
	list := List(1_000_000, lines(&calls)).Offset(500).ID("list")
	got := ansi.Strip(Render(12, 5, Box(rounded, list)))
	want := strings.Join([]string{
		"╭──────────╮",
		"│item 500  │",
		"│item 501  │",
		"│item 502  │",
		"╰──────────╯",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if calls > 10 {
		t.Fatalf("item was called %d times for 3 rows", calls)
	}

	f := Solve(12, 5, Box(rounded, list))
	if v, ok := f.List("list"); !ok || v != (ListView{First: 500, Visible: 3, Count: 1_000_000}) {
		t.Fatalf("view %+v, %v", v, ok)
	}
	if _, ok := Solve(4, 1, Text("x").ID("t")).List("t"); ok {
		t.Fatal("a node that is not a list reported a view")
	}
}

func TestListClampsAndKeeps(t *testing.T) {
	t.Parallel()

	var calls int
	view := func(n ListNode, h int) ListView {
		v, _ := Solve(10, h, n.ID("list")).List("list")
		return v
	}
	item := lines(&calls)

	tests := []struct {
		name string
		list ListNode
		h    int
		want ListView
	}{
		{"offset past the end fills the space", List(10, item).Offset(50), 4, ListView{First: 6, Visible: 4, Count: 10}},
		{"negative offset", List(10, item).Offset(-3), 4, ListView{First: 0, Visible: 4, Count: 10}},
		{"fewer items than space", List(2, item).Offset(1), 4, ListView{First: 0, Visible: 2, Count: 2}},
		{"kept below scrolls down the least", List(100, item).Offset(10).Keep(20), 4, ListView{First: 17, Visible: 4, Count: 100}},
		{"kept above scrolls up", List(100, item).Offset(10).Keep(3), 4, ListView{First: 3, Visible: 4, Count: 100}},
		{"kept in sight stays put", List(100, item).Offset(10).Keep(12), 4, ListView{First: 10, Visible: 4, Count: 100}},
		{"kept far away", List(100_000, item).Keep(99_999), 4, ListView{First: 99_996, Visible: 4, Count: 100_000}},
		{"kept out of range is ignored", List(100, item).Offset(10).Keep(500), 4, ListView{First: 10, Visible: 4, Count: 100}},
		{"gaps", List(10, item).Gap(1).Keep(5), 5, ListView{First: 3, Visible: 3, Count: 10}},
		{"empty", List(0, item), 4, ListView{}},
	}
	for _, tt := range tests {
		if got := view(tt.list, tt.h); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestListItemHeights(t *testing.T) {
	t.Parallel()

	// Items of 1, 2, 3, ... rows: the last one in sight is cut off, and not
	// counted as visible.
	tall := func(i int) Child { return Text(strings.TrimSuffix(strings.Repeat(fmt.Sprintf("%d\n", i), i+1), "\n")) }
	list := List(10, tall).ID("list")
	got := ansi.Strip(Render(1, 5, list))
	want := strings.Join([]string{"0", "1", "1", "2", "2"}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if v, _ := Solve(1, 5, list).List("list"); v.Visible != 2 {
		t.Fatalf("visible %d, want 2", v.Visible)
	}

	// Keeping an item taller than the list shows it from its top.
	if v, _ := Solve(3, 4, list.Keep(6)).List("list"); v.First != 6 || v.Visible != 1 {
		t.Fatalf("view %+v", v)
	}

	// Kept at the bottom, as far down as the items fit.
	if v, _ := Solve(3, 7, list.Keep(3)).List("list"); v.First != 2 {
		t.Fatalf("view %+v", v)
	}

	// Nil items take no room.
	holes := func(i int) Child {
		if i%2 == 1 {
			return nil
		}
		return Text(strconv.Itoa(i))
	}
	if got = ansi.Strip(Render(1, 3, List(6, holes))); got != "0\n2\n4" {
		t.Fatalf("got:\n%s", got)
	}
	// Nor any gap.
	if got = ansi.Strip(Render(1, 5, List(6, holes).Gap(1))); got != "0\n \n2\n \n4" {
		t.Fatalf("got:\n%q", got)
	}

	// Each item is built once a frame, however many before it are nil.
	var calls int
	sparse := func(i int) Child {
		calls++
		if i%1000 != 0 {
			return nil
		}
		return Text(strconv.Itoa(i))
	}
	if got = ansi.Strip(Render(4, 3, List(3000, sparse))); got != "0   \n1000\n2000" || calls != 2001 {
		t.Fatalf("item called %d times for 2001 items:\n%s", calls, got)
	}
}

func TestListDebug(t *testing.T) {
	t.Parallel()

	inner := func(int) Child { return List(2, func(i int) Child { return Text(strconv.Itoa(i)) }) }
	got := Debug(4, 4, List(2, inner))
	if !strings.Contains(got, "Text") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestListNaturalSize(t *testing.T) {
	t.Parallel()

	var calls int
	list := Column(Auto(List(3, lines(&calls))), Text("end"))
	if got := ansi.Strip(Render(0, 0, list)); got != "item 0\nitem 1\nitem 2\nend   " {
		t.Fatalf("got:\n%q", got)
	}
}

func TestListScrollbarAndMouse(t *testing.T) {
	t.Parallel()

	var calls, clicked, offset int
	clicked = -1
	item := func(i int) Child {
		calls++
		return Text(fmt.Sprintf("row %d", i)).On(on.Click(func() tea.Cmd { clicked = i; return nil }))
	}
	layout := func() Child {
		list := List(100, item).Offset(offset).ID("list").
			On(on.Scroll(func(_, dy int) tea.Cmd { offset += dy * 10; return nil }))
		return Box(rounded, list).Edge(Right, Stretch, ScrollbarOf("list"))
	}

	root := New()
	got := ansi.Strip(root.Render(10, 6, layout()))
	if !strings.Contains(got, "┃") {
		t.Fatalf("no scrollbar:\n%s", got)
	}

	// Items keep their handlers, and are told apart by index as they scroll.
	root.ClickAt(2, 2)
	if clicked != 1 {
		t.Fatalf("clicked %d, want 1", clicked)
	}
	root.Update(tea.MouseWheelMsg{X: 2, Y: 2, Button: tea.MouseWheelDown})
	root.Render(10, 6, layout())
	root.ClickAt(2, 2)
	if clicked != 11 {
		t.Fatalf("clicked %d after scrolling, want 11", clicked)
	}

	// The scrollbar follows the list to its end.
	offset = 1000
	got = ansi.Strip(root.Render(10, 6, layout()))
	lines := strings.Split(got, "\n")
	if !strings.HasSuffix(lines[4], "┃") || !strings.Contains(lines[4], "row 99") {
		t.Fatalf("got:\n%s", got)
	}
	if r, ok := root.Frame().Rect("list"); !ok || r != image.Rect(1, 1, 9, 5) {
		t.Fatalf("list at %v, %v", r, ok)
	}
}

// listRow is item i of the lists the tests select in.
func listRow(i int) Child { return Text(fmt.Sprintf("item %02d", i)) }

// listRows returns the text of items from up to to, as a selection reads
// them.
func listRows(from, to int) string {
	rows := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		rows = append(rows, fmt.Sprintf("item %02d", i))
	}
	return strings.Join(rows, "\n")
}

// wheel turns the wheel down, or up for a negative n, n steps at x, y,
// drawing tree after each.
func wheel(r *Root, w, h int, tree func() Child, x, y, n int) {
	b := tea.MouseWheelDown
	if n < 0 {
		b, n = tea.MouseWheelUp, -n
	}
	for range n {
		r.Update(tea.MouseWheelMsg{X: x, Y: y, Button: b})
		r.Render(w, h, tree())
	}
}

// scrolled returns an [on.Scroll] handler that moves *offset by the wheel,
// within 0 and most.
func scrolled(offset *int, most int) on.Handler {
	return on.Scroll(func(_, dy int) tea.Cmd { *offset = min(max(*offset+dy, 0), most); return nil })
}

func TestSelectListKeepsEnds(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child { return List(100, listRow).Offset(offset).Selectable() }
	r := New()
	selectDrag(r, 8, 4, tree, 0, [2]int{2, 1}, [2]int{5, 3})
	want := "em 01\nitem 02\nitem 0"
	wantSelection(t, r, want)
	shown := highlighted(r.Render(8, 4, tree()), 8)

	// Scrolled past both ends, nothing selected is in sight, but the
	// selection is still there, with all of its text.
	offset = 10
	wantHighlighted(t, r.Render(8, 4, tree()), 8, "........", "........", "........", "........")
	wantSelection(t, r, want)

	// With the anchor above, what is in sight up to the head is drawn.
	offset = 2
	wantHighlighted(t, r.Render(8, 4, tree()), 8, "item 02 ", "item 0..", "........", "........")
	wantSelection(t, r, want)

	// Back where it was, it is drawn as it was, to the cell.
	offset = 0
	wantHighlighted(t, r.Render(8, 4, tree()), 8, shown...)
	wantSelection(t, r, want)
}

func TestSelectListWheelWhileHeld(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child { return List(100, listRow).Offset(offset).Selectable().On(scrolled(&offset, 96)) }
	r := New()
	r.after = immediate
	r.Render(8, 4, tree())
	r.Update(down(2, 1))
	r.Render(8, 4, tree())
	r.Update(dragTo(3, 2))
	r.Render(8, 4, tree())

	// The wheel turned with the button held scrolls the anchor away, and
	// the head follows the pointer onto the items that come into sight.
	wheel(r, 8, 4, tree, 3, 2, 5)
	if offset != 5 {
		t.Fatalf("offset %d after the wheel, want 5", offset)
	}
	wantHighlighted(t, r.Render(8, 4, tree()), 8, "item 05 ", "item 06 ", "item....", "........")

	var msg SelectionMsg
	for _, m := range drain(r.Update(release(3, 2))) {
		if s, ok := m.(SelectionMsg); ok {
			msg = s
		}
	}
	if want := "em 01\n" + listRows(2, 7) + "\nitem"; msg.Text != want {
		t.Fatalf("SelectionMsg text %q, want %q", msg.Text, want)
	}

	// Turned back, the anchor is in sight again where it was.
	r.Render(8, 4, tree())
	wheel(r, 8, 4, tree, 3, 2, -5)
	wantHighlighted(t, r.Render(8, 4, tree()), 8, "........", "..em 01 ", "item 02 ", "item 03 ")
}

func TestSelectListScrollsPastTheEdge(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child {
		return Column(
			Fixed(4, List(20, listRow).Offset(offset).Selectable()).On(scrolled(&offset, 16)),
			Text("status"),
		)
	}
	r := New()
	r.Render(8, 5, tree())
	r.Update(down(0, 0))
	r.Render(8, 5, tree())
	// Held below the list, on the status line, the drag scrolls it, though
	// the items it built fill it exactly.
	if cmd := r.Update(dragTo(3, 4)); cmd == nil {
		t.Fatal("dragging past the edge of a list did not start scrolling")
	}
	r.Render(8, 5, tree())
	wantSelection(t, r, listRows(0, 4))

	// Each tick scrolls an item, and the selection takes it, with those
	// scrolled away above.
	for i := 1; i <= 3; i++ {
		if cmd := r.Update(selScrollMsg{root: r}); cmd == nil {
			t.Fatalf("tick %d did not schedule the next", i)
		}
		r.Render(8, 5, tree())
		if offset != i {
			t.Fatalf("offset %d after %d ticks", offset, i)
		}
		wantSelection(t, r, listRows(0, 4+i))
	}
	wantHighlighted(t, r.Render(8, 5, tree()), 8, "item 03 ", "item 04 ", "item 05 ", "item 06 ", "........")

	// To the last item, where the ticks stop.
	for range 20 {
		r.Update(selScrollMsg{root: r})
		r.Render(8, 5, tree())
	}
	if offset != 16 {
		t.Fatalf("offset %d after scrolling to the end, want 16", offset)
	}
	if cmd := r.Update(selScrollMsg{root: r}); cmd != nil {
		t.Fatal("still scrolling at the end")
	}
	r.Update(release(3, 4))
	wantSelection(t, r, listRows(0, 20))
}

func TestSelectListBuildsOnce(t *testing.T) {
	t.Parallel()

	for _, withText := range []bool{false, true} {
		t.Run(fmt.Sprintf("itemText=%v", withText), func(t *testing.T) {
			t.Parallel()

			built, texts := map[int]int{}, 0
			offset := 0
			tree := func() Child {
				n := List(100, func(i int) Child { built[i]++; return listRow(i) }).
					Offset(offset).Selectable().On(scrolled(&offset, 96))
				if withText {
					n = n.ItemText(func(i int) string { texts++; return fmt.Sprintf("item %02d", i) })
				}
				return n
			}
			// From item 1, scrolled with the wheel while held, to item 52.
			r := New()
			r.after = immediate
			r.Render(8, 4, tree())
			r.Update(down(2, 1))
			r.Render(8, 4, tree())
			r.Update(dragTo(3, 2))
			r.Render(8, 4, tree())
			wheel(r, 8, 4, tree, 3, 2, 50)
			r.Update(release(3, 2))
			r.Render(8, 4, tree())

			// A frame with the anchor scrolled away builds what one with no
			// selection does.
			clear(built)
			r.Render(8, 4, tree())
			frame := len(built)
			clear(built)
			Render(8, 4, tree())
			if len(built) != frame {
				t.Fatalf("a frame with the anchor scrolled away built %d items, one without %d", frame, len(built))
			}

			// Reading the text builds each item selected that is not in sight
			// once, or, given their text, only the one the anchor is in.
			clear(built)
			texts = 0
			wantSelection(t, r, "em 01\n"+listRows(2, 52)+"\nitem")
			switch {
			case withText && (texts != 48 || len(built) != 1 || built[1] != 1):
				t.Fatalf("read the text of %d items and built %v, want 48 and item 1", texts, built)
			case !withText && len(built) != 49:
				t.Fatalf("built %d items to read the text, want 49", len(built))
			}
			for i, n := range built {
				if n != 1 {
					t.Fatalf("item %d built %d times to read the text", i, n)
				}
			}
		})
	}
}

// mixedItems returns the items of a list of tables, viewports, text maps,
// and lists of their own two rows tall, which show two of their three
// items.
func mixedItems() func(i int) Child {
	bufs := map[int]*Buffer{}
	return func(i int) Child {
		switch i % 4 {
		case 0:
			return Row(Fixed(6, Text(fmt.Sprintf("key %02d", i))), Fill(Text(fmt.Sprintf("val %d", i))))
		case 1:
			if bufs[i] == nil {
				bufs[i] = NewBuffer(fmt.Sprintf("view %d a\nview %d b", i, i))
			}
			return Viewport(bufs[i])
		case 2:
			return &scroller{lines: []string{fmt.Sprintf("map %d", i)}, width: 12, height: 1}
		}
		return Column(Fixed(2, List(3, func(k int) Child { return Text(fmt.Sprintf("in %d.%d", i, k)) })))
	}
}

func TestSelectListReadsAsIfBuilt(t *testing.T) {
	t.Parallel()

	// Every item built: in a list tall enough to hold them all, from the
	// space before "a" in item 1's viewport to "ma" of item 10's text map.
	item := mixedItems()
	tall := func() Child { return List(12, item).ID("list").Selectable() }
	r := New()
	selectDrag(r, 12, 20, tall, 0, [2]int{6, 1}, [2]int{3, 15})
	want, ok := r.Selection()
	for _, part := range []string{" a\nview 1 b\n", "key 04 val 4", "in 3.0\nin 3.1\nin 3.2", "\nma"} {
		if !ok || !strings.Contains(want, part) {
			t.Fatalf("selected %q (%v) in a list of every item, want %q in it", want, ok, part)
		}
	}

	// The same, in a list of four rows scrolled with the wheel while held,
	// reads the items scrolled away as the tall one read them.
	offset := 0
	short := func() Child { return List(12, item).ID("list").Offset(offset).Selectable().On(scrolled(&offset, 11)) }
	r = New()
	r.after = immediate
	r.Render(12, 4, short())
	r.Update(down(6, 1))
	r.Render(12, 4, short())
	r.Update(dragTo(6, 2))
	r.Render(12, 4, short())
	wheel(r, 12, 4, short, 6, 2, 10)
	r.Update(dragTo(3, 0))
	r.Render(12, 4, short())
	r.Update(release(3, 0))
	r.Render(12, 4, short())
	wantSelection(t, r, want)

	// And so does all of it, selected at once.
	r.Render(12, 20, tall())
	if !r.SelectAll("list") {
		t.Fatal("select all found nothing in the tall list")
	}
	all, _ := r.Selection()
	if !strings.HasPrefix(all, "key 00 val 0\nview 1 a") || !strings.HasSuffix(all, "in 11.2") {
		t.Fatalf("selected %q, want all of the tall list", all)
	}
	offset = 5
	r.Render(12, 4, short())
	if !r.SelectAll("list") {
		t.Fatal("select all found nothing in the short list")
	}
	wantSelection(t, r, all)
}

func TestSelectAllOfList(t *testing.T) {
	t.Parallel()

	calls := 0
	item := func(i int) Child { calls++; return Text(fmt.Sprintf("item %03d", i)) }
	tree := func() Child {
		return Box(lipgloss.NewStyle(), Column(
			Text("head"),
			Fixed(3, List(1000, item).Offset(500).ID("list")),
			Fixed(3, List(1000, item).Offset(500).ID("own").Selectable()),
			Text("foot"),
		)).ID("pane").Selectable()
	}
	r := New()
	r.Render(8, 8, tree())

	// A selectable list takes every item, without building any to select
	// them.
	calls = 0
	if !r.SelectAll("own") {
		t.Fatal("SelectAll found no list")
	}
	if calls != 0 {
		t.Fatalf("selecting all built %d items", calls)
	}
	rows := make([]string, 1000)
	for i := range rows {
		rows[i] = fmt.Sprintf("item %03d", i)
	}
	items := strings.Join(rows, "\n")
	wantSelection(t, r, items)
	wantHighlighted(t, r.Render(8, 8, tree()), 8,
		"........", "........", "........", "........", "item 500", "item 501", "item 502", "........")

	// A list inside a selectable node, with text before and after it, is
	// taken whole too.
	if !r.SelectAll("list") {
		t.Fatal("SelectAll found no pane")
	}
	wantSelection(t, r, "head\n"+items+"\n"+items+"\nfoot")
}

func TestSelectListAcrossOtherText(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Column(Text("head"), Fixed(3, List(20, listRow).Offset(5)), Text("foot")).Selectable()
	}
	r := New()
	// From the text above the list to the text below it, past the items
	// it shows, takes those it does not.
	selectDrag(r, 8, 5, tree, 0, [2]int{1, 0}, [2]int{2, 4})
	wantSelection(t, r, "ead\n"+listRows(0, 20)+"\nfoo")
	wantHighlighted(t, r.Render(8, 5, tree()), 8, ".ead    ", "item 05 ", "item 06 ", "item 07 ", "foo.....")
}

func TestSelectListItemsChange(t *testing.T) {
	t.Parallel()

	count, base, offset := 10, 0, 0
	list := true
	tree := func() Child {
		var n Child = Text("none")
		if list {
			n = List(count, func(i int) Child { return listRow(base + i) }).Offset(offset)
		}
		return Column(Text("head"), Fixed(4, n)).Selectable()
	}
	r := New()
	selectDrag(r, 8, 5, tree, 0, [2]int{0, 2}, [2]int{3, 4})
	wantSelection(t, r, "item 01\nitem 02\nitem")

	// Items inserted before keep the selection on the same indexes, scrolled
	// away or not.
	base = 10
	r.Render(8, 5, tree())
	wantSelection(t, r, "item 11\nitem 12\nitem")
	offset = 6
	r.Render(8, 5, tree())
	wantSelection(t, r, "item 11\nitem 12\nitem")

	// Fewer items than the head's index end it.
	count = 3
	r.Render(8, 5, tree())
	wantSelection(t, r, "")
	count = 10
	r.Render(8, 5, tree())
	wantSelection(t, r, "")

	// So does the list leaving the frame.
	offset = 0
	selectDrag(r, 8, 5, tree, 0, [2]int{0, 1}, [2]int{3, 2})
	offset = 6
	r.Render(8, 5, tree())
	wantSelection(t, r, "item 10\nitem")
	list = false
	r.Render(8, 5, tree())
	list = true
	r.Render(8, 5, tree())
	wantSelection(t, r, "")
}

func TestSelectListItemPanics(t *testing.T) {
	t.Parallel()

	// The program drops items that a selection reaches, out of sight, and
	// reads the selection before the next frame: the item function is asked
	// for an item it no longer has, and its panic says what happened.
	for _, itemText := range []bool{false, true} {
		rows := make([]string, 40)
		for i := range rows {
			rows[i] = fmt.Sprintf("item %02d", i)
		}
		tree := func() Child {
			n := List(len(rows), func(i int) Child { return Text(rows[i]) }).ID("list").Selectable()
			if itemText {
				n = n.ItemText(func(i int) string { return rows[i] })
			}
			return n
		}
		r := New()
		r.Render(8, 4, tree())
		if !r.SelectAll("list") {
			t.Fatal("nothing to select")
		}
		rows = rows[:10]
		var got any
		func() {
			defer func() { got = recover() }()
			r.Selection()
		}()
		err, ok := got.(error)
		if !ok {
			t.Fatalf("ItemText %v: panicked with %v, want an error", itemText, got)
		}
		if _, wraps := errors.AsType[runtime.Error](err); !wraps {
			t.Errorf("ItemText %v: %v does not wrap what the function panicked with", itemText, err)
		}
		for _, want := range []string{"for item 10,", "outside of a frame", "index out of range", "drawn with 40 items"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ItemText %v: panic %q does not say %q", itemText, err, want)
			}
		}

		// Drawn again, the list has the items the selection is read from.
		r.Render(8, 4, tree())
		if !r.SelectAll("list") {
			t.Fatal("nothing to select")
		}
		if text, _ := r.Selection(); text != strings.Join(rows, "\n") {
			t.Errorf("ItemText %v: selected %q", itemText, text)
		}
	}
}

func TestSelectNestedList(t *testing.T) {
	t.Parallel()

	outerOff, innerOff := 0, 0
	tree := func() Child {
		return List(3, func(i int) Child {
			return Column(Fixed(2, List(10, func(k int) Child { return Text(fmt.Sprintf("o%d i%d", i, k)) }).Offset(innerOff)))
		}).Offset(outerOff).Selectable()
	}
	r := New()
	selectDrag(r, 6, 4, tree, 0, [2]int{0, 0}, [2]int{3, 1})
	wantSelection(t, r, "o0 i0\no0 i")

	// Both ends in items of the inner list scrolled away, and then in an
	// item of the outer one scrolled away.
	innerOff = 5
	wantHighlighted(t, r.Render(6, 4, tree()), 6, "......", "......", "......", "......")
	wantSelection(t, r, "o0 i0\no0 i")
	outerOff = 1
	r.Render(6, 4, tree())
	wantSelection(t, r, "o0 i0\no0 i")
	outerOff, innerOff = 0, 0
	wantHighlighted(t, r.Render(6, 4, tree()), 6, "o0 i0 ", "o0 i..", "......", "......")

	// From one outer item into the next takes all of the inner list it
	// leaves.
	selectDrag(r, 6, 4, tree, 0, [2]int{0, 0}, [2]int{3, 3})
	var want []string
	for k := range 10 {
		want = append(want, fmt.Sprintf("o0 i%d", k))
	}
	wantSelection(t, r, strings.Join(want, "\n")+"\no1 i0\no1 i")
}

func TestSelectListInScroll(t *testing.T) {
	t.Parallel()

	calls, y := 0, 0
	tree := func() Child {
		return Fixed(3, Scroll(List(20, func(i int) Child { calls++; return listRow(i) }).Selectable(), y))
	}
	r := New()
	selectDrag(r, 8, 3, tree, 0, [2]int{0, 0}, [2]int{3, 2})
	y = 10
	r.Render(8, 3, tree())
	// Every item is built in space of unbounded height, and none is built
	// again for the text.
	calls = 0
	wantSelection(t, r, "item 00\nitem 01\nitem")
	if calls != 0 {
		t.Fatalf("built %d items to read the text of a list that built them all", calls)
	}
}

func TestSelectListBlock(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child { return List(10, listRow).Offset(offset).Selectable() }
	r := New()
	selectDrag(r, 8, 3, tree, tea.ModAlt, [2]int{0, 0}, [2]int{3, 1})
	wantSelection(t, r, "item\nitem")
	// A rectangle is of the cells in sight, and is not while a corner is
	// scrolled away.
	offset = 1
	r.Render(8, 3, tree())
	wantSelection(t, r, "")
	offset = 0
	r.Render(8, 3, tree())
	wantSelection(t, r, "item\nitem")
}

func TestSelectListWords(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	offset := 0
	tree := func() Child {
		return List(20, func(i int) Child { return Text(fmt.Sprintf("alpha %02d beta", i)) }).
			Offset(offset).Selectable().On(scrolled(&offset, 17))
	}
	r := New(WithClock(func() time.Time { return now }))
	r.after = immediate
	r.Render(14, 3, tree())
	// A double click on "beta", dragged down a row, and scrolled while held,
	// takes the words whole at both ends.
	r.Update(down(10, 1))
	r.Update(release(10, 1))
	r.Render(14, 3, tree())
	r.Update(down(10, 1))
	r.Render(14, 3, tree())
	r.Update(dragTo(2, 2))
	r.Render(14, 3, tree())
	wheel(r, 14, 3, tree, 2, 2, 5)
	r.Update(release(2, 2))
	r.Render(14, 3, tree())
	wantSelection(t, r, "beta\nalpha 02 beta\nalpha 03 beta\nalpha 04 beta\nalpha 05 beta\nalpha 06 beta\nalpha")
}

func TestCopyListSelection(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	var copied string
	offset := 0
	tree := func() Child { return List(20, listRow).Offset(offset).ID("list").Selectable() }
	r := New(
		WithClock(func() time.Time { return now }),
		WithClipboard(func(text string) error { copied = text; return nil }),
	)
	selectDrag(r, 8, 3, tree, 0, [2]int{0, 1}, [2]int{3, 2})
	r.Render(8, 3, tree())
	if !r.SelectAll("list") {
		t.Fatal("SelectAll found no list")
	}
	offset = 10
	r.Render(8, 3, tree())
	cmd := r.CopySelection()
	if cmd == nil {
		t.Fatal("nothing copied")
	}
	drainSequences(cmd)
	if copied != listRows(0, 20) {
		t.Fatalf("copied %q, want all of the list", copied)
	}
	// The band sweeps across what is in sight of it.
	now = now.Add(flashFor / 2)
	if out := r.Render(8, 3, tree()); !strings.Contains(out, "48;2;66;66;66m") {
		t.Fatalf("no band half way through the flash: %q", out)
	}
}
