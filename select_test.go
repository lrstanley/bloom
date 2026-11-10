// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/fx"
	"github.com/lrstanley/bloom/on"
)

// selectDrag presses at the first point, drags through the rest, and
// releases at the last, drawing tree between events as Bubble Tea would,
// and returns the message the release sent, if any.
func selectDrag(r *Root, w, h int, tree func() Child, mod tea.KeyMod, pts ...[2]int) (SelectionMsg, bool) {
	r.after = immediate
	r.Render(w, h, tree())
	first, last := pts[0], pts[len(pts)-1]
	r.Update(tea.MouseClickMsg{X: first[0], Y: first[1], Button: tea.MouseLeft, Mod: mod})
	r.Render(w, h, tree())
	for _, p := range pts[1:] {
		r.Update(dragTo(p[0], p[1]))
		r.Render(w, h, tree())
	}
	cmd := r.Update(release(last[0], last[1]))
	r.Render(w, h, tree())
	// What the Root sends itself, such as a click held back, comes back to
	// it, as Bubble Tea would send it.
	for range 4 {
		var next []tea.Cmd
		for _, msg := range drain(cmd) {
			switch msg := msg.(type) {
			case SelectionMsg:
				return msg, true
			case selDoneMsg, clickMsg:
				next = append(next, r.Update(msg))
			}
		}
		cmd = tea.Batch(next...)
	}
	return SelectionMsg{}, false
}

// immediate is a Root's after for tests: it sends at once.
func immediate(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg { return fn(time.Time{}) }
}

// drain runs cmd and the commands it batches, returning their messages.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func wantSelection(t *testing.T, r *Root, want string) {
	t.Helper()
	got, ok := r.Selection()
	if want == "" {
		if ok {
			t.Fatalf("selected %q, want nothing", got)
		}
		return
	}
	if !ok || got != want {
		t.Fatalf("selected %q (%v), want %q", got, ok, want)
	}
}

var bordered = lipgloss.NewStyle().Border(lipgloss.NormalBorder())

func TestSelectInsideBox(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Row(
			Fixed(12, Box(bordered, Text("alpha one\nbeta two\ngamma"))).ID("left").Selectable(),
			Fill(Box(bordered, Text("other"))),
		)
	}
	r := New()
	// From "one" on the first line to "two" on the second, then past the
	// box's right side: it stays inside the box, and leaves its border out.
	msg, ok := selectDrag(r, 20, 5, tree, 0, [2]int{7, 1}, [2]int{4, 2}, [2]int{18, 2})
	if !ok {
		t.Fatal("no SelectionMsg sent")
	}
	if want := "one\nbeta two"; msg.Text != want || msg.ID != "left" {
		t.Fatalf("SelectionMsg = %+v, want %q in left", msg, want)
	}
	wantSelection(t, r, "one\nbeta two")

	// The selected cells take the terminal's colors swapped, and the border
	// does not.
	out := r.Render(20, 5, tree())
	if !strings.Contains(out, "48;2;66;66;66mone") {
		t.Fatalf("selection not drawn on a background between the terminal's colors:\n%q", out)
	}

	// A click ends it.
	r.Update(down(3, 3))
	r.Update(release(3, 3))
	wantSelection(t, r, "")
}

func TestSelectClickSelectsNothing(t *testing.T) {
	t.Parallel()

	var l log
	tree := func() Child {
		return Column(Text("item").On(on.Click(l.add("click"))), Text("body")).Selectable()
	}
	r := New()
	if _, ok := selectDrag(r, 10, 2, tree, 0, [2]int{1, 0}); ok {
		t.Fatal("a click sent a SelectionMsg")
	}
	wantSelection(t, r, "")
	expect(t, &l, "click")

	// Selecting within the item is not a click on it.
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{3, 0})
	wantSelection(t, r, "item")
	expect(t, &l)
}

func TestSelectDragHandlerWins(t *testing.T) {
	t.Parallel()

	var drags int
	tree := func() Child {
		return Column(
			Text("grip").On(on.Drag(func(on.Event) tea.Cmd { drags++; return nil })),
			Text("text"),
		).Selectable()
	}
	r := New()
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{3, 1})
	wantSelection(t, r, "")
	if drags == 0 {
		t.Fatal("the drag handler did not run")
	}
	// Outside the grip, the column selects.
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 1}, [2]int{3, 1})
	wantSelection(t, r, "text")
}

func TestSelectScrolledText(t *testing.T) {
	t.Parallel()

	lines := []string{"one", "two", "three", "four", "five"}
	offset := 0
	tree := func() Child {
		return Scroll(Text(strings.Join(lines, "\n")).Selectable(), offset)
	}
	r := New()
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{4, 1})
	wantSelection(t, r, "one\ntwo")

	// Scrolled away, the text selected is still there, and the selection
	// moves with it.
	offset = 3
	r.Render(10, 2, tree())
	wantSelection(t, r, "one\ntwo")
}

func TestSelectScrolledBoxFollowsItsText(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child {
		return Box(lipgloss.NewStyle(), Scroll(Column(Text("one"), Text("two"), Text("three")), offset)).Selectable()
	}
	r := New()
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{4, 1})
	wantSelection(t, r, "one\ntwo")
	// The selection is on the text inside the box, which scrolled, so it
	// moves with it, and is still there scrolled out of sight.
	offset = 2
	r.Render(10, 2, tree())
	wantSelection(t, r, "one\ntwo")
}

func TestSelectUnderDialog(t *testing.T) {
	t.Parallel()

	dialog := false
	tree := func() Child {
		var layers []LayerNode
		if dialog {
			layers = append(layers, Layer(Text("DIALOG")).Anchor(Start, Start))
		}
		return Overlay(Text("base text here").Selectable(), layers...)
	}
	r := New()
	selectDrag(r, 14, 1, tree, 0, [2]int{0, 0}, [2]int{8, 0})
	wantSelection(t, r, "base text")

	// The dialog covers the selection, which still has the text beneath.
	dialog = true
	out := ansi.Strip(r.Render(14, 1, tree()))
	if !strings.HasPrefix(out, "DIALOG") {
		t.Fatalf("dialog not drawn over the selection: %q", out)
	}
	wantSelection(t, r, "base text")
}

func TestSelectBlock(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("abcdef\nghijkl\nmnopqr").Selectable() }
	r := New()
	selectDrag(r, 6, 3, tree, tea.ModAlt, [2]int{1, 0}, [2]int{3, 2})
	wantSelection(t, r, "bcd\nhij\nnop")

	selectDrag(r, 6, 3, tree, 0, [2]int{1, 0}, [2]int{3, 2})
	wantSelection(t, r, "bcdef\nghijkl\nmnop")
}

func TestSelectWide(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("a日本b").Selectable() }
	r := New()
	// Starting on the second half of 日 takes all of it.
	selectDrag(r, 6, 1, tree, 0, [2]int{2, 0}, [2]int{4, 0})
	wantSelection(t, r, "日本")
}

func TestSelectNodeGone(t *testing.T) {
	t.Parallel()

	shown := true
	tree := func() Child {
		if !shown {
			return Text("other")
		}
		return Text("selectable").ID("text").Selectable()
	}
	r := New()
	selectDrag(r, 10, 1, tree, 0, [2]int{0, 0}, [2]int{3, 0})
	wantSelection(t, r, "sele")
	shown = false
	r.Render(10, 1, tree())
	r.Update(nil)
	wantSelection(t, r, "")
}

func TestSelectionColors(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("abc").Selectable() }
	r := New(WithSelectionStyle(lipgloss.NewStyle().Background(lipgloss.Color("#ff0000"))))
	selectDrag(r, 3, 1, tree, 0, [2]int{0, 0}, [2]int{1, 0})
	out := r.Render(3, 1, tree())
	if !strings.Contains(out, "48;2;255;0;0") || strings.Contains(out, "\x1b[7m") {
		t.Fatalf("selection not drawn with its colors: %q", out)
	}
}

func TestSelectLayer(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Overlay(Text("base base base\nbase base base\nbase base base"),
			Layer(Box(bordered, Column(Text("title"), Text("token")))).Anchor(Start, Start).ID("dialog").Selectable())
	}
	r := New()
	msg, _ := selectDrag(r, 14, 3, tree, 0, [2]int{1, 1}, [2]int{5, 1})
	if msg.Text != "title" || msg.ID != "dialog" {
		t.Fatalf("SelectionMsg = %+v, want title in dialog", msg)
	}
}

func TestSelectWordAndLine(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	tree := func() Child { return Text("GET /api/v1/orders 200\nnext line").Selectable() }
	r := New(WithClock(func() time.Time { return now }))
	click := func(x int) (SelectionMsg, bool) { return selectDrag(r, 22, 2, tree, 0, [2]int{x, 0}) }

	if _, ok := click(8); ok {
		t.Fatal("a click sent a SelectionMsg")
	}
	// A double click selects the word, which takes in a path whole.
	if msg, ok := click(8); !ok || msg.Text != "/api/v1/orders" {
		t.Fatalf("double click sent %+v (%v), want the path", msg, ok)
	}
	wantSelection(t, r, "/api/v1/orders")
	// A triple click selects the line.
	click(8)
	wantSelection(t, r, "GET /api/v1/orders 200")
	// A fourth starts over, as a click.
	click(8)
	wantSelection(t, r, "")

	// Clicks too far apart are not a double click.
	click(1)
	now = now.Add(time.Second)
	click(1)
	wantSelection(t, r, "")

	// A drag after a double click selects whole words.
	selectDrag(r, 22, 2, tree, 0, [2]int{1, 0}, [2]int{8, 0})
	wantSelection(t, r, "GET /api/v1/orders")
}

func TestSelectScrollsPastTheEdge(t *testing.T) {
	t.Parallel()

	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line " + string(rune('0'+i))
	}
	offset := 0
	tree := func() Child {
		return Column(
			Fixed(3, Scroll(Text(strings.Join(lines, "\n")).Selectable(), offset)).
				On(on.Scroll(func(_, dy int) tea.Cmd { offset = min(max(offset+dy, 0), 7); return nil })),
			Text("status"),
		)
	}
	r := New()
	r.Render(10, 4, tree())
	r.Update(down(0, 0))
	r.Render(10, 4, tree())
	// Held below the text, on the status line, the drag scrolls it.
	if cmd := r.Update(dragTo(3, 3)); cmd == nil {
		t.Fatal("dragging past the edge did not start scrolling")
	}
	r.Render(10, 4, tree())
	if cmd := r.Update(dragTo(4, 3)); cmd != nil {
		t.Fatal("a second tick was started while one is due")
	}
	r.Render(10, 4, tree())
	wantSelection(t, r, "line 0\nline 1\nline 2\nline")

	// Each tick scrolls a step, as the wheel does, and the selection
	// follows the pointer onto what scrolled into sight.
	r.Update(selScrollMsg{root: r})
	r.Render(10, 4, tree())
	if offset != 1 {
		t.Fatalf("offset %d after a tick, want 1", offset)
	}
	wantSelection(t, r, "line 0\nline 1\nline 2\nline 3\nline")
	for range 10 {
		r.Update(selScrollMsg{root: r})
		r.Render(10, 4, tree())
	}
	if offset != 7 {
		t.Fatalf("offset %d after scrolling to the end, want 7", offset)
	}
	// At the end, the ticks stop.
	if cmd := r.Update(selScrollMsg{root: r}); cmd != nil {
		t.Fatal("still scrolling at the end")
	}
	r.Update(release(4, 3))
	got, _ := r.Selection()
	if !strings.HasSuffix(got, "line 8\nline 9") {
		t.Fatalf("selected %q, want it to run to the last line", got)
	}
}

// hardWrapped is a component that wraps its text every w cells, and knows
// the text it wrapped.
type hardWrapped struct {
	s string
	w int
}

func (h hardWrapped) View() string {
	var rows []string
	for s := h.s; s != ""; s = s[min(h.w, len(s)):] {
		rows = append(rows, s[:min(h.w, len(s))])
	}
	return strings.Join(rows, "\n")
}

func (h hardWrapped) SelectedText(span Span) string {
	at := func(p image.Point) int { return min(p.Y*span.Size.W+p.X, len(h.s)) }
	return h.s[at(span.Start):min(at(span.End)+1, len(h.s))]
}

func TestSelectTextSource(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Box(bordered, hardWrapped{s: "abcdefghij", w: 4}).ID("box") }
	r := New()
	// Selectable without being made so; read from the text it wrapped, and
	// sent with the ID of the box it is in.
	msg, ok := selectDrag(r, 6, 5, tree, 0, [2]int{3, 1}, [2]int{2, 2})
	if !ok || msg.Text != "cdef" || msg.ID != "box" {
		t.Fatalf("SelectionMsg = %+v (%v), want the wrapped text joined", msg, ok)
	}
}

func TestSelectionKeepsTextReadable(t *testing.T) {
	t.Parallel()

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3a3a"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffcc00"))
	tree := func() Child { return Text(dim.Render("d") + bright.Render("b")).Selectable() }
	r := New(
		WithColors(lipgloss.Color("#ffffff"), lipgloss.Color("#000000")),
		WithSelectionStyle(lipgloss.NewStyle().Background(lipgloss.Color("#333333"))),
	)
	selectDrag(r, 2, 1, tree, 0, [2]int{0, 0}, [2]int{1, 0})
	out := r.Render(2, 1, tree())
	// The dim foreground is lost on the selection, so it takes the
	// terminal's; the bright one is kept, on the same background.
	for _, want := range []string{"38;2;255;255;255;48;2;51;51;51md", "51;51;51md\x1b[38;2;255;204;0mb"} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in %q", want, out)
		}
	}
}

func TestSelectMovingSlot(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	tree := func(swapped bool) Child {
		a := Fixed(6, Text("aaaaaa")).ID("a").Animate(linear...).MoveWith(fx.Fade{}).Selectable()
		b := Fixed(6, Text("bbbbbb")).ID("b").Animate(linear...).MoveWith(fx.Fade{})
		if swapped {
			a, b = b, a
		}
		return Row(a, b)
	}
	r := New(WithClock(func() time.Time { return now }))
	selectDrag(r, 12, 1, func() Child { return tree(false) }, 0, [2]int{0, 0}, [2]int{3, 0})
	wantSelection(t, r, "aaaa")

	// While it moves, the selection is read from, and drawn over, what the
	// slot draws.
	r.Render(12, 1, tree(true))
	now = now.Add(500 * time.Millisecond)
	r.Render(12, 1, tree(true))
	wantSelection(t, r, "aaaa")
}

func TestSelectDoubleClickSlop(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	tree := func() Child { return Text("alpha beta").Selectable() }
	r := New(WithClock(func() time.Time { return now }))
	selectDrag(r, 10, 1, tree, 0, [2]int{7, 0})
	// The second click a column over is still a double click.
	if msg, ok := selectDrag(r, 10, 1, tree, 0, [2]int{8, 0}); !ok || msg.Text != "beta" {
		t.Fatalf("double click a cell apart sent %+v (%v), want beta", msg, ok)
	}
	// Two columns over is not.
	now = now.Add(time.Second)
	selectDrag(r, 10, 1, tree, 0, [2]int{1, 0})
	selectDrag(r, 10, 1, tree, 0, [2]int{3, 0})
	wantSelection(t, r, "")
}

func TestSelectDoubleClickBlank(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("alpha   beta").Selectable() }
	r := New(WithClock(func() time.Time { return time.Unix(1, 0) }))
	selectDrag(r, 12, 1, tree, 0, [2]int{6, 0})
	if msg, ok := selectDrag(r, 12, 1, tree, 0, [2]int{6, 0}); ok {
		t.Fatalf("double click on a blank sent %+v", msg)
	}
	wantSelection(t, r, "")
}

func TestSelectDoubleClickWaitsForATriple(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("alpha beta").Selectable() }
	r := New(WithClock(func() time.Time { return time.Unix(1, 0) }))
	r.Render(10, 1, tree())
	click := func() tea.Cmd {
		r.Update(down(1, 0))
		r.Render(10, 1, tree())
		cmd := r.Update(release(1, 0))
		r.Render(10, 1, tree())
		return cmd
	}
	click()
	// The double click's message waits on a tick, which a third click
	// makes stale.
	r.after = func(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return fn(time.Time{}) }
	}
	var done tea.Msg
	for _, msg := range drain(click()) {
		if _, ok := msg.(selDoneMsg); ok {
			done = msg
		}
	}
	if done == nil {
		t.Fatal("double click did not wait to send its message")
	}
	triple := drain(click())
	if len(drain(r.Update(done))) != 0 {
		t.Fatal("the double click sent its message after a triple click")
	}
	if len(triple) != 1 || triple[0].(SelectionMsg).Text != "alpha beta" {
		t.Fatalf("triple click sent %v, want the line at once", triple)
	}
}

func TestSelectTrailingBlankRows(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Column(Text("one"), Text(""), Text("")).Selectable() }
	r := New()
	selectDrag(r, 5, 3, tree, 0, [2]int{0, 0}, [2]int{4, 2})
	wantSelection(t, r, "one")
}

func TestSelectWrappedText(t *testing.T) {
	t.Parallel()

	plain := lipgloss.NewStyle()
	tree := func() Child {
		return Column(
			Fixed(3, Text("alpha beta gamma delta").Style(plain)),
			Fixed(3, Text("abcdefghijkl\nnext").Style(plain)),
		).Selectable()
	}
	r := New()
	// Wrapped at spaces, the rows join with them again.
	selectDrag(r, 10, 6, tree, 0, [2]int{0, 0}, [2]int{9, 2})
	wantSelection(t, r, "alpha beta gamma delta")
	// Wrapped inside a word, they join with nothing, and a line break of
	// the text's own stays.
	selectDrag(r, 10, 6, tree, 0, [2]int{0, 3}, [2]int{9, 5})
	wantSelection(t, r, "abcdefghijkl\nnext")
}

func TestSelectAcrossNodes(t *testing.T) {
	t.Parallel()

	field := func(label, value string) Child {
		return Row(Fixed(6, Text(label)), Fill(Text(value)))
	}
	tree := func() Child {
		return Box(bordered, Column(field("id", "7f3a"), field("path", "/api"))).
			Edge(Top, Start, Text("title")).Selectable()
	}
	r := New()
	// From the first label to the last value: a value goes beside its
	// label, and the border and its title are left out.
	selectDrag(r, 14, 4, tree, 0, [2]int{0, 0}, [2]int{13, 3})
	wantSelection(t, r, "id 7f3a\npath /api")
	// From inside one value to inside the next.
	selectDrag(r, 14, 4, tree, 0, [2]int{9, 1}, [2]int{8, 2})
	wantSelection(t, r, "3a\npath /a")
}

func TestSelectWords(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	double := func(r *Root, text string, x int) string {
		tree := func() Child { return Text(text).Selectable() }
		selectDrag(r, 20, 1, tree, 0, [2]int{x, 0})
		msg, _ := selectDrag(r, 20, 1, tree, 0, [2]int{x, 0})
		now = now.Add(time.Second)
		return msg.Text
	}
	clock := WithClock(func() time.Time { return now })
	r := New(clock)
	for _, tt := range []struct {
		text string
		x    int
		want string
	}{
		{"don't stop", 1, "don't"},            // as Unicode finds words
		{"see /api/v1/x now", 6, "/api/v1/x"}, // joined across word characters
		{"日本語 text", 0, "日本語"},                // wide characters, taken whole
		{"(foo), bar", 2, "foo"},
	} {
		if got := double(r, tt.text, tt.x); got != tt.want {
			t.Errorf("double click on %q at %d selected %q, want %q", tt.text, tt.x, got, tt.want)
		}
	}
	// With no word characters, words are as Unicode finds them.
	if got := double(New(clock, WithWordChars("")), "see /api/v1/x now", 6); got != "api" {
		t.Errorf("double click with no word characters selected %q, want api", got)
	}
}

func TestSelectClickDelay(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	var l log
	tree := func() Child { return Column(Text("item one").On(on.Click(l.add("click")))).Selectable() }

	// A click inside a selectable node waits, and runs once it can no
	// longer become a double click.
	r := New(WithClock(func() time.Time { return now }))
	if _, ok := selectDrag(r, 10, 1, tree, 0, [2]int{1, 0}); ok {
		t.Fatal("a click sent a SelectionMsg")
	}
	expect(t, &l, "click")

	// A double click selects the word, and does not click.
	r = New(WithClock(func() time.Time { return now }))
	r.Render(10, 1, tree())
	r.Update(down(1, 0))
	r.Render(10, 1, tree())
	held := r.Update(release(1, 0))
	r.Render(10, 1, tree())
	if held == nil {
		t.Fatal("the click was not held back")
	}
	selectDrag(r, 10, 1, tree, 0, [2]int{1, 0})
	wantSelection(t, r, "item")
	expect(t, &l)

	// Off, a click runs at once.
	r = New(WithClock(func() time.Time { return now }), WithClickDelay(false))
	r.Render(10, 1, tree())
	r.Update(down(1, 0))
	r.Update(release(1, 0))
	expect(t, &l, "click")
}

func TestCopySelection(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	var copied string
	tree := func() Child { return Text("alpha beta").ID("text").Selectable() }
	r := New(
		WithClock(func() time.Time { return now }),
		WithClipboard(func(text string) error { copied = text; return nil }),
	)
	if r.CopySelection() != nil {
		t.Fatal("copied with nothing selected")
	}
	selectDrag(r, 10, 1, tree, 0, [2]int{0, 0}, [2]int{4, 0})
	cmd := r.CopySelection()
	if cmd == nil {
		t.Fatal("nothing copied")
	}
	wantSelection(t, r, "")

	// Half way through the flash, the band is over part of the text.
	selected := "48;2;66;66;66m"
	now = now.Add(flashFor / 2)
	if out := r.Render(10, 1, tree()); !strings.Contains(out, selected) {
		t.Fatalf("no band half way through the flash: %q", out)
	}
	now = now.Add(flashFor)
	if out := r.Render(10, 1, tree()); strings.Contains(out, selected) {
		t.Fatalf("the band is still drawn after the flash: %q", out)
	}

	var msg CopyMsg
	for _, m := range drainSequences(cmd) {
		if c, ok := m.(CopyMsg); ok {
			msg = c
		}
	}
	if copied != "alpha" || msg.Text != "alpha" || msg.ID != "text" || msg.Err != nil {
		t.Fatalf("copied %q, sent %+v, want alpha from text", copied, msg)
	}
}

// drainSequences is drain, running the commands of sequences as well,
// skipping those that wait.
func drainSequences(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range drain(cmd) {
		v := reflect.ValueOf(msg)
		if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
			out = append(out, msg)
			continue
		}
		for i := range v.Len() {
			if c, _ := reflect.TypeAssert[tea.Cmd](v.Index(i)); c != nil {
				if m := c(); m != nil {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

func TestSelectionStyle(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("ab").Selectable() }
	r := New(WithSelectionStyle(lipgloss.NewStyle().Reverse(true).Bold(true)))
	selectDrag(r, 2, 1, tree, 0, [2]int{0, 0}, [2]int{1, 0})
	if out := r.Render(2, 1, tree()); !strings.Contains(out, "\x1b[1;7mab") {
		t.Fatalf("selection not drawn reversed and bold: %q", out)
	}
}

func TestSelectTruncatedText(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	padded := lipgloss.NewStyle().Padding(0, 1)
	tree := func() Child {
		return Column(
			Fixed(1, Text("alpha beta gamma").Truncate()),
			Fixed(1, Text("7f3a9c2e-41b8-4d0e").Style(padded).Truncate()),
		).Selectable()
	}
	r := New(WithClock(func() time.Time { return now }))
	r.Render(10, 2, tree())
	if out := ansi.Strip(r.Render(10, 2, tree())); !strings.Contains(out, "alpha bet…") {
		t.Fatalf("text not cut: %q", out)
	}
	// Taking the "…" takes the rest of the line.
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{9, 0})
	wantSelection(t, r, "alpha beta gamma")
	// Short of the cut, it is what is shown.
	selectDrag(r, 10, 2, tree, 0, [2]int{0, 0}, [2]int{4, 0})
	wantSelection(t, r, "alpha")
	// A word the cut ends is taken whole, as is one in a padded style.
	for _, tt := range []struct {
		x, y int
		want string
	}{{7, 0, "beta"}, {3, 1, "7f3a9c2e-41b8-4d0e"}} {
		now = now.Add(time.Second)
		selectDrag(r, 10, 2, tree, 0, [2]int{tt.x, tt.y})
		selectDrag(r, 10, 2, tree, 0, [2]int{tt.x, tt.y})
		wantSelection(t, r, tt.want)
	}
}

func TestSelectReleaseWhereNotDrawn(t *testing.T) {
	t.Parallel()

	tree := func() Child { return Text("abcdef").Selectable() }
	r := New()
	r.Render(6, 1, tree())
	r.Update(down(0, 0))
	r.Render(6, 1, tree())
	r.Update(dragTo(2, 0))
	r.Render(6, 1, tree())
	// The release comes further on, with no frame drawn for it, as when
	// motion is throttled.
	var got string
	for _, msg := range drain(r.Update(release(4, 0))) {
		if m, ok := msg.(SelectionMsg); ok {
			got = m.Text
		}
	}
	if got != "abcde" {
		t.Fatalf("SelectionMsg text %q, want abcde", got)
	}
}

func TestSelectScrollsAContainer(t *testing.T) {
	t.Parallel()

	offset := 0
	tree := func() Child {
		return Column(
			Fixed(2, Scroll(Column(Text("one"), Text("two"), Text("three"), Text("four")).Selectable(), offset)).
				On(on.Scroll(func(_, dy int) tea.Cmd { offset = min(max(offset+dy, 0), 2); return nil })),
			Text("status"),
		)
	}
	r := New()
	r.Render(6, 3, tree())
	r.Update(down(0, 0))
	r.Render(6, 3, tree())
	if r.Update(dragTo(0, 2)) == nil {
		t.Fatal("dragging past the edge of a selectable column did not start scrolling")
	}
	r.Render(6, 3, tree())
	r.Update(selScrollMsg{root: r})
	if offset != 1 {
		t.Fatalf("offset %d after a tick, want 1", offset)
	}
}

func TestSelectAll(t *testing.T) {
	t.Parallel()

	tree := func() Child {
		return Column(
			Fixed(2, Box(lipgloss.NewStyle(), Scroll(Column(Text("one"), Text("two").ID("two"), Text("three")), 1)).
				ID("pane").Selectable()),
			Text("other").ID("other"),
		)
	}
	r := New()
	if r.SelectAll("pane") {
		t.Fatal("selected all before a frame was drawn")
	}
	r.Render(6, 3, tree())
	// All of it, including what is scrolled out of sight, at once.
	if !r.SelectAll("pane") {
		t.Fatal("SelectAll found no pane")
	}
	wantSelection(t, r, "one\ntwo\nthree")
	// And drawn in the next frame.
	if out := r.Render(6, 3, tree()); !strings.Contains(out, "48;2;66;66;66mtwo") {
		t.Fatalf("select all not drawn: %q", out)
	}
	// A node inside selects all of the selectable node it is in.
	r.ClearSelection()
	if !r.SelectAll("two") {
		t.Fatal("SelectAll found no selectable node around two")
	}
	wantSelection(t, r, "one\ntwo\nthree")
	// A selectable node with no text clears the selection, rather than
	// leaving one whose text is gone.
	empty := func() Child {
		return Column(Text("abc").ID("text").Selectable(), Fixed(1, nil).ID("empty").Selectable())
	}
	r2 := New()
	selectDrag(r2, 3, 2, empty, 0, [2]int{0, 0}, [2]int{1, 0})
	wantSelection(t, r2, "ab")
	if r2.SelectAll("empty") {
		t.Fatal("selected all of a node with no text")
	}
	wantSelection(t, r2, "")

	// Nothing selectable, or nothing at all, selects nothing.
	for _, id := range []string{"other", "missing"} {
		r.ClearSelection()
		if r.SelectAll(id) {
			t.Fatalf("SelectAll(%q) selected something", id)
		}
		wantSelection(t, r, "")
	}
}
