// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strconv"

	"bloomfx.dev/bloom"
)

// sidebar shows the selected function's signature and doc comment.
type sidebar struct {
	st *styles
	f  *fn // the selected function
}

// Layout lays the sidebar out as a column, its doc comment taking the
// space left.
func (s sidebar) Layout(bloom.Context) bloom.Child {
	f := s.f
	where := f.pkgPath
	if f.local() {
		where = fmt.Sprintf("%s:%d", f.file, f.line)
	}
	doc := f.doc
	switch {
	case f.kind == kindExternal:
		doc = "Declared outside the project, in " + f.pkgPath + "."
	case f.kind == kindInterface && doc == "":
		doc = "An interface method. What it calls are its implementations in the project."
	case doc == "":
		doc = "No doc comment."
	}
	counts := s.st.muted.Render("called by ") + s.st.text.Render(strconv.Itoa(len(f.callers))) +
		s.st.muted.Render("  calls ") + s.st.text.Render(strconv.Itoa(len(f.calls)))

	return bloom.Box(s.st.panel, bloom.Column(
		bloom.Fixed(1, nil),
		bloom.Text(f.pkg).Style(s.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Text(s.st.signature(f)),
		bloom.Fixed(1, nil),
		bloom.Fill(bloom.Text(doc).Style(s.st.muted)),
		bloom.Fixed(1, nil),
		bloom.Text(counts).Truncate(),
		bloom.Text(where).Style(s.st.faint).Truncate(),
		bloom.Fixed(1, nil),
	))
}
