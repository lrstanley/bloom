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

// SettingsPanel is the settings app: a click on a theme changes the
// desktop's colors to it.
type SettingsPanel struct{ st theme.Styles }

// NewSettings returns the settings app.
func NewSettings() *SettingsPanel { return &SettingsPanel{} }

// SetStyles sets the styles the settings app is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (s *SettingsPanel) SetStyles(st theme.Styles) *SettingsPanel {
	s.st = st
	return s
}

// Update takes the styles of a new theme.
func (s *SettingsPanel) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		s.st = *msg.Styles
	}
	return nil
}

func (s *SettingsPanel) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(theme.Tints)+2)
	rows = append(rows, bloom.Text("Theme").Style(s.st.Faint))
	for _, t := range theme.Tints {
		st, mark := s.st.Row, "  "
		if t == s.st.Tint {
			st, mark = s.st.RowOn, "✓ "
		}
		rows = append(rows, bloom.Text(mark+t.DisplayName).Style(st).HoverStyle(s.st.RowHover).Truncate().
			On(on.Click(func() tea.Cmd { return theme.Change(t) })))
	}
	return bloom.Column(append(rows, bloom.Space())...)
}
