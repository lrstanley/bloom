// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import "bloomfx.dev/bloom"

// statusbar is one line, cut to fit.
type statusbar struct{ msg string }

func (s statusbar) Layout(bloom.Context) bloom.Child { return bloom.Text(" " + s.msg).Truncate() }
