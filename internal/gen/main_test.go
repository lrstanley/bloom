// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerated fails when nodes.gen.go is not what the generator writes,
// such as after a change to builders.go without running go generate.
func TestGenerated(t *testing.T) {
	dir := filepath.Join("..", "..")
	want, err := generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, output))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of date with internal/gen: run go generate in the repository root", output)
	}
}

// TestComment checks that comment wraps paragraphs at width and keeps the
// blank line between them.
func TestComment(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"short", "One line.", "// One line.\n"},
		{"paragraphs", "One.\n\nTwo.", "// One.\n//\n// Two.\n"},
		{
			"wrapped",
			"ID sets the ID that focus, layout animation, anchoring, and [Frame.Rect] refer to the layer by.",
			"// ID sets the ID that focus, layout animation, anchoring, and [Frame.Rect]\n// refer to the layer by.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := comment(tt.text); got != tt.want {
				t.Errorf("comment(%q) =\n%s\nwant\n%s", tt.text, got, tt.want)
			}
		})
	}
}
