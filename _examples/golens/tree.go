// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"image"
	"slices"
	"strconv"
	"strings"
	"time"

	"bloomfx.dev/bloom"
	"bloomfx.dev/bloom/anim"
	"bloomfx.dev/bloom/fx"
	"bloomfx.dev/bloom/on"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	itemW    = 26 // the width of a function in the tree
	wireW    = 3  // the width of the wires between columns
	graphPad = 1  // the space around the tree
	sidebarW = 32
	minGraph = 40 // the narrowest the tree gets before the sidebar collapses
)

// colX is where column k of the tree starts.
func colX(k int) int { return graphPad + k*(itemW+wireW) }

// The IDs of nodes, for layout animation and for finding them on screen,
// by the key of a function in the tree, where it is: itemIDs.Sub(key) is the
// function, and wireIDs.Sub(key) the wire to it, from the one above it.
var (
	itemIDs = bloom.NewID("item")
	wireIDs = bloom.NewID("wire")
)

const (
	cameraID   = "camera"
	codeListID = "code"
	codeID     = "code-view"

	// cameraSize is the size of the camera's content, made the same however
	// big the tree is so that the camera only glides, rather than resizing
	// and reflowing the tree on its way.
	cameraSize = 1 << 13
)

// glide is how the functions and wires of the tree move as branches open and
// close.
var glide = []anim.Option{anim.Duration(240 * time.Millisecond), anim.Ease(anim.OutCubic)}

// depth is the column of the selected function.
func (m *model) depth() int { return len(m.path) - 1 }

// level returns the functions in column k: the roots, or what the function
// selected in column k-1 calls.
func (m *model) level(k int) []*fn {
	if k == 0 {
		return m.prog.roots
	}
	return m.nodeAt(k - 1).calls
}

// nodeAt returns the function selected in column k.
func (m *model) nodeAt(k int) *fn { return m.level(k)[m.path[k]] }

// cur returns the selected function.
func (m *model) cur() *fn { return m.nodeAt(m.depth()) }

// onPath reports whether f is selected in a column before k, as a call
// back up the path is recursion.
func (m *model) onPath(f *fn, k int) bool {
	fs := m.prog.roots
	for _, i := range m.path[:min(k, len(m.path))] {
		if fs[i] == f {
			return true
		}
		fs = fs[i].calls
	}
	return false
}

// selectPath selects the function at path, playing the preview's entrance
// when it changed.
func (m *model) selectPath(path []int) tea.Cmd {
	if slices.Equal(path, m.path) || len(path) == 0 {
		return nil
	}
	m.path = path
	m.calleeSel, m.callerSel, m.codeOff = 0, 0, 0
	m.follow()
	m.scrollToCallee()
	return m.reveal.Play(0, 1)
}

func (m *model) moveSibling(delta int) tea.Cmd {
	d := m.depth()
	i := min(max(m.path[d]+delta, 0), len(m.level(d))-1)
	return m.selectPath(append(slices.Clone(m.path[:d]), i))
}

// enter selects the first function the selected one calls.
func (m *model) enter() tea.Cmd {
	if len(m.cur().calls) == 0 {
		return nil
	}
	return m.selectPath(append(slices.Clone(m.path), 0))
}

// back selects the function that called the selected one.
func (m *model) back() tea.Cmd {
	if len(m.path) < 2 {
		return nil
	}
	return m.selectPath(slices.Clone(m.path[:len(m.path)-1]))
}

// jump selects f: up the path if it is on it, or else by the shortest path
// to it from a root.
func (m *model) jump(f *fn) tea.Cmd {
	for k := range m.path {
		if m.nodeAt(k) == f {
			return m.selectPath(slices.Clone(m.path[:k+1]))
		}
	}
	if path := m.prog.pathTo(f); path != nil {
		return m.selectPath(path)
	}
	return nil
}

// follow scrolls the graph as little as it can to show the selected function
// with its callees, and the column before it when there is room.
func (m *model) follow() {
	if m.prog == nil || len(m.path) == 0 {
		return
	}
	viewW, viewH := m.graphSize()
	d := m.depth()

	right := colX(d) + itemW
	if len(m.cur().calls) > 0 {
		right = colX(d+1) + itemW
	}
	lo, hi := right+graphPad-viewW, colX(d)-graphPad
	if m.camX < lo {
		m.camX = lo
	}
	if prev := colX(max(d-1, 0)) - graphPad; m.camX > prev {
		m.camX = max(prev, lo)
	}
	// Scrolled to the start of a column, rather than showing a sliver of
	// one at the left.
	m.camX = max(min(m.camX, hi), 0)
	if m.camX > 0 {
		const step = itemW + wireW
		m.camX = colX((m.camX - graphPad + step - 1) / step)
	}

	y := graphPad
	for _, i := range m.path {
		y += i
	}
	margin := min(4, viewH/4)
	// The callees open below the selected function: show as many as fit.
	bottom := y + min(len(m.cur().calls), viewH/2)
	if bottom+margin >= m.camY+viewH {
		m.camY = bottom + margin - viewH + 1
	}
	if y-margin < m.camY {
		m.camY = y - margin
	}
	m.camY = max(m.camY, 0)
}

// graphSize is the size of the tree's viewport.
func (m *model) graphSize() (w, h int) {
	w = m.w
	if m.w-sidebarW-1 >= minGraph {
		w = m.w - sidebarW - 1
	}
	return w, m.h - 2
}

// loading is a skeleton of the tree, shimmering while the project loads.
func (m *model) loading() bloom.Child {
	t := m.shimmer.Float()
	wires := m.st.wire.Render(" ─┬─\n  ├─\n  ├─\n  └─")
	return bloom.Centered(bloom.Column(
		bloom.Text("Go Lens").Style(m.st.bright),
		bloom.Text("loading "+m.dir+"…").Style(m.st.muted),
		bloom.Fixed(1, nil),
		bloom.Row(
			bloom.Skeleton(12).Shimmer(t),
			bloom.Text(wires),
			bloom.Skeleton(16, 14, 18, 11).Shimmer(t),
		),
	))
}

// graph is the tree, scrolled to follow the selection: a camera that glides
// from place to place, with the tree gliding inside it as branches open.
//
// Each function and each run of wire is a layer of its own, placed where the
// tree puts it, rather than nested in rows and columns: a node is clipped to
// the one it is in, so a function gliding down to make room for a branch
// would be cut off by a column that had already moved.
func (m *model) graph() bloom.Child {
	var layers []bloom.LayerNode
	m.place(&layers, 0, "r", graphPad)
	space := bloom.Column(bloom.Fixed(cameraSize, bloom.Row(bloom.Fixed(cameraSize, nil))))
	camera := bloom.Auto(bloom.Overlay(space, layers...)).
		ID(cameraID).
		Animate(anim.Duration(300*time.Millisecond), anim.Ease(anim.OutCubic))
	view := bloom.Transition(m.intro.Float(), bloom.Scroll(camera, m.camY).X(m.camX), fx.Wipe{From: bloom.Left})
	return bloom.Column(bloom.Fill(view)).On(on.Scroll(func(_, dy int) tea.Cmd { return m.moveSibling(dy) }))
}

// expanded reports whether function i of column k shows its callees.
func (m *model) expanded(k, i int) bool {
	return k < len(m.path) && m.path[k] == i && len(m.level(k)[i].calls) > 0
}

// place adds the layers of column k, from row y, each function followed by
// its callees when it is on the selected path, and returns the rows the
// column takes. key is where the column is in the tree.
func (m *model) place(layers *[]bloom.LayerNode, k int, key string, y int) int {
	fs := m.level(k)
	// The selected function's callees are a preview, revealed as it is
	// selected.
	preview := k > m.depth()
	add := func(l bloom.LayerNode) {
		l = l.Anchor(bloom.Start, bloom.Start).Animate(glide...)
		if preview {
			l = l.Transition(m.reveal.Float(), fx.Wipe{From: bloom.Left})
		}
		*layers = append(*layers, l)
	}
	sel := -1
	if k > 0 && k < len(m.path) {
		sel = m.path[k]
	}

	top, prev := y, y
	for i, f := range fs {
		id := key + "." + strconv.Itoa(i)
		add(bloom.Layer(m.item(k, i, f)).ID(itemIDs.Sub(id)).Offset(colX(k), y).Size(itemW, 1))
		if k > 0 {
			// The wire to this function, from the one above it.
			from := min(prev+1, y)
			add(bloom.Layer(m.wire(i, len(fs), sel)).ID(wireIDs.Sub(id)).Offset(colX(k)-wireW, from).Size(wireW, y-from+1))
		}
		prev = y
		if m.expanded(k, i) {
			y += m.place(layers, k+1, id, y)
		} else {
			y++
		}
	}
	return y - top
}

// wire is the wire to callee j of n, from the callee above it, with sel the
// callee on the selected path, if any, whose wire is lit up to it.
func (m *model) wire(j, n, sel int) bloom.Child {
	paint := func(s string, hot bool) string {
		if hot {
			return m.st.wireHot.Render(s)
		}
		return m.st.wire.Render(s)
	}
	onPath := sel >= 0 && j <= sel
	if j == 0 {
		mid := "┬"
		if n == 1 {
			mid = "─"
		}
		return bloom.Text(paint("─"+mid, sel >= 0) + paint("─", j == sel))
	}
	end := "├"
	if j == n-1 {
		end = "╰"
	}
	return wireRun{
		run: " " + paint("│", onPath) + " ",
		end: " " + paint(end, onPath) + paint("─", j == sel),
	}
}

// wireRun is a wire down to a callee, drawn for the height it is given, so
// that it ends at its callee as both glide.
type wireRun struct {
	run string
	end string
}

// Layout implements [bloom.Child].
func (w wireRun) Layout(ctx bloom.Context) bloom.Child {
	lines := make([]string, max(ctx.Height(), 1))
	for i := range lines {
		lines[i] = w.run
	}
	lines[len(lines)-1] = w.end
	return bloom.Text(strings.Join(lines, "\n"))
}

// item is function i of column k.
func (m *model) item(k, i int, f *fn) bloom.Child {
	d := m.depth()
	selected := k == d && i == m.path[d]
	box, hover, name := m.st.item, m.st.itemHover, m.st.name
	bar := " "
	switch {
	case selected:
		box, hover, name, bar = m.st.itemSel, m.st.itemSelHover, m.st.nameSel, "▌"
	case k < d && i == m.path[k]:
		box, bar = m.st.itemPath, "▏"
	case k > d:
		box = m.st.itemPreview
	}
	if f.kind == kindExternal && !selected {
		name = m.st.nameExternal
	}
	icon, iconSt := m.st.icon(f)

	var meta []string
	parent := mainPkg
	if k > 0 {
		parent = m.nodeAt(k - 1).pkg
	}
	if f.pkg != parent {
		meta = append(meta, ansi.Truncate(f.pkg, 10, "…"))
	}
	if m.onPath(f, k) {
		meta = append(meta, "↻")
	} else if n := len(f.calls); n > 0 {
		meta = append(meta, "• "+strconv.Itoa(n))
	}

	path := append(slices.Clone(m.path[:min(k, len(m.path))]), i)
	return bloom.Box(box, bloom.Row(
		bloom.Text(bar).Style(m.st.bar),
		bloom.Text(icon+" ").Style(iconSt),
		bloom.Fill(bloom.Text(f.name).Style(name).Truncate()),
		bloom.Text(" "+strings.Join(meta, " ")).Style(m.st.meta),
	)).HoverStyle(hover).On(
		on.Click(func() tea.Cmd { return m.selectPath(path) }),
		on.DoubleClick(m.openCode),
	)
}

// selectedAt is the middle of the selected function in the tree, in the
// last frame, for the code view to open from.
func (m *model) selectedAt() image.Point {
	r, ok := m.ui.Frame().Rect(itemIDs.Sub(m.treeKey(m.depth())))
	if !ok {
		return image.Point{}
	}
	return image.Pt((r.Min.X+r.Max.X)/2, r.Min.Y)
}

// treeKey returns the key of the function selected in column k: where it is in
// the tree.
func (m *model) treeKey(k int) string {
	var b strings.Builder
	b.WriteString("r")
	for _, i := range m.path[:k+1] {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(i))
	}
	return b.String()
}
