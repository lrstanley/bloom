// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// lineParts are what the lines of the model tests are made of: words,
// wide glyphs, tabs, and the sequences that set and reset each field of a
// style, and hyperlinks.
var lineParts = []string{
	"word", "a", "longerword", "日本", "\t", " ", "  ", "x-y", "-", "é",
	"\x1b[31m", "\x1b[1;4m", "\x1b[m", "\x1b[0m", "\x1b[38;5;196m", "\x1b[48;2;1;2;3m",
	"\x1b[4:3m", "\x1b[58;5;3m", "\x1b[22m", "\x1b[39m", "\x1b[7;9m", "\x1b[24m",
	"\x1b]8;;https://x.dev\x1b\\", "\x1b]8;id=1;https://y.dev\x07", "\x1b]8;;\x1b\\",
}

// randomLine returns a line of parts: plain most of the time, now and then
// styled, and rarely long enough to be kept as a long line.
func randomLine(rng *rand.Rand) string {
	parts := lineParts[:10]
	if rng.IntN(3) == 0 {
		parts = lineParts
	}
	n := rng.IntN(14)
	if rng.IntN(400) == 0 {
		n = LongLineBytes/3 + rng.IntN(200)
	}
	var b strings.Builder
	for range n {
		b.WriteString(parts[rng.IntN(len(parts))])
		if rng.IntN(2) == 0 {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// modelKey is what the reference works out the rows of a line for.
type modelKey struct {
	s   string
	key rowKey
}

// model is the reference the store is checked against: its lines in a
// slice, and what is worked out of them from scratch, each line kept from
// the last time it was asked for.
type model struct {
	lines   []string
	widths  map[string][2]int32
	rows    map[modelKey]int
	changes map[string]*penChange
}

func newModel() *model {
	return &model{widths: map[string][2]int32{}, rows: map[modelKey]int{}, changes: map[string]*penChange{}}
}

var modelMethods = [2]ansi.Method{ansi.GraphemeWidth, ansi.WcWidth}

// width returns how wide line s is measured with method, one of
// modelMethods, as measureLine returns it.
func (m *model) width(s string, method ansi.Method) int32 {
	w, ok := m.widths[s]
	if !ok {
		w = [2]int32{measureLine(s, modelMethods[0]), measureLine(s, modelMethods[1])}
		m.widths[s] = w
	}
	if method == modelMethods[1] {
		return w[1]
	}
	return w[0]
}

func (m *model) rowsOf(s string, k rowKey) int {
	if k.mode == None || k.width <= 0 {
		return 1
	}
	r, ok := m.rows[modelKey{s, k}]
	if !ok {
		if int(m.width(s, k.method)&widthMask) <= k.width {
			r = 1
		} else {
			r, _ = wrapLine(s, k.method, k.width, k.mode, nil)
		}
		m.rows[modelKey{s, k}] = r
	}
	return r
}

// change returns lineChange(s).
func (m *model) change(s string) *penChange {
	c, ok := m.changes[s]
	if !ok {
		c = lineChange(s)
		m.changes[s] = c
	}
	return c
}

// add appends the lines of text to m's, as AppendText adds them.
func (m *model) add(text string) {
	for line := range splitLines(text) {
		m.lines = append(m.lines, line)
	}
}

// pens returns the style each line starts in, read line after line.
func (m *model) pens() []pen {
	out := make([]pen, len(m.lines))
	var at pen
	for i, s := range m.lines {
		out[i] = at
		if strings.IndexByte(s, ansi.ESC) >= 0 {
			at = penAfter(s, at)
		}
	}
	return out
}

// modelKeys are the widths and modes the rows are checked at.
var modelKeys = []rowKey{
	{ansi.GraphemeWidth, Words, 1},
	{ansi.GraphemeWidth, Words, 7},
	{ansi.GraphemeWidth, Chars, 13},
	{ansi.GraphemeWidth, Words, 40},
	{ansi.WcWidth, Chars, 9},
	{ansi.GraphemeWidth, None, 9},
}

// checkStore fails t unless s holds m's lines, and works out of them what
// m does, reading them in an order rng picks.
func checkStore(t *testing.T, step string, s *Store, m *model, rng *rand.Rand) {
	t.Helper()
	n := len(m.lines)
	if s.Len() != n {
		t.Fatalf("%s: %d lines, want %d", step, s.Len(), n)
	}
	checkTree(t, step, s.root, true, m)
	if got, want := s.String(), strings.Join(m.lines, "\n"); got != want {
		t.Fatalf("%s: String differs", step)
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	switch rng.IntN(3) {
	case 0:
		slices.Reverse(order)
	case 1:
		rng.Shuffle(n, func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	pens := m.pens()
	for _, i := range order {
		if got := s.Line(i); got != m.lines[i] {
			t.Fatalf("%s: line %d is %q, want %q", step, i, got, m.lines[i])
		}
		if p := s.penOf(i); !p.equal(&pens[i]) {
			t.Fatalf("%s: line %d starts in %+v, want %+v", step, i, p, pens[i])
		}
	}
	// Rows cut out of lines are drawn in the style the lines start in.
	for range min(n, 8) {
		i := order[rng.IntN(n)]
		line, method := m.lines[i], ansi.GraphemeWidth
		w := int(m.width(line, method) & widthMask)
		a := rng.IntN(w + 1)
		e := a + 1 + rng.IntN(60)
		want := cutRow(line, checkpoint{pen: pens[i]}, m.width(line, method), method, a, e, nil)
		sameCells(t, fmt.Sprintf("%s: line %d cut from %d to %d", step, i, a, e), s.CutRow(i, method, a, e, nil), want)
		if got, text := s.CellText(i, method, a, e), cellText(line, checkpoint{pen: pens[i]}, method, a, e); got != text {
			t.Fatalf("%s: line %d's text from %d to %d is %q, want %q", step, i, a, e, got, text)
		}
	}
	for _, method := range modelMethods {
		meas := s.Measure(method)
		widest := 0
		for _, i := range order {
			want := m.width(m.lines[i], method)
			if got := s.flags(i, method); got != want {
				t.Fatalf("%s: line %d measured %x, want %x", step, i, got, want)
			}
			if meas.Width(i) != int(want&widthMask) {
				t.Fatalf("%s: line %d is %d wide, want %d", step, i, meas.Width(i), want&widthMask)
			}
			widest = max(widest, int(want&widthMask))
		}
		if meas.Widest != widest {
			t.Fatalf("%s: widest %d, want %d", step, meas.Widest, widest)
		}
	}
	keys := slices.Clone(modelKeys)
	rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	for _, k := range keys {
		checkRows(t, step, s, m, k, order)
	}
}

// checkRows fails t unless the rows of s wrapped as k says are m's.
func checkRows(t *testing.T, step string, s *Store, m *model, k rowKey, order []int) {
	t.Helper()
	x := s.Index(k.method, k.mode, k.width)
	before := make([]int, len(m.lines)+1)
	for i, line := range m.lines {
		before[i+1] = before[i] + m.rowsOf(line, k)
	}
	total := before[len(m.lines)]
	if x.Total() != total {
		t.Fatalf("%s %+v: %d rows, want %d", step, k, x.Total(), total)
	}
	uniform := total == len(m.lines)
	if x.Uniform() && !uniform {
		t.Fatalf("%s %+v: uniform with %d rows of %d lines", step, k, total, len(m.lines))
	}
	for _, i := range order {
		rows := before[i+1] - before[i]
		if got := x.RowsOf(i); got != rows {
			t.Fatalf("%s %+v: line %d takes %d rows, want %d", step, k, i, got, rows)
		}
		if got := x.RowsBefore(i); got != before[i] {
			t.Fatalf("%s %+v: %d rows before line %d, want %d", step, k, got, i, before[i])
		}
		if line, sub := x.LineAt(before[i]); line != i || sub != 0 {
			t.Fatalf("%s %+v: row %d is in line %d row %d, want line %d", step, k, before[i], line, sub, i)
		}
		if line, sub := x.LineAt(before[i+1] - 1); line != i || sub != rows-1 {
			t.Fatalf("%s %+v: row %d is in line %d row %d, want line %d row %d", step, k, before[i+1]-1, line, sub, i, rows-1)
		}
	}
	if got := x.RowsBefore(len(m.lines)); got != total {
		t.Fatalf("%s %+v: %d rows before the end, want %d", step, k, got, total)
	}
}

// subtreeLines returns the lines of the tree under n.
func subtreeLines(n *node) []string {
	if n == nil {
		return nil
	}
	if n.leaf() {
		return slices.Clone(n.lines)
	}
	var out []string
	for _, kid := range n.kids {
		out = append(out, subtreeLines(kid)...)
	}
	return out
}

// sameChange reports whether changes a and b set the same fields to the
// same values.
func sameChange(a, b *penChange) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.set == b.set && a.attrs == b.attrs && a.pen.equal(&b.pen)
}

// checkTree fails t unless the tree under n is balanced, its nodes hold
// what they may, what they sum up is so, and each of their cache slots in
// use holds what working it out from scratch, as m does, does. It returns
// the lines under n, and how deep its leaves are.
func checkTree(t *testing.T, step string, n *node, root bool, m *model) (lines []string, depth int) {
	t.Helper()
	if n == nil {
		return nil, 0
	}
	switch {
	case n.leaf() && n.size() > leafMax, !n.leaf() && n.size() > fanMax:
		t.Fatalf("%s: node of height %d holds %d", step, n.height, n.size())
	case !root && !n.okChild():
		t.Fatalf("%s: node of height %d holds %d", step, n.height, n.size())
	case root && !n.leaf() && len(n.kids) < 2:
		t.Fatalf("%s: root of one child", step)
	}
	// What each line or child is, worked out from scratch.
	var parts [][]string
	if n.leaf() {
		lines = slices.Clone(n.lines)
		for _, s := range lines {
			parts = append(parts, []string{s})
		}
	} else {
		depth = -1
		for _, kid := range n.kids {
			if kid.height != n.height-1 {
				t.Fatalf("%s: child of height %d under %d", step, kid.height, n.height)
			}
			kl, d := checkTree(t, step, kid, false, m)
			if depth >= 0 && d != depth {
				t.Fatalf("%s: leaves at depths %d and %d", step, depth, d)
			}
			depth = d
			parts = append(parts, kl)
			lines = append(lines, kl...)
		}
		depth++
	}
	sum := func(lines []string) (bytes int, pen *penChange) {
		for _, s := range lines {
			bytes += len(s) + 1
			pen = compose(pen, m.change(s))
		}
		return bytes, pen
	}
	bytes, pen := sum(lines)
	switch {
	case n.n != len(lines):
		t.Fatalf("%s: node of %d lines says %d", step, len(lines), n.n)
	case n.bytes != bytes:
		t.Fatalf("%s: node of %d bytes says %d", step, bytes, n.bytes)
	case !sameChange(n.pen, pen):
		t.Fatalf("%s: node's change to the style differs", step)
	}
	for k, part := range parts {
		b, p := sum(part)
		if n.leaf() {
			var kept *penChange
			if len(n.pens) > 0 {
				kept = n.pens[k]
			}
			if (n.pens == nil) != (n.pen == nil) || !sameChange(kept, p) {
				t.Fatalf("%s: leaf keeps line %d's change to the style wrong", step, k)
			}
			continue
		}
		kid := n.kids[k]
		if n.kidLines[k] != kid.n || n.kidLines[k] != len(part) || n.kidBytes[k] != b {
			t.Fatalf("%s: node keeps child %d's sums wrong", step, k)
		}
		var kept *penChange
		if len(n.kidPens) > 0 {
			kept = n.kidPens[k]
		}
		if kept != kid.pen || !sameChange(kept, p) {
			t.Fatalf("%s: node keeps child %d's change to the style wrong", step, k)
		}
	}
	for j := range n.m {
		ms := &n.m[j]
		if !ms.ok {
			continue
		}
		if len(ms.w) != n.size() {
			t.Fatalf("%s: node keeps %d widths of %d", step, len(ms.w), n.size())
		}
		widest := 0
		for k, part := range parts {
			w := int32(0)
			for _, s := range part {
				w = max(w, m.width(s, ms.method)&widthMask)
			}
			if n.leaf() && ms.w[k] != m.width(part[0], ms.method) || !n.leaf() && ms.w[k] != w {
				t.Fatalf("%s: node keeps %d's width wrong", step, k)
			}
			widest = max(widest, int(w))
		}
		if ms.widest != widest {
			t.Fatalf("%s: node keeps widest %d, want %d", step, ms.widest, widest)
		}
	}
	for j := range n.r {
		rs := &n.r[j]
		if !rs.ok {
			continue
		}
		total := 0
		for k, part := range parts {
			rows := 0
			for _, s := range part {
				rows += m.rowsOf(s, rs.key)
			}
			switch {
			case n.leaf() && len(rs.r) > 0 && int(rs.r[k]) != rows,
				n.leaf() && len(rs.r) == 0 && rows != 1,
				!n.leaf() && rs.rk[k] != rows:
				t.Fatalf("%s: node keeps %d's rows at %+v wrong", step, k, rs.key)
			}
			total += rows
		}
		if rs.total != total {
			t.Fatalf("%s: node keeps %d rows at %+v, want %d", step, rs.total, rs.key, total)
		}
	}
	return lines, depth
}

// snap is a snapshot taken in a model test, and the lines it holds.
type snap struct {
	root  *node
	lines []string
}

// TestStoreModel drives a store and a slice of lines through long random
// runs of the changes a store takes, appending, clearing, setting its
// lines, taking snapshots, and splitting it and joining it back with lines
// replaced, and checks after each that the store holds the slice's lines
// and works out of them what is worked out from scratch.
func TestStoreModel(t *testing.T) {
	t.Parallel()

	for seed := range uint64(3) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			runModel(t, rand.New(rand.NewPCG(seed, 99)), 100)
		})
	}
}

func runModel(t *testing.T, rng *rand.Rand, steps int) {
	t.Helper()
	s, m := &Store{}, newModel()
	var snaps []snap
	height := 0
	lines := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = randomLine(rng)
		}
		return out
	}
	for i := range steps {
		var step string
		switch op := rng.IntN(26); {
		case op < 6:
			step = "append a line"
			line := randomLine(rng)
			s.AppendText(line)
			m.lines = append(m.lines, line)
		case op < 9:
			step = "append lines"
			text := strings.Join(lines(rng.IntN(leafMax*fanMax/2)), "\n")
			s.AppendLines(text)
			m.add(text)
		case op < 10:
			step = "append lines one at a time"
			add := lines(rng.IntN(3 * leafMax))
			for _, l := range add {
				s.AppendText(l)
			}
			m.lines = append(m.lines, add...)
		case op < 11:
			step = "clear"
			s.Clear()
			m.lines = nil
		case op < 12:
			step = "set content"
			text := strings.Join(lines(rng.IntN(2*leafMax*fanMax)), "\n")
			s.Clear()
			m.lines = nil
			if text != "" {
				s.AppendText(text)
				m.add(text)
			}
		case op < 14:
			step = "snapshot"
			snaps = append(snaps, snap{s.snapshot(), slices.Clone(m.lines)})
		case op < 16 && len(snaps) > 0:
			// A store made of a snapshot reads it, and changes it without
			// changing the snapshot, or the store it was taken of.
			step = "change a snapshot"
			sn := snaps[rng.IntN(len(snaps))]
			o := &Store{root: sn.root}
			om := *m
			om.lines = slices.Clone(sn.lines)
			add := randomLine(rng)
			o.AppendText(add)
			om.lines = append(om.lines, add)
			checkStore(t, step, o, &om, rng)
		case op < 18 && len(snaps) > 0:
			step = "restore"
			sn := snaps[rng.IntN(len(snaps))]
			s.Restore(Snapshot{root: sn.root})
			m.lines = slices.Clone(sn.lines)
		case op < 24:
			// As an edit splices lines: a few, near each other, in place
			// in a leaf when they are in one, or many.
			step = "splice lines"
			a := rng.IntN(len(m.lines) + 1)
			b := a + rng.IntN(min(len(m.lines)-a, 3)+1)
			add := lines(rng.IntN(4))
			switch rng.IntN(8) {
			case 0:
				add = lines(rng.IntN(leafMax + 1))
			case 1:
				b = a + rng.IntN(min(len(m.lines)-a, 2*leafMax)+1)
			case 2:
				add = lines(leafMax + rng.IntN(3*leafMax))
			}
			s.splice(a, b, add)
			s.changed()
			m.lines = slices.Concat(m.lines[:a], add, m.lines[b:])
		default:
			step = "splice"
			a := rng.IntN(len(m.lines) + 1)
			b := a + rng.IntN(len(m.lines)-a+1)
			if rng.IntN(2) == 0 {
				b = a + rng.IntN(min(len(m.lines)-a, 3)+1)
			}
			add := lines(rng.IntN(3))
			if rng.IntN(4) == 0 {
				add = lines(rng.IntN(leafMax * fanMax))
			}
			l, rest := s.split(s.root, a)
			_, r := s.split(rest, b-a)
			s.root = s.join(s.join(l, s.build(add)), r)
			s.changed()
			m.lines = slices.Concat(m.lines[:a], add, m.lines[b:])
		}
		checkStore(t, fmt.Sprintf("step %d, %s", i, step), s, m, rng)
		if s.root != nil {
			height = max(height, s.root.height)
		}
		for k, sn := range snaps {
			got, _ := checkTree(t, fmt.Sprintf("step %d, %s, snapshot %d", i, step, k), sn.root, true, m)
			if !slices.Equal(got, sn.lines) {
				t.Fatalf("step %d, %s: snapshot %d changed", i, step, k)
			}
		}
	}
	if height < 2 {
		t.Fatalf("the tree grew only %d deep", height)
	}
}

// TestSnapshotUnchanged checks that a snapshot is not changed by lines
// appended after it, one at a time or many, nor by clearing the store,
// nor by working out what the store keeps of its lines.
func TestSnapshotUnchanged(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(11, 12))
	var s Store
	var want []string
	for range 3 * leafMax * fanMax {
		line := randomLine(rng)
		s.AppendText(line)
		want = append(want, line)
	}
	s.Index(ansi.GraphemeWidth, Words, 20)
	root := s.snapshot()
	n, bytes, pen := root.n, root.bytes, root.pen
	for range 500 {
		s.AppendText(randomLine(rng))
	}
	s.AppendLines(strings.Repeat("more\n", 5000))
	s.Index(ansi.GraphemeWidth, Words, 9)
	s.Index(ansi.WcWidth, Chars, 11)
	if got := subtreeLines(root); !slices.Equal(got, want) {
		t.Fatal("appending changed the snapshot")
	}
	s.Clear()
	s.AppendLines(strings.Repeat("again\n", 5000))
	if got := subtreeLines(root); !slices.Equal(got, want) {
		t.Fatal("clearing changed the snapshot")
	}
	if root.n != n || root.bytes != bytes || root.pen != pen {
		t.Fatal("the snapshot's sums changed")
	}
	checkTree(t, "snapshot", root, true, newModel())
	// Appending with no snapshot taken changes the nodes in place.
	before := s.root
	s.AppendText("in place")
	if s.root != before {
		t.Fatal("appending without a snapshot copied the root")
	}
}

// TestPenChanges checks that the changes lines make to the style, composed
// in any grouping and applied to a pen, give the style penAfter reads from
// the lines one after another.
func TestPenChanges(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(13, 14))
	starts := []pen{
		{},
		unsetPen,
		{style: uv.Style{Fg: ansi.Red, Attrs: uv.AttrBold | uv.AttrItalic, Underline: ansi.UnderlineCurly}},
		{style: uv.Style{Bg: ansi.IndexedColor(33), UnderlineColor: ansi.Blue}, link: uv.Link{URL: "https://z.dev"}},
	}
	for range 2000 {
		lines := make([]string, 1+rng.IntN(8))
		for i := range lines {
			var b strings.Builder
			for range rng.IntN(6) {
				b.WriteString(lineParts[rng.IntN(len(lineParts))])
			}
			lines[i] = b.String()
		}
		changes := make([]*penChange, len(lines))
		for i, s := range lines {
			changes[i] = lineChange(s)
		}
		for _, start := range starts {
			want := start
			for _, s := range lines {
				want = penAfter(s, want)
			}
			// Folded left, folded right, all at once, and split anywhere.
			var left *penChange
			for _, c := range changes {
				left = compose(left, c)
			}
			var right *penChange
			for _, c := range slices.Backward(changes) {
				right = compose(c, right)
			}
			at := rng.IntN(len(changes) + 1)
			parts := compose(composeAll(changes[:at]), composeAll(changes[at:]))
			for _, c := range []*penChange{left, right, composeAll(changes), parts} {
				got := start
				c.apply(&got)
				if !got.equal(&want) {
					t.Fatalf("lines %q from %+v: composed %+v, read %+v", lines, start, got, want)
				}
			}
			// One line at a time.
			got := start
			for _, c := range changes {
				c.apply(&got)
			}
			if !got.equal(&want) {
				t.Fatalf("lines %q from %+v: applied %+v, read %+v", lines, start, got, want)
			}
		}
	}
	if lineChange("plain text") != nil {
		t.Fatal("a line without escapes changes the style")
	}
	if lineChange("\x1b[2m0000001\x1b[m GET /") != styleReset {
		t.Fatal("a line ending in a reset does not share the reset")
	}
}

// TestSpliceSettles checks the edits within one leaf that leave it holding
// too much or too little, which long random runs come to only now and then:
// a root leaf split under a new root, a leaf merged with its neighbor and
// split again, as their sum is too much for one, and merged into the one
// leaf left, which takes the root's place.
func TestSpliceSettles(t *testing.T) {
	t.Parallel()

	lines := func(from, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("line %d", from+i)
		}
		return out
	}
	rng := rand.New(rand.NewPCG(1, 2))

	s, m := &Store{}, newModel()
	m.lines = lines(0, leafMax-4)
	s.AppendLines(m.lines...)
	add := lines(1000, 10)
	s.splice(30, 30, add)
	s.changed()
	m.lines = slices.Concat(m.lines[:30], add, m.lines[30:])
	if s.root.leaf() {
		t.Fatalf("a root leaf of %d lines was not split", s.root.size())
	}
	checkStore(t, "root split", s, m, rng)

	s, m = &Store{}, newModel()
	m.lines = lines(0, 2*leafMax)
	s.AppendLines(m.lines...)
	first, next := s.root.kids[0].size(), s.root.kids[1].size()
	if leafMin-1+next <= leafMax {
		t.Fatalf("leaves of %d and %d lines: too few to split again once merged", first, next)
	}
	cut := first - (leafMin - 1)
	s.splice(1, 1+cut, nil)
	s.changed()
	m.lines = slices.Concat(m.lines[:1], m.lines[1+cut:])
	if got := s.root.kids[0].size(); got < leafMin || got == first-cut {
		t.Fatalf("the first leaf holds %d lines, want it merged and split", got)
	}
	checkStore(t, "merged and split", s, m, rng)

	// Merged into one leaf that holds them, the root of it alone is
	// replaced by the leaf.
	s, m = &Store{}, newModel()
	m.lines = lines(0, leafMax+leafMin)
	s.AppendLines(m.lines...)
	first, next = s.root.kids[0].size(), s.root.kids[1].size()
	if len(s.root.kids) != 2 || leafMin-1+next > leafMax {
		t.Fatalf("%d leaves, of %d and %d lines: too many to merge into one", len(s.root.kids), first, next)
	}
	cut = first - (leafMin - 1)
	s.splice(1, 1+cut, nil)
	s.changed()
	m.lines = slices.Concat(m.lines[:1], m.lines[1+cut:])
	if !s.root.leaf() {
		t.Fatal("a root of one child was kept")
	}
	checkStore(t, "merged into the root", s, m, rng)
}

// TestDeepTree checks a tree four levels deep: loaded at once, appended
// to, split and joined, read in order and out of it.
func TestDeepTree(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(15, 16))
	m := newModel()
	m.lines = make([]string, leafMax*fanMax*fanMax+1234)
	for i := range m.lines {
		m.lines[i] = fmt.Sprintf("\x1b[2m%07d\x1b[m line %s", i, strings.Repeat("word ", i%17))
	}
	var s Store
	s.AppendLines(m.lines...)
	if s.root.height < 3 {
		t.Fatalf("a tree of %d lines is %d deep", len(m.lines), s.root.height)
	}
	for range 3000 {
		line := randomLine(rng)
		s.AppendText(line)
		m.lines = append(m.lines, line)
	}
	for _, at := range []int{0, 1, leafMax*fanMax + 5, len(m.lines) / 2, len(m.lines) - 7} {
		l, r := s.split(s.root, at)
		checkTree(t, fmt.Sprintf("left of %d", at), l, true, m)
		checkTree(t, fmt.Sprintf("right of %d", at), r, true, m)
		s.root = s.join(l, r)
		s.changed()
	}
	// Cutting out half the lines, in the middle.
	a, b := len(m.lines)/4, 3*len(m.lines)/4
	l, rest := s.split(s.root, a)
	_, r := s.split(rest, b-a)
	s.root = s.join(l, r)
	s.changed()
	m.lines = slices.Concat(m.lines[:a], m.lines[b:])
	order := make([]int, len(m.lines))
	for i := range order {
		order[i] = i
	}
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	pens := m.pens()
	for _, i := range order[:5000] {
		if s.Line(i) != m.lines[i] {
			t.Fatalf("line %d differs", i)
		}
		if p := s.penOf(i); !p.equal(&pens[i]) {
			t.Fatalf("line %d starts in %+v, want %+v", i, p, pens[i])
		}
	}
	checkRows(t, "deep", &s, m, rowKey{ansi.GraphemeWidth, Words, 30}, order[:5000])
	checkTree(t, "deep", s.root, true, m)
}

// BenchmarkStoreAppend appends a line at a time to a store of a million.
func BenchmarkStoreAppend(b *testing.B) {
	var s Store
	s.AppendLines(benchStoreLines(1_000_000)...)
	s.Index(ansi.GraphemeWidth, Words, 120)
	line := benchStoreLine(7)
	b.ReportAllocs()
	for b.Loop() {
		s.AppendText(line)
	}
}

// BenchmarkStoreLine looks up lines of a store of a million: each line
// after the last, and lines anywhere.
func BenchmarkStoreLine(b *testing.B) {
	var s Store
	s.AppendLines(benchStoreLines(1_000_000)...)
	b.Run("next", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_ = s.Line(i)
			_ = s.penOf(i)
			i = (i + 1) % s.Len()
		}
	})
	b.Run("anywhere", func(b *testing.B) {
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			i := rng.IntN(s.Len())
			_ = s.Line(i)
			_ = s.penOf(i)
		}
	})
	x := s.Index(ansi.GraphemeWidth, Words, 120)
	b.Run("row", func(b *testing.B) {
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			x.LineAt(rng.IntN(x.Total()))
		}
	})
}

func benchStoreLine(i int) string {
	return fmt.Sprintf("\x1b[2m%07d\x1b[m GET /api/v1/items/%d served in %dms %s", i, i*7, i%97, strings.Repeat("lorem ipsum ", 3+i%24))
}

// benchStoreLines returns the first n lines benchStoreLine makes.
func benchStoreLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = benchStoreLine(i)
	}
	return lines
}

// TestEditKeepsRoot checks that an edit within a leaf keeps what the root
// keeps of the lines, so that measuring the store after it costs a step,
// and that one across leaves leaves the root to work out again from the
// nodes it made, the rest kept.
func TestEditKeepsRoot(t *testing.T) {
	t.Parallel()

	var s Store
	s.AppendLines(benchStoreLines(100_000)...)
	s.Index(ansi.GraphemeWidth, Words, 30)
	for _, e := range []TextEdit{
		{Pos{500, 3}, Pos{500, 9}, "changed"},
		{Pos{70_000, 0}, Pos{70_000, 0}, "a line put in\n"},
		{Pos{30_000, 0}, Pos{30_001, 0}, ""},
		{Pos{9_000, 4}, Pos{9_000, 4}, strings.Repeat(" wider than the rest", 20)},
	} {
		s.Replace(e)
		if !s.Kept(ansi.GraphemeWidth, Words, 30) {
			t.Fatalf("%+v: the root measures its lines again", e)
		}
	}
	s.Replace(TextEdit{From: Pos{100, 2}, To: Pos{90_000, 1}})
	if s.Kept(ansi.GraphemeWidth, Words, 30) {
		t.Fatal("a root made by joining subtrees keeps rows it did not work out")
	}
	// Working them out again visits the nodes the edit made, which are few.
	measured := 0
	gather(s.root, 0, func(n *node) bool {
		if n.findM(ansi.GraphemeWidth) == nil {
			measured++
			return true
		}
		return false
	}, nil)
	if measured > 4*(s.root.height+1) {
		t.Fatalf("%d nodes to measure again", measured)
	}
}

func TestRowsKeptAreThoseUsedLast(t *testing.T) {
	t.Parallel()

	// Two viewports of a store, one of them changing width: the rows of the
	// other are asked for between each of its widths, and kept.
	var s Store
	s.AppendLines(benchStoreLines(10_000)...)
	const steady = 30
	for _, w := range []int{20, 21, 22, 23} {
		s.Index(ansi.GraphemeWidth, Words, steady)
		s.Index(ansi.GraphemeWidth, Words, w)
		if !s.Kept(ansi.GraphemeWidth, Words, steady) {
			t.Fatalf("width %d: the rows asked for just before were not kept", w)
		}
		// Down to the leaves, so that none of it is wrapped again.
		leaf, _ := s.find(5_000)
		if leaf.findR(rowKey{ansi.GraphemeWidth, Words, steady}) == nil {
			t.Fatalf("width %d: a leaf lost the rows asked for just before", w)
		}
	}
}

func TestCompactAfterMostIsRemoved(t *testing.T) {
	t.Parallel()

	// Once most of a large text is removed, the tree is copied, with its
	// lines, so that the storage it was built in, and the text its lines
	// were split from, are the store's to free.
	lines := benchStoreLines(50_000)
	text := strings.Join(lines, "\n")
	var s Store
	s.AppendLines(text)
	built := s.slab
	if built == nil || len(text) < 4*compactFrom {
		t.Fatalf("a text of %d bytes, built in a slab: %v", len(text), built != nil)
	}
	before := s.Snapshot()
	s.Replace(TextEdit{From: Pos{0, 0}, To: Pos{0, 0}, Text: "x"})
	if s.slab != built {
		t.Fatal("an edit that removes little copied the tree")
	}
	s.Replace(TextEdit{From: Pos{10, 0}, To: Pos{s.Len() - 10, 0}})
	want := "x" + strings.Join(lines[:10], "\n") + "\n" + strings.Join(lines[len(lines)-10:], "\n")
	if s.String() != want {
		t.Fatalf("the text left is %d bytes, want %d", len(s.String()), len(want))
	}
	if s.slab != nil || s.most != s.root.bytes {
		t.Fatalf("the tree was not copied: most %d of %d bytes", s.most, s.root.bytes)
	}
	// Its lines are its own, rather than parts of the text.
	first := s.Line(1)
	if at := strings.Index(text, first); at < 0 || unsafe.StringData(first) == unsafe.StringData(text[at:]) {
		t.Fatal("a line left is still part of the text it was split from")
	}
	// The snapshot taken before keeps the text as it was, the store can be
	// edited on, and set back to it.
	if got := before.String(); got != text {
		t.Fatalf("the snapshot holds %d bytes, want %d", len(got), len(text))
	}
	s.Replace(TextEdit{From: Pos{5, 0}, To: Pos{5, 0}, Text: "more\n"})
	if s.Len() != 21 || s.Line(5) != "more" {
		t.Fatalf("after an edit, %d lines, line 5 %q", s.Len(), s.Line(5))
	}
	s.Restore(before)
	if s.String() != text {
		t.Fatal("restored, the text is not as it was")
	}
}
