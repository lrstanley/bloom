// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"bloomfx.dev/bloom/anim"

	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/internal/linebuf"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// The benchmarks lay out and draw a dashboard on a 200×60 terminal: a
// header, a collapsible sidebar of 30 styled items, four stat cards, a
// scrolled list of 200 rows, a log panel, and a status bar. At 120 frames a
// second a frame has 8.3ms; the layout engine's share should stay a small
// part of that, leaving the rest to the application.

const benchW, benchH = 200, 60

type benchView struct{ s string }

func (v benchView) View() string { return v.s }

var (
	benchBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#5f87af"))
	benchItem     = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0c0c0"))
	benchSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#005f87")).Bold(true)
	benchHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#87afd7")).Bold(true)
	benchList     = benchListView()
)

func benchListView() benchView {
	var b strings.Builder
	for i := range 200 {
		fmt.Fprintf(&b, "\x1b[38;5;%dm%04d\x1b[m  service-%02d  \x1b[1mok\x1b[m  %s\n", 16+i%200, i, i%40, strings.Repeat("·", 40+i%30))
	}
	return benchView{s: strings.TrimSuffix(b.String(), "\n")}
}

func benchLog(frame int) benchView {
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "\x1b[2m%06d\x1b[m request served in %dms ✓\n", frame+i, (frame+i)%97)
	}
	return benchView{s: b.String()}
}

// benchTree builds the dashboard for a frame: selected picks the sidebar
// item, frame changes the log, and dialog, when not negative, is the
// progress of a dialog fading in over a dimmed backdrop.
func benchTree(selected, frame int, dialog float64) Child {
	items := make([]Child, 0, 31)
	for i := range 30 {
		st := benchItem
		if i == selected {
			st = benchSelected
		}
		items = append(items, Text(fmt.Sprintf(" • item %02d", i)).Style(st).ID(i))
	}
	items = append(items, Space())

	cards := make([]Child, 0, 4)
	for i := range 4 {
		cards = append(cards, Fill(Box(benchBorder, Text(fmt.Sprintf("metric %d\n%d req/s\n▁▂▃▅▇", i, 1000+i*37)))))
	}

	base := Column(
		Text("dashboard").Style(benchHeader),
		Fill(Row(
			Fixed(28, Box(benchBorder, Column(items...))).ID("sidebar").Collapsible(1),
			Fill(Column(
				Row(cards...).Gap(1),
				Fill(Box(benchBorder, Scroll(View(benchList), selected*3))),
			)).Min(60),
			Fixed(48, Box(benchBorder, View(benchLog(frame)))).ID("log").Collapsible(2),
		).Gap(1)),
		Text(fmt.Sprintf("frame %d", frame)).Style(benchHeader),
	)
	if dialog < 0 {
		return base
	}
	return Overlay(base,
		Layer(Box(benchBorder.Padding(1, 2), Text("Deploy service-07?\n\n[ yes ]  [ no ]"))).
			ID("dialog").
			Modal().
			Backdrop(fx.Dim{Amount: 0.6}).
			Shadow(2, 1, fx.Dim{Amount: 0.8}).
			Transition(dialog, fx.Fade{}, fx.Slide{From: fx.Bottom}),
	)
}

// BenchmarkSolveNested lays out rows and columns nested 16 deep.
func BenchmarkSolveNested(b *testing.B) {
	tree := nested(16, Text("abcdefghij"))
	b.ReportAllocs()
	for b.Loop() {
		Solve(benchW, benchH, tree)
	}
}

func BenchmarkSolve(b *testing.B) {
	tree := benchTree(3, 0, -1)
	b.ReportAllocs()
	for b.Loop() {
		Solve(benchW, benchH, tree)
	}
}

// BenchmarkRenderUnchanged draws the same frame over and over, which is
// laid out but not composited again.
func BenchmarkRenderUnchanged(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	tree := benchTree(3, 0, -1)
	b.ReportAllocs()
	for b.Loop() {
		r.View(benchW, benchH, tree)
	}
}

// BenchmarkRenderChanged draws a frame where the selection, the scroll, the
// log, and the status bar change every time, as when a key is held down.
func BenchmarkRenderChanged(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.View(benchW, benchH, benchTree(i%30, i, -1))
	}
}

// BenchmarkRenderDialog draws a dialog sliding and fading in over a dimmed
// backdrop with a shadow, with the progress changing every frame.
func BenchmarkRenderDialog(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.View(benchW, benchH, benchTree(3, 0, float64(i%60)/60))
	}
}

// BenchmarkFrameRoundTrip is BenchmarkRenderChanged plus what Bubble Tea
// does with the result: parsing the frame's string back into cells.
func BenchmarkFrameRoundTrip(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	scr := uv.NewScreenBuffer(benchW, benchH)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		v := r.View(benchW, benchH, benchTree(i%30, i, -1))
		uv.NewStyledString(v.Content).Draw(scr, scr.Bounds())
	}
}

// BenchmarkRenderReflow draws several nodes moving and resizing at 200×60.
func BenchmarkRenderReflow(b *testing.B) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	tree := func(p int) Child {
		return Row(
			Percent(p, Box(benchBorder, Text("sidebar"))).ID("sidebar").Animate(anim.Spring(14, 0.8)),
			Fill(benchTree(3, 0, -1)).ID("main").Animate(),
			Percent(30-p/2, Box(benchBorder, Text("details"))).ID("details").Animate(),
		)
	}
	r.View(benchW, benchH, tree(10))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		now = now.Add(time.Second / 120)
		r.View(benchW, benchH, tree(10+(i/30)%3*10))
	}
}

// BenchmarkRenderMoving draws three panes trading places at 200×60, flying
// over with fx.Fly, a turn every half second, so that every frame has panes
// in flight.
func BenchmarkRenderMoving(b *testing.B) {
	now := time.Unix(1, 0)
	r := New(WithClock(func() time.Time { return now }))
	opts := []anim.Option{anim.Duration(time.Second), anim.Ease(anim.Linear)}
	panes := []SlotNode{
		Fixed(40, Box(benchBorder, Text("sidebar"))).ID("sidebar"),
		Fill(benchTree(3, 0, -1)).ID("main"),
		Fixed(50, Box(benchBorder, Text("details"))).ID("details"),
	}
	tree := func(turn int) Child {
		row := make([]Child, len(panes))
		for i, p := range panes {
			row[(i+turn)%len(panes)] = p.Animate(opts...).MoveWith(fx.Fly{})
		}
		return Row(row...)
	}
	r.View(benchW, benchH, tree(0))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		now = now.Add(time.Second / 120)
		r.View(benchW, benchH, tree(1+i/60))
	}
}

// BenchmarkRenderWrappedText scrolls a text of ten thousand lines, styled
// and not, wrapped to fit, which is measured once rather than wrapped every
// frame.
func BenchmarkRenderWrappedText(b *testing.B) {
	lines := make([]string, 10_000)
	for i := range lines {
		lines[i] = benchLogLine(i)
	}
	plain := Text(strings.Join(lines, "\n"))
	for name, text := range map[string]TextNode{"styled": plain.Style(benchItem), "unstyled": plain} {
		b.Run(name, func(b *testing.B) {
			r := New(WithColors(color.White, color.Black))
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				r.View(benchW, benchH, Box(benchBorder, Scroll(text, i*3%10_000)))
			}
		})
	}
	// A row that measures the text at its own width and at what is left
	// beside a sidebar, both of which are kept.
	b.Run("unstyled beside", func(b *testing.B) {
		r := New(WithColors(color.White, color.Black))
		b.ReportAllocs()
		for b.Loop() {
			r.View(benchW, benchH, Row(Auto(plain).Align(Start), Fixed(10, Text("side"))))
		}
	})
}

// The viewport benchmarks draw a buffer of log lines, styled, of mixed
// lengths, in a bordered viewport filling a 200×60 terminal, with line
// numbers. Their cost should not grow with the lines in the buffer.

const benchLines = 1_000_000

var (
	benchLogLines []string
	benchLogOnce  sync.Once
)

func benchLogLine(i int) string {
	return fmt.Sprintf("\x1b[2m%07d\x1b[m GET /api/v1/items/%d served in %dms by worker-%02d %s",
		i, i*7, i%97, i%32, strings.Repeat("lorem ipsum ", 3+i%24))
}

func benchBuffer(b *testing.B, n int) *Buffer {
	b.Helper()
	benchLogOnce.Do(func() {
		benchLogLines = make([]string, benchLines)
		for i := range benchLogLines {
			benchLogLines[i] = benchLogLine(i)
		}
	})
	var buf Buffer
	buf.Append(benchLogLines[:n]...)
	return &buf
}

func benchViewport(buf *Buffer, pos ViewportPos, wrap WrapMode) Child {
	return Box(benchBorder, Viewport(buf).ID("log").At(pos).Wrap(wrap).LineNumbers(benchItem)).
		Edge(Right, Stretch, ScrollbarOf("log"))
}

// BenchmarkViewportScroll scrolls through a million lines, three rows a
// frame.
func BenchmarkViewportScroll(b *testing.B) {
	buf := benchBuffer(b, benchLines)
	r := New(WithColors(color.White, color.Black))
	r.View(benchW, benchH, benchViewport(buf, ViewportPos{}, WrapNone))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.View(benchW, benchH, benchViewport(buf, ViewportPos{Line: i * 3 % benchLines}, WrapNone))
	}
}

// BenchmarkViewportSelectAll selects all of the lines and scrolls through
// them, three rows a frame, as BenchmarkViewportScroll does, which it should
// cost about as much as, with the selection drawn over the rows in sight,
// for a thousand lines and a million alike.
func BenchmarkViewportSelectAll(b *testing.B) {
	for _, n := range []int{1000, benchLines} {
		b.Run(fmt.Sprintf("lines=%d", n), func(b *testing.B) {
			buf := benchBuffer(b, n)
			r := New(WithColors(color.White, color.Black))
			tree := func(line int) Child {
				return Box(benchBorder, Viewport(buf).ID("log").At(ViewportPos{Line: line}).LineNumbers(benchItem).Selectable()).
					Edge(Right, Stretch, ScrollbarOf("log"))
			}
			r.View(benchW, benchH, tree(0))
			if !r.SelectAll("log") {
				b.Fatal("SelectAll found nothing")
			}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				r.View(benchW, benchH, tree(i*3%(n-benchH)))
			}
		})
	}
}

// BenchmarkViewportScrollWrapped scrolls through a million lines wrapped at
// words, three rows a frame.
func BenchmarkViewportScrollWrapped(b *testing.B) {
	buf := benchBuffer(b, benchLines)
	r := New(WithColors(color.White, color.Black))
	r.View(benchW, benchH, benchViewport(buf, ViewportPos{}, WrapWords))
	v, _ := r.Frame().Viewport("log")
	b.ReportAllocs()
	for b.Loop() {
		r.View(benchW, benchH, benchViewport(buf, v.Pos, WrapWords))
		v, _ = r.Frame().Viewport("log")
		if v.AtBottom() {
			v.Pos = ViewportPos{}
		} else {
			v.Pos = v.Down(3)
		}
	}
}

// BenchmarkViewportTail appends a line a frame to a million, wrapped at
// words, and follows the end, as tailing a log does.
func BenchmarkViewportTail(b *testing.B) {
	buf := benchBuffer(b, benchLines)
	r := New(WithColors(color.White, color.Black))
	r.View(benchW, benchH, benchViewport(buf, ViewportEnd, WrapWords))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		buf.Append(benchLogLine(benchLines + i))
		r.View(benchW, benchH, benchViewport(buf, ViewportEnd, WrapWords))
	}
}

// BenchmarkViewportResize draws a hundred thousand lines wrapped at words
// at a width that changes every frame, as when the pane it is in animates
// open, which wraps every line again.
func BenchmarkViewportResize(b *testing.B) {
	buf := benchBuffer(b, benchLines/10)
	r := New(WithColors(color.White, color.Black))
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		r.View(benchW-i%60, benchH, benchViewport(buf, ViewportPos{Line: 5000}, WrapWords))
	}
}

// BenchmarkViewportSetContent replaces a million lines with SetContent and
// draws the first frame of them.
func BenchmarkViewportSetContent(b *testing.B) {
	benchBuffer(b, 0)
	s := strings.Join(benchLogLines, "\n")
	var buf Buffer
	r := New(WithColors(color.White, color.Black))
	b.ReportAllocs()
	for b.Loop() {
		buf.SetContent(s)
		r.View(benchW, benchH, benchViewport(&buf, ViewportPos{}, WrapWords))
	}
}

// The edit benchmarks change a buffer of a million styled log lines,
// drawn in a viewport, wrapped at words, before they start, so that what
// the buffer keeps of its lines is kept as it changes. Each edit is
// followed by the widths and rows a viewport asks for, which an edit keeps
// rather than works out again. Their cost should not grow with the lines
// in the buffer.

// benchEditBuffer returns a buffer of a million lines, drawn once in a
// viewport by r, and a function that asks it for what the viewport draws
// from, its widths and rows.
func benchEditBuffer(b *testing.B, r *Root) (buf *Buffer, index func()) {
	b.Helper()
	buf = benchBuffer(b, benchLines)
	r.View(benchW, benchH, benchViewport(buf, ViewportPos{Line: benchLines / 2}, WrapWords))
	v, _ := r.Frame().Viewport("log")
	return buf, func() { buf.s.Index(ansi.GraphemeWidth, linebuf.Words, v.Width) }
}

// benchPair returns two capital letters for edit i, which change the text
// they replace but for one edit in hundreds of those to the same place.
func benchPair(i int) string {
	return string([]byte{byte('A' + i%26), byte('A' + i/26%26)})
}

// BenchmarkBufferEdit makes the edits an editor makes to a million lines.
func BenchmarkBufferEdit(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	buf, index := benchEditBuffer(b, r)
	rng := rand.New(rand.NewPCG(1, 2))
	b.Run("char", func(b *testing.B) {
		// A character changed at a line anywhere.
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			at := Pos{Line: rng.IntN(benchLines), Col: 20}
			buf.Replace(at, Pos{Line: at.Line, Col: 22}, benchPair(i))
			index()
		}
	})
	b.Run("type", func(b *testing.B) {
		// Characters typed at one place, and taken back now and then.
		at := Pos{Line: benchLines / 2, Col: 30}
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			if i%64 == 63 {
				buf.Delete(Pos{Line: at.Line, Col: 30}, at)
				at.Col = 30
			} else {
				at = buf.Insert(at, "k").End()
			}
			index()
		}
	})
	b.Run("line", func(b *testing.B) {
		// A line put in anywhere, and one taken out anywhere.
		line := benchLogLine(7) + "\n"
		b.ReportAllocs()
		for b.Loop() {
			buf.Insert(Pos{Line: rng.IntN(benchLines)}, line)
			at := rng.IntN(benchLines)
			buf.Delete(Pos{Line: at}, Pos{Line: at + 1})
			index()
		}
	})
	b.Run("half", func(b *testing.B) {
		// Half of the lines taken out at once, from a snapshot of all.
		all := buf.Snapshot()
		b.ReportAllocs()
		for b.Loop() {
			buf.Restore(all)
			buf.Delete(Pos{Line: benchLines / 4}, Pos{Line: 3 * benchLines / 4})
			index()
		}
		buf.Restore(all)
	})
	b.Run("snapshot", func(b *testing.B) {
		// A snapshot taken before each change, as for undo.
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			buf.Snapshot()
			at := Pos{Line: rng.IntN(benchLines), Col: 20}
			buf.Replace(at, Pos{Line: at.Line, Col: 22}, benchPair(i))
			index()
		}
	})
}

// BenchmarkViewportEdit changes a character of a line in sight of a
// million lines, and draws the frame, scrolled three rows further each
// time, as BenchmarkViewportScroll draws it, which it should cost about as
// much as.
func BenchmarkViewportEdit(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	buf, _ := benchEditBuffer(b, r)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		line := i * 3 % benchLines
		buf.Replace(Pos{Line: line + 10, Col: 20}, Pos{Line: line + 10, Col: 22}, benchPair(i))
		r.View(benchW, benchH, benchViewport(buf, ViewportPos{Line: line}, WrapNone))
	}
}

// BenchmarkViewportRestore changes a character of a line in sight of a
// million lines, puts them back to a snapshot taken before, as undo does,
// and draws the frame, as BenchmarkViewportEdit does.
func BenchmarkViewportRestore(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	buf, _ := benchEditBuffer(b, r)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		line := i * 3 % benchLines
		before := buf.Snapshot()
		buf.Replace(Pos{Line: line + 10, Col: 20}, Pos{Line: line + 10, Col: 22}, benchPair(i))
		buf.Restore(before)
		r.View(benchW, benchH, benchViewport(buf, ViewportPos{Line: line}, WrapNone))
	}
}

// BenchmarkViewportCaret types a character at the caret of a viewport of a
// million lines, moves the caret three rows down, keeping its column, keeps
// it in sight, and draws the frame with the terminal cursor at it, as an
// editor does for each key, which it should cost about as much as
// BenchmarkViewportEdit.
func BenchmarkViewportCaret(b *testing.B) {
	r := New(WithColors(color.White, color.Black))
	buf, _ := benchEditBuffer(b, r)
	sel := Range{Anchor: Pos{Line: 10, Col: 20}, Head: Pos{Line: 10, Col: 20}}
	pos := ViewportPos{}
	cur := tea.NewCursor(0, 0)
	tree := func() Child {
		return Box(benchBorder, Viewport(buf).ID("log").At(pos).LineNumbers(benchItem).Caret(&sel).Cursor(cur)).
			Edge(Right, Stretch, ScrollbarOf("log"))
	}
	r.View(benchW, benchH, tree())
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		at := buf.Insert(sel.Head, benchPair(i)[:1]).End()
		sel = Range{Anchor: at, Head: at}
		v, _ := r.Frame().Viewport("log")
		sel = v.MoveRows(sel, 3, v.Column(sel)).Collapse()
		pos = v.Show(sel)
		if r.View(benchW, benchH, tree()).Cursor == nil {
			b.Fatal("no cursor at the caret")
		}
	}
}

// benchListItems are the rows of BenchmarkListSelection's list, made once so
// that building an item costs no formatting.
var benchListItems = func() []string {
	rows := make([]string, 1000)
	for i := range rows {
		rows[i] = fmt.Sprintf("%04d  service-%02d  ok  %s", i, i%40, strings.Repeat("·", 20+i%30))
	}
	return rows
}()

// BenchmarkListSelection scrolls a selectable list of a million items back
// and forth a row a frame, with nothing selected, and with selections that
// highlight about the same rows two ways: from an item in sight, and from
// one scrolled away far above, to the same item in sight; and over all the
// rows in sight, from items in sight, and with all of it selected. An end
// scrolled away, or a million items selected, should cost about what the
// same rows selected in sight do, since a frame builds only the items in
// sight.
func BenchmarkListSelection(b *testing.B) {
	const count, first = 1_000_000, 500_005
	item := func(i int) Child { return Text(benchListItems[i%len(benchListItems)]) }
	tree := func(offset int) Child {
		return Box(benchBorder, List(count, item).Offset(offset).ID("list").Selectable())
	}
	// drag selects from the cell at x0, y0 to the one at x1, y1 with the list
	// at offset.
	drag := func(r *Root, offset, x0, y0, x1, y1 int) {
		r.View(benchW, benchH, tree(offset))
		r.Update(down(x0, y0))
		r.View(benchW, benchH, tree(offset))
		r.Update(dragTo(x1, y1))
		r.View(benchW, benchH, tree(offset))
		r.Update(release(x1, y1))
	}
	for _, c := range []struct {
		name string
		pick func(r *Root)
	}{
		{"none", func(*Root) {}},
		// To column 9 of item 500,029, from the start of item 500,005, which
		// is in sight, and from item 500,000, which is not.
		{"in-view", func(r *Root) { drag(r, first, 1, 1, 10, 25) }},
		{"anchor-off", func(r *Root) { drag(r, first-5, 1, 1, 10, 30) }},
		// The rows in sight, from item 500,005 to the last, and all of it.
		{"view-all", func(r *Root) { drag(r, first, 1, 1, benchW-2, benchH-3) }},
		{"all", func(r *Root) {
			r.View(benchW, benchH, tree(first))
			if !r.SelectAll("list") {
				b.Fatal("SelectAll found nothing")
			}
		}},
	} {
		b.Run(c.name, func(b *testing.B) {
			r := New(WithColors(color.White, color.Black))
			c.pick(r)
			if _, ok := r.Selection(); !ok && c.name != "none" {
				b.Fatal("nothing selected")
			}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				r.View(benchW, benchH, tree(first-i%2))
			}
		})
	}
}

// BenchmarkBufferWordAt finds the word at a place in the middle of a line
// of tens of megabytes, which costs the text around it, rather than the
// line.
func BenchmarkBufferWordAt(b *testing.B) {
	buf := NewBuffer(strings.Repeat("some words-and_more ", 1<<21))
	mid := Pos{Col: len(buf.Text(0)) / 2}
	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := buf.WordAt(mid); !ok {
			b.Fatal("no word")
		}
		buf.NextWord(mid)
		buf.PrevWord(mid)
	}
}

// benchCells are the cells of a table of 60 rows of 5 columns, each too
// long for its column.
var benchCells = func() [][]string {
	rows := make([][]string, 60)
	for y := range rows {
		for x := range 5 {
			rows[y] = append(rows[y], fmt.Sprintf("row %02d column %d: %s", y, x, strings.Repeat("value ", 2+(x+y)%5)))
		}
	}
	return rows
}()

// benchTable is a table of benchCells, every one of which is cut to fit, with
// row selected highlighted.
func benchTable(selected int) Child {
	rows := make([]Child, len(benchCells))
	for y, cells := range benchCells {
		cols := make([]Child, len(cells))
		for x, s := range cells {
			cell := Text(s).Truncate()
			if y == selected {
				cell = cell.Style(benchSelected)
			}
			cols[x] = Fixed(16, cell)
		}
		rows[y] = Row(cols...)
	}
	return Column(rows...)
}

// BenchmarkRenderTruncated draws a table of 300 cells cut to fit, with the
// selection moving every frame, with tooltips off, and on, where each cell
// cut has a tooltip of its full text.
func BenchmarkRenderTruncated(b *testing.B) {
	for _, tt := range []struct {
		name string
		opts []Option
	}{
		{"off", nil},
		{"on", []Option{WithTooltips(Tooltips{})}},
	} {
		b.Run(tt.name, func(b *testing.B) {
			r := New(append([]Option{WithColors(color.White, color.Black)}, tt.opts...)...)
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				r.View(80, benchH, benchTable(i%len(benchCells)))
			}
		})
	}
}
