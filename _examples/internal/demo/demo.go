// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package demo holds what the examples' tests share: the steep options for
// a tour that is recorded when exporting is turned on, as with "make demos"
// from the repository root. Set STEEP_EXPORT_DIR to record somewhere else:
//
//	STEEP_EXPORT_DIR=/tmp/previews STEEP_EXPORT_MODE=on go test -count 1 ./...
//
// Set DEMO_FORMATS to a comma-separated list of extensions, such as
// "gif,mp4", to record those as well as the .webp the docs pages load.
package demo

import (
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/render"
	"github.com/charmbracelet/x/ansi"
)

// Options returns the steep options for a w×h true color tour, recorded to
// name.webp in docs/public/demos, where the docs pages load it from, when
// exporting is turned on. The extra effects are drawn after the others, as
// [effects.Mouse] must be.
func Options(name string, w, h int, extra ...reel.Effect) []steep.Option {
	return []steep.Option{
		steep.WithWindowSize(w, h),
		// Relative to the test's package directory, _examples/<example>.
		steep.WithArtifacts("../../docs/public/demos"),
		steep.WithTimeout(20 * time.Second),
		steep.WithColor(steep.ColorTrueColor),
		steep.WithRecording(reel.Profile{
			Outputs: reel.MustFiles(outputs(name)...),
			Render: []render.Option{
				render.WithWindowChrome(true),
				render.WithPadding(10),
				render.WithMargin(24, lipgloss.Color("#16161e")),
				render.WithBorderRadius(10),
			},
			Effects: append([]reel.Effect{effects.Keys(time.Second), effects.Pause()}, extra...),
		}),
	}
}

// ResizeInBand lets h.Resize reach the program: the harness has no
// pseudo-terminal to signal a resize with, so it is reported in band.
func ResizeInBand(t *testing.T, h *steep.Harness) {
	t.Helper()
	if _, err := h.Session().Write([]byte(ansi.SetModeInBandResize)); err != nil {
		t.Fatal(err)
	}
}

// outputs returns the files a tour named name is recorded to: name.webp, and
// one for each extension in DEMO_FORMATS.
func outputs(name string) []string {
	out := []string{name + ".webp"}
	for ext := range strings.SplitSeq(os.Getenv("DEMO_FORMATS"), ",") {
		if ext = strings.TrimPrefix(strings.TrimSpace(ext), "."); ext != "" && ext != "webp" {
			out = append(out, name+"."+ext)
		}
	}
	return out
}
