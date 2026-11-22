// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
)

// statusbar is a plain Bubble Tea component and a [bloom.Measurer]: one
// row, of any width, as the column it is in stretches it across.
type statusbar struct {
	st    *styles
	w, h  int          // the size of the window
	frame *bloom.Frame // the frame about to be drawn, or nil while it is solved
}

func (s statusbar) View() string { return s.st.bar.Render(" " + s.text()) }

func (statusbar) Measure(bloom.Size) bloom.Size { return bloom.Size{H: 1} }

// text says which panes fit and where the reader is, in the frame about to
// be drawn, and nothing while there is no frame yet.
func (s statusbar) text() string {
	if s.frame == nil {
		return ""
	}
	var panes []string
	for _, p := range []paneID{paneNav, paneReader, paneDetails} {
		if _, ok := s.frame.Rect(p); ok {
			panes = append(panes, string(p))
		}
	}
	v, _ := s.frame.Viewport(paneText)
	return fmt.Sprintf(
		"%d×%d · showing %s · row %d of %d · page %d",
		s.w, s.h, strings.Join(panes, ", "), v.Row+1, max(v.TotalRows-v.Rows, 0)+1, v.Rows,
	)
}
