// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package demo holds what the examples' tests share: the steep options for
// a tour that is recorded when exporting is turned on, as with "task demos"
// from the repository root. Set STEEP_EXPORT_DIR to record somewhere else:
//
//	STEEP_EXPORT_DIR=/tmp/previews STEEP_EXPORT_MODE=on go test -count 1 ./...
//
// Set DEMO_FORMATS to a comma-separated list of extensions, such as
// "gif,mp4", to record those as well as the .webp the docs pages load.
package demo

import (
	"image/color"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"getsteep.dev/steep"
	"getsteep.dev/steep/reel"
	"getsteep.dev/steep/reel/effects"
	"getsteep.dev/steep/reel/render"
	"github.com/charmbracelet/x/ansi"
)

// look is the margin a tour's window is framed on: a linear gradient, angle
// degrees clockwise from up, with a drop shadow under the window.
type look struct {
	angle  float64
	colors []color.Color
	shadow color.Color
}

var (
	// plum is how most tours are framed: bloom's plum fading to its plum
	// black, so the window stands out without pulling the eye from it.
	plum = look{
		angle:  160,
		colors: []color.Color{lipgloss.Color("#3a1835"), lipgloss.Color("#120c17")},
		shadow: color.NRGBA{R: 0x06, G: 0x03, B: 0x08, A: 0xd9},
	}

	// looks frames the tours that are shown off the most, the homepage's and
	// the motion pages', on brighter gradients of bloom's petals.
	looks = map[string]look{
		"showcase": {
			angle:  135,
			colors: []color.Color{lipgloss.Color("#ff6f91"), lipgloss.Color("#ff9a8b"), lipgloss.Color("#ffc29b")},
			shadow: color.NRGBA{R: 0x2b, G: 0x13, B: 0x28, A: 0xb3},
		},
		"effects": {
			angle:  145,
			colors: []color.Color{lipgloss.Color("#a3284f"), lipgloss.Color("#2b1328")},
			shadow: color.NRGBA{R: 0x0d, G: 0x0a, B: 0x12, A: 0xd9},
		},
		"transitions": {
			angle:  145,
			colors: []color.Color{lipgloss.Color("#c4625a"), lipgloss.Color("#2b1328")},
			shadow: color.NRGBA{R: 0x0d, G: 0x0a, B: 0x12, A: 0xd9},
		},
	}
)

// Options returns the steep options for a w×h true color tour, recorded to
// name.webp in docs/public/demos, where the docs pages load it from, when
// exporting is turned on. The extra effects are drawn after the others, as
// [effects.Mouse] must be.
func Options(name string, w, h int, extra ...reel.Effect) []steep.Option {
	frame, ok := looks[name]
	if !ok {
		frame = plum
	}

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
				// The margin is as wide as the shadow's offset and blur together, so
				// the image's edge doesn't cut the shadow off.
				render.WithMarginGradient(40, frame.angle, frame.colors...),
				render.WithShadow(0, 12, 28, frame.shadow),
				render.WithBorderRadius(10),
			},
			Effects: append([]reel.Effect{effects.Keys(time.Second), effects.Pause()}, extra...),
		}),
	}
}

// Recording reports whether the tours are being recorded, as "task demos"
// does: STEEP_EXPORT_MODE is on, always, or true. Exporting only on failure
// does not count.
var Recording = sync.OnceValue(func() bool {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("STEEP_EXPORT_MODE"))); v {
	case "on", "always":
		return true
	default:
		on, _ := strconv.ParseBool(v)
		return on
	}
})

// Hold returns how long a tour holds on a screen so that its recording can
// be followed: d when [Recording], and none otherwise, so the tests run
// quickly. Use it only where the hold is for whoever watches the recording,
// such as before the next key press, with a wait guarding the step after.
// Where an animation must finish before an assertion, or before the pointer
// goes to a spot that is still moving, sleep for the full time instead.
func Hold(d time.Duration) time.Duration {
	if Recording() {
		return d
	}
	return 0
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
