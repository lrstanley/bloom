// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"bloomfx.dev/bloom"
	"charm.land/lipgloss/v2"
)

// secret is a token shown masked, but for its first and last few
// characters. It implements bloom.TextSource, so that selecting it copies
// the token itself rather than what is shown.
type secret struct {
	value string
	head  int
	tail  int
	style lipgloss.Style
}

func (s secret) Layout(bloom.Context) bloom.Child {
	masked := strings.Repeat("•", len(s.value)-s.head-s.tail)
	return bloom.Text(s.style.Render(s.value[:s.head] + masked + s.value[len(s.value)-s.tail:]))
}

// SelectedText implements bloom.TextSource: the token is one row, a
// character to a cell.
func (s secret) SelectedText(span bloom.Span) string {
	from, to := span.Start.X, span.End.X+1
	if span.Start.Y > 0 {
		from = len(s.value)
	}
	if span.End.Y > 0 {
		to = len(s.value)
	}
	from, to = min(from, len(s.value)), min(to, len(s.value))
	return s.value[from:max(to, from)]
}
