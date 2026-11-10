// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/lrstanley/bubbletint/chromatint/v2"
)

// tok is a token on a line of source: the bytes it covers, and its type.
type tok struct {
	start, end int
	typ        chroma.TokenType
}

// highlight splits Go source into chroma's tokens for each of its lines. The
// source need not be a whole file: the lexer only needs tokens.
func highlight(lines []string) [][]tok {
	out := make([][]tok, len(lines))
	it, err := lexers.Go.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return out
	}
	ln, col := 0, 0
	for t := it(); t != chroma.EOF; t = it() {
		// Raw strings and block comments can span lines.
		for i, part := range strings.Split(t.Value, "\n") {
			if i > 0 {
				ln, col = ln+1, 0
			}
			if part != "" && ln < len(lines) && t.Type != chroma.TextWhitespace {
				out[ln] = append(out[ln], tok{start: col, end: col + len(part), typ: t.Type})
			}
			col += len(part)
		}
	}
	return out
}

// codeStyle is the chroma style made from the theme by chromatint, without
// backgrounds, so the code takes its pane's.
var codeStyle = func() *chroma.Style {
	s, err := chroma.NewStyle("golens", chromatint.StyleEntry(theme, false))
	if err != nil {
		panic(err)
	}
	return s
}()

// token returns the style of a type of token, from codeStyle.
func (st *styles) token(typ chroma.TokenType) lipgloss.Style {
	if s, ok := st.code[typ]; ok {
		return s
	}
	e := codeStyle.Get(typ)
	s := lipgloss.NewStyle()
	if e.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(e.Colour.String()))
	}
	s = s.Bold(e.Bold == chroma.Yes).Italic(e.Italic == chroma.Yes).Underline(e.Underline == chroma.Yes)
	st.code[typ] = s
	return s
}

// paint renders line with its tokens in st's colors. Tokens that start at a
// byte in mark are drawn marked, as a call site of the selected callee.
func (st *styles) paint(line string, toks []tok, mark func(col int) bool) string {
	var b strings.Builder
	plain := st.token(chroma.Text)
	at := 0
	for _, t := range toks {
		if t.start > at {
			b.WriteString(plain.Render(line[at:t.start]))
		}
		s := st.token(t.typ)
		if mark != nil && mark(t.start) {
			s = st.mark
		}
		b.WriteString(s.Render(line[t.start:t.end]))
		at = t.end
	}
	if at < len(line) {
		b.WriteString(plain.Render(line[at:]))
	}
	return b.String()
}
