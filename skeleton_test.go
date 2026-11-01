// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSkeleton(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node SkeletonNode
		w, h int
		want string
	}{
		{"natural size", Skeleton(3, 0, 5), 0, 0, "███  \n     \n█████"},
		{"cut to fit", Skeleton(8, 2), 4, 0, "████\n██  "},
		{"negative is blank", Skeleton(-1, 1), 0, 0, " \n█"},
		{"shimmering", Skeleton(2, 2).Shimmer(0.5), 0, 0, "██\n██"},
		{"styled", Skeleton(4).Style(lipgloss.NewStyle().Padding(0, 1)), 4, 1, " ██ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(Render(tt.node, tt.w, tt.h)); got != tt.want {
				t.Fatalf("rendered %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSkeletonColors checks that an unstyled skeleton is drawn in a color
// faded from the terminal's, and a styled one in its style's.
func TestSkeletonColors(t *testing.T) {
	t.Parallel()

	r := New(WithColors(lipgloss.Color("#ffffff"), lipgloss.Color("#000000")))
	if got := r.Render(4, 1, Skeleton(4)); !strings.Contains(got, "38;2;51;51;51m") {
		t.Fatalf("unstyled skeleton was not faded: %q", got)
	}
	styled := Skeleton(4).Style(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")))
	if got := r.Render(4, 1, styled); !strings.Contains(got, "38;2;255;0;0m") {
		t.Fatalf("styled skeleton lost its color: %q", got)
	}
}
