// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fx"
)

// stage is a line of text in the font shown, drawn with the paint in use,
// centered in a frame that says what the font is and who made it.
type stage struct {
	st     *styles
	font   int     // index into gallery, the font shown
	paint  paint   // the paint in use
	reveal float64 // 0 to 1, the glyphs of a new font appearing
}

func (s stage) Layout(bloom.Context) bloom.Child {
	font, p := gallery[s.font], s.paint
	info := font.Info()

	sample := fig.Text("Bloom!").
		Font(font).
		Paint(p.paint).
		Each(s.reveal, fx.Decode{Seed: uint32(s.font)}, fx.Fade{}). //nolint:gosec // a font's index.
		Spread(0.5, fig.Forward).
		Fallback(bloom.Text("too large for the window: make it bigger").Style(s.st.dim))

	return bloom.Box(s.st.stage, bloom.Centered(sample)).
		Edge(bloom.Top, bloom.Start, bloom.Text(" "+font.Name()+" ").Style(s.st.title)).
		Edge(bloom.Top, bloom.End, bloom.Text(fmt.Sprintf(" %d rows · %s · %s paint ", info.Height, layoutName(info.Horizontal), p.name)).Style(s.st.dim)).
		Edge(bloom.Bottom, bloom.Center, bloom.Text(" "+credit(info.Comments)+" ").Style(s.st.dim).Truncate())
}

// layoutName describes how a font's glyphs are put side by side.
func layoutName(m fig.Mode) string {
	switch m { //nolint:exhaustive // A font's own layout is never Default.
	case fig.Smushed, fig.Universal:
		return "smushed"
	case fig.Fitted:
		return "fitted"
	}
	return "full width"
}

// credit returns the first line of a font's comments that says who made it.
func credit(comments []string) string {
	for _, c := range comments {
		c = strings.TrimSpace(c)
		if strings.ContainsFunc(c, func(r rune) bool { return r >= 'A' && r <= 'z' }) && !strings.HasPrefix(c, "SPDX") {
			return c
		}
	}
	return "no credit given"
}
