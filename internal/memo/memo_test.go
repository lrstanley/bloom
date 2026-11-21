// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package memo

import (
	"fmt"
	"testing"
)

func TestRetire(t *testing.T) {
	t.Parallel()

	var tab Table[string]
	for i, v := range []string{"a", "b", "c"} {
		s := tab.At(i)
		s.OK, s.V = true, v
	}
	tab.Retire()

	// The next frame uses only the slot at 1, which keeps its value.
	if s := tab.At(1); !s.OK || s.V != "b" {
		t.Fatalf("slot 1 is %+v after a frame that used it", *s)
	}
	tab.Retire()
	for i, s := range tab.slots {
		if s.OK != (i == 1) {
			t.Fatalf("slot %d kept is %v", i, s.OK)
		}
	}

	// A slot past the end grows the table.
	if s := tab.At(9); s.OK || len(tab.slots) != 10 {
		t.Fatalf("slot 9 is %+v, in %d slots", *s, len(tab.slots))
	}
}

func TestMapRetire(t *testing.T) {
	t.Parallel()

	var m Map[int, int]
	for k := range 8 {
		s := m.At(k)
		s.OK, s.V = true, k
	}
	m.Retire()

	// The next frame uses only 1, which keeps its value, and the map, far
	// emptier than it was, is made again.
	before := fmt.Sprintf("%p", m.slots)
	if s := m.At(1); !s.OK || s.V != 1 {
		t.Fatalf("1 is %+v after a frame that used it", *s)
	}
	m.Retire()
	if len(m.slots) != 1 || m.slots[1] == nil || m.peak != 1 {
		t.Fatalf("kept %v, peak %d", m.slots, m.peak)
	}
	if fmt.Sprintf("%p", m.slots) == before {
		t.Fatal("not made again")
	}
}
