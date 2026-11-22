// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package apps

import (
	"fmt"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
)

// FileList is the files app: a click selects a file, and a double click
// opens it in a notes window of its own.
type FileList struct {
	st       theme.Styles
	selected int // the file selected
}

// NewFileList returns the files app, with the first file selected.
func NewFileList() *FileList { return &FileList{} }

// SetStyles sets the styles the files app is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (f *FileList) SetStyles(st theme.Styles) *FileList {
	f.st = st
	return f
}

// Update takes the styles of a new theme.
func (f *FileList) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		f.st = *msg.Styles
	}
	return nil
}

func (f *FileList) Layout(bloom.Context) bloom.Child {
	rows := make([]bloom.Child, 0, len(names)+2)
	rows = append(rows, bloom.Text(fmt.Sprintf("%d files, double click to open", len(names))).Style(f.st.Faint).Truncate())
	for i, name := range names {
		st := f.st.Row
		if i == f.selected {
			st = f.st.RowOn
		}
		label := fmt.Sprintf("%s %-12s %3d lines", Notes.Glyph(), name, len(files[name]))
		rows = append(rows, bloom.Text(label).Style(st).HoverStyle(f.st.RowHover).Truncate().On(
			on.Click(func() tea.Cmd { f.selected = i; return nil }),
			on.DoubleClick(on.Msg(OpenMsg{File: name})),
		))
	}
	rows = append(rows, bloom.Space())
	return bloom.Column(rows...)
}
