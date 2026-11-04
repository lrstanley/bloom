// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package anim

import "sync/atomic"

var motionOff atomic.Bool

// SetMotion turns animation on or off for the whole program. It is on by
// default. With it off, animations jump to where they are going. A [Sequence]
// still waits out its [Hold] steps when played, as those are about time rather
// than motion, but a looping one settles where its steps end and stops.
//
// Animations already running follow it from their next frame. A loop stopped
// by turning motion off stays stopped until [Sequence.Loop] is called again.
func SetMotion(on bool) { motionOff.Store(!on) }

// Motion reports whether animation is on. See [SetMotion].
func Motion() bool { return !motionOff.Load() }
