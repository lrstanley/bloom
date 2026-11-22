// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"
)

type article struct {
	title string
	tags  []string
	body  string
}

var articles = []article{
	{
		title: "Constraints go down",
		tags:  []string{"layout", "design", "flutter", "gio"},
		body:  essay("constraints", 18),
	},
	{
		title: "Sizes come up",
		tags:  []string{"sizing", "measure"},
		body:  essay("measurement", 26),
	},
	{
		title: "Parents place",
		tags:  []string{"arrange", "flex", "slots", "collapse", "ownership"},
		body:  essay("placement", 22),
	},
	{
		title: "Cells, not pixels",
		tags:  []string{"terminal", "unicode", "compositor"},
		body:  essay("cells", 30),
	},
}

// essay returns numbered paragraphs about topic, long enough to wrap at most
// widths, so that the reader reflows them as the terminal narrows and widens.
func essay(topic string, paragraphs int) string {
	const text = "§%d. A paragraph about %s, long enough to wrap at most widths so that " +
		"the reader has to reflow it as the terminal narrows and widens.\n\n"

	var b strings.Builder
	for i := range paragraphs {
		fmt.Fprintf(&b, text, i+1, topic)
	}
	return strings.TrimSpace(b.String())
}
