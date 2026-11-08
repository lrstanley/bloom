// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lrstanley/bloom/fig"
	"github.com/lrstanley/bloom/fig/fonts"
)

// parityCase is a layout compared against C figlet: the figlet arguments,
// and the Options that should give the same output.
type parityCase struct {
	name string
	args []string
	opts fig.Options
	text string
	utf8 bool // the text needs figlet's utf8.flc control file
}

const (
	textHello   = "Hello, World!"
	textPangram = "The quick brown fox jumps over the lazy dog 0123456789 #$%&*+-/<=>?@[]{}|~"
	textWrap    = "The quick brown fox jumps over the lazy dog, and then naps in a sunny spot."
	textLines   = "First line\nsecond line\n\nafter a blank line"
)

var parityCases = []parityCase{
	{name: "default", text: textHello},
	{name: "pangram", text: textPangram},
	{name: "full", args: []string{"-W"}, opts: fig.Options{Horizontal: fig.Full}, text: textHello},
	{name: "fitted", args: []string{"-k"}, opts: fig.Options{Horizontal: fig.Fitted}, text: textPangram},
	{name: "smushed", args: []string{"-S"}, opts: fig.Options{Horizontal: fig.Smushed}, text: textPangram},
	{name: "universal", args: []string{"-o"}, opts: fig.Options{Horizontal: fig.Universal}, text: textPangram},
	{name: "rules", args: []string{"-m", "5"}, opts: fig.Options{Rules: fig.RuleEqual | fig.RuleHierarchy}, text: textPangram},
	{name: "rtl", args: []string{"-R", "-l"}, opts: fig.Options{Direction: fig.RightToLeft, Justify: fig.Left}, text: textHello},
	{name: "ltr", args: []string{"-L"}, opts: fig.Options{Direction: fig.LeftToRight}, text: textHello},
	{name: "wrap", args: []string{"-w", "41"}, opts: fig.Options{Width: 40}, text: textWrap},
	{name: "wrap-center", args: []string{"-w", "61", "-c"}, opts: fig.Options{Width: 60, Justify: fig.Center}, text: textWrap},
	{name: "wrap-right", args: []string{"-w", "51", "-r"}, opts: fig.Options{Width: 50, Justify: fig.Right}, text: textWrap},
	{name: "wrap-rtl", args: []string{"-w", "51", "-R"}, opts: fig.Options{Width: 50, Direction: fig.RightToLeft}, text: textWrap},
	{name: "lines", text: textLines},
	{name: "paragraph", args: []string{"-p", "-w", "71"}, opts: fig.Options{Width: 70, Paragraph: true}, text: textLines},
	{name: "narrow", args: []string{"-w", "9"}, opts: fig.Options{Width: 8}, text: "Wide MW"},
	{name: "latin", args: []string{"-C", "utf8"}, text: "Ünïcödé ßtrasse ÄÖÜ äöü", utf8: true},
}

// update regenerates testdata/parity.golden from C figlet, whose path is in the
// FIGLET environment variable: figlet 2.2.5, built with TLF_FONTS, as its
// Makefile builds it.
var update = flag.Bool("update", false, "regenerate testdata/parity.golden from the figlet in $FIGLET")

// parityFile holds a hash of figlet's output for each font and case, rather
// than the output itself, which would be far larger.
const parityFile = "testdata/parity.golden"

// skipParity are the fonts and cases where fig differs from figlet on
// purpose.
func skipParity(font string, c parityCase) bool {
	switch {
	case font == "wideterm":
		// figlet counts a double-width character as one column; fig, as
		// TOIlet, counts the columns it takes.
		return true
	case font == "font-font":
		// Its UTF-8 byte order mark makes figlet refuse it.
		return true
	case c.utf8 && (font == "term" || font == "digital" || font == "bubble"):
		// Their glyphs above 127 are Latin-1, which figlet's UTF-8 decoding
		// drops; fig reads them as Latin-1.
		return true
	}
	return false
}

func TestParity(t *testing.T) {
	t.Parallel()

	if *update {
		writeParity(t)
	}
	want := readParity(t)

	for _, name := range fonts.Names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			font, _ := fonts.ByName(name)
			for _, c := range parityCases {
				if skipParity(name, c) {
					continue
				}
				got := font.Figure(c.text, c.opts).String()
				if parityHash(got) == want[name+" "+c.name] {
					continue
				}
				msg := "set FIGLET to the path of figlet 2.2.5 to see what it prints"
				if os.Getenv("FIGLET") != "" {
					msg = "--- want (figlet)\n" + figletRows(runFiglet(t, name, c))
				}
				t.Errorf("%s:\n--- got\n%s\n%s", c.name, got, msg)
			}
		})
	}
}

// parityHash returns a short hash of a figure's text.
func parityHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// writeParity runs figlet for each font and case, and writes the hash of
// what it prints to parityFile.
func writeParity(t *testing.T) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# The hash of C figlet's output for each font and case, from TestParity -update.\n")
	for _, name := range fonts.Names {
		for _, c := range parityCases {
			fmt.Fprintf(&b, "%s %s %s\n", name, c.name, parityHash(figletRows(runFiglet(t, name, c))))
		}
	}
	if err := os.WriteFile(parityFile, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runFiglet returns what figlet prints for case c in font name.
func runFiglet(t *testing.T, name string, c parityCase) string {
	t.Helper()
	bin := os.Getenv("FIGLET")
	if bin == "" {
		t.Fatal("-update needs FIGLET, the path of a figlet binary")
	}
	dir := t.TempDir()
	font, _ := fonts.ByName(name)
	src := fontSource(t, font)
	ext := ".flf"
	if strings.HasPrefix(src, "tlf2") {
		ext = ".tlf"
	}
	if err := os.WriteFile(filepath.Join(dir, name+ext), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utf8.flc"), []byte("u\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-d", dir, "-f", name}, c.args...)
	if !slices.Contains(c.args, "-w") {
		args = append(args, "-w", "100000", "-l")
	}
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec // the test's own figlet.
	cmd.Stdin = strings.NewReader(c.text)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("(figlet failed: %v)", err)
	}
	return string(out)
}

// fontSource returns the text of a bundled font, as figlet reads it.
func fontSource(t *testing.T, f *fig.Font) string {
	t.Helper()
	name := f.Name()
	for _, ext := range []string{".flf.gz", ".tlf.gz"} {
		data, err := os.ReadFile(filepath.Join("fonts", "data", name+ext))
		if err != nil {
			continue
		}
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatalf("no data for font %s", name)
	return ""
}

// figletRows removes each row's trailing spaces and the line break at the
// end, as [fig.Figure.String] leaves rows.
func figletRows(s string) string {
	rows := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.Join(rows, "\n")
}

// readParity reads parityFile: the hash for each font and case, by "font
// case".
func readParity(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(parityFile)
	if err != nil {
		t.Fatalf("%v (regenerate with -update and FIGLET set)", err)
	}
	out := map[string]string{}
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		font, rest, _ := strings.Cut(line, " ")
		name, hash, _ := strings.Cut(rest, " ")
		out[font+" "+name] = hash
	}
	return out
}
