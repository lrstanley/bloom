// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package memo

import "testing"

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
