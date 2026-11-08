// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package fonts holds FIGlet and TOIlet fonts for package fig: figlet's
// standard fonts, TOIlet's, and the fonts people use most, found by how often
// public code on GitHub asks for them.
//
// Each font is a function, such as [Standard] or [Doom], and a program only
// carries the fonts whose functions it calls: a font's data is embedded in
// the program by the linker only if its function is used. [ByName] and [All]
// use every function, and so carry every font.
//
// Fonts whose license is known carry an SPDX-License-Identifier line in
// their header comments, which [fig.Font.Info] returns, and are listed in
// LICENSES.md with their licenses. The others state no license. Where each
// font came from is in its function's documentation.
package fonts

//go:generate go run ./internal/gen

import (
	"sync"

	"github.com/lrstanley/bloom/fig"
)

// lazy is a font made on first use, so that only the fonts a program uses are
// linked into it: each font's data is referred to only from its function.
type lazy struct {
	once sync.Once
	font *fig.Font
}

func (l *lazy) get(name, data string) *fig.Font {
	l.once.Do(func() { l.font = fig.Lazy(name, data) })
	return l.font
}
