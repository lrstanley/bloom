// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
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

// TestSelectScrollsOutward checks that dragging past the edge of text in a
// scroll that is at its end, but partly scrolled out of the page it is in,
// scrolls the page, as the wheel would there.
func TestSelectScrollsOutward(t *testing.T) {
	t.Parallel()

	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line " + string(rune('0'+i))
	}
	inner, page := 7, 0
	clamped := func(at *int, most int) on.Handler {
		return on.Scroll(func(_, dy int) tea.Cmd {
			was := *at
			if *at = min(max(*at+dy, 0), most); *at == was {
				return on.Pass
			}
			return nil
		})
	}
	tree := func() Child {
		return Column(
			Fixed(4, Scroll(Column(
				Text("a\nb\nc"),
				Fixed(3, Scroll(Text(strings.Join(lines, "\n")).Selectable(), inner)).On(clamped(&inner, 7)),
			), page)).On(clamped(&page, 2)),
			Text("status"),
		)
	}
	r := New()
	r.Render(10, 5, tree())
	// Only the first row of the inner scroll, line 7, is in sight.
	r.Update(down(0, 3))
	r.Render(10, 5, tree())
	if cmd := r.Update(dragTo(3, 4)); cmd == nil {
		t.Fatal("dragging past the edge did not start scrolling")
	}
	r.Render(10, 5, tree())
	for range 5 {
		r.Update(selScrollMsg{root: r})
		r.Render(10, 5, tree())
	}
	if inner != 7 || page != 2 {
		t.Fatalf("inner at %d, page at %d; want the page scrolled to its end, 2, and the inner left at 7", inner, page)
	}
	r.Update(release(4, 4))
	if got, _ := r.Selection(); got != "line 7\nline 8\nline 9" {
		t.Fatalf("selected %q, want the lines the page scrolled into sight", got)
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

	tree := func() Child { return Box(bordered, View(hardWrapped{s: "abcdefghij", w: 4})).ID("box") }
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

	for name, text := range map[string]func(string) TextNode{
		"styled":   func(s string) TextNode { return Text(s).Style(lipgloss.NewStyle()) },
		"unstyled": Text,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tree := func() Child {
				return Column(
					Fixed(3, text("alpha beta gamma delta")),
					Fixed(3, text("abcdefghijkl\nnext")),
				).Selectable()
			}
			r := New()
			// Wrapped at spaces, the rows join with them again.
			selectDrag(r, 10, 6, tree, 0, [2]int{0, 0}, [2]int{9, 2})
			wantSelection(t, r, "alpha beta gamma delta")
			// Wrapped inside a word, they join with nothing, and a line break
			// of the text's own stays.
			selectDrag(r, 10, 6, tree, 0, [2]int{0, 3}, [2]int{9, 5})
			wantSelection(t, r, "abcdefghijkl\nnext")

			// Lines cut off rather than wrapped are not joined, even where
			// the next starts with what the cut left off.
			clipped := func() Child { return text("abcdefghij\nghij").NoWrap().Selectable() }
			selectDrag(r, 6, 2, clipped, 0, [2]int{0, 0}, [2]int{5, 1})
			wantSelection(t, r, "abcdef\nghij")
		})
	}
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

	wantCopy(t, cmd, "text", "alpha")
	if copied != "alpha" {
		t.Fatalf("copied %q, want alpha", copied)
	}
}

func TestCopySelectionMotionOff(t *testing.T) {
	setMotion(t, false)

	now := time.Unix(1, 0)
	tree := func() Child { return Text("alpha beta").ID("text").Selectable() }
	r := New(
		WithClock(func() time.Time { return now }),
		WithClipboard(func(string) error { return nil }),
	)
	ticks := 0 // the flash's ticks asked for, once selecting is done
	selected := "48;2;66;66;66m"
	copyHalfWay := func() string {
		t.Helper()
		selectDrag(r, 10, 1, tree, 0, [2]int{0, 0}, [2]int{4, 0})
		r.after = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { ticks++; return nil }
		if r.CopySelection() == nil {
			t.Fatal("nothing copied")
		}
		now = now.Add(flashFor / 2)
		return r.Render(10, 1, tree())
	}

	// With motion off, copying sweeps no band, nor asks for a tick.
	if out := copyHalfWay(); strings.Contains(out, selected) || ticks != 0 {
		t.Fatalf("band swept with motion off, %d ticks: %q", ticks, out)
	}

	// Back on, it sweeps again.
	anim.SetMotion(true)
	if out := copyHalfWay(); !strings.Contains(out, selected) || ticks != 1 {
		t.Fatalf("no band with motion back on, %d ticks: %q", ticks, out)
	}

	// Turned off part way, its next tick stops it, and asks for no other,
	// and the band is gone from the next render.
	anim.SetMotion(false)
	r.Update(flashMsg{root: r})
	if ticks != 1 {
		t.Fatal("the flash kept ticking with motion off")
	}
	if out := r.Render(10, 1, tree()); strings.Contains(out, selected) {
		t.Fatalf("band still drawn after motion was turned off: %q", out)
	}
	// Nor is it drawn when motion goes off before the tick.
	anim.SetMotion(true)
	copyHalfWay()
	anim.SetMotion(false)
	if out := r.Render(10, 1, tree()); strings.Contains(out, selected) {
		t.Fatalf("band drawn after motion was turned off, before its tick: %q", out)
	}
}

// wantCopy fails t unless cmd, from CopySelection, sends exactly one
// CopyMsg, of text from the node with ID id, without an error.
func wantCopy(t *testing.T, cmd tea.Cmd, id ID, text string) {
	t.Helper()
	var msgs []CopyMsg
	for _, m := range drainSequences(cmd) {
		if c, ok := m.(CopyMsg); ok {
			msgs = append(msgs, c)
		}
	}
	if len(msgs) != 1 || msgs[0].Text != text || msgs[0].ID != id || msgs[0].Err != nil {
		t.Fatalf("sent %+v, want one CopyMsg of %q from %v", msgs, text, id)
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

	// The selected cells are drawn reversed and bold, and the one after
	// them as it is.
	tree := func() Child { return Text("abc").Selectable() }
	r := New(WithSelectionStyle(lipgloss.NewStyle().Reverse(true).Bold(true)))
	selectDrag(r, 3, 1, tree, 0, [2]int{0, 0}, [2]int{1, 0})
	wantSelection(t, r, "ab")
	c := canvas.Parse(r.Render(3, 1, tree()), ansi.GraphemeWidth)
	const both = uv.AttrReverse | uv.AttrBold
	for x, want := range []uint8{both, both, 0} {
		if got := c.CellAt(x, 0).Style.Attrs & both; got != want {
			t.Errorf("cell %d drawn with attributes %b, want %b", x, got, want)
		}
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
				On(scrolled(&offset, 2)),
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
	r.Render(6, 3, tree())
	wantSelection(t, r, "one\ntwo\nthree\nf")
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
	for _, id := range []ID{"other", "missing"} {
		r.ClearSelection()
		if r.SelectAll(id) {
			t.Fatalf("SelectAll(%q) selected something", id)
		}
		wantSelection(t, r, "")
	}
}

// scroller is a TextMap that shows its lines from line top, wrapped at
// width cells less the gutter of two cells it draws beside each row, in
// height rows, as a component that scrolls and wraps its own text does.
type scroller struct {
	lines         []string
	top           int
	width, height int
}

// srow is a row of a scroller: the bytes of a line from from up to to.
type srow struct{ line, from, to int }

const gutterW = 2

// rows returns the rows the scroller shows.
func (s *scroller) rows() []srow {
	var rows []srow
	w := s.width - gutterW
	for i := s.top; i < len(s.lines) && len(rows) < s.height; i++ {
		line, from, cells := s.lines[i], 0, 0
		for at, c := range line {
			cw := ansi.StringWidth(string(c))
			if cells+cw > w {
				rows = append(rows, srow{line: i, from: from, to: at})
				from, cells = at, 0
			}
			cells += cw
		}
		rows = append(rows, srow{line: i, from: from, to: len(line)})
	}
	return rows[:min(len(rows), s.height)]
}

func (s *scroller) View() string {
	out := make([]string, s.height)
	rows := s.rows()
	for y := range out {
		text := ""
		if y < len(rows) {
			r := rows[y]
			text = "> " + s.lines[r.line][r.from:r.to]
		}
		out[y] = text + strings.Repeat(" ", max(s.width-ansi.StringWidth(text), 0))
	}
	return strings.Join(out, "\n")
}

func (s *scroller) clamp(p Pos) Pos {
	switch {
	case p.Line < 0:
		return Pos{}
	case p.Line >= len(s.lines):
		return Pos{Line: len(s.lines) - 1, Col: len(s.lines[len(s.lines)-1])}
	}
	return Pos{Line: p.Line, Col: min(max(p.Col, 0), len(s.lines[p.Line]))}
}

func (s *scroller) PosAt(cell image.Point, _ Size) Pos {
	rows := s.rows()
	switch {
	case cell.Y < 0:
		return Pos{Line: rows[0].line, Col: rows[0].from}
	case cell.Y >= len(rows):
		return s.clamp(Pos{Line: len(s.lines)})
	}
	r := rows[cell.Y]
	x := gutterW
	for at, c := range s.lines[r.line][r.from:r.to] {
		cw := ansi.StringWidth(string(c))
		if cell.X < x+cw {
			return Pos{Line: r.line, Col: r.from + at}
		}
		x += cw
	}
	return Pos{Line: r.line, Col: r.to}
}

func (s *scroller) Cells(from, to Pos, _ Size, row func(y, x0, x1 int)) {
	from, to = s.clamp(from), s.clamp(to)
	for y, r := range s.rows() {
		if r.line < from.Line || r.line > to.Line {
			continue
		}
		lo, hi := r.from, r.to
		if r.line == from.Line {
			lo = max(lo, from.Col)
		}
		if r.line == to.Line {
			hi = min(hi, to.Col)
		}
		if lo >= hi {
			continue
		}
		cells := func(b int) int { return gutterW + ansi.StringWidth(s.lines[r.line][r.from:b]) }
		row(y, cells(lo), cells(hi))
	}
}

func (s *scroller) Text(from, to Pos) string {
	from, to = s.clamp(from), s.clamp(to)
	if from.Line == to.Line {
		return s.lines[from.Line][from.Col:max(to.Col, from.Col)]
	}
	parts := []string{s.lines[from.Line][from.Col:]}
	parts = append(parts, s.lines[from.Line+1:to.Line]...)
	return strings.Join(append(parts, s.lines[to.Line][:to.Col]), "\n")
}

func TestSelectTextMap(t *testing.T) {
	t.Parallel()

	s := &scroller{lines: []string{"alpha beta", "gamma delta epsilon", "zeta", "eta theta", "iota"}, width: 10, height: 3}
	tree := func() Child {
		return Fixed(3, View(s)).ID("pane").On(on.Scroll(func(_, dy int) tea.Cmd {
			s.top = min(max(s.top+dy, 0), len(s.lines)-1)
			return nil
		}))
	}
	r := New()
	// Selectable without being made so. "alpha be" is the first row, beside
	// the gutter, and "ta" the second: the character under the pointer is
	// selected, and the gutter is not.
	msg, ok := selectDrag(r, 10, 3, tree, 0, [2]int{0, 0}, [2]int{3, 1})
	if !ok || msg.Text != "alpha beta" || msg.ID != "pane" {
		t.Fatalf("SelectionMsg = %+v (%v), want all of the first line", msg, ok)
	}
	wantHighlighted(t, r.Render(10, 3, tree()), 10, "..alpha be", "..ta......", "..........")

	// Scrolled, the selection stays on its text, and is still read from it
	// scrolled out of sight.
	s.top = 1
	wantHighlighted(t, r.Render(10, 3, tree()), 10, "..........", "..........", "..........")
	wantSelection(t, r, "alpha beta")

	// From "delta" on, held below the pane, the drag scrolls it through its
	// handler, and goes on to text that was out of sight.
	s.top = 0
	r.Render(10, 3, tree())
	r.Update(down(8, 2))
	r.Render(10, 3, tree())
	if cmd := r.Update(dragTo(4, 5)); cmd == nil {
		t.Fatal("dragging past the edge did not start scrolling")
	}
	r.Render(10, 3, tree())
	for range 2 {
		r.Update(selScrollMsg{root: r})
		r.Render(10, 3, tree())
	}
	if s.top != 2 {
		t.Fatalf("top %d after two ticks, want 2", s.top)
	}
	r.Update(dragTo(4, 1))
	r.Render(10, 3, tree())
	r.Update(release(4, 1))
	r.Render(10, 3, tree())
	wantSelection(t, r, "delta epsilon\nzeta\neta")

	// A copy reads it from the component's text.
	wantCopy(t, r.CopySelection(), "pane", "delta epsilon\nzeta\neta")
}

func TestSelectTextMapWordsAndWide(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	s := &scroller{lines: []string{"ab 日本 cd", "one two three four"}, width: 12, height: 3}
	tree := func() Child { return View(s) }
	r := New(WithClock(func() time.Time { return now }))
	// Ending on the first half of 本 takes all of it, and starting on the
	// second half of 日 too.
	selectDrag(r, 12, 3, tree, 0, [2]int{6, 0}, [2]int{7, 0})
	wantSelection(t, r, "日本")
	wantHighlighted(t, r.Render(12, 3, tree()), 12, ".....日本...", "............", "............")

	click := func(x, y int) { selectDrag(r, 12, 3, tree, 0, [2]int{x, y}) }
	// A double click selects the word, as drawn.
	now = now.Add(time.Second)
	click(7, 1)
	click(7, 1)
	wantSelection(t, r, "two")
	// A triple click the whole line, on all the rows it wraps to.
	click(7, 1)
	wantSelection(t, r, "one two three four")
	// The first row's wide characters are a dot each.
	wantHighlighted(t, r.Render(12, 3, tree()), 12, "..........", "..one two th", "..ree four..")
}

func TestSelectTextMapBesideText(t *testing.T) {
	t.Parallel()

	s := &scroller{lines: []string{"first line", "second"}, width: 8, height: 3}
	tree := func() Child {
		return Column(Text("title"), Fixed(3, View(s))).Selectable()
	}
	r := New()
	// From the title into the middle of the component, which wraps "first
	// line" to "first " and "line".
	selectDrag(r, 8, 4, tree, 0, [2]int{1, 0}, [2]int{3, 2})
	wantSelection(t, r, "itle\nfirst li")
	// Through it, from before it to after it.
	tree2 := func() Child {
		return Column(Text("title"), Fixed(3, View(s)), Text("end")).Selectable()
	}
	selectDrag(r, 8, 5, tree2, 0, [2]int{0, 0}, [2]int{1, 4})
	wantSelection(t, r, "title\nfirst line\nsecond\nen")
	// Inside it, alt held selects as a drag does, not a rectangle.
	selectDrag(r, 8, 5, tree2, tea.ModAlt, [2]int{4, 1}, [2]int{3, 3})
	wantSelection(t, r, "rst line\nse")
}

// input is an Editable of one line of plain text, as an input is.
type input struct {
	text string
	sel  Range
}

func (f *input) View() string { return f.text }

func (f *input) clamp(p Pos) Pos {
	if p.Line > 0 {
		return Pos{Col: len(f.text)}
	}
	return Pos{Col: min(max(p.Col, 0), len(f.text))}
}

func (f *input) PosAt(cell image.Point, _ Size) Pos { return f.clamp(Pos{Col: cell.X}) }

func (f *input) Cells(from, to Pos, _ Size, row func(y, x0, x1 int)) {
	row(0, f.clamp(from).Col, f.clamp(to).Col)
}

func (f *input) Text(from, to Pos) string { return f.text[f.clamp(from).Col:f.clamp(to).Col] }

func (f *input) Selection() Range { return f.sel }

func (f *input) Select(r Range) { f.sel = Range{Anchor: f.clamp(r.Anchor), Head: f.clamp(r.Head)} }

func caret(col int) Range { return Range{Anchor: Pos{Col: col}, Head: Pos{Col: col}} }

func span(anchor, head int) Range { return Range{Anchor: Pos{Col: anchor}, Head: Pos{Col: head}} }

func TestSelectEditableMouse(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	f := &input{text: "hello big world"}
	tree := func() Child { return Column(Fixed(1, View(f)).ID("input"), Text("other text").Selectable()) }
	r := New(WithClock(func() time.Time { return now }))

	// A click puts the caret under it.
	selectDrag(r, 15, 2, tree, 0, [2]int{3, 0})
	if f.sel != caret(3) {
		t.Fatalf("after a click, range %+v", f.sel)
	}
	wantSelection(t, r, "")
	// Shift moves the caret and keeps the anchor.
	now = now.Add(time.Second)
	selectDrag(r, 15, 2, tree, tea.ModShift, [2]int{8, 0})
	if f.sel != span(3, 8) {
		t.Fatalf("after shift+click, range %+v", f.sel)
	}
	wantSelection(t, r, "lo bi")
	// A drag runs from the caret at the press to the caret at the pointer,
	// either way, and sends what it selected.
	now = now.Add(time.Second)
	if msg, ok := selectDrag(r, 15, 2, tree, 0, [2]int{10, 0}, [2]int{6, 0}); !ok || msg.Text != "big " || msg.ID != "input" {
		t.Fatalf("SelectionMsg = %+v (%v)", msg, ok)
	}
	if f.sel != span(10, 6) {
		t.Fatalf("after a drag, range %+v", f.sel)
	}
	wantHighlighted(t, r.Render(15, 2, tree()), 15, "......big .....", "...............")

	// A double click selects the word, and a triple click the line.
	now = now.Add(time.Second)
	selectDrag(r, 15, 2, tree, 0, [2]int{12, 0})
	selectDrag(r, 15, 2, tree, 0, [2]int{12, 0})
	wantSelection(t, r, "world")
	selectDrag(r, 15, 2, tree, 0, [2]int{12, 0})
	wantSelection(t, r, "hello big world")

	// Selecting elsewhere collapses it to its caret.
	now = now.Add(time.Second)
	selectDrag(r, 15, 2, tree, 0, [2]int{0, 1}, [2]int{4, 1})
	if f.sel != caret(15) {
		t.Fatalf("after selecting elsewhere, range %+v", f.sel)
	}
	wantSelection(t, r, "other")
	// No highlight is drawn over an empty range.
	wantHighlighted(t, r.Render(15, 2, tree()), 15, "...............", "other..........")
}

func TestSelectEditableKeyboard(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	f := &input{text: "hello world"}
	tree := func() Child { return Column(Fixed(1, View(f)).ID("input"), Text("other text").Selectable()) }
	var copied string
	r := New(WithClock(func() time.Time { return now }), WithClipboard(func(s string) error { copied = s; return nil }))
	selectDrag(r, 11, 2, tree, 0, [2]int{0, 1}, [2]int{4, 1})
	wantSelection(t, r, "other")

	// A range the component makes itself, such as with shift+right, is the
	// selection at once, before a frame is drawn, and clears the other.
	f.sel = span(0, 5)
	wantSelection(t, r, "hello")
	if out := highlighted(r.Render(11, 2, tree()), 11); out[1] != "..........." {
		t.Fatalf("the other selection is still drawn: %q", out)
	}
	// Copying it leaves it as it is, and sweeps the band across it.
	cmd := r.CopySelection()
	if cmd == nil || f.sel != span(0, 5) {
		t.Fatalf("copy returned %v, left range %+v", cmd, f.sel)
	}
	wantCopy(t, cmd, "input", "hello")
	if copied != "hello" {
		t.Fatalf("copied %q", copied)
	}
	steady := highlighted(r.Render(11, 2, tree()), 11)[0]
	now = now.Add(flashFor / 2)
	if band := highlighted(r.Render(11, 2, tree()), 11)[0]; band == steady {
		t.Fatalf("no band half way through the flash: %q", band)
	}
	now = now.Add(flashFor)
	if got := highlighted(r.Render(11, 2, tree()), 11)[0]; got != "hello......" {
		t.Fatalf("the selection is not drawn after the flash: %q", got)
	}
	// A range that changes while the flash runs ends it.
	r.CopySelection()
	now = now.Add(flashFor / 2)
	f.sel = span(0, 4)
	if got := highlighted(r.Render(11, 2, tree()), 11)[0]; got != "hell......." {
		t.Fatalf("the flash went on over a changed range: %q", got)
	}

	// Clearing it collapses it to its caret.
	r.ClearSelection()
	if f.sel != caret(4) {
		t.Fatalf("after ClearSelection, range %+v", f.sel)
	}
	wantSelection(t, r, "")
	// Selecting all of it, by the ID of the slot it is in.
	if !r.SelectAll("input") || f.sel != span(0, 11) {
		t.Fatalf("after SelectAll, range %+v", f.sel)
	}
	wantSelection(t, r, "hello world")
	// No message is sent for what the component selects.
	f.sel = span(6, 11)
	for _, m := range drain(r.Update(nil)) {
		if _, ok := m.(SelectionMsg); ok {
			t.Fatal("a SelectionMsg was sent for a range the component made")
		}
	}
	wantSelection(t, r, "world")

	// CopySelection sees a range made since, at once, too.
	f.sel = span(0, 2)
	// It is the range made since the last copy.
	wantCopy(t, r.CopySelection(), "input", "he")
}

func TestSelectionTextKept(t *testing.T) {
	t.Parallel()

	// From item 1 to item 52 of a list showing four, scrolled back to the
	// top, so that the items read are built apart from the frame.
	built, texts := map[int]int{}, 0
	label := "item"
	withText := false
	offset := 0
	tree := func() Child {
		n := List(100, func(i int) Child { built[i]++; return Text(fmt.Sprintf("%s %02d", label, i)) }).
			Offset(offset).Selectable().On(scrolled(&offset, 96))
		if withText {
			n = n.ItemText(func(i int) string { texts++; return fmt.Sprintf("%s %02d", label, i) })
		}
		return n
	}
	r := New()
	r.after = immediate
	r.Render(8, 4, tree())
	r.Update(down(2, 1))
	r.Render(8, 4, tree())
	r.Update(dragTo(3, 2))
	r.Render(8, 4, tree())
	wheel(r, 8, 4, tree, 3, 2, 50)
	r.Update(release(3, 2))
	offset = 0
	r.Render(8, 4, tree())
	want := "em 01\n" + listRows(2, 52) + "\nitem"

	// Read again in the same frame, it builds nothing.
	clear(built)
	wantSelection(t, r, want)
	once := len(built)
	for range 3 {
		wantSelection(t, r, want)
	}
	if len(built) != once || once == 0 {
		t.Fatalf("built %d items over four reads, %d for the first", len(built), once)
	}
	for i, n := range built {
		if n != 1 {
			t.Fatalf("item %d built %d times over four reads", i, n)
		}
	}
	// The next frame reads the items scrolled away as it would build them
	// then, once.
	label = "next"
	r.Render(8, 4, tree())
	clear(built)
	for range 2 {
		wantSelection(t, r, "xt 01\n"+strings.ReplaceAll(listRows(2, 52), "item", "next")+"\nnext")
	}
	if len(built) != once {
		t.Fatalf("built %d items over two reads in the next frame, want %d", len(built), once)
	}

	// Copying it after reading it builds nothing either.
	label = "item"
	r.Render(8, 4, tree())
	wantSelection(t, r, want)
	clear(built)
	if cmd := r.CopySelection(); cmd == nil {
		t.Fatal("nothing copied")
	}
	if len(built) != 0 {
		t.Fatalf("built %d items to copy what was read", len(built))
	}

	// Given the text of each item, it is read once too.
	withText, label = true, "item"
	offset = 0
	r.Render(8, 4, tree())
	r.Update(down(2, 1))
	r.Render(8, 4, tree())
	r.Update(dragTo(3, 2))
	r.Render(8, 4, tree())
	wheel(r, 8, 4, tree, 3, 2, 50)
	r.Update(release(3, 2))
	offset = 0
	r.Render(8, 4, tree())
	texts = 0
	for range 4 {
		wantSelection(t, r, want)
	}
	if texts != 48 {
		t.Fatalf("read the text of %d items over four reads, want 48", texts)
	}
}

func TestSelectionTextFollowsRange(t *testing.T) {
	t.Parallel()

	// The range found again at the release, where the last frame did not
	// see the pointer, is read as it is then.
	tree := func() Child { return Text("hello world\nsecond line").ID("t").Selectable() }
	r := New()
	r.after = immediate
	r.Render(12, 2, tree())
	r.Update(down(0, 0))
	r.Render(12, 2, tree())
	r.Update(dragTo(4, 0))
	r.Render(12, 2, tree())
	wantSelection(t, r, "hello")
	r.Update(release(5, 1))
	wantSelection(t, r, "hello world\nsecond")

	// A selection made in the same frame is read as it is.
	r.Render(12, 2, tree())
	wantSelection(t, r, "hello world\nsecond")
	if !r.SelectAll("t") {
		t.Fatal("select all found nothing")
	}
	wantSelection(t, r, "hello world\nsecond line")
}

func TestSelectionTextFollowsEdits(t *testing.T) {
	t.Parallel()

	// In a viewport in sight, edited after the frame.
	b := NewBuffer("hello world\nsecond line")
	tree := func() Child { return Viewport(b).Selectable() }
	r := New()
	selectDrag(r, 12, 2, tree, 0, [2]int{0, 0}, [2]int{4, 1})
	wantSelection(t, r, "hello world\nsecon")
	b.Replace(Pos{Line: 0, Col: 0}, Pos{Line: 0, Col: 5}, "HOWDY")
	wantSelection(t, r, "HOWDY world\nsecon")
	b.Insert(Pos{Line: 1, Col: 0}, ">")
	wantSelection(t, r, "HOWDY world\n>secon")

	// In a viewport of an item of a list that the frame did not build.
	bufs := map[int]*Buffer{}
	offset := 0
	item := func(i int) Child {
		if bufs[i] == nil {
			bufs[i] = NewBuffer(fmt.Sprintf("view %d", i))
		}
		return Viewport(bufs[i])
	}
	list := func() Child { return List(20, item).Offset(offset).ID("list").Selectable() }
	r = New()
	r.Render(8, 4, list())
	if !r.SelectAll("list") {
		t.Fatal("select all found nothing")
	}
	got, _ := r.Selection()
	if !strings.Contains(got, "\nview 10\n") {
		t.Fatalf("selected %q, want item 10 in it", got)
	}
	bufs[10].Replace(Pos{}, Pos{Col: 4}, "VIEW")
	if got, _ = r.Selection(); !strings.Contains(got, "\nVIEW 10\n") {
		t.Fatalf("selected %q after item 10 was edited", got)
	}

	// The range of a viewport's caret, changed after the frame, and its
	// buffer, edited after it.
	b = NewBuffer("hello world\nsecond line")
	sel := Range{Anchor: Pos{0, 0}, Head: Pos{0, 5}}
	caret := func() Child { return Viewport(b).ID("v").Caret(&sel) }
	r = New()
	r.Render(12, 2, caret())
	wantSelection(t, r, "hello")
	sel.Head = Pos{1, 6}
	wantSelection(t, r, "hello world\nsecond")
	b.Replace(Pos{Line: 1, Col: 0}, Pos{Line: 1, Col: 6}, "SECOND")
	wantSelection(t, r, "hello world\nSECOND")
	// Shown with another buffer in the next frame.
	b = NewBuffer("other text\nin another buffer")
	r.Render(12, 2, caret())
	wantSelection(t, r, "other text\nin ano")

	// The range of another viewport's caret, the same range, made the
	// selection.
	b1, b2 := NewBuffer("first buffer"), NewBuffer("second buffer")
	sel1, sel2 := Range{Head: Pos{0, 5}}, Range{}
	two := func() Child {
		return Column(Fixed(1, Viewport(b1).Caret(&sel1)), Fixed(1, Viewport(b2).Caret(&sel2)))
	}
	r = New()
	r.Render(13, 2, two())
	wantSelection(t, r, "first")
	sel2 = sel1
	wantSelection(t, r, "secon")
}

func TestSelectionTextAskedAgain(t *testing.T) {
	t.Parallel()

	// A TextMap and an Editable tell their text when asked, which may have
	// changed since it was last asked.
	s := &scroller{lines: []string{"first line", "second"}, width: 12, height: 2}
	tree := func() Child { return Column(Text("title"), Fixed(2, View(s))).Selectable() }
	r := New()
	selectDrag(r, 12, 3, tree, 0, [2]int{0, 0}, [2]int{5, 2})
	wantSelection(t, r, "title\nfirst line\nseco")
	s.lines[1] = "SECOND"
	wantSelection(t, r, "title\nfirst line\nSECO")

	f := &input{text: "hello big world", sel: span(0, 5)}
	r = New()
	r.Render(15, 1, View(f))
	wantSelection(t, r, "hello")
	f.text = "HELLO big world"
	wantSelection(t, r, "HELLO")
}

func TestSelectionReadAfterEdits(t *testing.T) {
	t.Parallel()

	// A viewport the selection runs through, from the text before it to the
	// text after, is read to where its text ends now.
	b := NewBuffer("abcdef")
	through := func() Child { return Column(Text("head"), Fixed(1, Viewport(b)), Text("tail")).Selectable() }
	r := New()
	selectDrag(r, 8, 3, through, 0, [2]int{0, 0}, [2]int{3, 2})
	wantSelection(t, r, "head\nabcdef\ntail")
	b.Delete(Pos{}, Pos{Col: 1})
	wantSelection(t, r, "head\nbcdef\ntail")
	b.Insert(b.End(), "gh")
	wantSelection(t, r, "head\nbcdefgh\ntail")

	// A selection that starts past the end of a viewport's text, and runs on
	// into the next leaf, reads the same before the next frame as after it.
	b = NewBuffer("abc")
	from := func() Child { return Column(Fixed(1, Viewport(b)), Text("tail")).Selectable() }
	r = New()
	selectDrag(r, 8, 2, from, 0, [2]int{5, 0}, [2]int{2, 1})
	wantSelection(t, r, "\ntai")
	b.Insert(b.End(), "d")
	wantSelection(t, r, "d\ntai")
	r.Render(8, 2, from())
	wantSelection(t, r, "d\ntai")

	// A selection whose text is removed is gone before the next frame, as
	// it is after it.
	b = NewBuffer("abcde")
	gone := func() Child { return Viewport(b).Selectable() }
	r = New()
	selectDrag(r, 8, 1, gone, 0, [2]int{1, 0}, [2]int{2, 0})
	wantSelection(t, r, "bc")
	b.Delete(Pos{Col: 1}, Pos{Col: 3})
	wantSelection(t, r, "")
	r.Render(8, 1, gone())
	wantSelection(t, r, "")

	// A rectangle in a viewport stays on its rows as lines are put in above
	// them, and is lost with the text it was in.
	b = NewBuffer("abcd\nefgh\nijkl\nmnop")
	block := func() Child { return Viewport(b).Selectable() }
	r = New()
	selectDrag(r, 6, 5, block, tea.ModAlt, [2]int{1, 1}, [2]int{2, 2})
	wantSelection(t, r, "fg\njk")
	b.Insert(Pos{}, "new\n")
	wantSelection(t, r, "fg\njk")
	b.SetContent("other")
	wantSelection(t, r, "")

	// So is one with a corner in a viewport and the other in another leaf.
	b = NewBuffer("abcd\nefgh")
	across := func() Child { return Column(Fixed(2, Viewport(b)), Text("tail")).Selectable() }
	r = New()
	selectDrag(r, 6, 3, across, tea.ModAlt, [2]int{1, 1}, [2]int{2, 2})
	wantSelection(t, r, "fg\nai")
	b.SetContent("other\ntext")
	wantSelection(t, r, "")

	// A rectangle read from cells, across leaves, is not read from those of
	// a viewport edited since they were laid out, which may have fewer
	// rows now: it is there again in the next frame.
	b = NewBuffer("abcd\nefgh")
	r = New()
	selectDrag(r, 6, 3, across, tea.ModAlt, [2]int{1, 1}, [2]int{2, 2})
	wantSelection(t, r, "fg\nai")
	b.Delete(Pos{Col: 4}, Pos{Line: 1})
	wantSelection(t, r, "")
	r.Render(6, 3, across())
	wantSelection(t, r, "bc\n\nai")
	// Nor when the edit moves its corner out of where the viewport was.
	b = NewBuffer("abcd\nefgh")
	r = New()
	selectDrag(r, 6, 3, across, tea.ModAlt, [2]int{1, 1}, [2]int{2, 2})
	b.Insert(Pos{}, "new\n")
	wantSelection(t, r, "")
}

func TestSelectWordsAcrossLeaves(t *testing.T) {
	t.Parallel()

	// A double click dragged out of a viewport, whose words are found in its
	// text, into a text node, whose words are found in its cells, takes the
	// word at each end.
	now := time.Unix(1, 0)
	b := NewBuffer("view text")
	tree := func() Child { return Column(Fixed(1, Viewport(b)), Text("plain words here")).Selectable() }
	r := New(WithClock(func() time.Time { return now }))
	selectDrag(r, 16, 2, tree, 0, [2]int{1, 0})
	now = now.Add(time.Millisecond)
	selectDrag(r, 16, 2, tree, 0, [2]int{1, 0}, [2]int{8, 1})
	wantSelection(t, r, "view text\nplain words")
}

func TestViewportCaretRedrawnForItsText(t *testing.T) {
	t.Parallel()

	// A tab made spaces draws the same cells, with the range over fewer.
	b := NewBuffer("a\tz")
	sel := Range{Head: Pos{Col: 2}}
	tree := func() Child { return Viewport(b).ID("v").Caret(&sel) }
	r := New()
	wantHighlighted(t, r.Render(8, 1, tree()), 8, "a   ....")
	b.Replace(Pos{Col: 1}, Pos{Col: 2}, "   ")
	wantHighlighted(t, r.Render(8, 1, tree()), 8, "a ......")

	// And so does another buffer that draws the same cells.
	b = NewBuffer("a\tz")
	r = New()
	wantHighlighted(t, r.Render(8, 1, tree()), 8, "a   ....")
	b = NewBuffer("a   z")
	wantHighlighted(t, r.Render(8, 1, tree()), 8, "a ......")
}

// TestSelectEveryNode checks that text is selected in node types that could
// not be selectable before every node type was given Selectable.
func TestSelectEveryNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node Child
		want string
	}{
		{"wrap", Wrap(Text("alpha"), Text("beta")).Gap(1, 0), "alpha beta"},
		{"scroll", Scroll(Text("hidden\nalpha beta"), 1), "alpha beta"},
		{"center", Centered(Text("alpha beta")), "alpha beta"},
		{"marquee", Marquee("alpha beta", 0), "alpha beta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var node Child
			switch n := tt.node.(type) {
			case WrapNode:
				node = n.ID("node").Selectable()
			case ScrollNode:
				node = n.ID("node").Selectable()
			case CenterNode:
				node = n.ID("node").Selectable()
			case MarqueeNode:
				node = n.ID("node").Selectable()
			}
			tree := func() Child { return Column(Fixed(1, node), Text("other")) }
			msg, ok := selectDrag(New(), 12, 2, tree, 0, [2]int{0, 0}, [2]int{11, 1})
			if !ok {
				t.Fatal("no SelectionMsg sent")
			}
			if msg.Text != tt.want || msg.ID != "node" {
				t.Fatalf("SelectionMsg = %+v, want %q in node", msg, tt.want)
			}
		})
	}
}
