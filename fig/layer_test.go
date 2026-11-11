// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig_test

import (
	"strings"
	"testing"

	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"github.com/charmbracelet/x/ansi"
)

// layerOptions are layouts whose glyph layers are checked against the
// figure: smushed, overlapped either way, stacked vertically, and backwards.
var layerOptions = []fig.Options{
	{},
	{Horizontal: fig.Universal},
	{Tracking: -2},
	{Tracking: -2, Stacking: fig.EarlierOnTop},
	{Tracking: 2},
	{Vertical: fig.Smushed, Width: 30},
	{Vertical: fig.Universal, Width: 30, Stacking: fig.EarlierOnTop},
	{Direction: fig.RightToLeft},
	{Width: 25, Justify: fig.Center},
}

// TestLayersMakeTheFigure checks that drawing each glyph's layer over the
// ones before it, at rest, gives exactly the figure: what lets a TextNode
// switch between drawing a figure whole and drawing it glyph by glyph
// without anything changing on screen.
func TestLayersMakeTheFigure(t *testing.T) {
	t.Parallel()

	for _, name := range fonts.Names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			font, _ := fonts.ByName(name)
			for _, o := range layerOptions {
				f := font.Figure("Bloom, fig! 0123 /\\|[]", o)
				if got, want := composeLayers(f), f.String(); got != want {
					t.Fatalf("options %+v: layers make\n%s\nfigure is\n%s", o, got, want)
				}
			}
		})
	}
}

// TestLayersRowOffsets checks layers where a glyph's rows move by different
// amounts: justified, as figlet pads each row on its own; cut to fit a line
// right to left, as each row keeps its own end; and pushed along right to
// left by a glyph whose rows are of different lengths.
func TestLayersRowOffsets(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString("flf2a$ 2 2 4 0 0\n")
	for r := ' '; r <= '~'; r++ {
		switch r {
		case 'A':
			b.WriteString("ab@\nc@@\n")
			continue
		case 'B':
			b.WriteString("abcd@\nefg@@\n")
			continue
		}
		b.WriteString("@\n@@\n")
	}
	font, err := fig.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		text string
		opts fig.Options
	}{
		{"A", fig.Options{Width: 6, Justify: fig.Center}},
		{"A", fig.Options{Width: 6, Justify: fig.Right}},
		{"B", fig.Options{Width: 2, Direction: fig.RightToLeft}},
		{"AA", fig.Options{Direction: fig.RightToLeft, Horizontal: fig.Full}},
		{"AA", fig.Options{Direction: fig.RightToLeft, Tracking: 1}},
	} {
		f := font.Figure(tt.text, tt.opts)
		if got, want := composeLayers(f), f.String(); got != want {
			t.Errorf("%q with %+v: layers make %q, figure is %q", tt.text, tt.opts, got, want)
		}
	}
}

func composeLayers(f *fig.Figure) string {
	w, h := f.Size()
	c := canvas.New(w, h, ansi.GraphemeWidth)
	for _, i := range f.Order() {
		s, at := f.Layer(i, nil, true)
		if s == "" {
			continue
		}
		l := canvas.Parse(s, ansi.GraphemeWidth)
		for y := range l.Height() {
			for x := range l.Width() {
				if cell := l.CellAt(x, y); cell != nil && cell.Width > 0 && cell.Content != " " && cell.Content != "" {
					c.SetCell(at.X+x, at.Y+y, cell)
				}
			}
		}
	}
	rows := strings.Split(c.String(), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.Join(rows, "\n")
}
