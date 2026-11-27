// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anim

import "sync/atomic"

// MotionLevel is how much a program animates. See [SetMotion].
type MotionLevel uint8

const (
	// Full is all motion, the default.
	Full MotionLevel = iota
	// Reduced is motion without movement, for those who find it
	// uncomfortable: values still animate, so things still fade, but
	// layout snaps rather than gliding, and effects that move cells, such
	// as [fx.Fly] and [fx.Slide], fade instead.
	//
	// [fx.Fly]: https://pkg.go.dev/bloomfx.dev/bloom/fx#Fly
	// [fx.Slide]: https://pkg.go.dev/bloomfx.dev/bloom/fx#Slide
	Reduced
	// Off is no motion: animations jump to where they are going.
	Off
)

var level atomic.Uint32 // a MotionLevel

// SetMotion sets how much the whole program animates. It is [Full] by
// default. With it [Off], animations jump to where they are going. A
// [Sequence] still waits out its [Hold] steps when played, as those are
// about time rather than motion, but a looping one settles where its steps
// end and stops.
//
// Animations already running follow it from their next frame. A loop stopped
// by turning motion off stays stopped until [Sequence.Loop] is called again.
func SetMotion(l MotionLevel) { level.Store(uint32(min(l, Off))) }

// Motion reports how much the program animates. See [SetMotion].
func Motion() MotionLevel { return MotionLevel(level.Load()) } //nolint:gosec // only SetMotion stores it.

// Animates reports whether animations run, at any level but [Off].
func Animates() bool { return Motion() != Off }
