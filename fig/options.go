// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

// Options is how a [Font] lays text out. The zero value lays it out as C
// figlet does with no options. Options is comparable, and a font remembers
// the [Figure] for each text and Options it has laid out.
type Options struct {
	// Horizontal is how glyphs are put side by side. The zero value is the
	// font's own layout.
	Horizontal Mode
	// Vertical is how lines of text are stacked. The zero value stacks them
	// at full height, as figlet does, unless Strict is set, which uses the
	// font's own vertical layout.
	Vertical Mode
	// Rules replaces the font's smushing rules, as figlet's -m does, for
	// smushed layouts: its horizontal rules horizontally, and its vertical
	// rules vertically. Zero keeps the font's.
	Rules Rules
	// Direction is which way text is printed. The zero value is the font's
	// own direction.
	Direction Direction
	// Width wraps lines of text that would be wider than this many columns,
	// at spaces, as figlet does, or anywhere in a word too long for a line.
	// Zero never wraps.
	Width int
	// Paragraph joins lines of text that are not separated by a blank line,
	// as figlet's -p does, so that Width alone decides where lines break.
	Paragraph bool
	// Justify is where each line goes within Width. The zero value is the
	// left, or the right for a right-to-left font, as in figlet. It does
	// nothing without a Width.
	Justify Justify
	// Tracking adds columns between glyphs, after they are smushed. A
	// negative Tracking pushes glyphs further into each other than the
	// font's rules allow, overlapping their strokes, with Stacking deciding
	// which is drawn on top.
	Tracking int
	// Stacking is which glyph is drawn on top where glyphs overlap.
	Stacking Stacking
	// Strict follows the FIGfont standard where C figlet does not: glyphs
	// narrower than two columns may smush, and the zero value of Vertical
	// is the font's own vertical layout.
	Strict bool
	// Control maps the text before it is laid out, as figlet's -C option
	// does. Nil leaves it as it is.
	Control *Control
}

// Mode is a layout mode, horizontal or vertical.
type Mode uint8

const (
	// Default is the font's own layout.
	Default Mode = iota
	// Full puts glyphs side by side, or lines one under another, at their
	// full width or height.
	Full
	// Fitted moves glyphs, or lines, together until they touch.
	Fitted
	// Smushed moves them one step further where the touching characters can
	// be merged by the font's smushing rules, or by [Options.Rules]. With no
	// rules, it is Universal.
	Smushed
	// Universal moves them one step further, the later character
	// replacing the earlier where they meet.
	Universal
)

// Direction is which way text is printed.
type Direction uint8

const (
	// FontDirection is the font's own direction.
	FontDirection Direction = iota
	// LeftToRight prints text left to right.
	LeftToRight
	// RightToLeft prints text right to left.
	RightToLeft
)

// Justify is where lines go within [Options.Width].
type Justify uint8

const (
	// Auto is the left for left-to-right text and the right for
	// right-to-left text.
	Auto Justify = iota
	// Left puts lines at the left.
	Left
	// Center puts lines in the middle.
	Center
	// Right puts lines at the right.
	Right
)

// Stacking is which glyph is drawn on top where glyphs overlap.
type Stacking uint8

const (
	// LaterOnTop draws the glyph later in the text on top, as figlet does.
	LaterOnTop Stacking = iota
	// EarlierOnTop draws the glyph earlier in the text on top.
	EarlierOnTop
)

// Rules are the smushing rules of the FIGfont standard, as the bits of a
// font's Full_Layout.
type Rules uint16

// The horizontal smushing rules.
const (
	// RuleEqual merges two equal characters into one.
	RuleEqual Rules = 1 << iota
	// RuleUnderscore replaces "_" with any of "|/\[]{}()<>".
	RuleUnderscore
	// RuleHierarchy replaces a character with one of a later class, of
	// "|", "/\", "[]", "{}", "()", and "<>".
	RuleHierarchy
	// RulePair merges opposite brackets, "[]", "{}", and "()" either way
	// round, into "|".
	RulePair
	// RuleBigX merges "/\" into "|", "\/" into "Y", and "><" into "X".
	RuleBigX
	// RuleHardblank merges two hardblanks into one.
	RuleHardblank
)

// The vertical smushing rules.
const (
	// RuleVEqual merges two equal characters, one over the other.
	RuleVEqual Rules = 256 << iota
	// RuleVUnderscore replaces "_" with any of "|/\[]{}()<>" over or under it.
	RuleVUnderscore
	// RuleVHierarchy replaces a character with one of a later class over or
	// under it, as RuleHierarchy does.
	RuleVHierarchy
	// RuleVLine merges "-" and "_", one over the other, into "=".
	RuleVLine
	// RuleVSuper merges "|" over "|", and lets lines overlap further while
	// they keep meeting so.
	RuleVSuper
)

// The bits of a FIGfont Full_Layout.
const (
	smKern  = 64
	smSmush = 128
	hRules  = 63
	vRules  = 256 | 512 | 1024 | 2048 | 4096
	svFit   = 8192
	svSmush = 16384
)

// Merge is how a [Seam]'s characters were merged.
type Merge uint8

const (
	// MergeEqual is [RuleEqual] or [RuleVEqual].
	MergeEqual Merge = iota + 1
	// MergeUnderscore is [RuleUnderscore] or [RuleVUnderscore].
	MergeUnderscore
	// MergeHierarchy is [RuleHierarchy] or [RuleVHierarchy].
	MergeHierarchy
	// MergePair is [RulePair].
	MergePair
	// MergeBigX is [RuleBigX].
	MergeBigX
	// MergeHardblank is [RuleHardblank].
	MergeHardblank
	// MergeLine is [RuleVLine].
	MergeLine
	// MergeSuper is [RuleVSuper].
	MergeSuper
	// MergeUniversal is universal smushing.
	MergeUniversal
	// MergeOverlap is glyphs pushed together by a negative
	// [Options.Tracking], drawn by [Options.Stacking].
	MergeOverlap
)

// Side is whose character a [Seam] shows.
type Side uint8

const (
	// Earlier is the character of the glyph earlier in the text.
	Earlier Side = iota + 1
	// Later is the character of the glyph later in the text.
	Later
	// Both is a character both glyphs had.
	Both
	// Neither is a new character the two made, such as "|" from "[]".
	Neither
)
