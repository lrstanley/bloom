// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/fx"
)

type (
	debugPane int
	debugKey  string
)

func TestDebug(t *testing.T) {
	t.Parallel()
	panes := Column(
		Row(
			Fixed(10, Box(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()), Text("Inbox"))).ID("sidebar").Collapsible(1),
			Fill(benchView{"reader"}).ID("reader").Min(12),
		),
		Text("ready").Truncate(),
	)
	for _, tt := range []struct {
		name string
		c    Child
		w, h int
		want string
	}{
		{"panes", panes, 30, 5, `
Column 0,0 30x5
├─ Row 0,0 30x3
│  ├─ Fixed(10) id="sidebar" 0,0 10x3 collapsible=1
│  │  └─ Box 0,0 10x3
│  │     └─ Text "Inbox" 1,1 8x1
│  └─ Fill id="reader" 10,0 20x3 min=12
│     └─ bloom.benchView 10,0 20x3
└─ Text "ready" 0,3 30x1 truncate
`},
		{"collapsed", panes, 15, 5, `
Column 0,0 15x5
├─ Row 0,0 15x1
│  ├─ Fixed(10) id="sidebar" hidden collapsible=1
│  │  └─ Box hidden
│  │     └─ Text "Inbox" hidden
│  └─ Fill id="reader" 0,0 15x1 min=12
│     └─ bloom.benchView 0,0 15x1
└─ Text "ready" 0,1 15x1 truncate
`},
		{"overlay", Overlay(
			Column(
				Fixed(1, Scroll(Column(Text("a").ID(debugPane(1)), Text("b"), Text("c")), 1)),
				Marquee("scrolling", 0),
				Text("in").ID("in").Focusable(),
				Transition(0.5, Text("fading in over the frame beneath"), fx.Fade{}),
			),
			Layer(Text("drop")).AnchorTo("in", Bottom).Modal(),
			Layer(Text("gone")).AnchorTo("missing", Top),
			Layer(Text("toast")).ID(debugKey("a\nb")).Anchor(End, Start).Offset(-1, 0).Margin(1).Size(6, 1),
		), 20, 5, `
Overlay 0,0 20x5
├─ Column 0,0 20x5
│  ├─ Fixed(1) 0,0 20x1
│  │  └─ Scroll 0,0 20x1 offset=1
│  │     └─ Column 0,-1 20x3 visible=0,0 20x1
│  │        ├─ Text "a" id=bloom.debugPane(1) 0,-1 20x1 offscreen
│  │        ├─ Text "b" 0,0 20x1
│  │        └─ Text "c" 0,1 20x1 offscreen
│  ├─ bloom.MarqueeNode 0,1 20x1
│  │  └─ Text "scrolling" 0,1 20x1
│  ├─ Text "in" id="in" 0,2 20x1 focusable
│  └─ Transition 0,3 20x1 t=0.50 effects=fx.Fade
│     └─ Text "fading in over the fram…" 0,3 20x1
├─ Layer 0,3 4x1 anchorTo="in"/bottom modal scope
│  └─ Text "drop" 0,3 4x1
├─ Layer hidden anchorTo="missing"/top
│  └─ Text "gone" hidden
└─ Layer id=bloom.debugKey(a b) 12,1 6x1 anchor=end,start offset=-1,0 margin=1 size=6x1
   └─ Text "toast" 12,1 6x1
`},
		{"nil", nil, 10, 1, "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, want := Debug(tt.w, tt.h, tt.c), strings.TrimPrefix(tt.want, "\n"); got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
