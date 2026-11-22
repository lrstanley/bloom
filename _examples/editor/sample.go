// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import "fmt"

// sample is the text shown when no file is named.
const sample = `A small editor, built from bloom.Viewport and bloom.Buffer
===========================================================

The caret is a range kept in the model: bloom draws it, puts the terminal cursor at it, and sets it from the mouse. Everything else here is a keymap of a few dozen lines, and an undo history of snapshots.

Try it:
	- type anywhere, and undo it with ctrl+z, as one step;
	- select with shift and the arrows, or by dragging, or double clicking a word;
	- cut with ctrl+x, and paste it somewhere else with ctrl+v;
	- turn wrapping off with alt+z, and scroll sideways past the end of a long line.

Wide characters and tabs take the cells they are drawn in, and the caret never lands inside one: 日本語のテキスト, café, naïve, and	a	few	tabs.

Lines wrap at words, and the caret moves down the rows a line wraps to, keeping its column, as it does down the lines: this paragraph is one line, long enough to wrap two or three times at the width of most terminals, so that moving up and down through it shows the rows.
`

// The words generated lines are made of.
var (
	subjects = []string{"The editor", "A buffer", "Each viewport", "The caret", "This line", "A snapshot", "The terminal"}
	verbs    = []string{"draws", "keeps", "wraps", "measures", "restores", "follows", "shows"}
	objects  = []string{
		"only the rows in sight", "every line it is given", "the text under the cursor",
		"a million lines at once", "what changed since the last frame", "the column it was at",
	}
)

// generatedLine is line i of the generated text.
func generatedLine(i int) string {
	s := fmt.Sprintf("%s %s %s, on line %d.", subjects[i%len(subjects)], verbs[i/3%len(verbs)], objects[i/7%len(objects)], i+1)
	if i%5 == 0 {
		s += " It goes on a while longer than the lines around it, so that some of them wrap."
	}
	return s
}
