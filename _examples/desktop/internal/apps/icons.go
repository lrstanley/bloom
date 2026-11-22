// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package apps

import (
	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// Icons are the desktop's icons, one for each app: a click selects one, and
// a double click launches its app.
type Icons struct {
	st       theme.Styles
	selected App // the icon selected, or -1 for none
}

// NewIcons returns the desktop's icons, none of them selected.
func NewIcons() *Icons { return &Icons{selected: -1} }

// SetStyles sets the styles the icons are drawn with. They keep a copy of
// their own, which a [theme.ChangeMsg] in Update replaces.
func (ic *Icons) SetStyles(st theme.Styles) *Icons {
	ic.st = st
	return ic
}

// Update takes the styles of a new theme.
func (ic *Icons) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		ic.st = *msg.Styles
	}
	return nil
}

// Layer returns the icons as a layer of the desktop, beneath the windows.
func (ic *Icons) Layer() bloom.LayerNode {
	return bloom.Layer(ic).Anchor(bloom.Start, bloom.Start).Offset(1, 1).Transparent()
}

func (ic *Icons) Layout(bloom.Context) bloom.Child {
	items := make([]bloom.Child, len(All))
	for i, a := range All {
		st := ic.st.Icon
		if a == ic.selected {
			st = ic.st.IconOn
		}
		items[i] = bloom.Text(a.Glyph()+"\n"+a.Name()).Style(st).HoverStyle(ic.st.IconHover).On(
			on.Click(func() tea.Cmd { ic.selected = a; return nil }),
			on.DoubleClick(on.Msg(LaunchMsg{App: a})),
		)
	}
	return bloom.Column(items...).Gap(1)
}
