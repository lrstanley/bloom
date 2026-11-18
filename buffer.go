// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"iter"
	"math"
	"regexp"
	"strings"

	"bloomfx.dev/bloom/internal/linebuf"
	"bloomfx.dev/bloom/internal/textsel"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/graphemes"
)

// Pos is a place in a buffer's text: a line, and a byte of that line's text
// as [Buffer.Text] gives it, without styles, a tab one byte, the unit a
// [Mark] and a [Match] count in. A position is between two characters, or
// at the start or end of a line, so that Col is the length of the line's
// text at its end.
type Pos struct{ Line, Col int }

// Before reports whether p comes before q.
func (p Pos) Before(q Pos) bool { return p.Line < q.Line || p.Line == q.Line && p.Col < q.Col }

// Compare returns -1 when p comes before q, 1 when it comes after, and 0
// when they are the same.
func (p Pos) Compare(q Pos) int {
	switch {
	case p.Before(q):
		return -1
	case q.Before(p):
		return 1
	}
	return 0
}

// Range is a run of text from Anchor to Head, which may come before it. A
// selection is one, with the caret at Head, where it was moved to, and the
// Anchor where it started.
type Range struct {
	Anchor, Head Pos
	// EndOfRow says which side of a wrap the caret is drawn on. The place
	// where a line wraps is one position in its text but two places on the
	// screen, the end of a row and the start of the next, and a caret there
	// is drawn at the start of the next row unless EndOfRow is set, as the
	// end key and a click past the end of the row set it. It means nothing
	// anywhere else: it is ignored where Head is not a place its line wraps,
	// in a viewport whose lines do not wrap, and by everything that reads
	// the text, such as the selection drawn and copied.
	EndOfRow bool
}

// Empty reports whether the range holds no text, as a caret does.
func (r Range) Empty() bool { return r.Anchor == r.Head }

// WithHead returns r with its Head moved to p, its Anchor kept, and
// EndOfRow cleared, as a caret moved through the text, rather than by rows,
// is drawn at the start of a row. Moving the Head and then collapsing the
// range unless shift is held moves a caret or extends a selection:
//
//	m.sel = m.sel.WithHead(m.buf.NextWord(m.sel.Head))
//	if !shift {
//		m.sel = m.sel.Collapse()
//	}
func (r Range) WithHead(p Pos) Range { return Range{Anchor: r.Anchor, Head: p} }

// Collapse returns the empty range at r's Head, keeping EndOfRow, so that a
// caret at the end of a row stays there.
func (r Range) Collapse() Range { return Range{Anchor: r.Head, Head: r.Head, EndOfRow: r.EndOfRow} }

// From returns the end of the range that comes first.
func (r Range) From() Pos {
	if r.Head.Before(r.Anchor) {
		return r.Head
	}
	return r.Anchor
}

// To returns the end of the range that comes last.
func (r Range) To() Pos {
	if r.Head.Before(r.Anchor) {
		return r.Anchor
	}
	return r.Head
}

// Edit is a change to a buffer's text, as [Buffer.Edit] makes it: the text
// from From up to To replaced with Text.
type Edit struct {
	From, To Pos
	Text     string
}

// Buffer holds the lines of text a [Viewport] shows, such as a log or a
// file, kept in the model and changed as the text changes. A viewport only
// lays out and draws the lines in sight, and the buffer keeps what it works
// out about its lines, such as how wide each is, so that appending a line
// to a buffer of a million costs about as much as appending one to a buffer
// of ten.
//
// Lines may hold ANSI styles and hyperlinks. A style left open at the end
// of a line carries on into the next, as it would in a terminal, so the
// output of a syntax highlighter that styles a comment over many lines is
// drawn as it would be printed. Tabs are drawn as spaces to the next stop,
// every four columns.
//
// The zero value is an empty buffer, ready to use. A Buffer is not safe for
// use by more than one goroutine at a time: change it in Update, as the rest
// of the model is.
type Buffer struct {
	s   linebuf.Store
	log linebuf.ChangeLog

	wordChars    string // the characters besides letters and digits that join words
	wordCharsSet bool   // wordChars was set, rather than left to the default
	rootChars    string // those of the Root that last showed the buffer
	rootCharsSet bool   // a Root has shown the buffer
}

// NewBuffer returns a buffer holding s, as [Buffer.SetContent] sets it.
func NewBuffer(s string) *Buffer {
	b := &Buffer{}
	b.SetContent(s)
	return b
}

// SetContent replaces the buffer's lines with those of s, split at its
// newlines. A newline at the end of s ends its last line, rather than
// starting an empty one after it, and "" leaves the buffer empty.
func (b *Buffer) SetContent(s string) {
	b.s.Clear()
	if s != "" {
		b.s.AppendText(s)
	}
	b.log.Reset(b.s.Rev())
}

// SetLines replaces the buffer's lines with lines. A line holding newlines
// is split at them.
func (b *Buffer) SetLines(lines ...string) {
	b.s.Clear()
	b.Append(lines...)
	b.log.Reset(b.s.Rev())
}

// Append adds lines to the end of the buffer. A line holding newlines is
// split at them, and a newline at the end of one ends it, so lines read
// from a program's output can be given as they come. "" adds an empty
// line.
func (b *Buffer) Append(lines ...string) { b.s.AppendLines(lines...) }

// Clear removes every line.
func (b *Buffer) Clear() {
	b.s.Clear()
	b.log.Reset(b.s.Rev())
}

// Replace replaces the text from from up to to with text, and returns the
// change, to follow positions through it with. Text is split into lines at
// its newlines, a newline at its end starting an empty line, unlike with
// [Buffer.Append], and a carriage return before a newline is dropped.
// Removing a newline joins the lines either side of it.
//
// Both positions are clamped to the text, as [Buffer.Clamp] clamps them,
// and swapped when from comes after to. A buffer with no lines takes the
// change as though it held one empty line, and one that has been changed
// keeps at least one: removing all of its text leaves an empty line.
//
// Text is put in as it is given, and may hold escape sequences, as an
// appended line may. The text after the change is drawn in the style it
// was drawn in: the escape sequences of the text removed are left in its
// place as the one that sets the style they leave, when it is not the style
// the text before is in, so that removing a word that starts a color does
// not lose the color of the words after it.
//
// It costs a step a level of the buffer's tree, and works out again only
// the lines it changes, however many lines it removes.
func (b *Buffer) Replace(from, to Pos, text string) Change {
	return b.edit(linebuf.TextEdit{From: linebuf.Pos(from), To: linebuf.Pos(to), Text: text})
}

// Insert puts text at at, as [Buffer.Replace] puts it in, and returns the
// change.
func (b *Buffer) Insert(at Pos, text string) Change { return b.Replace(at, at, text) }

// Delete removes the text from from up to to, as [Buffer.Replace] removes
// it, and returns the change.
func (b *Buffer) Delete(from, to Pos) Change { return b.Replace(from, to, "") }

// Edit makes edits at once, as one change, each as [Buffer.Replace] makes
// it, such as the text typed at each of several carets. The positions of
// each are those of the buffer before any of them, and the edits may be
// given in any order. They must not overlap, though they may touch: of two
// that overlap, the one given later is dropped. Text put in at the same
// place by more than one goes in in the order they are given.
func (b *Buffer) Edit(edits ...Edit) Change {
	tes := make([]linebuf.TextEdit, len(edits))
	for i, e := range edits {
		tes[i] = linebuf.TextEdit{From: linebuf.Pos(e.From), To: linebuf.Pos(e.To), Text: e.Text}
	}
	return b.edit(tes...)
}

// edit makes edits with [linebuf.Store.Edit], noting the change for what
// follows positions in the buffer.
func (b *Buffer) edit(edits ...linebuf.TextEdit) Change {
	c, changed := b.s.Edit(edits...)
	if changed {
		b.log.Add(b.s.Rev(), c)
	}
	return Change{c: c}
}

// Change is what [Buffer.Replace] and the other edits did to a buffer, to
// follow positions through it with, such as the carets of an editor, or a
// selection, as the text changes under them. Its methods take positions in
// the buffer as it was before the change, and give them in the buffer as
// it is after. The zero value is a change that moves no position.
type Change struct{ c linebuf.Change }

// Map returns where position p of the buffer before the change is after
// it: unchanged before the text changed, moved with the text after it, and
// at the end of the text put in when it was within the text replaced. A
// position where text was put in stays before that text; see
// [Change.MapAfter].
func (c Change) Map(p Pos) Pos { return Pos(c.c.Map(linebuf.Pos(p), false)) }

// MapAfter is [Change.Map], but a position where text was put in goes
// after that text, as a caret where text is typed does.
func (c Change) MapAfter(p Pos) Pos { return Pos(c.c.Map(linebuf.Pos(p), true)) }

// MapRange returns where range r is after the change, its ends mapped so
// that text put in at either end is left out of it, and a caret, an empty
// range, mapped as [Change.Map] maps a position. EndOfRow is kept.
func (c Change) MapRange(r Range) Range {
	a, h := c.c.MapSpan(linebuf.Pos(r.Anchor), linebuf.Pos(r.Head))
	return Range{Anchor: Pos(a), Head: Pos(h), EndOfRow: r.EndOfRow}
}

// End returns where the text put in by the change's last edit ends, in the
// buffer after it, which is where a caret goes after typing.
func (c Change) End() Pos { return Pos(c.c.End()) }

// changesSince returns the changes made to the buffer's text after revision
// rev, in order, and true, or false when positions from then cannot be
// followed to now: the text was set, cleared or restored since, or more
// changes were made than the buffer keeps. Lines appended move no
// position, and need no change.
func (b *Buffer) changesSince(rev uint64) ([]linebuf.Change, bool) {
	return b.log.Since(rev, b.s.Rev())
}

// Len returns the number of lines.
func (b *Buffer) Len() int { return b.s.Len() }

// Line returns line i as it was given, with its styles, or "" when there is
// no such line.
func (b *Buffer) Line(i int) string {
	if i < 0 || i >= b.s.Len() {
		return ""
	}
	return b.s.Line(i)
}

// Text returns the text of line i without its styles, with its tabs, as the
// function given to [ViewportNode.Marks] is given it, or "" when there is no
// such line.
func (b *Buffer) Text(i int) string {
	if i < 0 || i >= b.s.Len() {
		return ""
	}
	return b.s.Text(i)
}

// String returns the buffer's lines, joined with newlines.
func (b *Buffer) String() string { return b.s.String() }

// Rev counts the changes made to the buffer, each edit, line appended, and
// time it is set, so that what is worked out from its text, such as the
// matches of a search, can be kept until it changes.
func (b *Buffer) Rev() uint64 { return b.s.Rev() }

// Clamp returns p moved to the nearest place in the buffer's text, as each
// method given a position moves it: the start for a line before the first,
// the end of the buffer for a line past the last, the end of the line for a
// Col past it, and the start of the character for a Col within one that
// takes more than a byte, or within a grapheme cluster, such as a letter
// and the accent on it.
func (b *Buffer) Clamp(p Pos) Pos { return Pos(b.s.Clamp(linebuf.Pos(p))) }

// End returns the end of the buffer's text: the end of its last line.
func (b *Buffer) End() Pos { return Pos(b.s.End()) }

// Next returns the position one character after p, clamped, as the right
// arrow moves a caret: past a grapheme cluster, such as a letter and the
// accent on it, or a tab, and from the end of a line to the start of the
// next. It is the end of the buffer at its end.
func (b *Buffer) Next(p Pos) Pos {
	p = b.Clamp(p)
	text := b.Text(p.Line)
	if p.Col >= len(text) {
		if p.Line+1 < b.Len() {
			return Pos{Line: p.Line + 1}
		}
		return p
	}
	g := graphemes.FromString(text[p.Col:])
	g.Next()
	return b.Clamp(Pos{Line: p.Line, Col: p.Col + g.End()})
}

// Prev returns the position one character before p, clamped, as the left
// arrow moves a caret, and from the start of a line to the end of the one
// before. It is the start of the buffer at its start.
func (b *Buffer) Prev(p Pos) Pos {
	p = b.Clamp(p)
	switch {
	case p.Col > 0:
		return b.Clamp(Pos{Line: p.Line, Col: p.Col - 1})
	case p.Line > 0:
		return b.Clamp(Pos{Line: p.Line - 1, Col: math.MaxInt})
	}
	return p
}

// SetWordChars sets the characters other than letters and digits that are
// part of the words of the buffer's text, as [Buffer.WordAt],
// [Buffer.NextWord], and [Buffer.PrevWord] find them, and as a double click
// in a [Viewport] of the buffer selects them. Words are found as Unicode
// text segmentation (UAX #29) finds them, and are joined across these
// characters: "_" keeps snake_case one word, as an editor of code wants,
// and with "", words are as Unicode finds them.
//
// A buffer never given any takes those of the [Root] that shows it in a
// viewport, set with [WithWordChars], from the first frame it is drawn in,
// so that a double click and these methods find the same words. Until
// then, and in a buffer never shown, they are bloom's default set,
// "@-./_~?&=%+#", which keeps a path, a URL, or an email address one word.
func (b *Buffer) SetWordChars(chars string) { b.wordChars, b.wordCharsSet = chars, true }

// shownWith notes the word characters of the Root drawing the buffer, which
// it finds words with while it has none of its own.
func (b *Buffer) shownWith(chars string) { b.rootChars, b.rootCharsSet = chars, true }

// ownChars returns the characters that join the buffer's words when no
// Root is asking: its own, or else those of the Root that last showed it,
// or else bloom's default.
func (b *Buffer) ownChars() string {
	if b.rootCharsSet {
		return b.chars(b.rootChars)
	}
	return b.chars(defaultWordChars)
}

// chars returns the characters that join the buffer's words: those set
// with SetWordChars, or else def.
func (b *Buffer) chars(def string) string {
	if b.wordCharsSet {
		return b.wordChars
	}
	return def
}

// WordAt returns the start and the end of the word that the character after
// p, clamped, is in, or failing that, of the word that ends at p, and
// whether there is one, such as to select the word under a caret. Words
// are found as [Buffer.SetWordChars] tells. Only the text around p, up to
// the nearest whitespace either side, is looked through, however long its
// line is.
func (b *Buffer) WordAt(p Pos) (from, to Pos, ok bool) {
	p = b.Clamp(p)
	return b.wordAt(p, b.ownChars(), true)
}

// wordAt returns the word the character after p, clamped, is in, with chars
// joining words, and with before, failing that the word that ends at p.
func (b *Buffer) wordAt(p Pos, chars string, before bool) (from, to Pos, ok bool) {
	text := b.Text(p.Line)
	f, t, ok := textsel.Word(text, p.Col, chars)
	if !ok && before && p.Col > 0 {
		if f, t, ok = textsel.Word(text, p.Col-1, chars); ok && t != p.Col {
			ok = false
		}
	}
	if !ok {
		return Pos{}, Pos{}, false
	}
	return Pos{Line: p.Line, Col: f}, Pos{Line: p.Line, Col: t}, true
}

// NextWord returns the end of the word p is in, or of the next word after
// it, clamped, as ctrl+right moves a caret in most editors: the spaces and
// punctuation between words are skipped, and line ends crossed. Words are
// found as [Buffer.SetWordChars] tells, so "don't" and "3.14" are a word
// each. It is the end of the buffer when no word follows p.
func (b *Buffer) NextWord(p Pos) Pos {
	p = b.Clamp(p)
	chars := b.ownChars()
	for line := p.Line; line < b.Len(); line++ {
		from := 0
		if line == p.Line {
			from = p.Col
		}
		if to, ok := textsel.WordEnd(b.Text(line), from, chars); ok {
			return Pos{Line: line, Col: to}
		}
	}
	return b.End()
}

// PrevWord returns the start of the word p is in, or of the word before
// it, clamped, as ctrl+left moves a caret, skipping what [Buffer.NextWord]
// skips. It is the start of the buffer when no word comes before p.
func (b *Buffer) PrevWord(p Pos) Pos {
	p = b.Clamp(p)
	chars := b.ownChars()
	for line := p.Line; line >= 0; line-- {
		text := b.Text(line)
		to := len(text)
		if line == p.Line {
			to = p.Col
		}
		if at, ok := textsel.WordStart(text, to, chars); ok {
			return Pos{Line: line, Col: at}
		}
	}
	return Pos{}
}

// TextIn returns the text from from up to to, clamped, and swapped when
// from comes after to, without styles, as [Buffer.Text] gives each line,
// with the lines joined with newlines, such as to copy a selection.
func (b *Buffer) TextIn(from, to Pos) string {
	from, to = b.Clamp(from), b.Clamp(to)
	if to.Before(from) {
		from, to = to, from
	}
	if from == to {
		return ""
	}
	if from.Line == to.Line {
		return b.Text(from.Line)[from.Col:to.Col]
	}
	var s strings.Builder
	s.WriteString(b.Text(from.Line)[from.Col:])
	for i := from.Line + 1; i < to.Line; i++ {
		s.WriteByte('\n')
		s.WriteString(b.Text(i))
	}
	s.WriteByte('\n')
	s.WriteString(b.Text(to.Line)[:to.Col])
	return s.String()
}

// Offset returns the byte of [Buffer.String] that p, clamped, is at,
// counting the bytes of the lines before it, with their styles and
// newlines, from what the buffer keeps of them, and then those of its
// line before it.
func (b *Buffer) Offset(p Pos) int { return b.s.Offset(b.s.Clamp(linebuf.Pos(p))) }

// PosOf returns the position that byte offset of [Buffer.String] is at,
// the inverse of [Buffer.Offset]: that of the character the byte is in, or
// of the next one when it is in an escape sequence, the end of a line for
// its newline, and the start or end of the buffer for an offset before or
// past it.
func (b *Buffer) PosOf(offset int) Pos { return Pos(b.s.PosOf(offset)) }

// Snapshot is the text of a [Buffer] as it was when [Buffer.Snapshot] took
// it, which it stays as the buffer goes on changing, to undo a change with
// [Buffer.Restore], or to read the text in another goroutine, such as to
// save it or search it, while the buffer goes on being changed. Unlike the
// buffer, its methods are safe to call from any goroutine. The zero value
// holds no lines.
type Snapshot struct{ s linebuf.Snapshot }

// Snapshot returns the buffer's text as it is, in a step, however many
// lines it holds: the snapshot shares the buffer's lines, and an edit after
// it copies the few parts of the buffer it changes, rather than changing
// them in place.
func (b *Buffer) Snapshot() Snapshot { return Snapshot{s: b.s.Snapshot()} }

// Restore puts the buffer's text back to snapshot s, in a step, keeping
// what the buffer worked out about the lines when it held them, such as how
// wide each is, so that undoing a change to a million lines measures
// nothing again. A snapshot of another buffer may be restored too: the two
// buffers then share their lines, and what they work out about them, and
// must be changed on the same goroutine, as the rest of a model is.
// Positions cannot be followed through a restore with a [Change].
func (b *Buffer) Restore(s Snapshot) {
	b.s.Restore(s.s)
	b.log.Reset(b.s.Rev())
}

// Len returns the number of lines.
func (s Snapshot) Len() int { return s.s.Len() }

// Line returns line i as it was given, with its styles, or "" when there
// is no such line.
func (s Snapshot) Line(i int) string {
	if i < 0 || i >= s.s.Len() {
		return ""
	}
	return s.s.Line(i)
}

// Text returns the text of line i without its styles, as [Buffer.Text]
// does, or "" when there is no such line.
func (s Snapshot) Text(i int) string {
	return linebuf.PlainText(s.Line(i), ansi.GraphemeWidth)
}

// String returns the lines, joined with newlines.
func (s Snapshot) String() string { return s.s.String() }

// Lines returns an iterator over the lines, as they were given, with their
// styles, and their numbers.
func (s Snapshot) Lines() iter.Seq2[int, string] { return s.s.Lines(0) }

// byteAt returns the byte of the text of line i of the buffer, as
// [Buffer.Text] gives it, of the glyph drawn at cell x of the line, measured
// with method: its first byte for each cell of a wide glyph or a tab, and
// the end of the text for a cell past the line's last glyph.
func (b *Buffer) byteAt(i int, method ansi.Method, x int) int { return b.s.ByteAt(i, method, x) }

// Match is where a regular expression matched the text of a line, as
// [Buffer.Find] finds it: the bytes from Start to End, End exclusive, of
// [Buffer.Text] of the line.
type Match struct {
	Line, Start, End int
}

// Find returns the first match of re that starts at or after byte at of the
// text of line, looking through the lines after it in turn, and whether
// there is one. It does not wrap around to the first line. To find the
// match after m, give m.Line and m.End, or m.Start+1 for an empty match.
func (b *Buffer) Find(re *regexp.Regexp, line, at int) (Match, bool) {
	if re == nil {
		return Match{}, false
	}
	for i := max(line, 0); i < b.s.Len(); i++ {
		text := b.Text(i)
		from := 0
		if i == line {
			from = min(max(at, 0), len(text))
		}
		if loc := re.FindStringIndex(text[from:]); loc != nil {
			return Match{Line: i, Start: from + loc[0], End: from + loc[1]}, true
		}
	}
	return Match{}, false
}

// FindBefore returns the last match of re that starts before byte at of the
// text of line, looking through the lines before it in turn, and whether
// there is one. To find the match before m, give m.Line and m.Start.
func (b *Buffer) FindBefore(re *regexp.Regexp, line, at int) (Match, bool) {
	if re == nil {
		return Match{}, false
	}
	for i := min(line, b.s.Len()-1); i >= 0; i-- {
		text := b.Text(i)
		limit := len(text) + 1
		if i == line {
			limit = min(max(at, 0), len(text)+1)
		}
		var last []int
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if loc[0] >= limit {
				break
			}
			last = loc
		}
		if last != nil {
			return Match{Line: i, Start: last[0], End: last[1]}, true
		}
	}
	return Match{}, false
}
