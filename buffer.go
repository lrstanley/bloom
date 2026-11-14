// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"regexp"

	"bloomfx.dev/bloom/internal/linebuf"
	"github.com/charmbracelet/x/ansi"
)

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
	s linebuf.Store
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
}

// SetLines replaces the buffer's lines with lines. A line holding newlines
// is split at them.
func (b *Buffer) SetLines(lines ...string) {
	b.s.Clear()
	b.Append(lines...)
}

// Append adds lines to the end of the buffer. A line holding newlines is
// split at them, and a newline at the end of one ends it, so lines read
// from a program's output can be given as they come. "" adds an empty
// line.
func (b *Buffer) Append(lines ...string) {
	b.s.Grow(len(lines))
	for _, l := range lines {
		b.s.AppendText(l)
	}
}

// Clear removes every line.
func (b *Buffer) Clear() { b.s.Clear() }

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
	return linebuf.PlainText(b.s.Line(i), ansi.GraphemeWidth)
}

// String returns the buffer's lines, joined with newlines.
func (b *Buffer) String() string { return b.s.String() }

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
