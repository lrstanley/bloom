// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
)

// details is the panel beside the reader, about the article open: its
// title, its tags, and how long it is.
type details struct {
	st  *styles
	art article
}

func (d details) Layout(bloom.Context) bloom.Child {
	paragraphs := fmt.Sprintf("%d paragraphs", strings.Count(d.art.body, "§"))
	info := bloom.Column(
		bloom.Text(d.art.title).Style(d.st.bold),
		bloom.Fixed(1, nil),
		d.st.chips(d.art.tags),
		bloom.Space(),
		bloom.Text(paragraphs).Style(d.st.note),
	)
	return bloom.Box(d.st.pane, info)
}

// chips returns tags as chips that wrap onto lines to fit.
func (st *styles) chips(tags []string) bloom.Child {
	cs := make([]bloom.Child, len(tags))
	for i, tag := range tags {
		cs[i] = bloom.Text("#" + tag).Style(st.chip)
	}
	return bloom.Wrap(cs...).Gap(1, 1)
}
