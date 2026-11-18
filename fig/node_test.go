// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig_test

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/canvas"
	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// plain renders c at w×h without styles, each row's trailing spaces
// removed.
func plain(c bloom.Child, w, h int) string {
	return trimRows(ansi.Strip(bloom.Render(w, h, c)))
}

// trimRows removes each row's trailing spaces and the blank rows at the end.
func trimRows(s string) string {
	rows := strings.Split(s, "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

// hideUntilDone hides every cell until t reaches 1.
var hideUntilDone = fx.Func(func(s *canvas.Surface, t float64) {
	if t >= 1 {
		return
	}
	for y := range s.Height() {
		for x := range s.Width() {
			s.Hide(x, y)
		}
	}
})

func TestTextNode(t *testing.T) {
	t.Parallel()

	small := fonts.Small()
	want := trimRows(small.Figure("Bloom", fig.Options{}).String())
	paint := fig.Gradient(lipgloss.Color("#ff6f91"), lipgloss.Color("#ffc29b"))
	tests := []struct {
		name  string
		node  fig.TextNode
		paint fig.Paint // that the strokes are drawn in, or nil
	}{
		{"static", fig.Text("Bloom").Font(small), nil},
		{"painted", fig.Text("Bloom").Font(small).Paint(paint), paint},
		{"each at rest", fig.Text("Bloom").Font(small).Each(1, hideUntilDone), nil},
		{"layers at rest", fig.Text("Bloom").Font(small).ID("t").Animate(), nil},
		{"layers painted", fig.Text("Bloom").Font(small).ID("t").Animate().Paint(paint.Blend()), paint.Blend()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(tt.node, 60, 10); got != want {
				t.Fatalf("got\n%s\nwant\n%s", got, want)
			}
			if tt.paint == nil {
				return
			}
			// Each stroke is in the color the figure painted alone has there.
			f := small.Figure("Bloom", fig.Options{})
			ref := canvas.Parse(f.Render(tt.paint), ansi.GraphemeWidth)
			got := canvas.Parse(bloom.Render(60, 10, tt.node), ansi.GraphemeWidth)
			w, h := f.Size()
			strokes := 0
			for y := range h {
				for x := range w {
					r, g := ref.CellAt(x, y), got.CellAt(x, y)
					if r.Content == " " || r.Style.Fg == nil {
						continue
					}
					strokes++
					if g.Style.Fg == nil || !sameColor(g.Style.Fg, r.Style.Fg) {
						t.Fatalf("stroke at %d,%d drawn in %v, want %v", x, y, g.Style.Fg, r.Style.Fg)
					}
				}
			}
			if strokes == 0 {
				t.Fatal("the figure painted alone has no colored strokes")
			}
		})
	}
}

// sameColor reports whether a and b are the same color.
func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestTextNodeSize(t *testing.T) {
	t.Parallel()

	n := fig.Text("Bloom").Font(fonts.Small())
	w, h := fonts.Small().Figure("Bloom", fig.Options{}).Size()
	if got := n.Measure(bloom.Size{W: 100, H: 100}); got != (bloom.Size{W: w, H: h}) {
		t.Fatalf("Measure = %v, want %dx%d", got, w, h)
	}
}

func TestFontChain(t *testing.T) {
	t.Parallel()

	n := fig.Text("Dashboard").Font(fonts.Big(), fonts.Standard(), fonts.Small(), fonts.Mini())
	tests := []struct {
		width int
		want  string
	}{
		{60, trimRows(fonts.Big().Figure("Dashboard", fig.Options{}).String())},
		{48, trimRows(fonts.Small().Figure("Dashboard", fig.Options{}).String())},
		{30, trimRows(fonts.Mini().Figure("Dashboard", fig.Options{}).String())},
		{6, "Dashb…"},
	}
	for _, tt := range tests {
		if got := plain(n, tt.width, 8); got != tt.want {
			t.Errorf("width %d: got\n%s\nwant\n%s", tt.width, got, tt.want)
		}
	}
}

func TestEachStaggers(t *testing.T) {
	t.Parallel()

	n := fig.Text("ab").Font(fonts.Mini())
	// Halfway, with the first glyph starting at 0 and the second at 0.5,
	// each taking half of t, the first is done and the second is not.
	got := plain(n.Each(0.5, hideUntilDone), 20, 4)
	f := fonts.Mini().Figure("ab", fig.Options{})
	w, h := f.Size()
	rows := make([]string, h)
	for y := range h {
		var row strings.Builder
		for x := range w {
			if c := f.At(x, y); c.Glyph == 0 {
				row.WriteString(c.Content)
			} else {
				row.WriteByte(' ')
			}
		}
		rows[y] = row.String()
	}
	if first := trimRows(strings.Join(rows, "\n")); got != first || first == "" {
		t.Fatalf("halfway shows\n%s\nwant only the first glyph of\n%s", got, f.String())
	}
	if start := plain(n.Each(0, hideUntilDone), 20, 4); start != "" {
		t.Fatalf("at 0 shows\n%s", start)
	}
	// With no spread, the glyphs start together, so halfway neither is done.
	if together := plain(n.Spread(0, fig.Forward).Each(0.5, hideUntilDone), 20, 4); together != "" {
		t.Fatalf("halfway with no spread shows\n%s", together)
	}
}

func TestMorph(t *testing.T) {
	t.Parallel()

	m := fig.NewMorph("0:0 1")
	_ = m.Set("0:0 2")
	node := m.Text().Font(fonts.Big()).Key(fig.ByPosition).Enter(hideUntilDone)
	// At the start, the leaving "1" is still whole, and the entering "2"
	// hidden: the old text. Where the colon meets the zeros, which stay,
	// the merged characters show its "(" and ")", not the zeros' "|".
	if got, want := plain(node, 60, 8), trimRows(fonts.Big().Figure("0:0 1", fig.Options{}).String()); got != want {
		t.Fatalf("at the start got\n%s\nwant\n%s", got, want)
	}
	if m.Current() != "0:0 2" {
		t.Fatalf("Current = %q", m.Current())
	}
}

func TestMorphJustified(t *testing.T) {
	t.Parallel()

	// Right justified, "B" and "C" stay where they were when "A" leaves,
	// though they are laid out from a different place: their merged "|"
	// stays shown.
	var b strings.Builder
	b.WriteString("flf2a$ 1 1 3 0 0 0 136\n")
	for r := ' '; r <= '~'; r++ {
		switch r {
		case 'A':
			b.WriteString("AA@@\n")
		case 'B':
			b.WriteString("[@@\n")
		case 'C':
			b.WriteString("]@@\n")
		default:
			b.WriteString("@@\n")
		}
	}
	font, err := fig.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	opts := fig.Options{Width: 10, Justify: fig.Right, Strict: true}
	m := fig.NewMorph("ABC")
	_ = m.Set("BC")
	node := m.Text().Font(font).Options(opts)
	// At the start, the leaving "A" is still whole: the old text.
	if got, want := plain(node, 10, 1), trimRows(font.Figure("ABC", opts).String()); got != want {
		t.Fatalf("at the start got %q, want %q", got, want)
	}
}

func TestTransparentBanner(t *testing.T) {
	t.Parallel()

	base := bloom.Text(strings.Repeat(strings.Repeat("x", 30)+"\n", 6))
	banner := fig.Text("o").Font(fonts.Small()).ID("b").Animate()
	got := plain(bloom.Overlay(base, bloom.Layer(banner).Anchor(bloom.Start, bloom.Start).Transparent()), 30, 6)
	// The banner's strokes are drawn over the x's, and its blank cells,
	// inside its bounds too, show them.
	f := fonts.Small().Figure("o", fig.Options{})
	fw, fh := f.Size()
	rows := make([]string, 6)
	for y := range rows {
		var row strings.Builder
		for x := range 30 {
			if c := f.At(x, y); x < fw && y < fh && c.Content != " " {
				row.WriteString(c.Content)
			} else {
				row.WriteByte('x')
			}
		}
		rows[y] = row.String()
	}
	if want := strings.Join(rows, "\n"); got != want || !strings.Contains(f.String(), " ") {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestMorphStaggersInTextOrder(t *testing.T) {
	t.Parallel()

	// Entering glyphs start in the order of the text, whichever is drawn on
	// top, and a space, which shows nothing, takes no turn.
	var b strings.Builder
	b.WriteString("flf2a$ 1 1 3 0 0 0 136\n")
	for r := ' '; r <= '~'; r++ {
		switch r {
		case 'A', 'B':
			b.WriteString(string(r) + "@@\n")
		default:
			b.WriteString("@@\n")
		}
	}
	font, err := fig.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(0, 0)
	m := fig.NewMorph("", anim.Clock(func() time.Time { return now }), anim.Duration(time.Second), anim.Ease(anim.Linear))
	cmd := m.Set("A B")
	msg := cmd()
	now = now.Add(time.Second / 2)
	m.Update(msg)
	// Halfway, the first of the two glyphs is done and the second has not
	// started.
	node := m.Text().Font(font).Options(fig.Options{Stacking: fig.EarlierOnTop}).Enter(hideUntilDone)
	if got := plain(node, 10, 1); got != "A" {
		t.Fatalf("halfway got %q, want %q", got, "A")
	}
}
