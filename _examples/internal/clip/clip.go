// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package clip writes text to the system clipboard, for the examples that
// copy text selected with the mouse, given to bloom.WithClipboard. Bloom
// sends the text to the terminal's clipboard with OSC 52 after it.
package clip

import (
	"errors"
	"os"
	"runtime"

	"github.com/atotto/clipboard"
)

// Native is whether text is written to the system clipboard at all, which
// tests turn off, to leave the clipboard of whoever runs them alone.
var Native = true

// ErrWriteFailed is returned when the system clipboard does not hold the
// text after it was written.
var ErrWriteFailed = errors.New("the clipboard did not take the text")

// Write writes text to the system clipboard, and reads it back to make sure
// it landed, as crush does: a write can fail quietly, and an earlier copy of
// the same text would hide that from the write alone. Where there is no
// system clipboard to reach, such as over SSH, or on Linux without xclip,
// xsel, or wl-copy, it returns nil, leaving the copy to OSC 52.
func Write(text string) error {
	if !Native || !available() {
		return nil
	}
	if err := clipboard.WriteAll(text); err != nil {
		return ErrWriteFailed
	}
	if text == "" {
		// Writing nothing empties the clipboard: nothing to read back.
		return nil
	}
	if got, err := clipboard.ReadAll(); err != nil || got != text {
		return ErrWriteFailed
	}
	return nil
}

// available reports whether there is a system clipboard to write to.
func available() bool {
	if clipboard.Unsupported {
		return false
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	}
	// Elsewhere, the clipboard belongs to a display server, which a session
	// over SSH has none of.
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != ""
}
