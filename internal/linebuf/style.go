// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package linebuf

import (
	"image/color"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// The fields of a [pen] that a [penChange] sets, besides the attribute bits.
const (
	setFg uint8 = 1 << iota
	setBg
	setUnderlineColor
	setUnderline
	setLink

	setStyle = setFg | setBg | setUnderlineColor | setUnderline
	setAll   = setStyle | setLink
)

// penChange is what a line, or a run of lines, does to the style text is
// drawn in: each field of a pen it sets, to the value in pen, and leaves
// the others as they were. Changes compose, the later one winning for each
// field it sets, so that the style a line starts in is the changes of the
// lines before it composed and applied to the zero pen. A nil *penChange
// changes nothing, as a line without escape sequences does.
//
// A penChange is never changed once made, so that nodes may share one.
type penChange struct {
	set   uint8 // the fields set, of setFg and the rest
	attrs uint8 // the attribute bits set
	pen   pen   // what they are set to; the fields left are zero
}

// styleReset is the change of a line that ends with its style reset, as
// most colored log lines do, shared by them all. A reset leaves the
// hyperlink as it was.
var styleReset = &penChange{set: setStyle, attrs: 0xff}

// unsetColor is a color no escape sequence sets, which a line is read from
// to tell the colors it sets from those it leaves.
type unsetColor struct{}

func (unsetColor) RGBA() (r, g, b, a uint32) { return 0, 0, 0, 0 }

func isUnset(c color.Color) bool {
	_, ok := c.(unsetColor)
	return ok
}

// unsetUnderline and unsetLink are, as unsetColor, an underline style and
// a hyperlink no escape sequence sets.
const unsetUnderline ansi.Underline = 0xff

var unsetLink = uv.Link{URL: "\x00", Params: "\x00"}

// unsetPen is a pen with every field set to what no escape sequence sets,
// and every attribute bit on.
var unsetPen = pen{
	style: uv.Style{Fg: unsetColor{}, Bg: unsetColor{}, UnderlineColor: unsetColor{}, Underline: unsetUnderline, Attrs: 0xff},
	link:  unsetLink,
}

// lineChange returns what line s does to the style text is drawn in, or nil
// when it does nothing. It reads the line from two pens, the zero pen and
// [unsetPen]: a field the line sets is the same after both, and a field it
// leaves is as each started.
func lineChange(s string) *penChange {
	if strings.IndexByte(s, ansi.ESC) < 0 {
		return nil
	}
	zero, unset := penAfter(s, pen{}), penAfter(s, unsetPen)
	var c penChange
	if !isUnset(unset.style.Fg) {
		c.set |= setFg
		c.pen.style.Fg = zero.style.Fg
	}
	if !isUnset(unset.style.Bg) {
		c.set |= setBg
		c.pen.style.Bg = zero.style.Bg
	}
	if !isUnset(unset.style.UnderlineColor) {
		c.set |= setUnderlineColor
		c.pen.style.UnderlineColor = zero.style.UnderlineColor
	}
	if unset.style.Underline != unsetUnderline {
		c.set |= setUnderline
		c.pen.style.Underline = zero.style.Underline
	}
	if unset.link != unsetLink {
		c.set |= setLink
		c.pen.link = zero.link
	}
	// A bit read as on from the zero pen, or off from the unset one, was
	// set by the line.
	c.attrs = ^(zero.style.Attrs ^ unset.style.Attrs)
	c.pen.style.Attrs = zero.style.Attrs & c.attrs
	switch {
	case c.set == 0 && c.attrs == 0:
		return nil
	case c.set == styleReset.set && c.attrs == styleReset.attrs && c.pen.zero():
		return styleReset
	}
	made := c
	return &made
}

// covers reports whether c sets every field and attribute bit that set and
// attrs name.
func (c *penChange) covers(set, attrs uint8) bool {
	return c.set&set == set && c.attrs&attrs == attrs
}

// apply changes p as c does.
func (c *penChange) apply(p *pen) {
	if c == nil {
		return
	}
	if c == styleReset {
		p.style = uv.Style{}
		return
	}
	if c.set&setFg != 0 {
		p.style.Fg = c.pen.style.Fg
	}
	if c.set&setBg != 0 {
		p.style.Bg = c.pen.style.Bg
	}
	if c.set&setUnderlineColor != 0 {
		p.style.UnderlineColor = c.pen.style.UnderlineColor
	}
	if c.set&setUnderline != 0 {
		p.style.Underline = c.pen.style.Underline
	}
	if c.set&setLink != 0 {
		p.link = c.pen.link
	}
	p.style.Attrs = p.style.Attrs&^c.attrs | c.pen.style.Attrs
}

// fill sets the fields of c that it leaves to those that earlier, a change
// before it, sets, making c the change of both, and reports whether it set
// any.
func (c *penChange) fill(earlier *penChange) bool {
	if earlier == nil {
		return false
	}
	missing := earlier.set &^ c.set
	bits := earlier.attrs &^ c.attrs
	if missing == 0 && bits == 0 {
		return false
	}
	if missing&setFg != 0 {
		c.pen.style.Fg = earlier.pen.style.Fg
	}
	if missing&setBg != 0 {
		c.pen.style.Bg = earlier.pen.style.Bg
	}
	if missing&setUnderlineColor != 0 {
		c.pen.style.UnderlineColor = earlier.pen.style.UnderlineColor
	}
	if missing&setUnderline != 0 {
		c.pen.style.Underline = earlier.pen.style.Underline
	}
	if missing&setLink != 0 {
		c.pen.link = earlier.pen.link
	}
	c.pen.style.Attrs |= earlier.pen.style.Attrs & bits
	c.set |= missing
	c.attrs |= bits
	return true
}

// compose returns the change of a and then b.
func compose(a, b *penChange) *penChange {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.covers(a.set, a.attrs):
		return b
	}
	c := new(penChange)
	*c = *b
	c.fill(a)
	return c
}

// composeAll returns the change of cs, one after another. It is the last
// of them when that sets all the others set, as a reset does, so that
// most runs of lines share their change rather than make one.
func composeAll(cs []*penChange) *penChange {
	i := len(cs) - 1
	for i >= 0 && cs[i] == nil {
		i--
	}
	if i < 0 {
		return nil
	}
	last := cs[i]
	acc, grew := *last, false
	for i--; i >= 0 && !acc.covers(setAll, 0xff); i-- {
		if acc.fill(cs[i]) {
			grew = true
		}
	}
	if !grew {
		return last
	}
	made := acc
	return &made
}
