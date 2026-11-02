// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package canvas

import (
	"image/color"
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// appendStyleDiff appends the escape sequence that changes the style from
// from to to, the same bytes as [uv.StyleDiff], without the allocations it
// makes for each change, which a frame makes thousands of. to must differ
// from from and not be zero.
func appendStyleDiff(b []byte, from, to *uv.Style) []byte {
	b = append(b, "\x1b["...)
	start := len(b)
	b = appendColorDiff(b, start, from.Fg, to.Fg, colorFg)
	b = appendColorDiff(b, start, from.Bg, to.Bg, colorBg)
	b = appendColorDiff(b, start, from.UnderlineColor, to.UnderlineColor, colorUnderline)
	b = appendAttrDiff(b, start, from, to)
	return append(b, 'm')
}

// param appends p to the parameters of an escape sequence that start at
// index start of b.
func param(b []byte, start int, p string) []byte {
	if len(b) > start {
		b = append(b, ';')
	}
	return append(b, p...)
}

// appendColorDiff appends the parameters that change a color for kind from
// from to to, if they differ.
func appendColorDiff(b []byte, start int, from, to color.Color, kind colorKind) []byte {
	if colorEqual(from, to) {
		return b
	}
	if len(b) > start {
		b = append(b, ';')
	}
	return appendColor(b, to, kind)
}

// appendAttrDiff appends the parameters that change the attributes and
// underline from from to to, in the order uv.StyleDiff writes them: the
// resets first, where bold and faint share one, and so do the blinks.
func appendAttrDiff(b []byte, start int, from, to *uv.Style) []byte {
	changed := from.Attrs ^ to.Attrs
	set := func(a uint8) bool { return to.Attrs&a != 0 }
	unset := func(a uint8) bool { return from.Attrs&a != 0 && to.Attrs&a == 0 }

	if unset(uv.AttrBold) || unset(uv.AttrFaint) {
		b = param(b, start, "22")
		changed |= uv.AttrBold | uv.AttrFaint
	}
	if changed&uv.AttrItalic != 0 && !set(uv.AttrItalic) {
		b = param(b, start, "23")
	}
	underlineChanged := from.Underline != to.Underline
	if underlineChanged && to.Underline == ansi.UnderlineNone {
		b = param(b, start, "24")
	}
	if unset(uv.AttrBlink) || unset(uv.AttrRapidBlink) {
		b = param(b, start, "25")
		changed |= uv.AttrBlink | uv.AttrRapidBlink
	}
	for _, r := range [...]struct {
		attr uint8
		code string
	}{{uv.AttrReverse, "27"}, {uv.AttrConceal, "28"}, {uv.AttrStrikethrough, "29"}} {
		if changed&r.attr != 0 && !set(r.attr) {
			b = param(b, start, r.code)
		}
	}

	for _, s := range [...]struct {
		attr uint8
		code string
	}{{uv.AttrBold, "1"}, {uv.AttrFaint, "2"}, {uv.AttrItalic, "3"}} {
		if changed&s.attr != 0 && set(s.attr) {
			b = param(b, start, s.code)
		}
	}
	if underlineChanged && to.Underline == ansi.UnderlineSingle {
		b = param(b, start, "4")
	}
	for _, s := range [...]struct {
		attr uint8
		code string
	}{
		{uv.AttrBlink, "5"},
		{uv.AttrRapidBlink, "6"},
		{uv.AttrReverse, "7"},
		{uv.AttrConceal, "8"},
		{uv.AttrStrikethrough, "9"},
	} {
		if changed&s.attr != 0 && set(s.attr) {
			b = param(b, start, s.code)
		}
	}
	if underlineChanged && to.Underline > ansi.UnderlineSingle {
		switch to.Underline { //nolint:exhaustive // None and single are handled above.
		case ansi.UnderlineDouble:
			b = param(b, start, "4:2")
		case ansi.UnderlineCurly:
			b = param(b, start, "4:3")
		case ansi.UnderlineDotted:
			b = param(b, start, "4:4")
		case ansi.UnderlineDashed:
			b = param(b, start, "4:5")
		}
	}
	return b
}

// styleEqual reports whether a and b are the same style, as [uv.Style.Equal]
// does, comparing the cheap fields first and blended colors directly.
func styleEqual(a, b *uv.Style) bool {
	if a.Attrs != b.Attrs || a.Underline != b.Underline {
		return false
	}
	return colorEqual(a.Fg, b.Fg) && colorEqual(a.Bg, b.Bg) && colorEqual(a.UnderlineColor, b.UnderlineColor)
}

// colorEqual reports whether a and b are the same color, as ultraviolet
// compares them: by their 16-bit channels, with a fast path for the color
// type blended colors have.
func colorEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if x, ok := a.(color.RGBA); ok {
		if y, isRGBA := b.(color.RGBA); isRGBA {
			return x == y
		}
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// colorKind is what a color in an escape sequence is for.
type colorKind uint8

const (
	colorFg colorKind = iota
	colorBg
	colorUnderline
)

// appendColor appends the parameters that set c for kind, as package ansi
// writes them.
func appendColor(b []byte, c color.Color, kind colorKind) []byte {
	if c == nil {
		return append(b, [...]string{"39", "49", "59"}[kind]...)
	}
	switch c := c.(type) {
	case ansi.BasicColor:
		if kind == colorUnderline {
			return strconv.AppendUint(append(b, "58;5;"...), uint64(c), 10)
		}
		if c > ansi.BrightWhite {
			return append(b, [...]string{"39", "49"}[kind]...)
		}
		code := 30 + 10*uint64(kind)
		if c >= ansi.BrightBlack {
			code += 60 - 8
		}
		return strconv.AppendUint(b, code+uint64(c), 10)
	case ansi.IndexedColor:
		b = append(b, [...]string{"38;5;", "48;5;", "58;5;"}[kind]...)
		return strconv.AppendUint(b, uint64(c), 10)
	}
	r, g, bl, _ := c.RGBA()
	b = append(b, [...]string{"38;2;", "48;2;", "58;2;"}[kind]...)
	b = strconv.AppendUint(b, uint64(shift(r)), 10)
	b = strconv.AppendUint(append(b, ';'), uint64(shift(g)), 10)
	return strconv.AppendUint(append(b, ';'), uint64(shift(bl)), 10)
}

// shift turns a 16-bit color channel into 8 bits, as package ansi does.
func shift(x uint32) uint32 {
	if x > 0xff {
		x >>= 8
	}
	return x
}
