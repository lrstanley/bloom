// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fig

import "image"

// Layer exposes a glyph's layer to the external tests.
func (f *Figure) Layer(i int, p Paint, rest bool) (string, image.Point) { return f.layer(i, p, rest) }

// Order exposes the order layers are drawn in to the external tests.
func (f *Figure) Order() []int { return f.order() }
