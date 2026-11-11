// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"
)

// passedThrough reports whether a cell of s parsed holds a command written
// to the terminal as it is.
func passedThrough(s string) bool {
	c := Parse(s, method)
	for y := range c.Height() {
		for x := range c.Width() {
			cell := c.CellAt(x, y)
			if cell == nil {
				continue
			}
			if strings.IndexByte(cell.Content, 0x1b) >= 0 {
				return true
			}
			for _, r := range cell.Content {
				if r >= 0x80 && r < 0xa0 {
					return true
				}
			}
			for i := range len(cell.Content) {
				switch cell.Content[i] {
				case 0x90, 0x98, 0x9e, 0x9f:
					if !utf8.ValidString(cell.Content) {
						return true
					}
				}
			}
		}
	}
	return false
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"plain", "hello", "hello"},
		{"styles and links kept", "a\x1b[31mb\x1b[m\x1b]8;;https://x\x1b\\c\x1b]8;;\x1b\\", "a\x1b[31mb\x1b[m\x1b]8;;https://x\x1b\\c\x1b]8;;\x1b\\"},
		{"wide and emoji kept", "世界🙂é", "世界🙂é"},
		{"APC", "a\x1b_Ga=d,d=A\x1b\\b", "ab"},
		{"DCS", "a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"tmux passthrough", "a\x1bPtmux;\x1b\x1b]52;c;aGk=\x07\x1b\\b", "ab"},
		{"C1 character", "a\u009fGx\x1b\\b", "aGxb"},
		{"other sequences", "a\x1b[2J\x1b[?25l\x1b]52;c;aGk=\x07\x1b]0;title\x1b\\\x1b7b", "ab"},
		{"controls", "a\x07b\x08c\x00d\x7fe\x85f", "abcdef"},
		{"tabs and line breaks kept", "a\tb\r\nc\n", "a\tb\r\nc\n"},
		{"invalid bytes replaced", "a\xffb\xe2\x9fc", "a\ufffdb\ufffdc"},
		{"SOS", "a\x1bXdata\x1b\\b", "ab"},
		{"PM", "a\x1b^data\x1b\\b", "ab"},
		{"one-byte APC", "a\x9fGa=d\x1b\\b", "ab"},
		{"one-byte DCS", "a\x90q\x1b\\b", "ab"},
		{"unterminated", "a\x1b_Gab", "a"},
		{"joined by taking one out", "a\x1b\x1b_x\x1b\\Pq\x1b\\b", "aPqb"},
		{"after an emoji", "🙂\x1b_Gx\x1b\\b", "🙂b"},
		{"hyperlink ended inside a character", "a\x1b]8;;ꛜb", "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Sanitize(tc.in)
			if got != tc.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if passedThrough(got) {
				t.Fatalf("Sanitize(%q) = %q, which passes a command through", tc.in, got)
			}
		})
	}
}

func TestSanitizeKeepsString(t *testing.T) { //nolint:paralleltest // AllocsPerRun.
	// Nothing to take out costs no copy.
	s := strings.Repeat("a\x1b[31mb\x1b[m 世界", 100)
	if got := Sanitize(s); unsafe.StringData(got) != unsafe.StringData(s) {
		t.Fatal("Sanitize copied a string with nothing to take out")
	}
	if n := testing.AllocsPerRun(10, func() { Sanitize(s) }); n != 0 {
		t.Fatalf("Sanitize allocated %v times for a string with nothing to take out", n)
	}
}

func TestCacheSanitized(t *testing.T) {
	t.Parallel()

	c := NewCache(method)
	s := "a\x1b_Gx\x1b\\b"
	if raw := c.Get(s); !strings.Contains(raw.Render(), "\x1b_G") {
		t.Fatalf("Get dropped the command: %q", raw.Render())
	}
	safe := c.Sanitized(s)
	if strings.Contains(safe.Render(), "\x1b_") || safe.String() != "ab" {
		t.Fatalf("Sanitized = %q", safe.Render())
	}
	if c.Sanitized(s) != safe {
		t.Fatal("Sanitized parsed again")
	}
	if plain := "ab"; c.Sanitized(plain) != c.Get(plain) {
		t.Fatal("Sanitized parsed a string with nothing to take out twice")
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range []string{
		"a\x1b_Gx\x1b\\b", "a\x1bPq\x1b\\b", "a\x9fx\x1b\\", "a\x1b\x1b_x\x1b\\Pq\x1b\\b",
		"\x1b]0;x\x1bPq\x1b\\", "\x1b[\x1bPq\x1b\\", "🙂\x9f\x1b\\", "\xe2\x9fx\x1b\\",
		"\x9b\x9b" + strings.Repeat(";", 40) + "A" + strings.Repeat(";", 32) + "0m",
		"\x1b[" + strings.Repeat(";", 40) + "0m", "\x9b" + strings.Repeat(":", 40) + "0m", "\x1bP" + strings.Repeat(";", 40) + "0q\x1b\\",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		Parse(s, method) // Must not panic.
		got := Sanitize(s)
		if passedThrough(got) {
			t.Fatalf("Sanitize(%q) = %q, which passes a command through", s, got)
		}
		if Sanitize(got) != got {
			t.Fatalf("Sanitize(%q) = %q, which sanitizes to %q", s, got, Sanitize(got))
		}
	})
}
