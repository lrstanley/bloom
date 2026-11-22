// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
)

// palette shows the tint's sixteen colors, as swatches that share the width.
type palette struct{ st *styles }

// Layout is the tint's name and tone over its two rows of swatches.
func (p palette) Layout(bloom.Context) bloom.Child {
	t := p.st.theme
	swatches := func(sts []lipgloss.Style) bloom.Child {
		row := make([]bloom.Child, len(sts))
		for i := range sts {
			row[i] = bloom.Fill(bloom.Box(sts[i], bloom.Text("")))
		}
		return bloom.Row(row...).Gap(1)
	}
	about := []string{string(t.Hue)}
	if t.Tone != "" {
		about = append(about, string(t.Tone))
	}
	return bloom.Column(
		bloom.Text(t.DisplayName).Style(p.st.highlight).Truncate(),
		bloom.Text(strings.Join(about, " · ")).Style(p.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Fixed(3, swatches(p.st.swatches[0])),
		bloom.Fixed(1, nil),
		bloom.Fixed(3, swatches(p.st.swatches[1])),
	)
}
