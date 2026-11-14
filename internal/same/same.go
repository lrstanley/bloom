// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package same tells whether the values a frame was drawn from are the same
// as the last frame's, so that what was worked out from them can be kept.
package same

import (
	"bytes"
	"reflect"
	"unsafe"

	"bloomfx.dev/bloom/fx"
	"charm.land/lipgloss/v2"
)

// Style reports whether a and b render the same. Styles cannot be compared
// with ==, so they are compared deeply, but first by their bytes, which is
// far cheaper and matches the usual case of styles copied from the same one
// each frame. A style with a transform function never matches, as the
// function may give something else each time, and is rendered every time.
func Style(a, b *lipgloss.Style) bool {
	if a.GetTransform() != nil || b.GetTransform() != nil {
		return false
	}
	size := unsafe.Sizeof(*a)
	if bytes.Equal(unsafe.Slice((*byte)(unsafe.Pointer(a)), size), unsafe.Slice((*byte)(unsafe.Pointer(b)), size)) {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// Effect reports whether a and b are equal, treating effects that cannot be
// compared, like an [fx.Func], as different.
func Effect(a, b fx.Effect) bool {
	if a == nil || b == nil {
		return a == b
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	return va.Type() == vb.Type() && Fixed(va) && a == b
}

// Fixed reports whether v is comparable and cannot change without changing
// v itself, so that it can be compared with == or be a map key. A value
// behind a pointer can change without the pointer changing, including one
// wrapped by another value, such as an effect wrapped by [fx.Delay]. The
// zero Value, of a nil interface, is fixed.
func Fixed(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	if !v.Comparable() {
		return false
	}
	switch v.Kind() { //nolint:exhaustive // Other kinds are plain values.
	case reflect.Pointer, reflect.UnsafePointer, reflect.Chan, reflect.Func, reflect.Map, reflect.Slice:
		return false
	case reflect.Interface:
		return v.IsNil() || Fixed(v.Elem())
	case reflect.Struct:
		for _, f := range v.Fields() {
			if !Fixed(f) {
				return false
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			if !Fixed(v.Index(i)) {
				return false
			}
		}
	}
	return true
}
