// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
	"strings"
)

// reader is a plain Bubble Tea component, placed with bloom.View.
type reader struct{ page int }

func (r reader) View() string {
	body := strings.Repeat("Lorem ipsum dolor sit amet. ", 3)
	return fmt.Sprintf("# %s\n\n%s", pages[r.page], body)
}
