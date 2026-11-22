// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"slices"
	"strings"

	"bloomfx.dev/bloom"
)

// deployLog is the deploys, the latest first, each marked by how it went.
type deployLog struct {
	st      *styles
	deploys []string
}

func (l deployLog) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(l.deploys)+1)
	for i, d := range slices.Backward(l.deploys) {
		mark, st := "✓", l.st.good
		if strings.HasSuffix(d, "back") {
			mark, st = "↺", l.st.bad
		}
		at := fmt.Sprintf("%02d:%02d ", 9+i/2, (i*17)%60)
		rows = append(rows, bloom.Row(
			bloom.Text(at).Style(l.st.dim),
			bloom.Text(mark+" ").Style(st),
			bloom.Fill(bloom.Text(d).Style(l.st.text).Truncate()),
		))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}
