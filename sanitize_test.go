// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// leaks reports whether out, a rendered frame, writes a command to the
// terminal as it is: an APC, DCS, SOS, or PM, or the start of one.
func leaks(out string) bool {
	for _, intro := range []string{"\x1bP", "\x1bX", "\x1b^", "\x1b_"} {
		if strings.Contains(out, intro) {
			return true
		}
	}
	for i := 0; i < len(out); {
		r, n := utf8.DecodeRuneInString(out[i:])
		switch {
		case r == utf8.RuneError && n == 1 && out[i] >= 0x80 && out[i] < 0xa0:
			return true
		case r >= 0x80 && r < 0xa0:
			return true
		}
		i += n
	}
	return false
}

// rawView is a component whose view is s.
type rawView string

func (v rawView) View() string { return string(v) }

var commands = map[string]string{
	"APC":          "ab\x1b_Ga=d,d=A\x1b\\cd",
	"DCS":          "ab\x1bPq#0;2;0;0;0\x1b\\cd",
	"SOS":          "ab\x1bXdata\x1b\\cd",
	"PM":           "ab\x1b^data\x1b\\cd",
	"one-byte APC": "ab\x9fGa=d\x1b\\cd",
	"one-byte DCS": "ab\x90q\x1b\\cd",
	"C1 character": "ab\u009fGx\x1b\\cd",
}

func TestTextDropsCommands(t *testing.T) {
	t.Parallel()

	for name, s := range commands {
		nodes := map[string]Child{
			"Text":          Text(s),
			"Text.Style":    Text(s).Style(lipgloss.NewStyle().Bold(true)),
			"Text.Truncate": Text(s).Truncate(),
			"Viewport":      Viewport(NewBuffer(s)),
			"Viewport wrapped and styled": Viewport(NewBuffer(s)).Wrap(WrapChars).
				LineStyle(func(int) lipgloss.Style { return lipgloss.NewStyle().Underline(true) }).
				Marks(MarkMatches(regexp.MustCompile("b"), lipgloss.NewStyle().Reverse(true))),
			"Viewport gutter": Viewport(NewBuffer("x")).Gutter(6, func(GutterRow) string { return s }),
		}
		for node, c := range nodes {
			if out := Render(10, 2, c); leaks(out) {
				t.Errorf("%s in %s: %q", name, node, out)
			}
		}
	}
}

func TestTrustedKeepsCommands(t *testing.T) {
	t.Parallel()

	const apc = "\x1b_Ga=d,d=A\x1b\\"
	for node, c := range map[string]Child{
		"Text":            Text("ab" + apc + "cd").Trusted(),
		"Text.Style":      Text("ab" + apc + "cd").Style(lipgloss.NewStyle().Bold(true)).Trusted(),
		"component":       rawView("ab" + apc + "cd"),
		"Viewport":        Viewport(NewBuffer("ab" + apc + "cd")).Trusted(),
		"Viewport gutter": Viewport(NewBuffer("x")).Gutter(3, func(GutterRow) string { return apc + ">" }).Trusted(),
	} {
		if out := Render(10, 1, c); !strings.Contains(out, apc) {
			t.Errorf("%s dropped the command: %q", node, out)
		}
	}
}

func TestViewportTrustedPlacesCommands(t *testing.T) {
	t.Parallel()

	const apc = "\x1b_Gx\x1b\\"
	tests := []struct {
		name string
		line string
		node func(ViewportNode) ViewportNode
		w    int
		want []string // the rows, with only the command kept of the sequences
	}{
		{"with the glyph after it", "ab" + apc + "cd", nil, 6, []string{"ab" + apc + "cd"}},
		{"on the row of the glyph after it", "ab" + apc + "cd", func(n ViewportNode) ViewportNode { return n.Wrap(WrapChars) }, 2, []string{"ab", apc + "cd"}},
		{"at the end of its line", "ab" + apc, func(n ViewportNode) ViewportNode { return n.Wrap(WrapChars) }, 1, []string{"a", "b" + apc}},
		{"not when its glyph is scrolled past", "ab" + apc + "cd", func(n ViewportNode) ViewportNode { return n.At(ViewportPos{X: 3}) }, 1, []string{"d"}},
		{"when the glyph after it starts the row", "ab" + apc + "cd", func(n ViewportNode) ViewportNode { return n.At(ViewportPos{X: 2}) }, 2, []string{apc + "cd"}},
		{"at a checkpoint of a long line", strings.Repeat("a", checkpointEvery) + apc + strings.Repeat("b", longLineBytes), func(n ViewportNode) ViewportNode { return n.At(ViewportPos{X: checkpointEvery}) }, 2, []string{apc + "bb"}},
		{"at a checkpoint after a combining mark", strings.Repeat("a", checkpointEvery) + apc + "\u0301" + strings.Repeat("b", longLineBytes), func(n ViewportNode) ViewportNode { return n.At(ViewportPos{X: checkpointEvery}) }, 2, []string{apc + "bb"}},
	}
	styles := regexp.MustCompile("\x1b\\[[0-9;:]*m")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := Viewport(NewBuffer(tc.line)).Trusted()
			if tc.node != nil {
				n = tc.node(n)
			}
			rows := strings.Split(Render(tc.w, len(tc.want), n), "\n")
			for i, row := range rows {
				kept := strings.Contains(row, apc)
				row = strings.TrimRight(strings.ReplaceAll(row, apc, "\x00"), " ")
				row = strings.ReplaceAll(styles.ReplaceAllString(row, ""), "\x00", apc)
				if i >= len(tc.want) || row != tc.want[i] {
					t.Fatalf("rows %q, want %q (row %d kept the command: %v)", rows, tc.want, i, kept)
				}
			}
		})
	}
}

func TestTooManyParams(t *testing.T) {
	t.Parallel()

	seps := strings.Repeat(";", 40)
	for name, s := range map[string]string{
		"CSI":          "a\x1b[" + seps + "0mb",
		"one-byte CSI": "a\x9b" + seps + "0mb",
		"DCS":          "a\x1bP" + seps + "0q\x1b\\b",
		"subparams":    "a\x1b[38" + strings.Repeat(":", 40) + "2mb",
		"joined":       "\x9b\x9b" + seps + "A" + strings.Repeat(";", 32) + "0m",
	} {
		lines := strings.Repeat(s+"\n", 3) + strings.Repeat("x", 10<<10) + s
		buf := NewBuffer(lines)
		nodes := map[string]Child{
			"Text":         Text(s),
			"Text.Trusted": Text(s).Trusted(),
			"Text.Style":   Text(s).Style(lipgloss.NewStyle().Bold(true)),
			"component":    rawView(s),
			"Viewport":     Viewport(buf).Trusted(),
			"Viewport wrapped and styled": Viewport(buf).Wrap(WrapWords).
				LineStyle(func(int) lipgloss.Style { return lipgloss.NewStyle().Underline(true) }).
				Marks(MarkMatches(regexp.MustCompile("b"), lipgloss.NewStyle().Reverse(true))),
			"Viewport at a long line": Viewport(buf).At(ViewportPos{Line: 3, X: 5 << 10}),
		}
		for node, c := range nodes {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s in %s: panic: %v", name, node, r)
					}
				}()
				Render(20, 4, c)
			}()
		}
		buf.Append(s)
		buf.Text(0)
		buf.Find(regexp.MustCompile("b"), 0, 0)
	}
}

func FuzzRenderUntrusted(f *testing.F) {
	for _, s := range commands {
		f.Add(s)
	}
	f.Add("a\x1b[" + strings.Repeat(";", 40) + "0mb")
	f.Add("a\x1b[31mb\x1b]8;;https://x\x1b\\c\x1b]8;;\x1b\\\td́e")
	f.Add("0\x1b]8;;ꛜ")
	f.Add("\x1b]8;;" + strings.Repeat("a", 4091) + "ꛛ\x07x")
	f.Fuzz(func(t *testing.T, s string) {
		for node, c := range map[string]Child{
			"Text":     Text(s),
			"Viewport": Viewport(NewBuffer(s)).Wrap(WrapWords),
			"Viewport styled": Viewport(NewBuffer(s)).At(ViewportPos{X: 1}).
				LineStyle(func(int) lipgloss.Style { return lipgloss.NewStyle().Underline(true) }).
				Marks(MarkMatches(regexp.MustCompile("[a-z]"), lipgloss.NewStyle().Reverse(true))),
		} {
			if out := Render(7, 3, c); leaks(out) {
				t.Fatalf("%s: %q leaks: %q", node, s, out)
			}
		}
		Render(7, 3, rawView(s))
		Render(7, 3, Viewport(NewBuffer(s)).Trusted())
	})
}
