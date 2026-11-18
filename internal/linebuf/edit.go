// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"iter"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Pos is a place in the text of a store's lines: a line, and a byte of the
// line's text as [PlainText] gives it with ansi.GraphemeWidth, without its
// styles, a tab one byte. It is between two glyphs, or at an end.
type Pos struct{ Line, Col int }

// Before reports whether p comes before q.
func (p Pos) Before(q Pos) bool { return p.Line < q.Line || p.Line == q.Line && p.Col < q.Col }

// TextEdit is a change to the text of a store: the text from From up to To
// replaced with Text.
type TextEdit struct {
	From, To Pos
	Text     string
}

// textMethod is how the text of a line is cut into glyphs, as [PlainText]
// is given it for the text a [Pos] counts the bytes of.
const textMethod = ansi.GraphemeWidth

// textBytes returns the bytes of the text of the line a scanner found a
// glyph of: those of the glyph, one for the first space of a tab, and none
// for the rest.
func textBytes(sc *scanner) int {
	switch {
	case sc.tabFirst:
		return 1
	case sc.tab:
		return 0
	}
	return len(sc.g)
}

// textCol returns the byte of line s that byte col of its text is at, and
// col, moved back to the start of the glyph it falls in, or to the end of
// the text past it. The byte is the one just after the glyph before col,
// before the escape sequences between it and the next, so that text put
// there is drawn in the style of the text before it.
func textCol(s string, col int) (raw, at int) {
	if col <= 0 {
		return 0, 0
	}
	if simpleLine(s) {
		col = min(col, len(s))
		return col, col
	}
	return textColFrom(s, col, checkpoint{})
}

// textColFrom is textCol for a line that is not only printable ASCII,
// scanned from checkpoint from, which is at or before byte col of its text.
func textColFrom(s string, col int, from checkpoint) (raw, at int) {
	raw, at = from.i, from.t
	sc := scanner{s: s, i: from.i, x: from.x, method: textMethod}
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		n := textBytes(&sc)
		if k != glyphText || n == 0 {
			continue
		}
		if col < at+n {
			return raw, at
		}
		at += n
		raw = sc.at + n
		if at == col {
			break
		}
	}
	return raw, at
}

// colOf returns the byte of the text of line s that byte raw of the line
// is at: that of the glyph raw falls in, or of the next glyph when it falls
// before one, in an escape sequence, say.
func colOf(s string, raw int) int {
	if simpleLine(s) {
		return min(max(raw, 0), len(s))
	}
	at := 0
	sc := newScanner(s, textMethod)
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		n := textBytes(&sc)
		if k != glyphText || n == 0 {
			continue
		}
		if raw < sc.at+n {
			return at
		}
		at += n
	}
	return at
}

// textLen returns how many bytes the text of line s is, as [PlainText]
// gives it.
func textLen(s string) int {
	if simpleLine(s) {
		return len(s)
	}
	n := 0
	sc := newScanner(s, textMethod)
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		if k == glyphText {
			n += textBytes(&sc)
		}
	}
	return n
}

// TextLen returns how many bytes the text of s is, as [PlainText] gives it
// with ansi.GraphemeWidth.
func TextLen(s string) int { return textLen(s) }

// Clamp returns p moved to the nearest place in the store's text: the start
// for a line before the first, the end for one past the last, the end of
// the line for a byte past it, and the start of the glyph, such as a
// character of many bytes or a grapheme cluster, that a byte falls in.
func (t *Store) Clamp(p Pos) Pos {
	n := t.Len()
	switch {
	case n == 0 || p.Line < 0:
		return Pos{}
	case p.Line >= n:
		return t.End()
	case p.Col <= 0:
		return Pos{Line: p.Line}
	}
	s := t.Line(p.Line)
	if t.plain(p.Line, s) {
		return Pos{Line: p.Line, Col: min(p.Col, len(s))}
	}
	// A long line is scanned from the checkpoint before the byte, as a
	// caret far along it is clamped each frame it is drawn in.
	_, col := textColFrom(s, p.Col, t.textFrom(p.Line, s, textMethod, p.Col))
	return Pos{Line: p.Line, Col: col}
}

// textFrom returns the checkpoint to scan line i, which is s, from for
// byte col of its text, measured with method: the last one at or before it
// for a long line, and the line's start otherwise.
func (t *Store) textFrom(i int, s string, method ansi.Method, col int) checkpoint {
	if len(s) < LongLineBytes || col < CheckpointEvery {
		return checkpoint{}
	}
	l := t.longLine(i, method)
	l.reach(s, col, true)
	k, _ := slices.BinarySearchFunc(l.cps, col+1, func(cp checkpoint, at int) int { return cp.t - at })
	return l.cps[max(k-1, 0)]
}

// End returns the end of the store's text.
func (t *Store) End() Pos {
	n := t.Len()
	if n == 0 {
		return Pos{}
	}
	s := t.Line(n - 1)
	if t.plain(n-1, s) {
		return Pos{Line: n - 1, Col: len(s)}
	}
	return Pos{Line: n - 1, Col: textLen(s)}
}

// Text returns the text of line i without its styles, with its tabs, as
// [PlainText] gives it with ansi.GraphemeWidth: the line itself when it is
// only printable ASCII, which for a long line is known from measuring it,
// without reading it again.
func (t *Store) Text(i int) string {
	s := t.Line(i)
	if t.plain(i, s) {
		return s
	}
	return PlainText(s, textMethod)
}

// plain reports whether line i, which is s, is only printable ASCII, its
// text a byte a cell: for a long line, as measuring it found, which is kept
// until it changes, rather than by reading all of it again.
func (t *Store) plain(i int, s string) bool {
	if len(s) < LongLineBytes {
		return simpleLine(s)
	}
	return t.flags(i, textMethod)&plainLine != 0
}

// Offset returns the byte of [Store.String] that p, as [Store.Clamp]
// returns it, is at.
func (t *Store) Offset(p Pos) int {
	if t.root == nil {
		return 0
	}
	raw, _ := textCol(t.Line(p.Line), p.Col)
	return t.bytesBefore(p.Line) + raw
}

// PosOf returns the place in the text that byte off of [Store.String] is
// at: that of the glyph it falls in, or of the next one when it falls in
// an escape sequence, the end of a line for its newline, and the end of the
// text past it.
func (t *Store) PosOf(off int) Pos {
	if t.root == nil || off <= 0 {
		return Pos{}
	}
	if off >= t.root.bytes-1 {
		return t.End()
	}
	line, in := t.lineAtByte(off)
	return Pos{Line: line, Col: colOf(t.Line(line), in)}
}

// bytesBefore returns how many bytes of [Store.String] come before line i.
func (t *Store) bytesBefore(i int) int {
	n, sum := t.root, 0
	for !n.leaf() {
		k := 0
		for last := len(n.kids) - 1; k < last && i >= n.kidLines[k]; k++ {
			i -= n.kidLines[k]
			sum += n.kidBytes[k]
		}
		n = n.kids[k]
	}
	for _, s := range n.lines[:i] {
		sum += len(s) + 1
	}
	return sum
}

// lineAtByte returns the line that byte off of [Store.String] is in, a
// byte before its end, and the byte of the line it is, which is the line's
// length for its newline.
func (t *Store) lineAtByte(off int) (line, in int) {
	n := t.root
	for !n.leaf() {
		k := 0
		for last := len(n.kids) - 1; k < last && off >= n.kidBytes[k]; k++ {
			off -= n.kidBytes[k]
			line += n.kidLines[k]
		}
		n = n.kids[k]
	}
	last := len(n.lines) - 1
	for j, s := range n.lines {
		if off <= len(s) || j == last {
			return line + j, min(off, len(s))
		}
		off -= len(s) + 1
	}
	return line, 0
}

// Replace makes edits to the store's text, all at once, as one change:
// each replaces the text from its From up to its To with its Text, split
// into lines at its newlines, a newline at its end starting an empty line,
// without the carriage return before a newline. The places of the edits
// must be as [Store.Clamp] returns them, for the text before any of them,
// each From at or before its To, and the edits in order, none overlapping
// the next, though they may touch. A store with no lines takes them as
// though it held one empty line.
//
// The text after an edit is drawn in the style it was drawn in before: the
// escape sequences of the text removed, and of the lines between, are left
// in its place as one that sets the style they leave, when it is not the
// style already in force there. Text is put in as it is given, and may hold
// escape sequences of its own, as an appended line may.
//
// Each edit works out again only the lines it adds, and an edit within the
// lines of one leaf of the tree changes the nodes on the path to it in
// place, where no snapshot shares them, and what they keep of the lines
// with them, so that the store is measured again in a step a level.
func (t *Store) Replace(edits ...TextEdit) {
	if len(edits) == 0 {
		return
	}
	if t.root == nil {
		t.splice(0, 0, []string{""})
	}
	t.most = max(t.most, t.root.bytes)
	if len(edits) == 1 {
		ra, rb := t.rawOf(&edits[0])
		t.replace(&edits[0], ra, rb)
		t.compact()
		t.changed()
		return
	}
	// The bytes each edit is at are found before any is made: the edits are
	// made last first, which leaves the bytes before each as they were, but
	// may join the text after an earlier one into a glyph across its place,
	// such as a combining mark put where it ends.
	raws := make([][2]int, len(edits))
	for i := range edits {
		raws[i][0], raws[i][1] = t.rawOf(&edits[i])
	}
	for i := len(edits) - 1; i >= 0; i-- {
		t.replace(&edits[i], raws[i][0], raws[i][1])
	}
	t.compact()
	t.changed()
}

// compactFrom is the least bytes a store's lines must have been for the
// store to be built again once most of them are removed.
const compactFrom = 1 << 20

// compact copies the tree, and its lines, once they are a quarter of what
// they have been, so that the rest can be freed: the lines and nodes a tree
// is built from share their storage, and the lines the text they were
// split from, all of which the lines left would otherwise keep. The copy
// keeps what the tree kept of its lines, such as how wide they are, and a
// snapshot taken before keeps what it shares, as it must.
func (t *Store) compact() {
	if t.most < compactFrom || t.root == nil || t.root.bytes > t.most/4 {
		return
	}
	// What the store holds of the tree as it was goes, or it would keep it,
	// and the scratch it was worked out in, which was sized for it.
	t.slab, t.spare, t.snapped = nil, nil, nil
	t.fin, t.finOK = cursor{}, false
	t.long = [4]*longLine{}
	t.work, t.pool, t.todo = work{}, nil, nil
	t.root = t.copyTree(t.root)
	t.most = t.root.bytes
}

// copyTree returns a copy of the tree under n, with copies of its lines.
func (t *Store) copyTree(n *node) *node {
	c := t.copyNode(n)
	for i, s := range c.lines {
		c.lines[i] = strings.Clone(s)
	}
	for i, kid := range c.kids {
		c.kids[i] = t.copyTree(kid)
	}
	return c
}

// rawOf returns the bytes of their lines that edit e starts at and ends
// before.
func (t *Store) rawOf(e *TextEdit) (ra, rb int) {
	first := t.Line(e.From.Line)
	last := first
	if e.To.Line != e.From.Line {
		last = t.Line(e.To.Line)
	}
	ra, _ = textCol(first, e.From.Col)
	rb, _ = textCol(last, e.To.Col)
	return ra, rb
}

// replace makes edit e, which starts at byte ra of its first line and ends
// before byte rb of its last, as [Store.Replace] does, without noting the
// change.
func (t *Store) replace(e *TextEdit, ra, rb int) {
	if e.From == e.To && e.Text == "" {
		return
	}
	a, b := e.From.Line, e.To.Line
	first := t.Line(a)
	last := first
	if b != a {
		last = t.Line(b)
	}
	tail := t.tailOf(a, first, ra, b, last, rb, e.Text)
	if strings.IndexByte(e.Text, '\n') < 0 {
		s := first[:ra] + e.Text + tail
		switch {
		case a == b && s == first:
			return
		case ra == 0 && e.Text == "" && tail == last:
			// Whole lines removed, the last one kept as it is.
			t.splice(a, b, nil)
			return
		}
		t.splice(a, b+1, []string{s})
		return
	}
	lines := make([]string, 0, strings.Count(e.Text, "\n")+1)
	rest := e.Text
	for {
		seg, after, more := strings.Cut(rest, "\n")
		if !more {
			lines = append(lines, seg+tail)
			break
		}
		seg = strings.TrimSuffix(seg, "\r")
		if len(lines) == 0 {
			seg = first[:ra] + seg
		}
		lines = append(lines, seg)
		rest = after
	}
	// The first and last lines kept as they were, rather than measured
	// again, as when a newline is put at the start or end of a line.
	lo, hi := a, b+1
	if lines[0] == first {
		lines[0] = first
		lines, lo = lines[1:], lo+1
	}
	if lo <= b && lines[len(lines)-1] == last {
		lines, hi = lines[:len(lines)-1], hi-1
	}
	t.splice(lo, hi, lines)
}

// tailOf returns what follows the text an edit puts in, which replaces the
// text from byte ra of line a, first, up to byte rb of line b, last: the
// rest of last, after the escape sequence that sets the style it was drawn
// in, when that is not the style in force at ra. When the edit removes
// text, a rest of last that holds no text, only escape sequences, is
// replaced by the one that sets the style they leave, so that removing the
// end of a styled line leaves no sequences that do nothing, unless the
// text put in has sequences of its own.
func (t *Store) tailOf(a int, first string, ra, b int, last string, rb int, text string) string {
	rest := last[rb:]
	if a == b && ra == rb {
		// Nothing removed.
		return rest
	}
	removed := first[ra:]
	if a == b {
		removed = first[ra:rb]
	}
	restSeqs := strings.IndexByte(rest, ansi.ESC) >= 0
	if !restSeqs && (t.root.pen == nil || a == b && strings.IndexByte(removed, ansi.ESC) < 0) {
		// No line sets a style, or the text removed does not change it.
		return rest
	}
	from := penAfter(first[:ra], t.penOf(a))
	var to pen
	if a == b {
		to = penAfter(removed, from)
	} else {
		to = penAfter(last[:rb], t.penOf(b))
	}
	if restSeqs && strings.IndexByte(text, ansi.ESC) < 0 && textLen(rest) == 0 {
		return styleTo(from, penAfter(rest, to))
	}
	return styleTo(from, to) + rest
}

// styleTo returns the escape sequences that set the style and hyperlink of
// to, after text drawn in from, or "" when they are the same.
func styleTo(from, to pen) string {
	var s string
	if !sameSGR(&from.style, &to.style) {
		s = ansi.ResetStyle
		if !ZeroStyle(&to.style) {
			s += to.style.String()
		}
	}
	if from.link != to.link {
		if to.link.URL == "" {
			s += ansi.ResetHyperlink()
		} else {
			s += ansi.SetHyperlink(to.link.URL, to.link.Params)
		}
	}
	return s
}

// splice replaces lines a up to b with lines, which it copies: in the leaf
// they are in, when they are all in one, or else by splitting the tree at
// a and b and joining it again around a tree of lines, so that removing a
// lot of lines drops whole subtrees.
func (t *Store) splice(a, b int, lines []string) {
	if a == b && len(lines) == 0 {
		return
	}
	t.finOK = false
	if t.root == nil {
		t.root = t.treeOf(lines)
		return
	}
	if len(lines) <= leafMax && t.spliceLeaf(a, b, lines) {
		return
	}
	l, rest := t.split(t.root, a)
	_, r := t.split(rest, b-a)
	t.root = t.join(t.join(l, t.treeOf(lines)), r)
}

// treeOf returns a tree of a copy of lines, or nil for none.
func (t *Store) treeOf(lines []string) *node {
	switch {
	case len(lines) == 0:
		return nil
	case len(lines) > leafMax:
		return t.build(slices.Clone(lines))
	}
	n := &node{own: t.token(), lines: append(make([]string, 0, leafMax+1), lines...)}
	for k, s := range n.lines {
		if c := lineChange(s); c != nil {
			if n.pens == nil {
				n.pens = make([]*penChange, len(n.lines), leafMax+1)
			}
			n.pens[k] = c
		}
	}
	n.sumLeaf()
	return n
}

// spliceLeaf is [Store.splice] for lines a up to b in one leaf, or a == b
// at the start or end of one, which it reports whether they are, changing
// nothing when they are not. It changes the leaf and the nodes on the path
// to it in place, copying those a snapshot shares, splits or merges with a
// neighbor each that ends up holding too much or too little, and works out
// again what each keeps of the nodes it changed, so that what the root
// keeps is kept.
func (t *Store) spliceLeaf(a, b int, lines []string) bool {
	var path [maxDepth]step
	depth, n, i := 0, t.root, a
	for !n.leaf() {
		k, in := n.locate(i)
		if in+b-a > n.kidLines[k] {
			return false
		}
		path[depth] = step{n, k}
		depth++
		n, i = n.kids[k], in
	}
	n = t.mut(t.root)
	t.root = n
	for d := range depth {
		path[d].n = n
		kid := t.mut(n.kids[path[d].k])
		n.kids[path[d].k] = kid
		n = kid
	}
	t.leafReplace(n, i, i+b-a, lines)
	for d := depth - 1; d >= 0; d-- {
		t.settle(path[d].n, path[d].k)
	}
	t.settleRoot()
	return true
}

// leafReplace replaces lines j up to e of leaf n, which the store owns,
// with lines, no more than a leaf holds, and works out for them what each
// of n's cache slots keeps.
func (t *Store) leafReplace(n *node, j, e int, lines []string) {
	var changes [leafMax]*penChange
	styled := false
	for k, s := range lines {
		changes[k] = lineChange(s)
		styled = styled || changes[k] != nil
	}
	if styled && n.pens == nil {
		n.pens = make([]*penChange, len(n.lines), leafMax+1)
	}
	if n.pens != nil {
		n.pens = reslice(n.pens, j, e, len(lines))
		copy(n.pens[j:], changes[:len(lines)])
	}
	old := len(n.lines)
	n.lines = reslice(n.lines, j, e, len(lines))
	copy(n.lines[j:], lines)
	n.sumLeaf()
	for k := range n.m {
		ms := &n.m[k]
		if !ms.ok {
			continue
		}
		ms.w = reslice(ms.w, j, e, len(lines))
		for x, s := range lines {
			ms.w[j+x] = measureLine(s, ms.method)
		}
		ms.widest = widestOf(ms.w)
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
		if len(rs.r) == 0 {
			rs.r = ones(rs.r, old)
		}
		rs.r = reslice(rs.r, j, e, len(lines))
		for x, s := range lines {
			rs.r[j+x] = int32(lineRows(s, ms.w[j+x], rs.key, &t.work.starts)) //nolint:gosec // A line has far fewer rows than 1<<31.
		}
		rs.r, rs.total = leafRows(rs.r, len(n.lines))
	}
}

// reslice returns s with s[j:e] replaced by n values to be set, in s's own
// storage when it has room.
func reslice[T any](s []T, j, e, n int) []T {
	old := len(s)
	size := old - (e - j) + n
	if size > cap(s) {
		s = slices.Grow(s, size-old)
	}
	full := s[:max(old, size)]
	copy(full[j+n:], full[e:old])
	if size < old {
		clear(full[size:old])
	}
	return full[:size]
}

// settle has child k of inner node n, which the store owns, hold what a
// child may again after an edit under it: split in two when it holds too
// much, or merged with a neighbor, and split again if that holds too much,
// when it holds too little. It works out again what n keeps of the
// children it changed.
func (t *Store) settle(n *node, k int) {
	kid := n.kids[k]
	most := leafMax
	if !kid.leaf() {
		most = fanMax
	}
	switch {
	case kid.size() > most:
		r := t.splitAt(kid, kid.size()/2)
		t.refresh(n, k)
		t.insertKid(n, k+1, r)
		return
	case kid.okChild():
		t.refresh(n, k)
		return
	}
	l := k
	if k+1 == len(n.kids) {
		l = k - 1
	}
	left := t.mut(n.kids[l])
	n.kids[l] = left
	t.absorb(left, n.kids[l+1])
	t.removeKid(n, l+1)
	if left.size() <= most {
		t.refresh(n, l)
		return
	}
	r := t.splitAt(left, left.size()/2)
	t.refresh(n, l)
	t.insertKid(n, l+1, r)
}

// settleRoot has the root hold what it may again after an edit: split in
// two under a new root when it holds too much, replaced by its child when
// it holds only one, and nil when it holds no lines.
func (t *Store) settleRoot() {
	n := t.root
	most := leafMax
	if !n.leaf() {
		most = fanMax
	}
	switch {
	case n.size() > most:
		r := t.splitAt(n, n.size()/2)
		t.root = t.newInner(append(make([]*node, 0, fanMax+1), n, r))
	case n.leaf() && len(n.lines) == 0:
		t.root = nil
	default:
		for !n.leaf() && len(n.kids) == 1 {
			n = n.kids[0]
		}
		t.root = n
	}
}

// splitAt moves the lines or children of n, which the store owns, from at
// on to a new node after it, which it returns.
func (t *Store) splitAt(n *node, at int) *node {
	if n.leaf() {
		return t.splitLeaf(n, at)
	}
	return t.splitInner(n, at)
}

// absorb adds the lines or children of r, a node of the same height, to
// the end of n, which the store owns, with what n's cache slots keep of
// them, from r's: a slot r lacks is dropped.
func (t *Store) absorb(n, r *node) {
	size := n.size()
	if n.leaf() {
		n.lines = append(n.lines, r.lines...)
		if n.pens != nil || r.pens != nil {
			if n.pens == nil {
				n.pens = make([]*penChange, size, len(n.lines)+1)
			}
			n.pens = appendPens(n.pens, r.pens, len(r.lines))
		}
		n.sumLeaf()
	} else {
		n.kids = append(n.kids, r.kids...)
		n.sumInner()
	}
	for k := range n.m {
		ms := &n.m[k]
		if !ms.ok {
			continue
		}
		rm := r.findM(ms.method)
		if rm == nil {
			ms.ok = false
			continue
		}
		ms.w = append(ms.w, rm.w...)
		ms.widest = max(ms.widest, rm.widest)
	}
	for k := range n.r {
		rs := &n.r[k]
		if !rs.ok {
			continue
		}
		rr := r.findR(rs.key)
		if rr == nil {
			rs.ok = false
			continue
		}
		if n.leaf() {
			if len(rs.r) > 0 || len(rr.r) > 0 {
				if len(rs.r) == 0 {
					rs.r = ones(rs.r, size)
				}
				rs.r = appendRows(rs.r, rr.r, len(r.lines))
			}
		} else {
			rs.rk = append(rs.rk, rr.rk...)
		}
		rs.total += rr.total
	}
}

// removeKid removes child k of inner node n, which the store owns, with
// what n keeps of it.
func (t *Store) removeKid(n *node, k int) {
	n.n -= n.kidLines[k]
	n.bytes -= n.kidBytes[k]
	n.kids = slices.Delete(n.kids, k, k+1)
	n.kidLines = slices.Delete(n.kidLines, k, k+1)
	n.kidBytes = slices.Delete(n.kidBytes, k, k+1)
	if n.kidPens != nil {
		n.kidPens = slices.Delete(n.kidPens, k, k+1)
		n.pen = composeAll(n.kidPens)
	}
	for s := range n.m {
		if ms := &n.m[s]; ms.ok {
			ms.w = slices.Delete(ms.w, k, k+1)
			ms.widest = widestOf(ms.w)
		}
	}
	for s := range n.r {
		if rs := &n.r[s]; rs.ok {
			rs.total -= rs.rk[k]
			rs.rk = slices.Delete(rs.rk, k, k+1)
		}
	}
}

// ByteAt returns the byte of the text of line i, as [PlainText] gives it,
// of the glyph drawn at cell x of the line, measured with method: its first
// byte for each of the cells of a wide glyph or a tab, and the end of the
// text for a cell past the line's last glyph. A long line is scanned from
// the checkpoint before x.
func (t *Store) ByteAt(i int, method ansi.Method, x int) int {
	s := t.Line(i)
	switch {
	case x <= 0:
		return 0
	case t.plain(i, s):
		return min(x, len(s))
	}
	var from checkpoint
	if len(s) >= LongLineBytes && x >= CheckpointEvery {
		from = t.cutFrom(i, method, x)
	}
	at, tab := from.t, 0
	sc := scanner{s: s, i: from.i, x: from.x, method: method}
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		switch {
		case k != glyphText:
			continue
		case sc.tab && !sc.tabFirst:
			if sc.x == x {
				return tab
			}
			continue
		case sc.w > 0 && x < sc.x+sc.w:
			return at
		case sc.tabFirst:
			tab = at
		}
		at += textBytes(&sc)
	}
	return at
}

// CellOf returns the cell that byte col of the text of line i, as
// [PlainText] gives it, is drawn at, measured with method: the first cell
// of the glyph it is in, the cell of the glyph before it for a combining
// mark drawn on that, and the cell after the line's last glyph for the end
// of the text. A long line is scanned from the checkpoint before col.
func (t *Store) CellOf(i int, method ansi.Method, col int) int {
	s := t.Line(i)
	switch {
	case col <= 0:
		return 0
	case t.plain(i, s):
		return min(col, len(s))
	}
	from := t.textFrom(i, s, method, col)
	at, base := from.t, from.x
	sc := scanner{s: s, i: from.i, x: from.x, method: method}
	for k := sc.next(); k != glyphEnd; k = sc.next() {
		n := textBytes(&sc)
		if k != glyphText || n == 0 {
			continue
		}
		if sc.w > 0 {
			base = sc.x
		}
		if col < at+n {
			return base
		}
		at += n
	}
	return sc.x
}

// Snapshot is the lines of a store as they were when [Store.Snapshot] took
// it, which they stay as the store goes on changing. Its methods read only
// what the nodes hold of their lines, which is never written again once a
// snapshot shares them, so that they may be called from any goroutine while
// the store is changed. The zero value holds no lines.
type Snapshot struct{ root *node }

// Snapshot returns the store's lines as they are, in a step: the nodes of
// the tree are shared with it, and a change copies those it touches rather
// than change them in place.
func (t *Store) Snapshot() Snapshot {
	if t.root != nil && t.root != t.snapped {
		t.snapped = t.snapshot()
	}
	return Snapshot{root: t.root}
}

// Restore puts the store's lines back to those of snapshot s, in a step:
// the store shares the nodes of s, and what they keep of their lines, such
// as how wide each is, so that nothing is measured again. A snapshot of
// another store may be restored too, the two then sharing what they keep,
// which is safe while both are changed on one goroutine.
func (t *Store) Restore(s Snapshot) {
	t.root = s.root
	// The storage the last tree was built in is no longer the tree's alone
	// to take over once cleared.
	t.slab = nil
	t.snapped = s.root
	// The lines it shares with the snapshot are not the store's to free.
	t.most = 0
	t.changed()
}

// Len returns the number of lines.
func (s Snapshot) Len() int {
	if s.root == nil {
		return 0
	}
	return s.root.n
}

// Line returns line i, which must be one of the snapshot's.
func (s Snapshot) Line(i int) string {
	n := s.root
	for !n.leaf() {
		var k int
		k, i = n.locate(i)
		n = n.kids[k]
	}
	return n.lines[i]
}

// String returns the lines, joined with newlines.
func (s Snapshot) String() string { return joinLines(s.root) }

// Lines returns an iterator over the lines from line from on, and their
// numbers.
func (s Snapshot) Lines(from int) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		if s.root == nil || from >= s.root.n {
			return
		}
		from = max(from, 0)
		var c cursor
		c.seek(s.root, from)
		for j := from - c.start; ; j = 0 {
			for ; j < len(c.leaf.lines); j++ {
				if !yield(c.start+j, c.leaf.lines[j]) {
					return
				}
			}
			if !c.next() {
				return
			}
		}
	}
}

// joinLines returns the lines of the tree under root, joined with newlines.
func joinLines(root *node) string {
	if root == nil {
		return ""
	}
	var s strings.Builder
	s.Grow(root.bytes - 1)
	var c cursor
	c.seek(root, 0)
	for {
		for k, line := range c.leaf.lines {
			if k > 0 || c.start > 0 {
				s.WriteByte('\n')
			}
			s.WriteString(line)
		}
		if !c.next() {
			return s.String()
		}
	}
}
