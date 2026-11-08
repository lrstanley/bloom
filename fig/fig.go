// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package fig renders text in FIGlet and TOIlet fonts: large letters made of
// ordinary characters, as bloom nodes or as plain strings.
//
//	fig.Text("bloom").Font(fonts.Slant(), fonts.Small()).
//		Paint(fig.Gradient(rose, apricot))
//
// A [Font] lays text out into a [Figure] the way C figlet does, byte for
// byte: full width, fitted, smushed by the font's rules or universally, with
// hardblanks, right-to-left fonts, word wrapping, and justification. Lines of
// text can also be smushed vertically, which figlet does not do. A Figure
// remembers, for every cell, which letter of the text drew it, and for every
// cell where two letters overlap, both letters and the rule that merged them.
// That is what lets [TextNode] color letters, animate them one at a time with
// bloom's effects, and pull overlapping letters apart without losing strokes.
//
// Fonts are in package fonts, each behind a function, so a program only
// carries the fonts it uses. [Parse] loads any other.
package fig
