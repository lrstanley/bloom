// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"slices"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// The lines of a [Store] are kept in a B-tree: leaves hold up to leafMax
// lines, inner nodes up to fanMax children, and every node but the root
// holds at least the minimum, so that a tree of a million lines is four
// levels deep, and finding, changing, adding or removing a line anywhere
// touches a node a level.
const (
	leafMax = 64
	leafMin = leafMax / 4
	fanMax  = 32
	fanMin  = fanMax / 4

	// maxDepth is deeper than a tree of the most lines memory can hold,
	// each node at its minimum.
	maxDepth = 32
)

// token is what owns the nodes an edit may change in place. A node owned
// by another, older one is shared with a snapshot, and is copied to change.
type token struct{ _ byte }

// node is a node of the tree of lines: a leaf, of height 0, holding lines,
// or an inner node holding children, all of the same height, one less than
// its own. Beside its children, an inner node keeps what it sums up about
// each of them, so that finding the child that holds a line, or a row,
// reads only the node.
//
// What a node holds is in two parts. Its content, its lines or children
// and what they sum up to, is never written again once the node is shared
// with a snapshot, which may be read while the tree is changed. Its cache,
// what is worked out from the content, such as the widths of its lines, is
// derived, and may be worked out and written again even while it is shared.
type node struct {
	own    *token
	height int

	// The lines, the bytes of the lines and a newline each, and what the
	// lines do to the style text is drawn in.
	n, bytes int
	pen      *penChange

	// A leaf's lines, and each one's change to the style, or nil while no
	// line has one.
	lines []string
	pens  []*penChange

	// An inner node's children, and the lines, bytes and change to the
	// style of each, the last nil while no child has one.
	kids     []*node
	kidLines []int
	kidBytes []int
	kidPens  []*penChange

	m [2]measureSlot // most recently worked out first
	r [2]rowSlot
}

// measureSlot is how wide the lines of a node are, measured with a width
// method, when ok: for a leaf, each line's width, with what measuring it
// told of it in the bits above, as [measureLine] returns it, and for an
// inner node the widest line of each child.
type measureSlot struct {
	ok     bool
	method ansi.Method
	widest int
	w      []int32
}

// rowKey is what the rows of lines are worked out for: the width they are
// wrapped to with a mode, measured with a method.
type rowKey struct {
	method ansi.Method
	mode   Mode
	width  int
}

// rowSlot is how many rows the lines of a node take, wrapped as key says,
// when ok: for a leaf, the rows of each line, or none while each takes one,
// and for an inner node the rows of each child.
type rowSlot struct {
	ok    bool
	key   rowKey
	total int
	r     []int32
	rk    []int
}

// leaf reports whether n holds lines rather than children.
func (n *node) leaf() bool { return n.height == 0 }

// size returns how many lines or children n holds.
func (n *node) size() int {
	if n.leaf() {
		return len(n.lines)
	}
	return len(n.kids)
}

// okChild reports whether n holds enough to be a child of a node other
// than the root.
func (n *node) okChild() bool {
	if n.leaf() {
		return len(n.lines) >= leafMin
	}
	return len(n.kids) >= fanMin
}

// locate returns the child of inner node n that line i of it is in, and
// the line of the child it is: the last child for a line past its end.
func (n *node) locate(i int) (k, in int) {
	last := len(n.kidLines) - 1
	for k := range last {
		if i < n.kidLines[k] {
			return k, i
		}
		i -= n.kidLines[k]
	}
	return last, i
}

// findM returns the widths of n's lines measured with method, or nil when
// they are not kept.
func (n *node) findM(method ansi.Method) *measureSlot {
	if n.m[0].ok && n.m[0].method == method {
		return &n.m[0]
	}
	if n.m[1].ok && n.m[1].method == method {
		return &n.m[1]
	}
	return nil
}

// takeM returns a slot of n to measure its lines with method in, the
// first, freeing the one worked out longest ago if both are in use. The
// slot keeps the storage of the one it frees, for reuse.
func (n *node) takeM(method ansi.Method) *measureSlot {
	if !n.m[0].ok {
		n.m[0] = measureSlot{method: method, w: n.m[0].w[:0]}
		return &n.m[0]
	}
	old := n.m[1]
	n.m[1] = n.m[0]
	n.m[0] = measureSlot{method: method, w: old.w[:0]}
	return &n.m[0]
}

// findR returns the rows of n's lines wrapped as k says, or nil when they
// are not kept.
func (n *node) findR(k rowKey) *rowSlot {
	if n.r[0].ok && n.r[0].key == k {
		return &n.r[0]
	}
	if n.r[1].ok && n.r[1].key == k {
		return &n.r[1]
	}
	return nil
}

// takeR is [node.takeM] for rows, but for those wrapped as keep says, which
// are kept though they were worked out longest ago.
func (n *node) takeR(k, keep rowKey) *rowSlot {
	if !n.r[0].ok || n.r[1].ok && n.r[1].key == keep {
		n.r[0] = rowSlot{key: k, r: n.r[0].r[:0], rk: n.r[0].rk[:0]}
		return &n.r[0]
	}
	old := n.r[1]
	n.r[1] = n.r[0]
	n.r[0] = rowSlot{key: k, r: old.r[:0], rk: old.rk[:0]}
	return &n.r[0]
}

// token returns the token that owns the nodes the store may change in
// place.
func (t *Store) token() *token {
	if t.tok == nil {
		t.tok = new(token)
	}
	return t.tok
}

// snapshot returns the root of the tree as it is, which stays as it is:
// the nodes are owned by a token retired here, so that later changes copy
// the nodes they touch rather than change them in place.
func (t *Store) snapshot() *node {
	t.tok = new(token)
	return t.root
}

// mut returns n to change in place: n itself when the store owns it, or
// a copy of it the store owns, with what n kept of its lines.
func (t *Store) mut(n *node) *node {
	if n.own == t.token() {
		return n
	}
	return t.copyNode(n)
}

// copyNode returns a copy of n that the store owns, in storage of its own,
// with what n kept of its lines.
func (t *Store) copyNode(n *node) *node {
	c := &node{own: t.token(), height: n.height, n: n.n, bytes: n.bytes, pen: n.pen}
	if n.leaf() {
		c.lines = append(make([]string, 0, leafMax+1), n.lines...)
		if n.pens != nil {
			c.pens = append(make([]*penChange, 0, leafMax+1), n.pens...)
		}
	} else {
		c.kids = append(make([]*node, 0, fanMax+1), n.kids...)
		c.kidLines = append(make([]int, 0, fanMax+1), n.kidLines...)
		c.kidBytes = append(make([]int, 0, fanMax+1), n.kidBytes...)
		if n.kidPens != nil {
			c.kidPens = append(make([]*penChange, 0, fanMax+1), n.kidPens...)
		}
	}
	for k := range n.m {
		if n.m[k].ok {
			c.m[k] = n.m[k]
			c.m[k].w = slices.Clone(n.m[k].w)
		}
	}
	for k := range n.r {
		if n.r[k].ok {
			c.r[k] = n.r[k]
			c.r[k].r, c.r[k].rk = slices.Clone(n.r[k].r), slices.Clone(n.r[k].rk)
		}
	}
	return c
}

// sumLeaf works out what leaf n sums up from its lines, and drops its
// changes to the style when none has one.
func (n *node) sumLeaf() {
	n.n, n.bytes = len(n.lines), len(n.lines)
	for _, s := range n.lines {
		n.bytes += len(s)
	}
	n.pen = composeAll(n.pens)
	if n.pen == nil {
		n.pens = nil
	}
}

// sumInner works out what inner node n sums up from its children.
func (n *node) sumInner() {
	n.height = n.kids[0].height + 1
	n.kidLines, n.kidBytes = n.kidLines[:0], n.kidBytes[:0]
	n.kidPens = n.kidPens[:0]
	n.n, n.bytes = 0, 0
	styled := false
	for _, kid := range n.kids {
		n.kidLines = append(n.kidLines, kid.n)
		n.kidBytes = append(n.kidBytes, kid.bytes)
		n.n += kid.n
		n.bytes += kid.bytes
		styled = styled || kid.pen != nil
	}
	if !styled {
		n.kidPens, n.pen = nil, nil
		return
	}
	for _, kid := range n.kids {
		n.kidPens = append(n.kidPens, kid.pen)
	}
	n.pen = composeAll(n.kidPens)
}

// newLeaf returns a leaf of lines, with pens their changes to the style,
// or nil.
func (t *Store) newLeaf(lines []string, pens []*penChange) *node {
	n := &node{own: t.token(), lines: lines, pens: pens}
	n.sumLeaf()
	return n
}

// newInner returns an inner node of kids, which it keeps.
func (t *Store) newInner(kids []*node) *node {
	n := &node{
		own:      t.token(),
		kids:     kids,
		kidLines: make([]int, 0, fanMax+1),
		kidBytes: make([]int, 0, fanMax+1),
	}
	n.sumInner()
	return n
}

// refresh works out again what inner node n keeps of child k, which has
// changed: its sums, and those in each of n's cache slots, worked out for
// the child, if it lacks them, from what it keeps.
func (t *Store) refresh(n *node, k int) {
	kid := n.kids[k]
	n.n += kid.n - n.kidLines[k]
	n.kidLines[k] = kid.n
	n.bytes += kid.bytes - n.kidBytes[k]
	n.kidBytes[k] = kid.bytes
	if kid.pen != nil && n.kidPens == nil {
		n.kidPens = make([]*penChange, len(n.kids), fanMax+1)
	}
	if n.kidPens != nil {
		n.kidPens[k] = kid.pen
		n.pen = composeAll(n.kidPens)
	}
	for s := range n.m {
		ms := &n.m[s]
		if !ms.ok {
			continue
		}
		old, w := int(ms.w[k]), measureNode(kid, ms.method, &t.work).widest
		ms.w[k] = int32(w) //nolint:gosec // Bounded by widthMask.
		switch {
		case w >= ms.widest:
			ms.widest = w
		case old == ms.widest:
			ms.widest = widestOf(ms.w)
		}
	}
	for s := range n.r {
		rs := &n.r[s]
		if !rs.ok {
			continue
		}
		rows := rowsNode(kid, rs.key, t.rowsKept(rs.key), &t.work)
		rs.total += rows - rs.rk[k]
		rs.rk[k] = rows
	}
}

// insertKid inserts kid as child k of inner node n.
func (t *Store) insertKid(n *node, k int, kid *node) {
	n.kids = slices.Insert(n.kids, k, kid)
	n.kidLines = slices.Insert(n.kidLines, k, 0)
	n.kidBytes = slices.Insert(n.kidBytes, k, 0)
	if n.kidPens != nil {
		n.kidPens = slices.Insert(n.kidPens, k, nil)
	}
	for s := range n.m {
		if n.m[s].ok {
			n.m[s].w = slices.Insert(n.m[s].w, k, 0)
		}
	}
	for s := range n.r {
		if n.r[s].ok {
			n.r[s].rk = slices.Insert(n.r[s].rk, k, 0)
		}
	}
	t.refresh(n, k)
}

// push adds line s, whose change to the style is c, at the end of the tree
// under n, which it changes in place where the store owns it. It returns
// the node to put in n's place, and a node to put after it when n split.
func (t *Store) push(n *node, s string, c *penChange) (put, after *node) {
	n = t.mut(n)
	if n.leaf() {
		t.pushLeaf(n, s, c)
		if len(n.lines) <= leafMax {
			return n, nil
		}
		return n, t.splitLeaf(n, appendSplit(len(n.lines), leafMax, leafMin))
	}
	k := len(n.kids) - 1
	kid, split := t.push(n.kids[k], s, c)
	n.kids[k] = kid
	t.refresh(n, k)
	if split == nil {
		return n, nil
	}
	t.insertKid(n, k+1, split)
	if len(n.kids) <= fanMax {
		return n, nil
	}
	return n, t.splitInner(n, appendSplit(len(n.kids), fanMax, fanMin))
}

// appendSplit returns where to split a node holding size lines or
// children, one more than most, after one was added at its end: as late
// as leaves the node after it the least it may hold, so that a tree made
// by appending is nearly full.
func appendSplit(size, most, least int) int { return min(most, size-least) }

// pushLeaf adds line s, whose change to the style is c, at the end of leaf
// n, measuring and wrapping it for each of n's cache slots.
func (t *Store) pushLeaf(n *node, s string, c *penChange) {
	if len(n.lines) == cap(n.lines) {
		n.lines = slices.Grow(n.lines, leafMax+1-len(n.lines))
	}
	n.lines = append(n.lines, s)
	if c != nil && n.pens == nil {
		n.pens = make([]*penChange, len(n.lines)-1, leafMax+1)
	}
	if n.pens != nil {
		n.pens = append(n.pens, c)
	}
	n.n++
	n.bytes += len(s) + 1
	n.pen = compose(n.pen, c)
	for k := range n.m {
		ms := &n.m[k]
		if ms.ok {
			w := measureLine(s, ms.method)
			ms.w = append(ms.w, w)
			ms.widest = max(ms.widest, int(w&widthMask))
		}
	}
	for k := range n.r {
		rs := &n.r[k]
		if !rs.ok {
			continue
		}
		ms := n.findM(rs.key.method)
		if ms == nil {
			rs.ok = false
			continue
		}
		rows := lineRows(s, ms.w[len(ms.w)-1], rs.key, &t.work.starts)
		if rows > 1 && len(rs.r) == 0 {
			rs.r = ones(rs.r, len(n.lines)-1)
		}
		if len(rs.r) > 0 {
			rs.r = append(rs.r, int32(rows)) //nolint:gosec // A line has far fewer rows than 1<<31.
		}
		rs.total += rows
	}
}

// ones returns n rows of one, in r's storage when it has room.
func ones(r []int32, n int) []int32 {
	if cap(r) < n+1 {
		r = make([]int32, n, leafMax+1)
	}
	r = r[:n]
	for i := range r {
		r[i] = 1
	}
	return r
}

// splitLeaf moves the lines of leaf n from at on to a new leaf after it,
// which it returns, with their widths and rows.
func (t *Store) splitLeaf(n *node, at int) *node {
	r := &node{own: t.token()}
	r.lines = append(make([]string, 0, leafMax+1), n.lines[at:]...)
	clear(n.lines[at:])
	n.lines = n.lines[:at]
	if n.pens != nil {
		r.pens = append(make([]*penChange, 0, leafMax+1), n.pens[at:]...)
		clear(n.pens[at:])
		n.pens = n.pens[:at]
	}
	n.sumLeaf()
	r.sumLeaf()
	splitSlots(n, r, at)
	return r
}

// splitInner moves the children of inner node n from at on to a new node
// after it, which it returns, with what n keeps of them.
func (t *Store) splitInner(n *node, at int) *node {
	r := &node{
		own:      t.token(),
		kids:     append(make([]*node, 0, fanMax+1), n.kids[at:]...),
		kidLines: make([]int, 0, fanMax+1),
		kidBytes: make([]int, 0, fanMax+1),
	}
	clear(n.kids[at:])
	n.kids = n.kids[:at]
	n.sumInner()
	r.sumInner()
	splitSlots(n, r, at)
	return r
}

// splitSlots moves what the cache slots of n keep of its lines or
// children from at on to the same slots of r, which now holds them.
func splitSlots(n, r *node, at int) {
	for k := range n.m {
		ms := &n.m[k]
		if !ms.ok {
			continue
		}
		rw := append(make([]int32, 0, cap(ms.w)), ms.w[at:]...)
		ms.w = ms.w[:at]
		ms.widest = widestOf(ms.w)
		r.m[k] = measureSlot{ok: true, method: ms.method, widest: widestOf(rw), w: rw}
	}
	for k := range n.r {
		rs := &n.r[k]
		if !rs.ok {
			continue
		}
		rr := rowSlot{ok: true, key: rs.key}
		if n.leaf() {
			if len(rs.r) > 0 {
				rr.r = append(make([]int32, 0, cap(rs.r)), rs.r[at:]...)
				rs.r = rs.r[:at]
			}
			rs.r, rs.total = leafRows(rs.r, at)
			rr.r, rr.total = leafRows(rr.r, len(r.lines))
		} else {
			rr.rk = append(make([]int, 0, cap(rs.rk)), rs.rk[at:]...)
			rs.rk = rs.rk[:at]
			rs.total, rr.total = sumInts(rs.rk), sumInts(rr.rk)
		}
		r.r[k] = rr
	}
}

// leafRows returns rows, the rows of each of the n lines of a leaf, or none
// while each takes one, with none when each does, and how many they are.
func leafRows(rows []int32, n int) (kept []int32, total int) {
	if len(rows) == 0 {
		return rows, n
	}
	more := false
	for _, r := range rows {
		total += int(r)
		more = more || r > 1
	}
	if !more {
		rows = rows[:0]
	}
	return rows, total
}

func sumInts(s []int) int {
	t := 0
	for _, v := range s {
		t += v
	}
	return t
}

// widestOf returns the widest of widths, without what measuring told of
// them.
func widestOf(widths []int32) int {
	w := int32(0)
	for _, v := range widths {
		w = max(w, v&widthMask)
	}
	return int(w)
}

// appendRows appends to dst rows, the rows of n lines, or none while each
// takes one.
func appendRows(dst, rows []int32, n int) []int32 {
	if len(rows) > 0 {
		return append(dst, rows...)
	}
	for range n {
		dst = append(dst, 1)
	}
	return dst
}

// join returns the tree of the lines of a and then those of b, either of
// which may be nil for no lines. It makes new nodes rather than change
// those of a and b, and keeps every node but the root holding at least
// the minimum, with every leaf at the same depth.
func (t *Store) join(a, b *node) *node {
	switch {
	case a == nil || a.n == 0:
		return b
	case b == nil || b.n == 0:
		return a
	case a.height < b.height:
		if a.height == b.height-1 && a.okChild() {
			return t.mergeKids(append([]*node{a}, b.kids...))
		}
		j := t.join(a, b.kids[0])
		if j.height == b.height-1 {
			return t.mergeKids(append([]*node{j}, b.kids[1:]...))
		}
		return t.mergeKids(append(slices.Clone(j.kids), b.kids[1:]...))
	case a.height > b.height:
		last := len(a.kids) - 1
		if b.height == a.height-1 && b.okChild() {
			return t.mergeKids(append(slices.Clone(a.kids), b))
		}
		j := t.join(a.kids[last], b)
		if j.height == a.height-1 {
			return t.mergeKids(append(slices.Clone(a.kids[:last]), j))
		}
		return t.mergeKids(append(slices.Clone(a.kids[:last]), j.kids...))
	case a.okChild() && b.okChild():
		return t.newInner(append(make([]*node, 0, fanMax+1), a, b))
	case a.leaf():
		return t.mergeLeaves(a, b)
	}
	return t.mergeKids(append(slices.Clone(a.kids), b.kids...))
}

// mergeKids returns a node of kids, nodes of the same height that may each
// be a child, or two under a new root when there are too many for one.
func (t *Store) mergeKids(kids []*node) *node {
	if len(kids) <= fanMax {
		return t.newInner(append(make([]*node, 0, fanMax+1), kids...))
	}
	at := appendSplit(len(kids), fanMax, fanMin)
	l := t.newInner(append(make([]*node, 0, fanMax+1), kids[:at]...))
	r := t.newInner(append(make([]*node, 0, fanMax+1), kids[at:]...))
	return t.newInner(append(make([]*node, 0, fanMax+1), l, r))
}

// mergeLeaves returns a leaf of the lines of leaves a and then b, or two
// under a new root when there are too many for one, keeping what a and b
// both keep of them.
func (t *Store) mergeLeaves(a, b *node) *node {
	n := t.leafPart(a, 0, len(a.lines))
	t.absorb(n, b)
	if len(n.lines) <= leafMax {
		return n
	}
	r := t.splitLeaf(n, appendSplit(len(n.lines), leafMax, leafMin))
	return t.newInner(append(make([]*node, 0, fanMax+1), n, r))
}

// appendPens appends to dst pens, the changes of n lines, or nil when none
// has one.
func appendPens(dst, pens []*penChange, n int) []*penChange {
	if pens != nil {
		return append(dst, pens...)
	}
	return append(dst, make([]*penChange, n)...)
}

// split returns the trees of the lines of the tree under n before line i,
// and from it on, either nil for none. It makes new nodes rather than
// change those of n.
func (t *Store) split(n *node, i int) (l, r *node) {
	switch {
	case n == nil:
		return nil, nil
	case i <= 0:
		return nil, n
	case i >= n.n:
		return n, nil
	case n.leaf():
		return t.leafPart(n, 0, i), t.leafPart(n, i, len(n.lines))
	}
	k, in := n.locate(i)
	if in == 0 {
		return t.rootOf(n.kids[:k]), t.rootOf(n.kids[k:])
	}
	kl, kr := t.split(n.kids[k], in)
	return t.join(t.rootOf(n.kids[:k]), kl), t.join(kr, t.rootOf(n.kids[k+1:]))
}

// leafPart returns a new leaf of lines lo up to hi of leaf n, with what n
// keeps of them.
func (t *Store) leafPart(n *node, lo, hi int) *node {
	var pens []*penChange
	if n.pens != nil {
		pens = append(make([]*penChange, 0, leafMax+1), n.pens[lo:hi]...)
	}
	p := t.newLeaf(append(make([]string, 0, leafMax+1), n.lines[lo:hi]...), pens)
	for k := range n.m {
		if ms := &n.m[k]; ms.ok {
			w := append(make([]int32, 0, leafMax+1), ms.w[lo:hi]...)
			p.m[k] = measureSlot{ok: true, method: ms.method, widest: widestOf(w), w: w}
		}
	}
	for k := range n.r {
		rs := &n.r[k]
		if !rs.ok {
			continue
		}
		var rows []int32
		if len(rs.r) > 0 {
			rows = append(make([]int32, 0, leafMax+1), rs.r[lo:hi]...)
		}
		rr := rowSlot{ok: true, key: rs.key}
		rr.r, rr.total = leafRows(rows, hi-lo)
		p.r[k] = rr
	}
	return p
}

// rootOf returns the tree of kids, children of a node: none, the one, or
// a new node of them.
func (t *Store) rootOf(kids []*node) *node {
	switch len(kids) {
	case 0:
		return nil
	case 1:
		return kids[0]
	}
	return t.newInner(append(make([]*node, 0, fanMax+1), kids...))
}

// chunk returns the span from lo up to hi of chunk i of n things cut into
// count chunks of up to most, each but a lone one at least least: all of
// most but the last two, which share what is left when it is too little
// for one.
func chunk(n, most, least, i int) (lo, hi int) {
	count := (n + most - 1) / most
	last := n - (count-1)*most
	shift := 0
	if count > 1 && last < least {
		shift = least - last
	}
	lo, hi = i*most, (i+1)*most
	switch i {
	case count - 1:
		lo -= shift
		hi = n
	case count - 2:
		hi -= shift
	}
	return lo, hi
}

// slabs is the storage a tree was built in: its lines, its leaves, with
// the storage of what they keep of their lines, and their lines' changes
// to the style. Once the tree is cleared, while no snapshot shares it, the
// next tree built takes it over rather than make its own, as a buffer set
// again and again, such as to the output of a command run each second,
// would otherwise make a tree's worth of garbage each time.
type slabs struct {
	tok    *token
	lines  []string
	leaves []node
	pens   []*penChange
	levels []levelSlab // the levels of inner nodes above the leaves
}

// levelSlab is the storage of a level of a tree built: the nodes of the
// level below, as the children of its own, its own, and what they keep of
// their children.
type levelSlab struct {
	kids     []*node
	nodes    []node
	kidLines []int
	kidBytes []int
	kidPens  []*penChange
}

// reuse returns s[:n] when it has room, or a new slice of n.
func reuse[T any](s []T, n int) []T {
	if cap(s) >= n {
		clear(s[n:cap(s)])
		return s[:n]
	}
	return make([]T, n)
}

// lineSlab returns storage for n lines, from the slabs of the tree last
// cleared when it has room.
func (t *Store) lineSlab(n int) []string {
	if sp := t.spare; sp != nil && cap(sp.lines) >= n {
		return sp.lines[:0]
	}
	return make([]string, 0, n)
}

// build returns a tree of lines, made bottom up: each leaf a window of
// lines, which it keeps, and each level as full as it can be. The changes
// of the lines to the style are worked out in parallel, as each leaf's
// are its own.
func (t *Store) build(lines []string) *node {
	if len(lines) == 0 {
		return nil
	}
	sp := t.spare
	t.spare = nil
	tok := t.token()
	count := (len(lines) + leafMax - 1) / leafMax
	if sp == nil {
		sp = &slabs{}
	}
	made := &slabs{tok: tok, lines: lines, leaves: reuse(sp.leaves, count)}
	var pensOnce sync.Once
	pens := func() []*penChange {
		pensOnce.Do(func() { made.pens = reuse(sp.pens, len(lines)) })
		return made.pens
	}
	inParallel(len(lines), func(lo, hi int) {
		// The leaves whose first lines would be in the span, were each
		// full.
		var tmp [leafMax]*penChange
		for i := (lo + leafMax - 1) / leafMax; i < min((hi+leafMax-1)/leafMax, count); i++ {
			a, b := chunk(len(lines), leafMax, leafMin, i)
			n := &made.leaves[i]
			n.recycle()
			n.own, n.lines = tok, lines[a:b:b]
			styled := false
			for k, s := range n.lines {
				tmp[k] = lineChange(s)
				styled = styled || tmp[k] != nil
			}
			if styled {
				n.pens = pens()[a:b:b]
				copy(n.pens, tmp[:b-a])
			}
			n.sumLeaf()
		}
	})
	t.slab = made
	nodes := made.leaves
	for d := 0; ; d++ {
		var old levelSlab
		if d < len(sp.levels) {
			old = sp.levels[d]
		}
		kids := reuse(old.kids, len(nodes))
		for i := range nodes {
			kids[i] = &nodes[i]
		}
		if len(kids) == 1 {
			return kids[0]
		}
		made.levels = append(made.levels, buildLevel(kids, tok, &old))
		nodes = made.levels[d].nodes
	}
}

// recycle empties n to be made a new leaf of, keeping the storage of its
// cache slots for the new leaf's.
func (n *node) recycle() {
	*n = node{m: n.m, r: n.r}
	for k := range n.m {
		n.m[k].ok = false
		n.r[k].ok = false
	}
}

// buildLevel returns the level above nodes, owned by tok, made in the
// storage of old where it has room.
func buildLevel(kids []*node, tok *token, old *levelSlab) levelSlab {
	count := (len(kids) + fanMax - 1) / fanMax
	l := levelSlab{
		kids:     kids,
		nodes:    reuse(old.nodes, count),
		kidLines: reuse(old.kidLines, len(kids)),
		kidBytes: reuse(old.kidBytes, len(kids)),
	}
	for _, n := range kids {
		if n.pen != nil {
			l.kidPens = reuse(old.kidPens, len(kids))
			break
		}
	}
	for i := range l.nodes {
		lo, hi := chunk(len(kids), fanMax, fanMin, i)
		n := &l.nodes[i]
		n.recycle()
		n.own, n.kids = tok, kids[lo:hi:hi]
		n.kidLines, n.kidBytes = l.kidLines[lo:lo:hi], l.kidBytes[lo:lo:hi]
		if l.kidPens != nil {
			n.kidPens = l.kidPens[lo:lo:hi]
		}
		n.sumInner()
	}
	return l
}
