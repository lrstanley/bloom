// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fonts_test

import (
	"slices"
	"strings"
	"testing"

	"bloomfx.dev/bloom/fig"
	"bloomfx.dev/bloom/fig/fonts"
)

func TestFonts(t *testing.T) {
	t.Parallel()

	all := fonts.All()
	if len(all) != len(fonts.Names) {
		t.Fatalf("All has %d fonts, Names %d", len(all), len(fonts.Names))
	}
	if !slices.IsSorted(fonts.Names) {
		t.Errorf("Names are not sorted: %v", fonts.Names)
	}
	for i, name := range fonts.Names {
		f, ok := fonts.ByName(name)
		if !ok || f != all[i] {
			t.Errorf("ByName(%q) = %v, %v; want All()[%d]", name, f, ok, i)
			continue
		}
		if err := f.Err(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if f.Name() != name {
			t.Errorf("%s: Name() = %q", name, f.Name())
		}
		info := f.Info()
		if info.Height < 1 || info.Glyphs < 95 {
			t.Errorf("%s: height %d, %d glyphs", name, info.Height, info.Glyphs)
		}
		if w, h := f.Figure("Hello", fig.Options{}).Size(); w == 0 || h != info.Height {
			t.Errorf("%s: \"Hello\" is %dx%d", name, w, h)
		}
		spdx := 0
		for _, c := range info.Comments {
			if strings.HasPrefix(c, "SPDX-License-Identifier: ") {
				spdx++
			}
		}
		if spdx > 1 {
			t.Errorf("%s: %d SPDX lines", name, spdx)
		}
	}
	if _, ok := fonts.ByName("no such font"); ok {
		t.Error("ByName found a font that does not exist")
	}
}

func TestLicensedFontsSayWhich(t *testing.T) {
	t.Parallel()

	want := map[string]string{"standard": "BSD-3-Clause", "pagga": "WTFPL", "ansi-compact": "MIT", "doom": ""}
	for name, license := range want {
		f, _ := fonts.ByName(name)
		got := ""
		for _, c := range f.Info().Comments {
			if l, ok := strings.CutPrefix(c, "SPDX-License-Identifier: "); ok {
				got = l
			}
		}
		if got != license {
			t.Errorf("%s: license %q, want %q", name, got, license)
		}
	}
}

func TestStandardIsFigsDefault(t *testing.T) {
	t.Parallel()

	if fonts.Standard() != fig.Standard() {
		t.Fatal("fonts.Standard is not fig.Standard")
	}
}
