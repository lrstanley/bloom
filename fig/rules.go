// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import "strings"

// smusher merges characters horizontally, as C figlet's smushem does, for
// the glyph being added and the one before it.
type smusher struct {
	fc     *face
	mode   int // the smushmode: Full_Layout bits
	rtl    bool
	strict bool
	prev   int // the width of the glyph added before, as figlet's previouscharwidth
	cur    int // the width of the glyph being added, as figlet's currcharwidth
}

// hierarchy are the classes of [RuleHierarchy], each replaced by later ones.
var hierarchy = [...]string{"|", `/\`, "[]", "{}", "()", "<>"}

// cset reports whether c is in set the way C's strchr finds it, which also
// finds the NUL that ends every string: a row's end is in every set.
func cset(set string, c rune) bool { return c == 0 || strings.ContainsRune(set, c) }

// rn returns the rune of s, 0 for a row's end, or -1 for a sub-character
// that is not a single rune.
func (fc *face) rn(s sub) rune {
	if s == subNone {
		return 0
	}
	return fc.runes[s]
}

// smushem merges l, the character on the left, and r, on the right,
// returning how, or subNone when they cannot be merged.
//
//nolint:gocognit // it follows figlet's smushem rule by rule.
func (s *smusher) smushem(l, r sub) (sub, Merge) {
	if l == subSpace {
		return r, 0
	}
	if r == subSpace {
		return l, 0
	}
	if l == subCont || r == subCont {
		// The second half of a double-width character merges with nothing.
		return subNone, 0
	}
	if !s.strict && (s.prev < 2 || s.cur < 2) {
		return subNone, 0
	}
	if s.mode&smSmush == 0 {
		return subNone, 0
	}
	if s.mode&hRules == 0 {
		switch {
		case l == subHard:
			return r, MergeUniversal
		case r == subHard, s.rtl:
			return l, MergeUniversal
		}
		return r, MergeUniversal
	}
	rules := Rules(s.mode) //nolint:gosec // the rule bits are the low 15.
	if rules&RuleHardblank != 0 && l == subHard && r == subHard {
		return l, MergeHardblank
	}
	if l == subHard || r == subHard {
		return subNone, 0
	}
	if rules&RuleEqual != 0 && l == r {
		return l, MergeEqual
	}
	lc, rc := s.fc.rn(l), s.fc.rn(r)
	if rules&RuleUnderscore != 0 {
		if lc == '_' && cset(`|/\[]{}()<>`, rc) {
			return r, MergeUnderscore
		}
		if rc == '_' && cset(`|/\[]{}()<>`, lc) {
			return l, MergeUnderscore
		}
	}
	if rules&RuleHierarchy != 0 {
		for i, class := range hierarchy[:len(hierarchy)-1] {
			later := strings.Join(hierarchy[i+1:], "")
			// figlet compares "|" itself, and finds the other classes with
			// strchr, so a row's end is in them but is not "|".
			in := func(c rune) bool { return cset(class, c) && (i > 0 || c == '|') }
			if in(lc) && cset(later, rc) {
				return r, MergeHierarchy
			}
			if in(rc) && cset(later, lc) {
				return l, MergeHierarchy
			}
		}
	}
	if rules&RulePair != 0 {
		switch string([]rune{lc, rc}) {
		case "[]", "][", "{}", "}{", "()", ")(":
			return s.fc.bar, MergePair
		}
	}
	if rules&RuleBigX != 0 {
		switch {
		case lc == '/' && rc == '\\':
			return s.fc.bar, MergeBigX
		case lc == '\\' && rc == '/':
			return s.fc.y, MergeBigX
		case lc == '>' && rc == '<':
			return s.fc.x, MergeBigX
		}
	}
	return subNone, 0
}

// vsmush merges u, over l, as the vertical rules of mode say, returning how,
// or subNone when they cannot be merged.
func (fc *face) vsmush(u, l sub, mode int) (sub, Merge) {
	if blank(u) {
		return l, 0
	}
	if blank(l) {
		return u, 0
	}
	if u == subCont || l == subCont {
		return subNone, 0
	}
	if mode&vRules == 0 {
		if l == subHard {
			return u, MergeUniversal
		}
		return l, MergeUniversal
	}
	if u == subHard || l == subHard {
		return subNone, 0
	}
	rules := Rules(mode) //nolint:gosec // the rule bits are the low 15.
	uc, lc := fc.rn(u), fc.rn(l)
	switch {
	case rules&RuleVEqual != 0 && u == l:
		return u, MergeEqual
	case rules&RuleVUnderscore != 0 && uc == '_' && has(`|/\[]{}()<>`, lc):
		return l, MergeUnderscore
	case rules&RuleVUnderscore != 0 && lc == '_' && has(`|/\[]{}()<>`, uc):
		return u, MergeUnderscore
	}
	if rules&RuleVHierarchy != 0 {
		for i, class := range hierarchy[:len(hierarchy)-1] {
			later := strings.Join(hierarchy[i+1:], "")
			if has(class, uc) && has(later, lc) {
				return l, MergeHierarchy
			}
			if has(class, lc) && has(later, uc) {
				return u, MergeHierarchy
			}
		}
	}
	switch {
	case rules&RuleVLine != 0 && (uc == '-' && lc == '_' || uc == '_' && lc == '-'):
		return fc.eq, MergeLine
	case rules&RuleVSuper != 0 && uc == '|' && lc == '|':
		return u, MergeSuper
	}
	return subNone, 0
}

// has reports whether c is in set.
func has(set string, c rune) bool { return c > 0 && strings.ContainsRune(set, c) }

// blank reports whether s shows nothing and takes no part in smushing: a
// space, or the end of a row.
func blank(s sub) bool { return s == subSpace || s == subNone }
