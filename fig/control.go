// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import "strings"

// Control is a FIGlet control file (.flc): a mapping applied to text before
// it is laid out, such as from a keyboard layout to the letters of a Hebrew
// font. Give one to a layout with [Options.Control].
//
// Its "t" (translate) commands, number pairs, and "f" (freeze) commands are
// followed as figlet follows them. The commands that choose how input bytes
// are decoded ("b", "u", "h", "j", and "g") are ignored: text is always UTF-8.
type Control struct {
	// stages are mappings separated by freezes. In each stage, the first
	// mapping whose range holds a character applies, and its result is what
	// the next stage sees.
	stages [][]mapping
}

type mapping struct {
	lo, hi, offset rune
}

// ParseControl reads a control file. As in figlet, what it does not
// understand is skipped, so any data is a control file.
func ParseControl(data []byte) *Control {
	c := &Control{stages: [][]mapping{nil}}
	in := &cread{s: string(data)}
	add := func(m mapping) { c.stages[len(c.stages)-1] = append(c.stages[len(c.stages)-1], m) }
	for {
		cmd := in.get()
		switch {
		case cmd < 0:
			return c
		case cmd == 't':
			in.skipSpace()
			first := in.tchar()
			last := first
			if d := in.get(); d == '-' {
				last = in.tchar()
			} else {
				in.unget(d)
			}
			in.skipSpace()
			add(mapping{lo: first, hi: last, offset: in.tchar() - first})
			in.skipLine()
		case cmd == '-' || cmd >= '0' && cmd <= '9':
			in.unget(cmd)
			first := in.num()
			in.skipSpace()
			add(mapping{lo: first, hi: first, offset: in.num() - first})
			in.skipLine()
		case cmd == 'f':
			c.stages = append(c.stages, nil)
			in.skipLine()
		case cmd == 'b' || cmd == 'u' || cmd == 'h' || cmd == 'j' || cmd == '\r' || cmd == '\n':
		default: // 'g', comments, and anything else
			in.skipLine()
		}
	}
}

// Map returns r as the control file maps it.
func (c *Control) Map(r rune) rune {
	if c == nil {
		return r
	}
	for _, stage := range c.stages {
		for _, m := range stage {
			if r >= m.lo && r <= m.hi {
				r += m.offset
				break
			}
		}
	}
	return r
}

// cread reads a control file the way figlet does, a byte at a time.
type cread struct {
	s string
	i int
}

func (r *cread) get() rune {
	if r.i >= len(r.s) {
		r.i = len(r.s) + 1
		return -1
	}
	r.i++
	return rune(r.s[r.i-1])
}

func (r *cread) unget(c rune) {
	if c >= 0 || r.i > len(r.s) {
		r.i--
	}
}

func (r *cread) skipSpace() {
	for {
		c := r.get()
		if c < 0 || !strings.ContainsRune(" \t\n\v\f\r", c) {
			r.unget(c)
			return
		}
	}
}

func (r *cread) skipLine() {
	for {
		if c := r.get(); c < 0 || c == '\n' {
			return
		}
	}
}

// num reads a number as figlet's readnum does: "0x" for hexadecimal, "0" for
// octal, and, as in figlet, any of 0-9 and A-F as a digit in any base.
func (r *cread) num() rune {
	r.skipSpace()
	sign := rune(1)
	if c := r.get(); c == '-' {
		sign = -1
	} else {
		r.unget(c)
	}
	base := rune(10)
	if c := r.get(); c == '0' {
		if c = r.get(); c == 'x' || c == 'X' {
			base = 16
		} else {
			base = 8
			r.unget(c)
		}
	} else {
		r.unget(c)
	}
	var acc rune
	for {
		c := r.get()
		d := strings.IndexRune("0123456789ABCDEF", toUpper(c))
		if c < 0 || d < 0 {
			r.unget(c)
			return acc * sign
		}
		acc = acc*base + rune(d) //nolint:gosec // a digit, under 16.
	}
}

// tchar reads a character of a "t" command: a byte, an escape such as \n, or
// an escaped number such as \0x41.
func (r *cread) tchar() rune {
	c := r.get()
	if c == '\n' || c == '\r' {
		r.unget(c)
		return 0
	}
	if c != '\\' {
		return c
	}
	next := r.get()
	switch next {
	case 'a':
		return 7
	case 'b':
		return 8
	case 'e':
		return 27
	case 'f':
		return 12
	case 'n':
		return 10
	case 'r':
		return 13
	case 't':
		return 9
	case 'v':
		return 11
	}
	if next == '-' || next == 'x' || next >= '0' && next <= '9' {
		r.unget(next)
		return r.num()
	}
	return next
}

func toUpper(c rune) rune {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}
