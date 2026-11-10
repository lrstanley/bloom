// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lrstanley/bloom/_examples/internal/demo"
	"github.com/lrstanley/steep"
	"github.com/lrstanley/steep/reel"
	"github.com/lrstanley/steep/reel/effects"
)

// at returns the cell where text first appears on screen, or -1, -1.
func at(view, text string) (x, y int) {
	for y, line := range strings.Split(view, "\n") {
		if before, _, ok := strings.Cut(line, text); ok {
			return len([]rune(before)), y
		}
	}
	return -1, -1
}

func TestAnalyze(t *testing.T) {
	t.Parallel()

	prog, err := analyze("testdata/notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.roots) != 1 || prog.roots[0].qualified() != "main.main" {
		t.Fatalf("roots = %v, want only main.main", prog.roots)
	}
	find := func(name string) *fn {
		for _, f := range prog.all {
			if f.qualified() == name {
				return f
			}
		}
		t.Fatalf("no function %s", name)
		return nil
	}

	// An interface method calls its implementations.
	iface := find("render.Renderer.Render")
	if got := names(iface.calls); got != "Text.Render JSON.Render" && got != "JSON.Render Text.Render" {
		t.Errorf("Renderer.Render calls %q", got)
	}
	// Callees are in the order they first appear, with their call sites.
	add := find("main.add")
	if got, want := names(add.calls), "Join New Store.Add tagsIn Println Store.Save"; got != want {
		t.Errorf("add calls %q, want %q", got, want)
	}
	tags := find("main.tagsIn")
	s := add.sites[tags][0]
	if line := add.src[s.line]; line[s.col:s.col+s.n] != "tagsIn" {
		t.Errorf("site of tagsIn is %q", line[s.col:s.col+s.n])
	}
	if path := prog.pathTo(find("render.wrap")); len(path) != 5 {
		t.Errorf("path to wrap = %v, want 5 steps", path)
	}
}

// TestAnalyzeUntrusted loads source with a line directive, which moves
// reported lines away from the file's own, and control characters, which
// must not reach the terminal.
func TestAnalyzeUntrusted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := "package main\n\n//line generated.go:100\n\n// main says \x1b]0;hi\x07 hello.\r\nfunc main() { helper() }\n\nfunc helper() {}\n"
	for name, s := range map[string]string{"go.mod": "module example.com/untrusted\n\ngo 1.25\n", "main.go": src} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prog, err := analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	main := prog.roots[0]
	if main.line != 5 || len(main.calls) != 1 || main.sites[main.calls[0]][0].line != 1 {
		t.Errorf("main at line %d, calls %v at %v", main.line, main.calls, main.sites)
	}
	for _, s := range append(main.src, main.doc) {
		if strings.ContainsAny(s, "\x1b\x07\r") {
			t.Errorf("control characters in %q", s)
		}
	}
}

func names(fs []*fn) string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.name
	}
	return strings.Join(out, " ")
}

func TestGoLens(t *testing.T) {
	t.Parallel()

	h := steep.NewHarness(t, newModel("testdata/notes"), demo.Options("golens", 110, 30, effects.Mouse())...)
	h.Wait("example.com/notes").Wait("run").Sleep(900 * time.Millisecond)
	step := func(key string) { h.Key(key).Sleep(280 * time.Millisecond) }

	// Into run, whose callees open beside it, and down to add.
	step("right")
	step("down")
	step("right")
	h.Wait("parseArgs splits")
	for range 5 {
		step("down")
	}
	h.Wait("add saves a note")
	step("right")
	h.Wait("tagsIn")
	step("down")
	step("down")
	step("right")
	h.Wait("normalize").Sleep(400 * time.Millisecond)

	// A click selects a function anywhere in the tree.
	x, y := at(h.View(), "normalize")
	h.LeftClick(x+2, y).Wait("normalize sorts tags").Sleep(600 * time.Millisecond)
	step("left")
	step("left")
	h.Wait("add saves a note")

	// The code view, with each call of the selected callee marked, and a
	// walk into one of them and back out.
	h.Key("enter").Wait("Called by").Sleep(700 * time.Millisecond)
	for range 3 {
		step("down")
	}
	h.Key("enter").Wait("CutPrefix").Sleep(800 * time.Millisecond)
	h.Key("left").Wait("strings.Join(args").Sleep(700 * time.Millisecond)
	step("tab")
	h.Key("esc").WaitNot("Called by").Sleep(600 * time.Millisecond)

	// Search jumps anywhere, by the shortest path from main.
	h.Key("/").Sleep(300 * time.Millisecond)
	h.Type("wrap", reel.Delay(120*time.Millisecond)).Wait("example.com/notes/render").Sleep(500 * time.Millisecond)
	h.Key("enter").Wait("wrap breaks s").Sleep(1200 * time.Millisecond)
	h.AssertSnapshot()

	h.Key("q")
	h.WaitFinished()
}
