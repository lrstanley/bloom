// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/fx"
)

// dialog is the dialog that shows an API token once it is created, over
// the rest while it is open.
type dialog struct {
	st   styles
	open bool
}

// Layer returns the dialog, as a modal layer.
func (d *dialog) Layer() bloom.LayerNode {
	// The whole dialog is selectable: a selection there runs across
	// the nodes inside it.
	return bloom.Layer(bloom.Box(d.st.dialog, bloom.Column(
		bloom.Text("API token created").Style(d.st.dialogTitle),
		bloom.Fixed(1, nil),
		secret{value: "blm_live_9Qk2Xv7rLp4TzW8nB3sHc6Yd", head: 9, tail: 4, style: d.st.token},
		bloom.Fixed(1, nil),
		bloom.Text("Drag over the token, then ctrl+c:\nthe whole token is copied. It is\nnot shown again.").Style(d.st.dialogText),
	))).ID("dialog").Selectable().Modal().Backdrop(fx.Dim{Amount: 0.5})
}
