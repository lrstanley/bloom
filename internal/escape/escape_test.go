// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package escape

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// decodes reports whether decoding s with a parser does not panic.
func decodes(s string) (ok bool) {
	defer func() { ok = recover() == nil }()
	p := ansi.NewParser()
	var state byte
	for s != "" {
		_, _, n, next := ansi.DecodeSequence(s, state, p)
		state, s = next, s[max(n, 1):]
	}
	return true
}

func TestOverflows(t *testing.T) {
	t.Parallel()

	seps := func(n int) string { return strings.Repeat(";", n) }
	tests := []struct {
		s    string
		want bool
	}{
		{"\x1b[31m", false},
		{"\x1b[" + seps(30) + "0m", false},
		{"\x1b[" + seps(40) + "0m", true},
		{"\x1b[?" + seps(40) + "0h", true},
		{"\x1b[38" + strings.Repeat(":", 40) + "2m", true},
		{"\x9b" + seps(40) + "0m", true},
		{"\x1bP" + seps(40) + "0q\x1b\\", true},
		{"\x90" + seps(40) + "0q\x1b\\", true},
		{"\x1b [" + seps(40) + "0m", true},
		{"\x1b]" + seps(40) + "0\x07", false}, // An OSC has no parameters.
		{"a" + "\x1b[" + seps(40) + "0m", false},
		{"\x1b[0<" + seps(40) + "0m", false}, // It ends at the marker.
	}
	for _, tc := range tests {
		if got := Overflows(tc.s); got != tc.want {
			t.Errorf("Overflows(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestDropOverflows(t *testing.T) {
	t.Parallel()

	seps := strings.Repeat(";", 40)
	tests := []struct{ in, want string }{
		{"a\x1b[31mb", "a\x1b[31mb"},
		{"a\x1b[" + seps + "0mb", "ab"},
		{"a\x9b" + seps + "0mb", "ab"},
		{"a\x1bP" + seps + "0qdata\x1b\\b", "ab"},
		{"a\x1bP" + seps + "0qdata", "a"},
		// What a sequence cut short does not join what follows it.
		{"\x9b\x9b" + seps + "A" + strings.Repeat(";", 32) + "0mb", strings.Repeat(";", 32) + "0mb"},
		{"\x1b [;\x1b[" + seps + "A" + strings.Repeat(";", 32) + "0mb", strings.Repeat(";", 32) + "0mb"},
		{"\x1b7\x1b[" + seps + "Ab", "\x1b7b"},
		{"\x1b[ ;\x1b[" + seps + "Ab", "\x1b[ ;b"},
		{"\x1b[0<\x1b[" + seps + "Ab", "\x1b[0<b"},
		// It ends where the decoder ends it.
		{"a\x1b[" + seps + " ;b", "a;b"},
		{"a\x1bP" + seps + "<b", "a<b"},
		{"a\x1bP" + seps + "0qdata\x9cb", "ab"},
		// A string the decoder ends inside a character leaves the rest of it.
		{"\x1b]8;;✛" + seps + "0mb", "\x1b]8;;\xe2\x9cb"},
		// A byte of a character is not an introducer.
		{"✛" + seps + "0m", "✛" + seps + "0m"},
	}
	for _, tc := range tests {
		got := DropOverflows(tc.in)
		if got != tc.want {
			t.Errorf("DropOverflows(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if !decodes(got) {
			t.Errorf("DropOverflows(%q) = %q, which panics when decoded", tc.in, got)
		}
	}
}

func TestCleanLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seq  string
		want bool
	}{
		{"\x1b]8;;https://example.com\x1b\\", true},
		{"\x1b]8;id=1;https://example.com/ü\x07", true},
		{"\x1b]8;;", true},
		{"\x1b]8;;https://x\x81y\x07", false},
		{"\x1b]8;;https://x\u009cy\x07", false},
		{"\x1b]8;;https://x\x1by\x07", false},
		{"\x1b]8;;https://x\ty\x07", false},
		{"\x1b]8;;https://x\x9c", true},
		{"\x1b]8;;https://xꛜ", false}, // The decoder ends it inside the ꛜ.
		{"\x1b]8;;" + strings.Repeat("a", linkData-3) + "\x07", true},
		{"\x1b]8;;" + strings.Repeat("a", linkData-5) + "ꛛ\x07", false}, // A parser cuts it inside the ꛛ.
	}
	for _, tc := range tests {
		if got := CleanLink(tc.seq); got != tc.want {
			t.Errorf("CleanLink(%q) = %v, want %v", tc.seq, got, tc.want)
		}
	}
}

func FuzzDropOverflows(f *testing.F) {
	f.Add("\x9b\x9b" + strings.Repeat(";", 40) + "A" + strings.Repeat(";", 32) + "0m")
	f.Add("\x1b[?" + strings.Repeat(";", 40) + "0h")
	f.Fuzz(func(t *testing.T, s string) {
		got := DropOverflows(s)
		if !decodes(got) {
			t.Fatalf("DropOverflows(%q) = %q, which panics when decoded", s, got)
		}
		if again := DropOverflows(got); again != got {
			t.Fatalf("DropOverflows(%q) = %q, which drops to %q", s, got, again)
		}
	})
}
