// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package same

import (
	"reflect"
	"strings"
	"testing"

	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
)

func TestSameStyle(t *testing.T) {
	t.Parallel()

	a := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	b := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Padding(1)
	c := a.Bold(true)
	upper := a.Transform(strings.ToUpper)
	switch {
	case !Style(&a, &a):
		t.Fatal("a style differs from itself")
	case !Style(&a, &b):
		t.Fatal("equal styles built apart differ")
	case Style(&a, &c):
		t.Fatal("different styles match")
	case Style(&upper, &upper):
		t.Fatal("a style with a transform matches")
	}
}

func TestEffect(t *testing.T) {
	t.Parallel()

	dim := fx.Dim{Amount: 0.5}
	pointer := &fx.Dim{Amount: 0.5}
	switch {
	case !Effect(dim, fx.Dim{Amount: 0.5}):
		t.Fatal("equal effects differ")
	case Effect(dim, fx.Dim{Amount: 0.6}):
		t.Fatal("different effects match")
	case Effect(pointer, pointer):
		t.Fatal("an effect behind a pointer matches, though it can change")
	case !Effect(nil, nil) || Effect(dim, nil):
		t.Fatal("nil effects are not told apart")
	}
}

func TestFixed(t *testing.T) {
	t.Parallel()

	type inner struct{ P *int }
	x := 1
	for v, want := range map[any]bool{
		1:                      true,
		[2]int{1, 2}:           true,
		struct{ a, b int }{}:   true,
		inner{}:                false,
		[1]inner{}:             false,
		struct{ e any }{e: 1}:  true,
		struct{ e any }{e: &x}: false,
	} {
		if got := Fixed(reflect.ValueOf(v)); got != want {
			t.Errorf("Fixed(%#v) = %v, want %v", v, got, want)
		}
	}
	if !Fixed(reflect.ValueOf(nil)) {
		t.Error("the zero Value is not fixed")
	}
}
