// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"strconv"
	"sync/atomic"
)

// ID names a node for focus, for lookups such as [Frame.Rect], and for
// anchoring and layout animation. IDs must be unique in a frame: two nodes
// with the same one are not told apart.
//
// Make the IDs of an application or a component with [NewID], which never
// returns the same ID twice, however they are named, and the IDs of their
// parts with [ID.Sub]:
//
//	var (
//		buttonConfirm = bloom.NewID("confirm")
//		dialogConfirm = bloom.NewID("confirm") // not the button's
//	)
//
//	type tabs struct{ id bloom.ID }
//
//	func newTabs() tabs { return tabs{id: bloom.NewID("tabs")} }
//
//	// in Layout, tab i:
//	bloom.Text(label).ID(t.id.Sub(i))
//
// Any string is an ID too, for a small program or a test, where keeping
// them apart is left to you:
//
//	bloom.Fixed(24, m.sidebar).ID("sidebar")
//	m.ui.Click("sidebar")
//
// The empty ID is no ID; [ID.IsZero] reports it.
type ID string

// IsZero reports whether id is the empty ID, the zero value of ID, which
// names no node.
func (id ID) IsZero() bool { return id == "" }

// ids counts the IDs made by NewID.
var ids atomic.Uint64

// NewID returns an ID that no other call returns, whatever its name, shown
// as name followed by a number, such as "select#3". Make it once and keep
// it: in a package's variables for an application's IDs, or in a component
// when it is made, for each instance of it to have its own. A component
// that holds it can be copied.
func NewID(name string) ID {
	return ID(name + "#" + strconv.FormatUint(ids.Add(1), 10))
}

// Sub returns the ID of a part of the node with ID id: a named part, such as
// Sub("title"), or one of many, such as Sub(i) for row i. It is id followed
// by "/" and the part, so two IDs made by [NewID] from names without a "/"
// never share a part. Keeping the parts of one ID apart is left to you:
// Sub(1) and Sub("1") are the same ID, and so are Sub("a/b") and
// Sub("a").Sub("b").
//
// Each call makes a new string. A node drawn every frame can call it in
// Layout, as it costs as much as an ID made of a struct did; keep the result
// to make it only once.
func (id ID) Sub[P string | int](part P) ID {
	if p, ok := any(part).(string); ok {
		return id + "/" + ID(p)
	}
	var n [20]byte
	return id + "/" + ID(strconv.AppendInt(n[:0], int64(any(part).(int)), 10))
}
