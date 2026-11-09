// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Font is a FIGlet (.flf) or TOIlet (.tlf) font. It is safe for concurrent
// use, and lays out each text and [Options] once, remembering the result.
type Font struct {
	name  string
	parse func() (*face, error) // the font's glyphs, parsed once

	mu   sync.Mutex
	memo map[memoKey]*Figure
}

// face is a parsed font.
type face struct {
	height   int
	baseline int
	layout   int // the header's Full_Layout, or one made from Old_Layout
	rtl      bool
	comments []string
	subs     []string // sub-character strings, by sub
	runes    []rune   // each sub-character's rune, or -1 when it is not a single rune
	glyphs   map[rune]*glyph
	missing  *glyph

	// The characters smushing rules make, which a font may not have.
	bar, y, x, eq sub
}

// glyph is a FIGcharacter: its rows of sub-characters, with endmarks and
// trailing blanks removed as C figlet removes them, so rows may differ in
// length. Its width is the length of its first row, as in figlet.
type glyph struct {
	rows  [][]sub
	width int
}

// sub is a sub-character: an index into a font's table of sub-characters.
type sub uint16

const (
	subNone  sub = iota // past the end of a row, C figlet's NUL
	subSpace            // a blank
	subCont             // the second cell of a double-width sub-character
	subHard             // the font's hardblank, which shows as a space
)

// maxSubs is how many distinct sub-characters a font may hold.
const maxSubs = 1 << 16

// memoLimit is how many figures a font remembers before it starts over.
const memoLimit = 256

type memoKey struct {
	text string
	opts Options
}

// deutsch are the characters that follow the ASCII ones in every font.
var deutsch = [7]rune{196, 214, 220, 228, 246, 252, 223}

// Errors returned by [Parse].
var (
	// ErrNotFont is returned for data that is not a FIGlet or TOIlet font.
	ErrNotFont = errors.New("fig: not a FIGlet 2 or TOIlet font")
	// ErrTooLarge is returned for a font far larger than any real one: taller
	// or with wider glyphs than fig lays out, with longer characters in a
	// cell, or with more distinct sub-characters than it can hold.
	ErrTooLarge = errors.New("fig: font is too large")
)

// Parse reads a font: a FIGlet .flf or TOIlet .tlf file, either of which may
// be zip compressed, as figlet allows, or gzip compressed. Line endings may be
// CRLF.
func Parse(data []byte) (*Font, error) {
	data, err := decompress(data)
	if err != nil {
		return nil, err
	}
	fc, err := parseFace(string(data))
	if err != nil {
		return nil, err
	}
	return &Font{parse: func() (*face, error) { return fc, nil }}, nil
}

// ParseFS reads the font at name in fsys, as [Parse] does. The font is named
// after the file, without its extension.
func ParseFS(fsys fs.FS, name string) (*Font, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("fig: %w", err)
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, name)
	}
	base := path.Base(name)
	f.name = strings.TrimSuffix(strings.TrimSuffix(base, ".gz"), path.Ext(strings.TrimSuffix(base, ".gz")))
	return f, nil
}

// Lazy returns a font named name that is parsed from data, as [Parse] does,
// the first time it is used. It is how package fonts holds its fonts. If data
// is not a font, the font has no glyphs and [Font.Err] reports why.
func Lazy(name, data string) *Font {
	return &Font{name: name, parse: sync.OnceValues(func() (*face, error) {
		b, err := decompress([]byte(data))
		if err != nil {
			return newFace(1), err
		}
		fc, err := parseFace(string(b))
		if err != nil {
			return newFace(1), err
		}
		return fc, nil
	})}
}

// load returns the font's glyphs, parsing a lazy font the first time.
func (f *Font) load() *face {
	fc, _ := f.parse()
	return fc
}

// Err reports why a font made by [Lazy] could not be parsed, or nil.
func (f *Font) Err() error {
	_, err := f.parse()
	return err
}

// Name returns the font's name: the one given to [Lazy], the file name for
// [ParseFS], and empty for [Parse].
func (f *Font) Name() string { return f.name }

// Info describes a font, from its header.
type Info struct {
	// Height is the height of every glyph, in rows.
	Height int
	// Baseline is how many rows from the top capital letters rest on.
	Baseline int
	// Horizontal and Vertical are the layouts the font asks for: [Full],
	// [Fitted], [Smushed] by its Rules, or [Universal] when it smushes with
	// none.
	Horizontal, Vertical Mode
	// Rules are the smushing rules the font asks for.
	Rules Rules
	// RightToLeft reports whether the font prints right to left.
	RightToLeft bool
	// Comments are the header's comment lines, which hold the font's
	// author, history, and license, if it has one.
	Comments []string
	// Glyphs is how many characters the font has.
	Glyphs int
}

// Info returns what the font's header says about it.
func (f *Font) Info() Info {
	fc := f.load()
	return Info{
		Height: fc.height, Baseline: fc.baseline, RightToLeft: fc.rtl,
		Horizontal: layoutMode(fc.layout, smKern, smSmush, hRules),
		Vertical:   layoutMode(fc.layout, svFit, svSmush, vRules),
		Rules:      Rules(fc.layout & (hRules | vRules)), //nolint:gosec // the rules' bits.
		Comments:   append([]string(nil), fc.comments...), Glyphs: len(fc.glyphs),
	}
}

// layoutMode returns the mode of the bits fit, smush, and rules of a
// Full_Layout.
func layoutMode(layout, fit, smush, rules int) Mode {
	switch {
	case layout&smush != 0 && layout&rules != 0:
		return Smushed
	case layout&smush != 0:
		return Universal
	case layout&fit != 0:
		return Fitted
	}
	return Full
}

// Has reports whether the font has a glyph for r. Characters it lacks are
// shown as its glyph for code 0, if it has one, and otherwise not at all.
func (f *Font) Has(r rune) bool {
	_, ok := f.load().glyphs[r]
	return ok
}

// decompress returns data unzipped or gunzipped, if it is compressed.
func decompress(data []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("fig: %w", err)
		}
		if len(zr.File) == 0 {
			return nil, ErrNotFont
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			return nil, fmt.Errorf("fig: %w", err)
		}
		defer rc.Close()
		return readAll(rc)
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("fig: %w", err)
		}
		defer zr.Close()
		return readAll(zr)
	}
	return data, nil
}

// maxFontSize bounds how much a compressed font may unpack to.
const maxFontSize = 16 << 20

func readAll(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxFontSize+1))
	if err != nil {
		return nil, fmt.Errorf("fig: %w", err)
	}
	if len(b) > maxFontSize {
		return nil, ErrTooLarge
	}
	return b, nil
}

// lines splits s into lines without their line endings.
type lines struct {
	s string
}

func (l *lines) next() (string, bool) {
	if l.s == "" {
		return "", false
	}
	line, rest, _ := strings.Cut(l.s, "\n")
	l.s = rest
	return strings.TrimSuffix(line, "\r"), true
}

// newFace returns a face h rows tall with no glyphs, for a font that could
// not be parsed.
func newFace(h int) *face {
	fc := &face{height: h}
	fc.init()
	return fc
}

// init sets up the tables of a face whose height is known, before its
// glyphs are read.
func (fc *face) init() {
	fc.subs = []string{"", " ", "", " "}
	fc.runes = []rune{-1, ' ', -1, -1}
	fc.glyphs = make(map[rune]*glyph, 128)
	fc.missing = &glyph{rows: make([][]sub, fc.height)}
}

// parseFace parses a font's text as C figlet's readfont does.
func parseFace(s string) (*face, error) {
	in := &lines{s: strings.TrimPrefix(s, "\ufeff")}
	header, _ := in.next()
	fc, hard, cmt, herr := parseHeader(header)
	if herr != nil {
		return nil, herr
	}
	for range cmt {
		c, ok := in.next()
		if !ok {
			break
		}
		fc.comments = append(fc.comments, c)
	}
	// The hardblank is interned first, so that it is never mistaken for
	// another sub-character, and shows as a space.
	p := parser{face: fc, hard: hard, index: map[string]sub{}}
	fc.init()
	for r := rune(' '); r <= '~'; r++ {
		if err := p.glyph(in, r); err != nil {
			return nil, err
		}
	}
	for _, r := range deutsch {
		if err := p.glyph(in, r); err != nil {
			return nil, err
		}
	}
	for {
		line, ok := in.next()
		if !ok {
			break
		}
		code, ok := parseCode(line)
		if !ok {
			break
		}
		if err := p.glyph(in, code); err != nil {
			return nil, err
		}
	}
	if g, ok := fc.glyphs[0]; ok {
		fc.missing = g
	}
	for _, m := range []struct {
		dst *sub
		s   string
	}{{&fc.bar, "|"}, {&fc.y, "Y"}, {&fc.x, "X"}, {&fc.eq, "="}} {
		s, err := p.intern(m.s)
		if err != nil {
			return nil, err
		}
		*m.dst = s
	}
	return fc, nil
}

// maxHeight and maxWidth bound the size of a glyph, in rows and columns, and
// maxCluster the bytes in one of its cells, enough for an emoji sequence.
// The largest known fonts' glyphs are about 25 rows and 100 columns; a font
// claiming far more is damaged, and would make every figure that large.
const maxHeight, maxWidth, maxCluster = 256, 256, 32

// parseHeader parses a font's first line, as C figlet's scanf of
// "%*c%c %d %*d %d %d %d %d %d" after the four-byte signature does, and
// returns its hardblank and how many comment lines follow it.
func parseHeader(h string) (fc *face, hard string, cmt int, err error) {
	if !strings.HasPrefix(h, "flf2") && !strings.HasPrefix(h, "tlf2") {
		return nil, "", 0, ErrNotFont
	}
	rest := h[4:]
	_, n := utf8.DecodeRuneInString(rest) // the "a"
	if n == 0 {
		return nil, "", 0, ErrNotFont
	}
	rest = rest[n:]
	_, n = utf8.DecodeRuneInString(rest)
	if n == 0 {
		return nil, "", 0, ErrNotFont
	}
	hard = rest[:n]
	var nums []int
	for field := range strings.FieldsSeq(rest[n:]) {
		v, aerr := strconv.Atoi(field)
		if aerr != nil {
			break
		}
		nums = append(nums, v)
	}
	// height, baseline, max length, old layout, comment lines, then the
	// optional print direction and full layout.
	if len(nums) < 5 {
		return nil, "", 0, ErrNotFont
	}
	if nums[0] > maxHeight {
		return nil, "", 0, ErrTooLarge
	}
	fc = &face{height: max(nums[0], 1), baseline: nums[1]}
	old := nums[3]
	cmt = max(nums[4], 0)
	if len(nums) > 5 {
		fc.rtl = nums[5] == 1
	}
	switch {
	case len(nums) > 6:
		fc.layout = nums[6]
	case old == 0:
		fc.layout = smKern
	case old < 0:
		fc.layout = 0
	default:
		fc.layout = old&31 | smSmush
	}
	return fc, hard, cmt, nil
}

// parser interns a font's sub-characters as it reads its glyphs.
type parser struct {
	*face

	hard  string
	index map[string]sub
}

// glyph reads the next glyph, with code r, as C figlet's readfontchar does.
// A later glyph with the same code replaces an earlier one, as in figlet.
func (p *parser) glyph(in *lines, r rune) error {
	g := &glyph{rows: make([][]sub, p.height)}
	for i := range p.height {
		line, _ := in.next()
		line = strings.TrimRight(line, " \t\n\v\f\r")
		if line != "" {
			end, _ := utf8.DecodeLastRuneInString(line)
			line = strings.TrimRight(line, string(end))
		}
		row, err := p.row(line)
		if err != nil {
			return err
		}
		if len(row) > maxWidth {
			return ErrTooLarge
		}
		g.rows[i] = row
	}
	g.width = len(g.rows[0])
	p.glyphs[r] = g
	return nil
}

// row splits a glyph row into sub-characters: grapheme clusters, a double-
// width one followed by a continuation cell.
func (p *parser) row(line string) ([]sub, error) {
	if !utf8.ValidString(line) {
		line = latin1(line)
	}
	row := make([]sub, 0, len(line))
	for line != "" {
		cluster, width := ansi.FirstGraphemeCluster(line, ansi.GraphemeWidth)
		if len(cluster) > maxCluster {
			return nil, ErrTooLarge
		}
		line = line[len(cluster):]
		s, err := p.intern(cluster)
		if err != nil {
			return nil, err
		}
		row = append(row, s)
		if width > 1 {
			row = append(row, subCont)
		}
	}
	return row, nil
}

func (p *parser) intern(cluster string) (sub, error) {
	if cluster == " " {
		return subSpace, nil
	}
	if cluster == p.hard {
		return subHard, nil
	}
	if s, ok := p.index[cluster]; ok {
		return s, nil
	}
	if len(p.subs) >= maxSubs {
		return 0, ErrTooLarge
	}
	s := sub(len(p.subs)) //nolint:gosec // at most maxSubs.
	p.index[cluster] = s
	p.subs = append(p.subs, cluster)
	r, n := utf8.DecodeRuneInString(cluster)
	if n != len(cluster) {
		r = -1
	}
	p.runes = append(p.runes, r)
	return s, nil
}

// latin1 decodes s as ISO 8859-1, for fonts that are not UTF-8.
func latin1(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for i := range len(s) {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}

// parseCode parses the character code at the start of a code-tagged glyph's
// line as C's "%li" does: decimal, 0x hexadecimal, or 0 octal, with a sign.
func parseCode(line string) (rune, bool) {
	s := strings.TrimLeft(line, " \t\v\f\r")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	base, digits := 10, "0123456789"
	switch {
	case len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') && isDigit(s[2], "0123456789abcdefABCDEF"):
		base, digits, s = 16, "0123456789abcdefABCDEF", s[2:]
	case s != "" && s[0] == '0':
		base, digits = 8, "01234567"
	}
	end := 0
	for end < len(s) && isDigit(s[end], digits) {
		end++
	}
	if end == 0 {
		return 0, false
	}
	v, err := strconv.ParseInt(s[:end], base, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return rune(v), true //nolint:gosec // codes outside rune's range cannot be typed, as in figlet.
}

func isDigit(c byte, digits string) bool { return strings.IndexByte(digits, c) >= 0 }

// Figure lays text out with opts. A font remembers the figures it lays out,
// so the same text and options are laid out once.
func (f *Font) Figure(text string, opts Options) *Figure {
	k := memoKey{text: text, opts: opts}
	f.mu.Lock()
	fig, ok := f.memo[k]
	f.mu.Unlock()
	if ok {
		return fig
	}
	fig = f.layout(text, opts)
	f.mu.Lock()
	if f.memo == nil || len(f.memo) >= memoLimit {
		f.memo = make(map[memoKey]*Figure, 16)
	}
	f.memo[k] = fig
	f.mu.Unlock()
	return fig
}
