// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/graphemes"
)

// editParts are what the lines and the text put in of the edit model test
// are made of: words, wide glyphs, combining marks, emoji, tabs, and the
// sequences that set and reset a style, and hyperlinks.
var editParts = []string{
	"word", "a", "longer", "日本", "\t", " ", "x-y", "é", "e\u0301", "👍🏽", "🇯🇵", "ok",
	"\x1b[31m", "\x1b[1;4m", "\x1b[m", "\x1b[0m", "\x1b[38;5;196m", "\x1b[48;2;1;2;3m",
	"\x1b[22m", "\x1b[39m", "\x1b]8;;https://x.dev\x1b\\", "\x1b]8;;\x1b\\",
}

// editPlain is how many of editParts hold no escape sequence.
const editPlain = 12

// randomText returns text made of editParts: plain or styled, now and then
// of many lines, an empty one, or one longer than a long line.
func randomText(rng *rand.Rand, lines int) string {
	parts := editParts[:editPlain]
	if rng.IntN(3) == 0 {
		parts = editParts
	}
	var b strings.Builder
	for l := range lines {
		if l > 0 {
			if rng.IntN(6) == 0 {
				b.WriteString("\r\n")
			} else {
				b.WriteByte('\n')
			}
		}
		n := rng.IntN(10)
		if rng.IntN(150) == 0 {
			n = LongLineBytes/3 + rng.IntN(100)
		}
		for range n {
			b.WriteString(parts[rng.IntN(len(parts))])
			if rng.IntN(2) == 0 {
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// editBuf drives a store as a Buffer drives its own, for the tests of
// edits.
type editBuf struct{ Store }

func (b *editBuf) Replace(from, to Pos, text string) Change {
	c, _ := b.Store.Edit(TextEdit{From: from, To: to, Text: text})
	return c
}

func (b *editBuf) Insert(at Pos, text string) Change { return b.Replace(at, at, text) }

func (b *editBuf) Delete(from, to Pos) Change { return b.Replace(from, to, "") }

// Edit makes edits, leaving them as they were given, which Store.Edit
// sorts.
func (b *editBuf) Edit(edits ...TextEdit) Change {
	c, _ := b.Store.Edit(slices.Clone(edits)...)
	return c
}

func (b *editBuf) Append(text string) { b.AppendLines(text) }

func (b *editBuf) SetContent(text string) {
	b.Clear()
	if text != "" {
		b.AppendText(text)
	}
}

func (b *editBuf) Offset(p Pos) int { return b.Store.Offset(b.Clamp(p)) }

// refBuf is the reference the store is checked against: its lines as
// they were given, in a slice, and everything else worked out from them
// from scratch.
type refBuf struct {
	lines []string
}

// refSplit returns the lines of s, split as Append splits them.
func refSplit(s string) []string {
	out := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i := range out {
		out[i] = strings.TrimSuffix(out[i], "\r")
	}
	return out
}

// refTokens returns the text of line s, as the tests make lines, without
// its escape sequences, with the byte of s each byte of the text is.
func refTokens(s string) (text string, at []int) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == ansi.ESC {
			i += len(refSeq(s[i:]))
			continue
		}
		b.WriteByte(s[i])
		at = append(at, i)
		i++
	}
	return b.String(), at
}

// refSeq returns the escape sequence at the start of s: a control sequence
// or a hyperlink, ended by ST or BEL.
func refSeq(s string) string {
	if strings.HasPrefix(s, "\x1b]") {
		for j := 2; j < len(s); j++ {
			switch {
			case s[j] == '\a':
				return s[:j+1]
			case s[j] == ansi.ESC && j+1 < len(s) && s[j+1] == '\\':
				return s[:j+2]
			}
		}
		return s
	}
	for j := 2; j < len(s); j++ {
		if s[j] >= 0x40 && s[j] <= 0x7e {
			return s[:j+1]
		}
	}
	return s
}

func refText(s string) string {
	t, _ := refTokens(s)
	return t
}

// refBounds returns the places in text a position may be at: the starts of
// its grapheme clusters, and its end.
func refBounds(text string) []int {
	var out []int
	g := graphemes.FromString(text)
	for g.Next() {
		out = append(out, g.Start())
	}
	return append(out, len(text))
}

// refRaw returns the byte of line s that byte col of its text, a bound, is
// at: just after the text before it.
func refRaw(s string, col int) int {
	if col == 0 {
		return 0
	}
	_, at := refTokens(s)
	return at[col-1] + 1
}

func (r *refBuf) clamp(p Pos) Pos {
	switch {
	case len(r.lines) == 0 || p.Line < 0:
		return Pos{}
	case p.Line >= len(r.lines):
		last := len(r.lines) - 1
		return Pos{Line: last, Col: len(refText(r.lines[last]))}
	}
	bounds := refBounds(refText(r.lines[p.Line]))
	col := 0
	for _, b := range bounds {
		if b <= p.Col {
			col = b
		}
	}
	return Pos{Line: p.Line, Col: col}
}

// refPen is the style and hyperlink text is drawn in.
type refPen struct {
	style uv.Style
	link  uv.Link
}

func (p *refPen) same(q refPen) bool {
	a, b := p.style, q.style
	return a.Fg == b.Fg && a.Bg == b.Bg && a.UnderlineColor == b.UnderlineColor &&
		a.Underline == b.Underline && a.Attrs == b.Attrs && p.link == q.link
}

// read changes p as the escape sequences of s do.
func (p *refPen) read(s string) {
	parser := ansi.NewParser()
	for i := 0; i < len(s); {
		if s[i] != ansi.ESC {
			i++
			continue
		}
		seq := refSeq(s[i:])
		i += len(seq)
		switch {
		case strings.HasPrefix(seq, "\x1b["):
			parser.Reset()
			for j := range len(seq) {
				parser.Advance(seq[j])
			}
			if parser.Command() == 'm' {
				uv.ReadStyle(parser.Params(), &p.style)
			}
		case strings.HasPrefix(seq, "\x1b]8;"):
			uv.ReadLink([]byte(seq[2:len(seq)-len(refSeqEnd(seq))]), &p.link)
		}
	}
}

func refSeqEnd(seq string) string {
	if strings.HasSuffix(seq, "\a") {
		return "\a"
	}
	return "\x1b\\"
}

// penAt returns the pen text is drawn in at byte raw of line i.
func (r *refBuf) penAt(i, raw int) refPen {
	var p refPen
	for _, s := range r.lines[:i] {
		p.read(s)
	}
	p.read(r.lines[i][:raw])
	return p
}

// refStyleTo is what follows the text an edit puts in to set the style to
// to after from.
func refStyleTo(from, to refPen) string {
	var s string
	f, t := from, to
	f.link, t.link = uv.Link{}, uv.Link{}
	if !f.same(t) {
		s = "\x1b[m"
		if !to.style.IsZero() {
			s += to.style.String()
		}
	}
	if from.link != to.link {
		if to.link.URL == "" {
			s += ansi.ResetHyperlink()
		} else {
			s += ansi.SetHyperlink(to.link.URL, to.link.Params)
		}
	}
	return s
}

// replace makes edit e, its places clamped and in order, as the store
// makes it.
func (r *refBuf) replace(from, to Pos, text string) {
	if len(r.lines) == 0 {
		r.lines = []string{""}
	}
	first, last := r.lines[from.Line], r.lines[to.Line]
	ra, rb := refRaw(first, from.Col), refRaw(last, to.Col)
	pFrom, pTo := r.penAt(from.Line, ra), r.penAt(to.Line, rb)
	rest := last[rb:]
	var tail string
	if from != to && strings.Contains(rest, "\x1b") && !strings.Contains(text, "\x1b") && refText(rest) == "" {
		end := pTo
		end.read(rest)
		tail = refStyleTo(pFrom, end)
	} else {
		tail = refStyleTo(pFrom, pTo) + rest
	}
	segs := strings.Split(text, "\n")
	for i := range len(segs) - 1 {
		segs[i] = strings.TrimSuffix(segs[i], "\r")
	}
	segs[0] = first[:ra] + segs[0]
	segs[len(segs)-1] += tail
	r.lines = slices.Concat(r.lines[:from.Line], segs, r.lines[to.Line+1:])
}

// refEdit is an edit the reference made, in bytes of the text of all of
// the lines joined with newlines, before it, and the text it put in.
type refEdit struct {
	from, to int
	ins      string
}

// edit makes edits as Store.Edit makes them, and returns them in order,
// as refEdits.
func (r *refBuf) edit(edits []TextEdit) []refEdit {
	if len(r.lines) == 0 {
		r.lines = []string{""}
	}
	type kept struct {
		from, to Pos
		text     string
		order    int
	}
	var ks []kept
	for i, e := range edits {
		from, to := r.clamp(e.From), r.clamp(e.To)
		if to.Before(from) {
			from, to = to, from
		}
		ok := true
		for _, k := range ks {
			if k.from.Before(to) && from.Before(k.to) {
				ok = false
			}
		}
		if ok {
			ks = append(ks, kept{from, to, e.Text, i})
		}
	}
	slices.SortStableFunc(ks, func(a, b kept) int {
		if c := a.from.Compare(b.from); c != 0 {
			return c
		}
		return a.to.Compare(b.to)
	})
	starts := lineStarts(r.texts())
	out := make([]refEdit, len(ks))
	for i, k := range ks {
		ins := strings.ReplaceAll(k.text, "\r\n", "\n")
		out[i] = refEdit{absOf(starts, k.from), absOf(starts, k.to), refText(ins)}
	}
	for _, k := range slices.Backward(ks) {
		if k.from != k.to || k.text != "" {
			r.replace(k.from, k.to, k.text)
		}
	}
	return out
}

func (r *refBuf) texts() []string {
	out := make([]string, len(r.lines))
	for i, s := range r.lines {
		out[i] = refText(s)
	}
	return out
}

// absOf returns the byte of the texts joined with newlines that p is at,
// given starts, the byte each text starts at.
func absOf(starts []int, p Pos) int { return starts[p.Line] + p.Col }

// lineStarts returns the byte each of texts starts at, joined with
// newlines, and one past the end.
func lineStarts(texts []string) []int {
	out := make([]int, len(texts)+1)
	for i, t := range texts {
		out[i+1] = out[i] + len(t) + 1
	}
	return out
}

// posOfAbs is the inverse of absOf.
func posOfAbs(starts []int, off int) Pos {
	i, found := slices.BinarySearch(starts, off)
	if !found {
		i--
	}
	i = min(i, len(starts)-2)
	return Pos{Line: i, Col: off - starts[i]}
}

// refMap returns where byte p of the text before edits is after them.
func refMap(edits []refEdit, p int, after bool) int {
	delta := 0
	for _, e := range edits {
		switch {
		case p < e.from:
			return p + delta
		case p == e.from && (e.from < e.to || !after):
			return e.from + delta
		case p < e.to:
			return e.from + delta + len(e.ins)
		}
		delta += len(e.ins) - (e.to - e.from)
	}
	return p + delta
}

// snapRef is a snapshot taken in the model test, and what it held.
type snapRef struct {
	s     Snapshot
	lines []string
}

// TestEditModel drives a store and the reference through long random
// runs of edits, appends, setting and clearing, snapshots and restores, and
// checks after each that the store holds the reference's lines, that what
// it works out of them, its widths and rows, and positions and offsets, is
// what working them out from scratch gives, that each change maps
// positions as mapping them through the reference's text does, and that
// the snapshots still hold what they did.
func TestEditModel(t *testing.T) {
	t.Parallel()

	for seed := range uint64(6) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			t.Parallel()
			runEditModel(t, rand.New(rand.NewPCG(seed, 7)), 260)
		})
	}
}

func runEditModel(t *testing.T, rng *rand.Rand, steps int) {
	t.Helper()
	var b editBuf
	r := &refBuf{}
	var snaps []snapRef
	randPos := func() Pos {
		n := len(r.lines)
		line := rng.IntN(n+2) - 1
		if line >= 0 && line < n && rng.IntN(3) > 0 {
			return Pos{Line: line, Col: rng.IntN(len(refText(r.lines[line]))+4) - 1}
		}
		return Pos{Line: line, Col: rng.IntN(40) - 2}
	}
	near := func(p Pos) Pos {
		q := Pos{Line: p.Line + rng.IntN(3), Col: rng.IntN(30)}
		if rng.IntN(3) == 0 {
			q.Line = p.Line
			q.Col = p.Col + rng.IntN(10)
		}
		return q
	}
	for i := range steps {
		var step string
		var edits []TextEdit
		before := r.texts()
		var change Change
		switch op := rng.IntN(30); {
		case op < 8:
			step = "replace"
			from := randPos()
			to := near(from)
			if rng.IntN(10) == 0 {
				to = randPos()
			}
			text := randomText(rng, 1+rng.IntN(2)*rng.IntN(4))
			if rng.IntN(4) == 0 {
				text = ""
			}
			edits = []TextEdit{{from, to, text}}
			switch rng.IntN(3) {
			case 0:
				change = b.Replace(from, to, text)
			case 1:
				if text == "" {
					change = b.Delete(from, to)
				} else {
					edits[0].To = from
					change = b.Insert(from, text)
				}
			default:
				change = b.Edit(edits...)
			}
		case op < 11:
			step = "type"
			at := randPos()
			for k := range 1 + rng.IntN(6) {
				text := editParts[rng.IntN(editPlain)]
				if k%3 == 2 {
					text = "\n"
				}
				before = r.texts()
				ref := r.edit([]TextEdit{{at, at, text}})
				c := b.Insert(at, text)
				checkChange(t, fmt.Sprintf("step %d, type %q", i, text), c, ref, before, r.texts())
				at = c.End()
			}
		case op < 14:
			step = "edit at once"
			for range 2 + rng.IntN(4) {
				from := randPos()
				edits = append(edits, TextEdit{from, near(from), randomText(rng, 1+rng.IntN(2))})
				if rng.IntN(3) == 0 {
					edits[len(edits)-1].To = from
				}
			}
			change = b.Edit(edits...)
		case op < 18:
			step = "append"
			text := randomText(rng, 1+rng.IntN(3))
			if rng.IntN(8) == 0 {
				text = randomText(rng, 200+rng.IntN(800))
			}
			b.Append(text)
			r.lines = append(r.lines, refSplit(text)...)
		case op < 19:
			step = "set content"
			text := randomText(rng, rng.IntN(300))
			b.SetContent(text)
			r.lines = nil
			if text != "" {
				r.lines = refSplit(text)
			}
		case op < 20:
			step = "clear"
			b.Clear()
			r.lines = nil
		case op < 24:
			step = "snapshot"
			snaps = append(snaps, snapRef{b.Snapshot(), slices.Clone(r.lines)})
		case op < 27 && len(snaps) > 0:
			step = "restore"
			sn := snaps[rng.IntN(len(snaps))]
			b.Restore(sn.s)
			r.lines = slices.Clone(sn.lines)
		default:
			step = "delete lines"
			if len(r.lines) == 0 {
				continue
			}
			a := rng.IntN(len(r.lines))
			e := a + rng.IntN(len(r.lines)-a+1)
			edits = []TextEdit{{Pos{Line: a}, Pos{Line: e}, ""}}
			change = b.Delete(Pos{Line: a}, Pos{Line: e})
		}
		label := fmt.Sprintf("step %d, %s", i, step)
		if edits != nil {
			ref := r.edit(edits)
			checkChange(t, label, change, ref, before, r.texts())
		}
		checkStoreEdits(t, label, &b, r, rng)
		for k, sn := range snaps {
			if sn.s.Len() != len(sn.lines) || sn.s.String() != strings.Join(sn.lines, "\n") {
				t.Fatalf("%s: snapshot %d changed", label, k)
			}
		}
	}
}

// checkChange fails t unless change c maps every position of the text
// before, and then some past it, as mapping it through the reference's
// edits does, and ends where the last of them ends.
func checkChange(t *testing.T, step string, c Change, edits []refEdit, before, after []string) {
	t.Helper()
	if len(before) == 0 {
		before = []string{""}
	}
	bs, as := lineStarts(before), lineStarts(after)
	for line, text := range before {
		for col := 0; col <= len(text); col++ {
			p := Pos{Line: line, Col: col}
			abs := absOf(bs, p)
			for _, mode := range []bool{false, true} {
				want := posOfAbs(as, refMap(edits, abs, mode))
				got := c.Map(p, false)
				if mode {
					got = c.Map(p, true)
				}
				if got != want {
					t.Fatalf("%s: %v maps to %v (after %v), want %v; edits %+v", step, p, got, mode, want, edits)
				}
			}
		}
	}
	if len(edits) > 0 {
		last := edits[len(edits)-1]
		delta := 0
		for _, e := range edits[:len(edits)-1] {
			delta += len(e.ins) - (e.to - e.from)
		}
		if want := posOfAbs(as, last.from+delta+len(last.ins)); c.End() != want {
			t.Fatalf("%s: change ends at %v, want %v", step, c.End(), want)
		}
	}
}

// editKeys are the widths and modes the rows of the edit model test are
// checked at.
var editKeys = []struct {
	method ansi.Method
	mode   Mode
	width  int
}{
	{ansi.GraphemeWidth, Words, 9},
	{ansi.GraphemeWidth, Chars, 31},
	{ansi.WcWidth, Words, 17},
}

// checkStoreEdits fails t unless b holds r's lines, and works out of them
// what a store given them from scratch does.
func checkStoreEdits(t *testing.T, step string, b *editBuf, r *refBuf, rng *rand.Rand) {
	t.Helper()
	n := len(r.lines)
	if b.Len() != n {
		t.Fatalf("%s: %d lines, want %d", step, b.Len(), n)
	}
	if got, want := b.String(), strings.Join(r.lines, "\n"); got != want {
		for i := range min(n, b.Len()) {
			if b.Line(i) != r.lines[i] {
				t.Fatalf("%s: line %d is %q, want %q", step, i, b.Line(i), r.lines[i])
			}
		}
		t.Fatalf("%s: String differs", step)
	}
	var fresh Store
	fresh.AppendLines(r.lines...)
	texts := r.texts()
	off := 0
	for i, s := range r.lines {
		if b.Line(i) != s {
			t.Fatalf("%s: line %d is %q, want %q", step, i, b.Line(i), s)
		}
		text := texts[i]
		if b.Text(i) != text {
			t.Fatalf("%s: line %d's text is %q, want %q", step, i, b.Text(i), text)
		}
		bounds := refBounds(text)
		_, at := refTokens(s)
		bound := 0 // the bound at or before col
		for col := -1; col <= len(text)+1; col++ {
			for bound+1 < len(bounds) && bounds[bound+1] <= col {
				bound++
			}
			if len(s) > 2000 && col%89 != 0 && col < len(text) {
				continue
			}
			p, want := Pos{Line: i, Col: col}, Pos{Line: i, Col: bounds[bound]}
			if col < 0 {
				want.Col = 0
			}
			if got := b.Clamp(p); got != want {
				t.Fatalf("%s: %v clamps to %v, want %v", step, p, got, want)
			}
		}
		raw := func(col int) int {
			if col == 0 {
				return 0
			}
			return at[col-1] + 1
		}
		for k, col := range bounds {
			if len(s) > 2000 && k%97 != 0 && k != len(bounds)-1 {
				continue
			}
			p := Pos{Line: i, Col: col}
			o := b.Offset(p)
			if want := off + raw(col); o != want {
				t.Fatalf("%s: %v is at byte %d, want %d", step, p, o, want)
			}
			if got := b.PosOf(o); got != p {
				t.Fatalf("%s: byte %d is at %v, want %v", step, o, got, p)
			}
		}
		// Each byte of the line is at the bound at or before the text
		// before it.
		if len(s) < 2000 {
			col := 0
			bound = 0
			for r := 0; r <= len(s); r++ {
				for col < len(at) && at[col] < r {
					col++
				}
				for bound+1 < len(bounds) && bounds[bound+1] <= col {
					bound++
				}
				want := Pos{Line: i, Col: bounds[bound]}
				if got := b.PosOf(off + r); got != want {
					t.Fatalf("%s: byte %d of line %d is at %v, want %v", step, r, i, got, want)
				}
			}
		}
		off += len(s) + 1
	}
	if got, want := b.End(), r.clamp(Pos{Line: n}); got != want {
		t.Fatalf("%s: ends at %v, want %v", step, got, want)
	}
	if n == 0 {
		return
	}
	order := rng.Perm(n)
	for _, method := range []ansi.Method{ansi.GraphemeWidth, ansi.WcWidth} {
		got, want := b.Measure(method), fresh.Measure(method)
		if got.Widest != want.Widest {
			t.Fatalf("%s: widest %d, want %d", step, got.Widest, want.Widest)
		}
		for _, i := range order {
			if got.Width(i) != want.Width(i) {
				t.Fatalf("%s: line %d is %d wide, want %d", step, i, got.Width(i), want.Width(i))
			}
		}
	}
	for _, k := range editKeys {
		got, want := b.Index(k.method, k.mode, k.width), fresh.Index(k.method, k.mode, k.width)
		if got.Total() != want.Total() {
			t.Fatalf("%s %+v: %d rows, want %d", step, k, got.Total(), want.Total())
		}
		for _, i := range order {
			if got.RowsOf(i) != want.RowsOf(i) || got.RowsBefore(i) != want.RowsBefore(i) {
				t.Fatalf("%s %+v: line %d takes rows %d after %d, want %d after %d",
					step, k, i, got.RowsOf(i), got.RowsBefore(i), want.RowsOf(i), want.RowsBefore(i))
			}
		}
	}
}

// TestSnapshotsWhileEditing reads snapshots in other goroutines while the
// store they were taken of is edited, appended to, and measured and
// wrapped at changing widths, as it is under the race detector.
func TestSnapshotsWhileEditing(t *testing.T) {
	t.Parallel()

	type taken struct {
		s     Snapshot
		lines []string
	}
	rng := rand.New(rand.NewPCG(5, 6))
	var b editBuf
	b.SetContent(randomText(rng, 2000))
	snaps := make(chan taken)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 4 {
		wg.Go(func() {
			for sn := range snaps {
				if sn.s.Len() != len(sn.lines) {
					errs <- fmt.Errorf("a snapshot of %d lines has %d", len(sn.lines), sn.s.Len())
					return
				}
				for i, line := range sn.s.Lines(0) {
					if line != sn.lines[i] || sn.s.Line(i) != line || PlainText(sn.s.Line(i), ansi.GraphemeWidth) != refText(line) {
						errs <- fmt.Errorf("line %d of a snapshot differs", i)
						return
					}
				}
				if sn.s.String() != strings.Join(sn.lines, "\n") {
					errs <- errors.New("a snapshot's text differs")
					return
				}
			}
		})
	}
	for i := range 400 {
		switch rng.IntN(4) {
		case 0:
			b.Append(randomText(rng, 1))
		default:
			from := Pos{Line: rng.IntN(b.Len()), Col: rng.IntN(20)}
			to := Pos{Line: from.Line + rng.IntN(3), Col: rng.IntN(20)}
			b.Replace(from, to, randomText(rng, 1+rng.IntN(2)))
		}
		b.Index(ansi.GraphemeWidth, Words, 20+i%40)
		b.Measure(ansi.WcWidth)
		if i%10 == 0 {
			lines := make([]string, b.Len())
			for k := range lines {
				lines[k] = b.Line(k)
			}
			snaps <- taken{b.Snapshot(), lines}
		}
	}
	close(snaps)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestRestoreKeepsMeasures checks that a store put back to a snapshot
// keeps what it worked out of its lines, rather than measure them again,
// and may be restored into another store.
func TestRestoreKeepsMeasures(t *testing.T) {
	t.Parallel()

	b := logLikeStore(5000)
	b.Index(ansi.GraphemeWidth, Words, 30)
	before := b.Snapshot()
	b.Replace(Pos{2500, 3}, Pos{2600, 1}, "edit")
	b.Index(ansi.GraphemeWidth, Words, 30)
	b.Restore(before)
	if !b.Kept(ansi.GraphemeWidth, Words, 30) {
		t.Fatal("a restored store measures its lines again")
	}
	var other editBuf
	other.Restore(before)
	if other.String() != before.String() || other.Len() != 5000 {
		t.Fatal("another store restored a snapshot wrong")
	}
	other.Insert(Pos{}, "x")
	if before.Line(0) == other.Line(0) || b.Line(0) != before.Line(0) {
		t.Fatal("editing a store restored from another changed the snapshot")
	}
}

// logLikeStore returns a store of n lines like those of a log.
func logLikeStore(n int) *editBuf {
	var b editBuf
	for i := range n {
		b.AppendLines(fmt.Sprintf("\x1b[2m%07d\x1b[m GET /api/v1/items/%d served in %dms by worker-%02d %s",
			i, i*7, i%97, i%32, strings.Repeat("lorem ipsum ", 3+i%24)))
	}
	return &b
}

// FuzzStoreReplace checks a replace of lines made of the parts the model
// test makes them of against the reference.
func FuzzStoreReplace(f *testing.F) {
	f.Add([]byte{1, 2, 3, 0xff, 4, 5}, []byte{0xff, 7}, 0, 1, 1, 0)
	f.Add([]byte{12, 13, 1, 0xff, 14, 2, 0xff, 20}, []byte{3}, 1, 0, 2, 4)
	f.Fuzz(func(t *testing.T, content, text []byte, fl, fc, tl, tc int) {
		build := func(bs []byte) string {
			var s strings.Builder
			for _, c := range bs {
				if c == 0xff {
					s.WriteByte('\n')
					continue
				}
				s.WriteString(editParts[int(c)%len(editParts)])
			}
			return s.String()
		}
		in, ins := build(content), build(text)
		var b editBuf
		b.SetContent(in)
		r := &refBuf{}
		if in != "" {
			r.lines = refSplit(in)
		}
		edit := TextEdit{Pos{fl % 8, fc % 64}, Pos{tl % 8, tc % 64}, ins}
		before := r.texts()
		c := b.Edit(edit)
		checkChange(t, "replace", c, r.edit([]TextEdit{edit}), before, r.texts())
		if got, want := b.String(), strings.Join(r.lines, "\n"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
