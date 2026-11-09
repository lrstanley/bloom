// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/on"
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
	got := ansi.Strip(Render(Box(rounded, list), 12, 5))
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

	f := Solve(Box(rounded, list), 12, 5)
	if v, ok := f.List("list"); !ok || v != (ListView{First: 500, Visible: 3, Count: 1_000_000}) {
		t.Fatalf("view %+v, %v", v, ok)
	}
	if _, ok := Solve(Text("x").ID("t"), 4, 1).List("t"); ok {
		t.Fatal("a node that is not a list reported a view")
	}
}

func TestListClampsAndKeeps(t *testing.T) {
	t.Parallel()

	var calls int
	view := func(n ListNode, h int) ListView {
		v, _ := Solve(n.ID("list"), 10, h).List("list")
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
	got := ansi.Strip(Render(list, 1, 5))
	want := strings.Join([]string{"0", "1", "1", "2", "2"}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if v, _ := Solve(list, 1, 5).List("list"); v.Visible != 2 {
		t.Fatalf("visible %d, want 2", v.Visible)
	}

	// Keeping an item taller than the list shows it from its top.
	if v, _ := Solve(list.Keep(6), 3, 4).List("list"); v.First != 6 || v.Visible != 1 {
		t.Fatalf("view %+v", v)
	}

	// Kept at the bottom, as far down as the items fit.
	if v, _ := Solve(list.Keep(3), 3, 7).List("list"); v.First != 2 {
		t.Fatalf("view %+v", v)
	}

	// Nil items take no room.
	holes := func(i int) Child {
		if i%2 == 1 {
			return nil
		}
		return Text(strconv.Itoa(i))
	}
	if got = ansi.Strip(Render(List(6, holes), 1, 3)); got != "0\n2\n4" {
		t.Fatalf("got:\n%s", got)
	}
	// Nor any gap.
	if got = ansi.Strip(Render(List(6, holes).Gap(1), 1, 5)); got != "0\n \n2\n \n4" {
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
	if got = ansi.Strip(Render(List(3000, sparse), 4, 3)); got != "0   \n1000\n2000" || calls != 2001 {
		t.Fatalf("item called %d times for 2001 items:\n%s", calls, got)
	}
}

func TestListDebug(t *testing.T) {
	t.Parallel()

	inner := func(int) Child { return List(2, func(i int) Child { return Text(strconv.Itoa(i)) }) }
	got := Debug(List(2, inner), 4, 4)
	if !strings.Contains(got, "Text") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestListNaturalSize(t *testing.T) {
	t.Parallel()

	var calls int
	list := Column(Auto(List(3, lines(&calls))), Text("end"))
	if got := ansi.Strip(Render(list, 0, 0)); got != "item 0\nitem 1\nitem 2\nend   " {
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
