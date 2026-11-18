// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"slices"
	"strings"
)

// Compare returns -1 when p comes before q, 1 when it comes after, and 0
// when they are the same.
func (p Pos) Compare(q Pos) int {
	switch {
	case p.Before(q):
		return -1
	case q.Before(p):
		return 1
	}
	return 0
}

// Edit makes edits at once, as one change, and returns it: each clamped,
// with its ends swapped when From comes after To, and with the positions of
// the store before any of them, in any order, and sorted in place. Of two
// that overlap, rather than touch, the one given later is dropped, and text
// put in at the same place by more than one goes in in the order given. It
// reports whether the text changed: not when every edit puts nothing in
// place of nothing, in a store with lines, which takes the change as though
// it held one empty line.
func (t *Store) Edit(edits ...TextEdit) (Change, bool) {
	switch len(edits) {
	case 0:
		return Change{}, false
	case 1:
		e := edits[0]
		from, to := t.Clamp(e.From), t.Clamp(e.To)
		if to.Before(from) {
			from, to = to, from
		}
		c := Change{one: edited{from: from, to: to, nfrom: from, nend: EndOf(from, e.Text)}}
		if t.Len() > 0 && from == to && e.Text == "" {
			return c, false
		}
		t.Replace(TextEdit{From: from, To: to, Text: e.Text})
		return c, true
	}
	// The edits kept, in order, are put at the start of edits, which none
	// of those not yet looked at are in.
	kept := edits[:0]
	for _, e := range edits {
		from, to := t.Clamp(e.From), t.Clamp(e.To)
		if to.Before(from) {
			from, to = to, from
		}
		te := TextEdit{From: from, To: to, Text: e.Text}
		// Where it goes among those kept, after those it ties with.
		i, _ := slices.BinarySearchFunc(kept, te, func(k, e TextEdit) int {
			if c := k.From.Compare(e.From); c != 0 {
				return c
			}
			if c := k.To.Compare(e.To); c != 0 {
				return c
			}
			return -1
		})
		if i > 0 && overlaps(kept[i-1], te) || i < len(kept) && overlaps(te, kept[i]) {
			continue
		}
		kept = slices.Insert(kept, i, te)
	}
	changed := make([]edited, len(kept))
	noop := t.Len() > 0
	for i, e := range kept {
		nfrom := e.From
		if i > 0 {
			nfrom = changed[i-1].shift(e.From)
		}
		changed[i] = edited{from: e.From, to: e.To, nfrom: nfrom, nend: EndOf(nfrom, e.Text)}
		noop = noop && e.From == e.To && e.Text == ""
	}
	c := Change{one: changed[len(changed)-1], more: changed}
	if noop {
		return c, false
	}
	t.Replace(kept...)
	return c, true
}

// overlaps reports whether edits a and b, a not after b, share more than
// a place where they touch.
func overlaps(a, b TextEdit) bool { return a.From.Before(b.To) && b.From.Before(a.To) }

// EndOf returns where text ends, put in at at.
func EndOf(at Pos, text string) Pos {
	_, last, ok := strings.CutLast(text, "\n")
	if !ok {
		return Pos{Line: at.Line, Col: at.Col + TextLen(text)}
	}
	return Pos{Line: at.Line + strings.Count(text, "\n"), Col: TextLen(last)}
}

// Change is what [Store.Edit] did to a store's text, to follow positions
// through it with. Its methods take positions in the text as it was before
// the change, and give them in the text as it is after. The zero value is
// a change that moves no position.
type Change struct {
	one  edited   // the last edit
	more []edited // every edit in order, when there are more than one
}

// edited is an edit as a change made it: the text from from up to to of
// the store before, which is from nfrom up to nend after it.
type edited struct {
	from, to, nfrom, nend Pos
}

// past reports whether p is past edit e, at or after its end, to be shifted
// by it rather than moved into it: after its end, or at the end of text it
// replaced, or where it put text in when after.
func (e *edited) past(p Pos, after bool) bool {
	c := p.Compare(e.to)
	return c > 0 || c == 0 && (e.from != e.to || after)
}

// shift returns p, past edit e, moved as e moves the text after it.
func (e *edited) shift(p Pos) Pos {
	if p.Line == e.to.Line {
		return Pos{Line: e.nend.Line, Col: e.nend.Col + p.Col - e.to.Col}
	}
	return Pos{Line: p.Line + e.nend.Line - e.to.Line, Col: p.Col}
}

// at returns p, not past edit e, where e moves it: unchanged before it, to
// the start of its text at its start, and to the end of its text within
// the text it replaced.
func (e *edited) at(p Pos) (Pos, bool) {
	switch {
	case p.Before(e.from):
		return p, false
	case p == e.from:
		return e.nfrom, true
	}
	return e.nend, true
}

// Map returns where position p of the text before the change is after it:
// unchanged before the text changed, moved with the text after it, and at
// the end of the text put in when it was within the text replaced. A
// position where text was put in stays before that text, or with after,
// goes after it, as a caret where text is typed does.
func (c Change) Map(p Pos, after bool) Pos {
	if c.more == nil {
		if c.one.past(p, after) {
			return c.one.shift(p)
		}
		p, _ = c.one.at(p)
		return p
	}
	// The edits p is past come first.
	i, _ := slices.BinarySearchFunc(c.more, p, func(e edited, p Pos) int {
		if e.past(p, after) {
			return -1
		}
		return 1
	})
	if i < len(c.more) {
		if q, in := c.more[i].at(p); in {
			return q
		}
	}
	if i == 0 {
		return p
	}
	return c.more[i-1].shift(p)
}

// MapSpan returns where the run of text from a to b, either way round, is
// after the change, its ends mapped so that text put in at either end is
// left out of it, and an empty one mapped as Map maps a position.
func (c Change) MapSpan(a, b Pos) (from, to Pos) {
	switch {
	case a == b:
		p := c.Map(b, false)
		return p, p
	case a.Before(b):
		return c.Map(a, true), c.Map(b, false)
	}
	return c.Map(a, false), c.Map(b, true)
}

// End returns where the text put in by the change's last edit ends, in the
// text after it, which is where a caret goes after typing.
func (c Change) End() Pos { return c.one.nend }

// logSize is how many changes a [ChangeLog] keeps.
const logSize = 32

// ChangeLog is the last few changes made to a store by its edits, each
// with the [Store.Rev] it made, to follow positions through from one
// revision to a later one. The zero value is ready to use.
type ChangeLog struct {
	ring [logSize]logged
	n    int    // changes added, the last at ring[(n-1)%logSize]
	lost uint64 // the revision of the last change no longer kept
	cut  uint64 // a change at this revision, such as setting the text, moved positions it does not tell of
}

type logged struct {
	rev uint64
	c   Change
}

// Add notes change c, which made revision rev.
func (l *ChangeLog) Add(rev uint64, c Change) {
	if l.n >= logSize {
		l.lost = l.ring[l.n%logSize].rev
	}
	l.ring[l.n%logSize] = logged{rev: rev, c: c}
	l.n++
}

// Reset notes a change at revision rev that positions cannot be followed
// through.
func (l *ChangeLog) Reset(rev uint64) { *l = ChangeLog{cut: rev} }

// Since returns the changes made after revision rev, up to revision now,
// in order, and true, or false when positions from then cannot be followed
// to now: the log was reset since, or more changes were made than it
// keeps. Lines appended move no position, and need no change.
func (l *ChangeLog) Since(rev, now uint64) ([]Change, bool) {
	switch {
	case rev > now || rev < l.cut || rev < l.lost:
		return nil, false
	case rev == now:
		return nil, true
	}
	first := l.n - min(l.n, logSize)
	for first < l.n && l.ring[first%logSize].rev <= rev {
		first++
	}
	out := make([]Change, 0, l.n-first)
	for i := first; i < l.n; i++ {
		out = append(out, l.ring[i%logSize].c)
	}
	return out, true
}
