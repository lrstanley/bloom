// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"iter"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/charmbracelet/x/ansi"
)

// Store holds the lines of a buffer, and what it works out about them, such
// as how wide each is, so that appending a line to a store of a million
// costs about as much as appending one to a store of ten. A style left open
// at the end of a line carries on into the next.
//
// The lines are kept in a balanced tree, whose nodes keep what they sum up
// about their lines, such as how many rows they wrap to, so that finding a
// line or a row costs a node a level, and keep what is worked out about
// their lines, so that a change works out again only the lines it changed.
//
// The zero value is an empty store, ready to use. A Store is not safe for
// use by more than one goroutine at a time.
type Store struct {
	root *node
	tok  *token       // owns the nodes that may be changed in place
	rev  uint64       // counts changes
	fin  cursor       // the leaf last looked up, valid while finOK
	work work         // scratch for working out what the nodes keep
	pool []*work      // and for each processor sharing the work
	todo []*node      // scratch for the subtrees they share
	long [4]*longLine // the long lines last shown, most recent first
	rows [2]rowKey    // how the rows last asked for were wrapped, most recent first

	// The storage of the tree last built, and of one cleared, which the
	// next tree built may take over.
	slab, spare *slabs

	snapped *node // the root last shared with a snapshot

	// most is the most bytes the lines were before an edit, since the tree
	// was last copied whole, to copy it again once most of them are
	// removed; see compact.
	most int

	finOK bool
}

// longLine is what a buffer keeps of a line at least LongLineBytes long
// that a viewport shows: its checkpoints, measured with method, as far
// along it as they were wanted, or to its end once done, and the cells its
// rows start at when last wrapped, to width with mode. It is found by the
// line's text, wherever the line is, so that it is kept as lines are added
// and removed before it, and by the style the line starts in, its first
// checkpoint's.
type longLine struct {
	data   *byte
	size   int
	method ansi.Method
	cps    []checkpoint
	done   bool
	width  int
	mode   Mode
	starts []int
}

// reach adds checkpoints to l, of line s, until there is one within
// CheckpointEvery cells before cell a, or the line ends, scanning on from
// the last one rather than from the line's start. With byText, a is a byte
// of the line's text rather than a cell, and checkpoints are added until
// one is past it.
func (l *longLine) reach(s string, a int, byText bool) {
	last := l.cps[len(l.cps)-1]
	enough := func(cp checkpoint) bool {
		if byText {
			return cp.t > a
		}
		return a < cp.x+CheckpointEvery
	}
	if l.done || enough(last) {
		return
	}
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	sc := last.scanner(s, l.method, p)
	// A checkpoint starts at the first command before its glyph, if there
	// is one, so that a trusted viewport draws it with the glyph.
	sc.pass = true
	cmd, cmdText := -1, 0
	var cmdPen pen
	next, text := last.x+CheckpointEvery, last.t
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		if k == glyphPass && cmd < 0 {
			cmd, cmdPen, cmdText = sc.at, sc.pen, text
		}
		if k != glyphText {
			continue
		}
		if sc.x >= next && sc.w > 0 && !sc.tab {
			cp := checkpoint{x: sc.x, i: sc.at, t: text, pen: sc.pen}
			if cmd >= 0 {
				cp.i, cp.pen, cp.t = cmd, cmdPen, cmdText
			}
			l.cps = append(l.cps, cp)
			if enough(cp) {
				return
			}
			next = sc.x + CheckpointEvery
		}
		text += textBytes(&sc)
		if sc.w > 0 {
			cmd = -1
		}
	}
	l.done = true
}

// longLine returns what the buffer keeps of line i, a long line, measured
// with method, working it out the first time it is asked for.
func (b *Store) longLine(i int, method ansi.Method) *longLine {
	s, start := b.Line(i), b.penOf(i)
	data := unsafe.StringData(s)
	for k, l := range b.long {
		if l != nil && l.data == data && l.size == len(s) && l.method == method && l.cps[0].pen.equal(&start) {
			copy(b.long[1:k+1], b.long[:k])
			b.long[0] = l
			return l
		}
	}
	l := &longLine{data: data, size: len(s), method: method, cps: []checkpoint{{pen: start}}}
	copy(b.long[1:], b.long[:len(b.long)-1])
	b.long[0] = l
	return l
}

// cutFrom returns where to scan line i from, measured with method, to reach
// cell a: its start, or the checkpoint before a of a long line.
func (b *Store) cutFrom(i int, method ansi.Method, a int) checkpoint {
	s := b.Line(i)
	if len(s) < LongLineBytes || a < CheckpointEvery {
		return checkpoint{pen: b.penOf(i)}
	}
	l := b.longLine(i, method)
	l.reach(s, a, false)
	k, _ := slices.BinarySearchFunc(l.cps, a+1, func(cp checkpoint, x int) int { return cp.x - x })
	return l.cps[max(k-1, 0)]
}

// RowStarts returns the cells the rows of line i start at, wrapped to w
// with mode and measured with method: worked out in scratch, or kept for a
// long line, which must not be changed.
func (b *Store) RowStarts(i int, method ansi.Method, w int, mode Mode, scratch *[]int) []int {
	s := b.Line(i)
	if len(s) < LongLineBytes {
		_, *scratch = wrapLine(s, method, w, mode, (*scratch)[:0])
		return *scratch
	}
	l := b.longLine(i, method)
	if l.starts == nil || l.width != w || l.mode != mode {
		_, l.starts = wrapLine(s, method, w, mode, l.starts[:0])
		l.width, l.mode = w, mode
	}
	return l.starts
}

// changed notes a change to the lines.
func (b *Store) changed() {
	b.rev++
	b.finOK = false
}

// Clear removes every line.
func (b *Store) Clear() {
	if b.slab != nil {
		if b.slab.tok == b.tok {
			b.spare = b.slab
		}
		b.slab = nil
	}
	b.root, b.snapped, b.most = nil, nil, 0
	b.long = [4]*longLine{}
	b.changed()
}

// AppendText adds the lines of s, split at its newlines, which is one
// empty line for "". A newline at the end of s ends its last line.
func (b *Store) AppendText(s string) { b.AppendLines(s) }

// AppendLines adds the lines of each of texts in turn, as [Store.AppendText]
// does: a few a line at a time, and many made into a tree of their own,
// joined to the end of the lines.
func (b *Store) AppendLines(texts ...string) {
	b.changed()
	count := 0
	for _, s := range texts {
		count += strings.Count(s, "\n") + 1
		if count > leafMax {
			break
		}
	}
	if count <= leafMax {
		for _, s := range texts {
			for line := range splitLines(s) {
				b.appendLine(line)
			}
		}
		return
	}
	count = 0
	for _, s := range texts {
		count += strings.Count(s, "\n") + 1
	}
	lines := b.lineSlab(count)
	for _, s := range texts {
		for line := range splitLines(s) {
			lines = append(lines, line)
		}
	}
	b.root = b.join(b.root, b.build(lines))
}

// splitLines returns the lines of s, split at its newlines, which is one
// empty line for "", without the carriage return before a newline. A
// newline at the end of s ends its last line.
func splitLines(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		rest := strings.TrimSuffix(s, "\n")
		for {
			line, after, more := strings.Cut(rest, "\n")
			if !yield(strings.TrimSuffix(line, "\r")) || !more {
				return
			}
			rest = after
		}
	}
}

// appendLine adds one line at the end.
func (b *Store) appendLine(s string) {
	c := lineChange(s)
	if b.root == nil {
		b.root = b.newLeaf(make([]string, 0, leafMax+1), nil)
	}
	root, split := b.push(b.root, s, c)
	if split != nil {
		root = b.newInner(append(make([]*node, 0, fanMax+1), root, split))
	}
	b.root = root
}

// Len returns the number of lines.
func (b *Store) Len() int {
	if b.root == nil {
		return 0
	}
	return b.root.n
}

// Line returns line i as it was given, with its styles.
func (b *Store) Line(i int) string {
	leaf, j := b.find(i)
	return leaf.lines[j]
}

// String returns the lines, joined with newlines.
func (b *Store) String() string { return joinLines(b.root) }

// Rev counts the changes to the store, for those that keep what they worked
// out from its lines.
func (b *Store) Rev() uint64 { return b.rev }

// CutRow returns the cells from a up to end of line i, measured with
// method, as a string that draws them on its own, as [cutRow] does.
func (b *Store) CutRow(i int, method ansi.Method, a, end int, st *RowStyle) string {
	leaf, j := b.find(i)
	s, flags := leaf.lines[j], measureNode(leaf, method, &b.work).w[j]
	return cutRow(s, b.cutFrom(i, method, a), flags, method, a, end, st)
}

// CellText returns the text of the glyphs of line i that start in the cells
// from a to end, inclusive, as [cellText] does.
func (b *Store) CellText(i int, method ansi.Method, a, end int) string {
	return cellText(b.Line(i), b.cutFrom(i, method, a), method, a, end)
}

// cursor is a place in the tree of lines: a leaf, the path to it from the
// root, and the line its first line is, to read the lines in order a leaf
// at a time. It may keep the style a line of the leaf starts in, worked out
// from the lines before it, which it carries on to the next leaf.
type cursor struct {
	path  [maxDepth]step
	depth int
	leaf  *node
	start int
	penAt int // the line pen is the style of, or -1 for none
	pen   pen
}

// step is a node on the path to a leaf, and the child of it the path takes.
type step struct {
	n *node
	k int
}

// seek puts c at the leaf holding line i of the tree under root.
func (c *cursor) seek(root *node, i int) {
	c.depth, c.start, c.penAt = 0, 0, -1
	n := root
	for !n.leaf() {
		k, in := n.locate(i)
		c.path[c.depth] = step{n, k}
		c.depth++
		c.start += i - in
		i = in
		n = n.kids[k]
	}
	c.leaf = n
}

// next moves c to the next leaf, and reports whether there is one.
func (c *cursor) next() bool {
	d := c.depth - 1
	for d >= 0 && c.path[d].k+1 >= len(c.path[d].n.kids) {
		d--
	}
	if d < 0 {
		return false
	}
	if c.penAt >= 0 {
		// The style the next leaf starts in.
		if c.leaf.pens != nil {
			for _, p := range c.leaf.pens[c.penAt-c.start:] {
				p.apply(&c.pen)
			}
		}
		c.penAt = c.start + len(c.leaf.lines)
	}
	c.start += len(c.leaf.lines)
	c.path[d].k++
	n := c.path[d].n.kids[c.path[d].k]
	for d++; d < c.depth; d++ {
		c.path[d] = step{n, 0}
		n = n.kids[0]
	}
	c.leaf = n
	return true
}

// find returns the leaf holding line i, and the line of the leaf it is,
// from the leaf last looked up when it is that one or the next.
func (b *Store) find(i int) (leaf *node, j int) {
	c := &b.fin
	if b.finOK {
		if k := i - c.start; k >= 0 && k < len(c.leaf.lines) {
			return c.leaf, k
		}
		if i == c.start+len(c.leaf.lines) && c.next() {
			return c.leaf, i - c.start
		}
	}
	c.seek(b.root, i)
	b.finOK = true
	return c.leaf, i - c.start
}

// penOf returns the style line i starts in.
func (b *Store) penOf(i int) pen {
	if b.root == nil || b.root.pen == nil {
		return pen{}
	}
	leaf, j := b.find(i)
	c := &b.fin
	if c.penAt >= c.start && c.penAt <= i {
		if leaf.pens != nil {
			for _, p := range leaf.pens[c.penAt-c.start : j] {
				p.apply(&c.pen)
			}
		}
	} else {
		root := b.root.pen
		var acc penChange
		penBefore(b.root, i, &acc, root.set, root.attrs)
		c.pen = acc.pen
	}
	c.penAt = i
	return c.pen
}

// penBefore fills in acc, a change, with the changes of the lines before
// line i of the tree under n, the latest first, until it sets each field
// and attribute bit that set and attrs name, and reports whether it does.
// Those are the ones some line of the store sets: the style a line starts
// in is acc applied to the zero pen once it does, or once it has every
// line before it.
func penBefore(n *node, i int, acc *penChange, set, attrs uint8) bool {
	if n.leaf() {
		if n.pens == nil {
			return false
		}
		for k := i - 1; k >= 0; k-- {
			if acc.fill(n.pens[k]) && acc.covers(set, attrs) {
				return true
			}
		}
		return false
	}
	k, in := n.locate(i)
	if penBefore(n.kids[k], in, acc, set, attrs) {
		return true
	}
	if n.kidPens == nil {
		return false
	}
	for k--; k >= 0; k-- {
		if acc.fill(n.kidPens[k]) && acc.covers(set, attrs) {
			return true
		}
	}
	return false
}

// Measure is how wide the lines of a store are, measured with a width
// method, as they were when it was made.
type Measure struct {
	s      *Store
	method ansi.Method
	Widest int
}

// Measure returns how wide the store's lines are with method, measuring
// only the lines not measured yet: all of them the first time, in parallel.
func (b *Store) Measure(method ansi.Method) Measure {
	m := Measure{s: b, method: method}
	if b.root != nil {
		m.Widest = b.measureAll(method).widest
	}
	return m
}

// Width returns how many cells line i takes.
func (m Measure) Width(i int) int { return int(m.s.flags(i, m.method) & widthMask) }

// flags returns how many cells line i takes, measured with method, with
// what measuring it told of it in the bits above.
func (b *Store) flags(i int, method ansi.Method) int32 {
	leaf, j := b.find(i)
	return measureNode(leaf, method, &b.work).w[j]
}

// Index is how many rows the lines of a store take, wrapped to a width
// with a mode, as they were when it was made, and where in them the text
// is: the positions in the text that cells of the rows show, and the cells
// that positions are drawn at. Rows are counted over all of the lines,
// wrapped, from 0, and the cells of a row from its first.
type Index struct {
	s       *Store
	key     rowKey
	n       int // lines
	total   int // rows
	uniform bool
}

// Index returns how many rows the store's lines take, wrapped to width
// with mode and measured with method, working out only the lines not
// worked out yet for them: all of them the first time, in parallel.
func (b *Store) Index(method ansi.Method, mode Mode, width int) Index {
	m := b.Measure(method)
	x := Index{s: b, key: rowKey{method: method, mode: mode, width: width}, n: b.Len()}
	if mode == None || width <= 0 || m.Widest <= width {
		// One row a line, which needs nothing kept.
		x.uniform, x.total = true, x.n
		return x
	}
	if b.rows[0] != x.key {
		b.rows[1], b.rows[0] = b.rows[0], x.key
	}
	x.total = b.rowsAll(x.key)
	return x
}

// rowsKept returns how the rows asked for last, other than as k says, were
// wrapped: those a node keeps when it makes room for rows wrapped as k
// says, so that two viewports of the store, one of them changing width,
// do not have the other's rows worked out again and again.
func (b *Store) rowsKept(k rowKey) rowKey {
	if b.rows[0] == k {
		return b.rows[1]
	}
	return b.rows[0]
}

// Kept reports whether the store keeps how wide its lines are, measured
// with method, and how many rows they take, wrapped to width with mode, so
// that [Store.Measure] and [Store.Index] cost a step.
func (b *Store) Kept(method ansi.Method, mode Mode, width int) bool {
	if b.root == nil {
		return true
	}
	ms := b.root.findM(method)
	return ms != nil && (mode == None || width <= 0 || ms.widest <= width || b.root.findR(rowKey{method, mode, width}) != nil)
}

// RowsBefore returns how many rows the lines before line i take.
func (x Index) RowsBefore(i int) int {
	if x.uniform {
		return i
	}
	w := &x.s.work
	rows, n := 0, x.s.root
	for {
		rs := rowSlotOf(n, x.key, x.s.rowsKept(x.key), w)
		if rs == nil {
			// Every line of n is a row.
			return rows + i
		}
		if n.leaf() {
			if len(rs.r) == 0 {
				return rows + i
			}
			for _, r := range rs.r[:i] {
				rows += int(r)
			}
			return rows
		}
		k := 0
		for last := len(n.kids) - 1; k < last && i >= n.kidLines[k]; k++ {
			i -= n.kidLines[k]
			rows += rs.rk[k]
		}
		n = n.kids[k]
	}
}

// RowsOf returns how many rows line i takes.
func (x Index) RowsOf(i int) int {
	if x.uniform {
		return 1
	}
	leaf, j := x.s.find(i)
	rs := rowSlotOf(leaf, x.key, x.s.rowsKept(x.key), &x.s.work)
	if rs == nil || len(rs.r) == 0 {
		return 1
	}
	return int(rs.r[j])
}

// Total returns how many rows all the lines take.
func (x Index) Total() int { return x.total }

// Uniform reports whether every line takes one row.
func (x Index) Uniform() bool { return x.uniform }

// LineAt returns the line that row is in, and how far into the line's rows
// it is, for a row of the lines.
func (x Index) LineAt(row int) (line, sub int) {
	if x.uniform {
		return row, 0
	}
	if row >= x.total {
		return x.n, row - x.total
	}
	w := &x.s.work
	n := x.s.root
	for {
		rs := rowSlotOf(n, x.key, x.s.rowsKept(x.key), w)
		if rs == nil {
			return line + row, 0
		}
		if n.leaf() {
			if len(rs.r) == 0 {
				return line + row, 0
			}
			for _, r := range rs.r {
				if row < int(r) {
					return line, row
				}
				row -= int(r)
				line++
			}
			return line, row
		}
		k := 0
		for last := len(n.kids) - 1; k < last && row >= rs.rk[k]; k++ {
			row -= rs.rk[k]
			line += n.kidLines[k]
		}
		n = n.kids[k]
	}
}

// rowSlotOf returns the rows of n's lines wrapped as k says, worked out if
// n lacks them, or nil when every line of n is a row.
func rowSlotOf(n *node, k, keep rowKey, w *work) *rowSlot {
	if rs := n.findR(k); rs != nil {
		return rs
	}
	if measureNode(n, k.method, w).widest <= k.width {
		return nil
	}
	rowsNode(n, k, keep, w)
	return n.findR(k)
}

// measureAll returns the widths of the root's lines measured with method,
// measuring first, in parallel, the subtrees that lack them.
func (b *Store) measureAll(method ansi.Method) *measureSlot {
	if ms := b.root.findM(method); ms != nil {
		return ms
	}
	if !b.parallel(
		func(n *node) bool { return n.findM(method) == nil },
		func(n *node, w *work) { measureNode(n, method, w) }) {
		b.work.reserve(b.root.n)
	}
	return measureNode(b.root, method, &b.work)
}

// rowsAll returns how many rows the root's lines take wrapped as k says,
// working out first, in parallel, the subtrees that lack them.
func (b *Store) rowsAll(k rowKey) int {
	if rs := b.root.findR(k); rs != nil {
		return rs.total
	}
	keep := b.rowsKept(k)
	if !b.parallel(
		func(n *node) bool {
			ms := n.findM(k.method)
			return n.findR(k) == nil && (ms == nil || ms.widest > k.width)
		},
		func(n *node, w *work) { rowsNode(n, k, keep, w) }) {
		b.work.reserve(b.root.n)
	}
	return rowsNode(b.root, k, keep, &b.work)
}

// measureNode returns the widths of n's lines measured with method,
// measuring them, and those of its children, if it lacks them.
func measureNode(n *node, method ansi.Method, w *work) *measureSlot {
	if ms := n.findM(method); ms != nil {
		return ms
	}
	ms := n.takeM(method)
	if n.leaf() {
		ms.w = w.int32s(ms.w, len(n.lines))
		measureLines(n.lines, method, ms.w)
	} else {
		ms.w = w.int32s(ms.w, len(n.kids))
		for k, kid := range n.kids {
			ms.w[k] = int32(measureNode(kid, method, w).widest) //nolint:gosec // Bounded by widthMask.
		}
	}
	ms.widest, ms.ok = widestOf(ms.w), true
	return ms
}

// rowsNode returns how many rows n's lines take wrapped as k says, working
// them out, and those of its children, if it lacks them, unless every line
// of n is a row. A node with no room for them keeps those wrapped as keep
// says.
func rowsNode(n *node, k, keep rowKey, w *work) int {
	if rs := n.findR(k); rs != nil {
		return rs.total
	}
	ms := measureNode(n, k.method, w)
	if ms.widest <= k.width {
		return n.n
	}
	widths := ms.w
	rs := n.takeR(k, keep)
	if n.leaf() {
		rows := w.int32s(rs.r, len(n.lines))
		countRowsInto(n.lines, widths, k, rows, &w.starts)
		rs.r, rs.total = leafRows(rows, len(n.lines))
	} else {
		rs.rk = w.ints(rs.rk, len(n.kids))
		rs.total = 0
		for i, kid := range n.kids {
			rs.rk[i] = rowsNode(kid, k, keep, w)
			rs.total += rs.rk[i]
		}
	}
	rs.ok = true
	return rs.total
}

// work is scratch for working out what nodes keep, one for each goroutine
// doing so, with storage to hand out to the nodes, made a lot at a time.
type work struct {
	starts []int
	i32    []int32
	i64    []int
	// How much storage to make, at most, and how much was made last, as
	// each time it makes twice as much, up to the most.
	want32, want64 int
	made32, made64 int
}

// reserve has w make storage for the nodes of a tree of n lines as it
// needs it, in larger and larger slabs, up to enough for all of them.
func (w *work) reserve(n int) {
	w.want32, w.want64 = n+n/leafMax, n/leafMax+fanMax
	w.made32, w.made64 = 0, 0
}

// int32s returns n values of storage: s's own when it has room, or some of
// w's, which no other node has.
func (w *work) int32s(s []int32, n int) []int32 {
	if cap(s) >= n {
		return s[:n]
	}
	if len(w.i32) < n {
		w.made32 = max(n, min(2*w.made32, w.want32), 4*leafMax)
		w.i32 = make([]int32, w.made32)
	}
	s = w.i32[:n:n]
	w.i32 = w.i32[n:]
	return s
}

// ints is [work.int32s] for ints.
func (w *work) ints(s []int, n int) []int {
	if cap(s) >= n {
		return s[:n]
	}
	if len(w.i64) < n {
		w.made64 = max(n, min(2*w.made64, w.want64), 4*fanMax)
		w.i64 = make([]int, w.made64)
	}
	s = w.i64[:n:n]
	w.i64 = w.i64[n:]
	return s
}

// parallel calls fn, in parallel, for the subtrees of the tree that need
// says need it, splitting it into subtrees enough to share between the
// processors, once it holds a lot of lines, and reports whether it did. It
// leaves the nodes above them, which are few, to the caller.
func (b *Store) parallel(need func(*node) bool, fn func(n *node, w *work)) bool {
	workers := runtime.GOMAXPROCS(0)
	if b.root.n < parallelLines || workers < 2 {
		return false
	}
	grain := max(b.root.n/(4*workers), leafMax)
	tasks := gather(b.root, grain, need, b.todo[:0])
	b.todo = tasks
	defer clear(tasks)
	lines := 0
	for _, n := range tasks {
		lines += n.n
	}
	if lines < parallelLines || len(tasks) < 2 {
		return false
	}
	workers = min(workers, len(tasks))
	for len(b.pool) < workers {
		b.pool = append(b.pool, &work{})
	}
	var wg sync.WaitGroup
	for from, done, g := 0, 0, 1; from < len(tasks); g++ {
		// Each worker takes the tasks up to its share of the lines.
		to, got := from, 0
		for to < len(tasks) && (done+got < lines*g/workers || to == from) {
			got += tasks[to].n
			to++
		}
		part, w := tasks[from:to], b.pool[g-1]
		w.reserve(got)
		wg.Go(func() {
			for _, n := range part {
				fn(n, w)
			}
		})
		from, done = to, done+got
	}
	wg.Wait()
	return true
}

// gather appends to out, in order, the largest subtrees under n of grain
// lines or fewer, or leaves, that need says need working out.
func gather(n *node, grain int, need func(*node) bool, out []*node) []*node {
	if !need(n) {
		return out
	}
	if n.leaf() || n.n <= grain {
		return append(out, n)
	}
	for _, kid := range n.kids {
		out = gather(kid, grain, need, out)
	}
	return out
}

// measureLines sets widths to how wide each of lines is, measured with
// method, as [measureLine] returns it.
func measureLines(lines []string, method ansi.Method, widths []int32) {
	for i, s := range lines {
		widths[i] = measureLine(s, method)
	}
}

// parallelLines is how many lines there must be to measure, or to count
// the rows of, before the lines are shared between the processors, which is
// worth it for a buffer set or wrapped again at a new width, such as while
// the pane it is in animates, but not for a few lines appended.
const parallelLines = 16 << 10

// inParallel calls fn for spans of n lines, from lo up to hi, that cover all
// of them, sharing a lot of lines between the processors.
func inParallel(n int, fn func(lo, hi int)) {
	workers := min(runtime.GOMAXPROCS(0), n/(parallelLines/4))
	if n < parallelLines || workers < 2 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for lo := 0; lo < n; lo += chunk {
		hi := min(lo+chunk, n)
		wg.Go(func() { fn(lo, hi) })
	}
	wg.Wait()
}

// countRowsOf sets rows to how many rows each of lines, as wide as widths
// says, takes wrapped to width with mode.
func countRowsOf(lines []string, widths []int32, method ansi.Method, width int, mode Mode, rows []int32) {
	var starts []int
	countRowsInto(lines, widths, rowKey{method: method, mode: mode, width: width}, rows, &starts)
}

// countRowsInto is [countRowsOf] for lines wrapped as k says, with scratch
// for the rows' starts.
func countRowsInto(lines []string, widths []int32, k rowKey, rows []int32, scratch *[]int) {
	for i, s := range lines {
		rows[i] = int32(lineRows(s, widths[i], k, scratch)) //nolint:gosec // A line has far fewer rows than 1<<31.
	}
}

// lineRows returns how many rows line s, of width w as [measureLine]
// returns it, takes wrapped as k says, with scratch for the rows' starts.
func lineRows(s string, w int32, k rowKey, scratch *[]int) int {
	if int(w&widthMask) <= k.width || k.mode == None || k.width <= 0 {
		return 1
	}
	if k.mode == Chars && w&wideLine == 0 {
		return (int(w&widthMask) + k.width - 1) / k.width
	}
	n, starts := wrapLine(s, k.method, k.width, k.mode, (*scratch)[:0])
	*scratch = starts
	return n
}
