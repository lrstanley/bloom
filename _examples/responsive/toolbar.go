// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
)

// toolbar is a component: it shows its actions with labels when there is
// room, and only their icons when there is not.
type toolbar struct {
	st    *styles
	title string
}

func (t toolbar) Layout(ctx bloom.Context) bloom.Child {
	actions := []string{"⟳ refresh", "✎ edit", "⇪ share", "⚙ settings"}
	children := []bloom.Child{
		bloom.Text("◆ " + t.title).Style(t.st.title),
		bloom.Fill(bloom.Text("").Style(t.st.bar)),
	}
	for _, a := range actions {
		if ctx.Width() < 70 {
			a, _, _ = strings.Cut(a, " ")
		}
		children = append(children, bloom.Text(a).Style(t.st.action))
	}
	return bloom.Row(children...)
}
