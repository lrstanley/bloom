// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"math"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	uv "github.com/charmbracelet/ultraviolet"
)

const heading = "Mission Control"

// headerFonts are the fonts the header steps down through.
var headerFonts = []*fig.Font{fonts.ANSICompact(), fonts.Doom(), fonts.Small(), fonts.Mini()}

// header is the heading in the largest of headerFonts that fits, or in
// plain text when none does, with a shine sweeping across its strokes now
// and then.
type header struct {
	st      styles
	shimmer anim.Sequence // 0 to 1 across the header, then a pause, over and over
}

// newHeader makes the header, its shine not yet started.
func newHeader() header {
	return header{
		shimmer: anim.NewSequence(0,
			anim.Jump(0),
			anim.To(1, anim.Duration(1400*time.Millisecond)),
			anim.Hold(2*time.Second),
		).With(anim.Ease(anim.Linear), anim.FPS(60)),
	}
}

// setStyles sets the styles the header is drawn with, of which it keeps a copy.
func (hd *header) setStyles(st styles) *header {
	hd.st = st
	return hd
}

func (hd *header) Layout(bloom.Context) bloom.Child {
	return fig.Text(heading).
		Font(headerFonts...).
		Paint(hd.shine(hd.shimmer.Float())).
		Fallback(bloom.Text(strings.ToUpper(heading)).Style(hd.st.plain).Truncate())
}

// shine returns the header's paint with a band of light t of the way across
// it, diagonally, brightening the strokes it passes over.
func (hd *header) shine(t float64) fig.Paint {
	if t <= 0 || t >= 1 {
		return hd.st.headerPaint
	}
	return fig.PaintFunc(func(f *fig.Figure, c fig.Cell) uv.Style {
		st := hd.st.headerPaint.Shade(f, c)
		w, h := f.Size()
		const band = 6.0
		at := -band + t*(float64(w+2*h)+2*band)
		if d := math.Abs(float64(c.X+2*(h-1-c.Y)) - at); d < band && st.Fg != nil {
			st.Fg = blend(st.Fg, hd.st.theme.BrightWhite, 0.85*(1-d/band))
		}
		return st
	})
}

// headerFont returns the name of the font the header is shown in at the
// window's width, w.
func headerFont(w int) string {
	for _, f := range headerFonts {
		if fw, _ := f.Figure(heading, fig.Options{}).Size(); fw <= w {
			return f.Name()
		}
	}
	return "plain text"
}
