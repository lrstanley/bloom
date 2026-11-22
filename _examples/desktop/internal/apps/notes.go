// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package apps

import (
	"strings"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/_examples/desktop/internal/theme"
	tea "charm.land/bubbletea/v2"
)

// Notepad is the notes app: the text of a file, which the wheel scrolls.
type Notepad struct {
	st   theme.Styles
	id   bloom.ID // the viewport's, made for each window's notes
	text *bloom.Buffer
	at   bloom.ViewportPos // where it is scrolled to
}

// NewNotepad returns the notes app, showing file.
func NewNotepad(file string) *Notepad {
	return &Notepad{id: bloom.NewID("notes"), text: bloom.NewBuffer(strings.Join(files[file], "\n"))}
}

// SetStyles sets the styles the notes app is drawn with. It keeps a copy of
// its own, which a [theme.ChangeMsg] in Update replaces.
func (n *Notepad) SetStyles(st theme.Styles) *Notepad {
	n.st = st
	return n
}

// Update takes the styles of a new theme.
func (n *Notepad) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(theme.ChangeMsg); ok {
		n.st = *msg.Styles
	}
	return nil
}

// ScrollID returns the ID of the notes' viewport, for the scrollbar on the
// edge of their window.
func (n *Notepad) ScrollID() bloom.ID { return n.id }

func (n *Notepad) Layout(bloom.Context) bloom.Child {
	return bloom.Viewport(n.text).ID(n.id).At(n.at).
		Wrap(bloom.WrapWords).Style(n.st.Body).
		OnScroll(func(p bloom.ViewportPos) tea.Cmd { n.at = p; return nil })
}
