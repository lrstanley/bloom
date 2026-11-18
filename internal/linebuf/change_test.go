// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import "testing"

func TestChangeLog(t *testing.T) {
	t.Parallel()

	var l ChangeLog
	c := func(col int) Change { return Change{one: edited{nend: Pos{Col: col}}} }
	for rev := uint64(1); rev <= 3; rev++ {
		l.Add(rev, c(int(rev)))
	}
	for _, tc := range []struct {
		rev  uint64
		ends []int
		ok   bool
	}{
		{0, []int{1, 2, 3}, true},
		{2, []int{3}, true},
		{3, nil, true},
		{4, nil, false}, // a revision to come
	} {
		got, ok := l.Since(tc.rev, 3)
		if ok != tc.ok || len(got) != len(tc.ends) {
			t.Fatalf("since %d: %d changes, %v", tc.rev, len(got), ok)
		}
		for i, e := range tc.ends {
			if got[i].End().Col != e {
				t.Fatalf("since %d: change %d ends at %v, want column %d", tc.rev, i, got[i].End(), e)
			}
		}
	}

	// A reset cuts off what came before it.
	l.Reset(5)
	if _, ok := l.Since(3, 5); ok {
		t.Fatal("changes followed through a reset")
	}
	if got, ok := l.Since(5, 5); !ok || len(got) != 0 {
		t.Fatalf("no changes since the reset: %d, %v", len(got), ok)
	}

	// More changes than it keeps.
	l = ChangeLog{}
	for rev := uint64(1); rev <= logSize+8; rev++ {
		l.Add(rev, c(int(rev)))
	}
	if _, ok := l.Since(7, logSize+8); ok {
		t.Fatal("changes followed past those kept")
	}
	if got, ok := l.Since(8, logSize+8); !ok || len(got) != logSize || got[0].End().Col != 9 {
		t.Fatalf("the changes kept: %d, %v", len(got), ok)
	}
}

func TestStoreEditSorts(t *testing.T) {
	t.Parallel()

	// The edits are sorted in place, the one that overlaps dropped, and
	// those the store keeps are taken in order.
	s := newStore("0123456789")
	edits := []TextEdit{
		{From: Pos{0, 8}, To: Pos{0, 8}, Text: "Z"},
		{From: Pos{0, 5}, To: Pos{0, 2}, Text: "X"}, // swapped
		{From: Pos{0, 4}, To: Pos{0, 7}, Text: "Y"}, // overlaps X
	}
	c, changed := s.Edit(edits...)
	if !changed || s.String() != "01X567Z89" {
		t.Fatalf("got %q, changed %v", s.String(), changed)
	}
	if got := c.Map(Pos{0, 9}, false); got != (Pos{0, 8}) {
		t.Fatalf("the end maps to %v", got)
	}
	if edits[0].From != (Pos{0, 2}) || edits[1].From != (Pos{0, 8}) {
		t.Fatalf("edits left as %+v", edits[:2])
	}

	// Each edit is at the place it was given in the text as it was, though
	// the edit after it joins what follows into a glyph across that place.
	const acute = "\u0301"
	s = newStore("ab")
	s.Edit(TextEdit{From: Pos{0, 1}, To: Pos{0, 1}, Text: "X"}, TextEdit{From: Pos{0, 1}, To: Pos{0, 1}, Text: acute})
	if want := "aX" + acute + "b"; s.String() != want {
		t.Fatalf("got %q, want %q", s.String(), want)
	}
	s = newStore("ab")
	s.Edit(TextEdit{From: Pos{0, 0}, To: Pos{0, 1}}, TextEdit{From: Pos{0, 1}, To: Pos{0, 1}, Text: acute})
	if want := acute + "b"; s.String() != want {
		t.Fatalf("got %q, want %q", s.String(), want)
	}

	// Edits that put nothing in place of nothing change nothing.
	s = newStore("01X567Z89")
	rev := s.Rev()
	if _, again := s.Edit(TextEdit{From: Pos{0, 1}, To: Pos{0, 1}}, TextEdit{From: Pos{0, 3}, To: Pos{0, 3}}); again || s.Rev() != rev {
		t.Fatal("empty edits changed the store")
	}
}
