// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"fmt"
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
	"charm.land/lipgloss/v2"
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

// The IDs of nodes, for layout animation and for finding them on screen.
type (
	itemID string // a function in the tree, by where it is
	wireID string // the wire to a function in the tree, from the one above it
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

func (m *model) View() tea.View {
	v := m.ui.View(m.w, m.h, m.layout())
	v.AltScreen = true
	// The terminal takes the theme's background, where terminals let
	// programs set it.
	v.BackgroundColor = theme.Bg
	// All motion, not only while a button is held, for the hover styles.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// graphSize is the size of the tree's viewport.
func (m *model) graphSize() (w, h int) {
	w = m.w
	if m.w-sidebarW-1 >= minGraph {
		w = m.w - sidebarW - 1
	}
	return w, m.h - 2
}

// codeRows is how many lines of code the code view shows.
func (m *model) codeRows() int { return max(m.h-5, 1) }

func (m *model) layout() bloom.Child {
	switch {
	case m.err != nil:
		return bloom.Centered(bloom.Box(m.st.dialog.Width(min(80, m.w-4)), bloom.Column(
			bloom.Text("couldn't load "+m.dir).Style(m.st.accent),
			bloom.Fixed(1, nil),
			bloom.Text(m.err.Error()).Style(m.st.text),
		)))
	case m.prog == nil:
		return m.loading()
	case len(m.prog.roots) == 0:
		return bloom.Centered(bloom.Text("no functions in " + m.dir).Style(m.st.muted))
	}

	base := bloom.Column(
		bloom.Fixed(1, m.topbar()),
		bloom.Fill(bloom.Row(
			bloom.Fixed(sidebarW, m.sidebar()).Collapsible(1),
			bloom.Fill(m.graph()).Min(minGraph),
		).Gap(1)),
		bloom.Fixed(1, m.footer(
			"↑↓", "move", "←→", "walk", "enter", "code", "/", "search", "q", "quit",
		)),
	)
	// The code view and search are built only while they show.
	code, search := bloom.Child(bloom.Text("")), bloom.Child(bloom.Text(""))
	if m.code.Visible() {
		code = m.codeView()
	}
	if m.search.Visible() {
		search = m.searchView()
	}
	return bloom.Overlay(base,
		bloom.Layer(code).
			ID(codeID).
			Size(m.w, m.h).
			Modal().
			Presence(m.code, fx.Iris{At: m.selectedAt()}, fx.Fade{}),
		bloom.Layer(search).
			Size(min(72, m.w-4), 0).
			Anchor(bloom.Center, bloom.Start).
			Offset(0, max(m.h/6, 1)).
			Modal().
			Backdrop(fx.Dim{Amount: 0.5}).
			Presence(m.search, fx.Fade{}, fx.Slide{From: bloom.Top}).
			On(on.ClickOutside(m.search.Hide)),
	)
}

// selectedAt is the middle of the selected function in the tree, in the
// last frame, for the code view to open from.
func (m *model) selectedAt() image.Point {
	r, ok := m.ui.Frame().Rect(itemID(m.treeKey(m.depth())))
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

// topbar is the path to the selected function, and the project.
func (m *model) topbar() bloom.Child {
	right := m.st.muted.Render(m.prog.module) + "  " + m.st.accent.Render("Go Lens")
	room := m.w - lipgloss.Width(right) - 3
	sep := m.st.faint.Render(" › ")

	var parts []string
	width := 0
	for k := m.depth(); k >= 0; k-- {
		name := m.nodeAt(k).name
		w := ansi.StringWidth(name) + 3
		if width+w > room && len(parts) > 0 {
			parts = append(parts, m.st.faint.Render("…"))
			break
		}
		width += w
		if k == m.depth() {
			parts = append(parts, m.st.bright.Render(name))
		} else {
			parts = append(parts, m.st.muted.Render(name))
		}
	}
	slices.Reverse(parts)
	return bloom.Row(
		bloom.Fill(bloom.Text(" "+strings.Join(parts, sep)).Truncate()),
		bloom.Text(right+" "),
	)
}

// footer shows key hints, given as pairs of keys and what they do.
func (m *model) footer(hints ...string) bloom.Child {
	var b strings.Builder
	b.WriteString(" ")
	for i := 0; i+1 < len(hints); i += 2 {
		if i > 0 {
			b.WriteString(m.st.faint.Render("  ·  "))
		}
		b.WriteString(m.st.text.Render(hints[i]) + " " + m.st.muted.Render(hints[i+1]))
	}
	return bloom.Text(b.String()).Truncate()
}

// sidebar shows the selected function's signature and doc comment.
func (m *model) sidebar() bloom.Child {
	f := m.cur()
	where := f.pkgPath
	if f.local() {
		where = fmt.Sprintf("%s:%d", f.file, f.line)
	}
	doc := f.doc
	switch {
	case f.kind == kindExternal:
		doc = "Declared outside the project, in " + f.pkgPath + "."
	case f.kind == kindInterface && doc == "":
		doc = "An interface method. What it calls are its implementations in the project."
	case doc == "":
		doc = "No doc comment."
	}
	counts := m.st.muted.Render("called by ") + m.st.text.Render(strconv.Itoa(len(f.callers))) +
		m.st.muted.Render("  calls ") + m.st.text.Render(strconv.Itoa(len(f.calls)))

	return bloom.Box(m.st.panel, bloom.Column(
		bloom.Fixed(1, nil),
		bloom.Text(f.pkg).Style(m.st.muted).Truncate(),
		bloom.Fixed(1, nil),
		bloom.Text(m.st.signature(f)).Style(lipgloss.NewStyle()),
		bloom.Fixed(1, nil),
		bloom.Fill(bloom.Text(doc).Style(m.st.muted)),
		bloom.Fixed(1, nil),
		bloom.Text(counts).Truncate(),
		bloom.Text(where).Style(m.st.faint).Truncate(),
		bloom.Fixed(1, nil),
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
		add(bloom.Layer(m.item(k, i, f)).ID(itemID(id)).Offset(colX(k), y).Size(itemW, 1))
		if k > 0 {
			// The wire to this function, from the one above it.
			from := min(prev+1, y)
			add(bloom.Layer(m.wire(i, len(fs), sel)).ID(wireID(id)).Offset(colX(k)-wireW, from).Size(wireW, y-from+1))
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
type wireRun struct{ run, end string }

// View implements [bloom.Child].
func (w wireRun) View() string { return w.end }

// Layout implements [bloom.Layouter].
func (w wireRun) Layout(ctx bloom.Context) bloom.Child {
	lines := make([]string, max(ctx.Height, 1))
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
	box, name := m.st.item, m.st.name
	bar := " "
	switch {
	case selected:
		box, name, bar = m.st.itemSel, m.st.nameSel, "▌"
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
	)).HoverStyle(m.st.itemHover).On(
		on.Click(func() tea.Cmd { return m.selectPath(path) }),
		on.DoubleClick(m.openCode),
	)
}

// codeView shows the selected function's code, between the functions that
// call it and those it calls.
func (m *model) codeView() bloom.Child {
	f := m.cur()
	icon, iconSt := m.st.icon(f)
	where := f.pkgPath
	if f.local() {
		where = fmt.Sprintf("%s:%d", f.file, f.line)
	}
	header := bloom.Row(
		bloom.Text(icon+" ").Style(iconSt),
		bloom.Fill(bloom.Text(m.st.muted.Render(f.pkg+".")+m.st.bright.Render(f.name)).Truncate()),
		bloom.Text(where).Style(m.st.faint),
	)

	var body bloom.Child
	if f.local() {
		gutter := len(strconv.Itoa(f.line+len(f.src))) + 2
		body = bloom.List(len(f.src), func(i int) bloom.Child { return m.codeLine(f, i, gutter) }).
			ID(codeListID).
			Offset(m.codeOff).
			On(on.Scroll(func(_, dy int) tea.Cmd {
				m.codeOff = min(max(m.codeOff+3*dy, 0), max(len(f.src)-m.codeRows()/2, 0))
				return nil
			}))
	} else {
		note := "Declared outside the project, in " + f.pkgPath + ", so there is no source to show."
		if f.kind == kindInterface {
			note = "An interface method, so there is no body to show. Its implementations in the project are on the right."
		}
		body = bloom.Column(
			bloom.Text(m.st.signature(f)).Style(lipgloss.NewStyle()),
			bloom.Fixed(1, nil),
			bloom.Text(note).Style(m.st.muted),
		)
	}
	from := m.swapFrom
	if from == 0 {
		from = bloom.Bottom
	}
	code := bloom.Box(m.st.panel, bloom.Column(
		bloom.Fixed(1, header),
		bloom.Fixed(1, nil),
		bloom.Fill(bloom.Transition(m.swap.Float(), body, fx.Slide{From: from}, fx.Fade{})),
	))

	return bloom.Box(lipgloss.NewStyle().Background(theme.Bg), bloom.Column(
		bloom.Fixed(1, m.topbar()),
		bloom.Fill(bloom.Row(
			bloom.Fixed(26, m.calls("Called by", f.callers, paneCallers, m.callerSel)).Collapsible(1),
			bloom.Fill(code).Min(40),
			bloom.Fixed(30, m.calls("Calls", f.calls, paneCalls, m.calleeSel)).Collapsible(2),
		).Gap(1)),
		bloom.Fixed(1, m.footer(
			"↑↓", "select", "enter", "walk in", "←", "back", "tab", "callers", "esc", "tree", "/", "search",
		)),
	))
}

// calls is a list of functions in the code view: those that call the
// selected one, or those it calls.
func (m *model) calls(title string, fs []*fn, p pane, sel int) bloom.Child {
	focused := m.focus == p
	titleSt := m.st.muted
	if focused {
		titleSt = m.st.bright
	}
	head := titleSt.Render(title) + "  " + m.st.accent.Render(strconv.Itoa(len(fs)))
	list := bloom.List(len(fs), func(i int) bloom.Child {
		f := fs[i]
		st := m.st.row
		switch {
		case i == sel && focused:
			st = m.st.rowSel
		case i == sel:
			st = m.st.rowSelBlur
		}
		icon, iconSt := m.st.icon(f)
		name := f.name
		if f.pkg != m.cur().pkg {
			name = f.pkg + "." + name
		}
		return bloom.Box(st, bloom.Row(
			bloom.Text(icon+" ").Style(iconSt),
			bloom.Fill(bloom.Text(name).Truncate()),
		)).
			HoverStyle(st.Background(bgHover)).
			On(
				on.Click(func() tea.Cmd {
					m.focus = p
					if p == paneCalls {
						m.calleeSel = i
						m.scrollToCallee()
					} else {
						m.callerSel = i
					}
					return nil
				}),
				on.DoubleClick(func() tea.Cmd {
					if p == paneCalls {
						return m.goCallee(i)
					}
					return m.goCaller(f)
				}),
			)
	}).Keep(sel)
	return bloom.Box(m.st.panel.Padding(0), bloom.Column(
		bloom.Fixed(1, nil),
		bloom.Fixed(1, bloom.Text(" "+head).Truncate()),
		bloom.Fixed(1, nil),
		bloom.Fill(list),
	))
}

// codeLine is line i of f's code, with the selected callee's call sites
// marked. A click on any call site walks into what it calls.
func (m *model) codeLine(f *fn, i, gutter int) bloom.Child {
	var marked *fn
	if m.calleeSel < len(f.calls) {
		marked = f.calls[m.calleeSel]
	}
	num := m.st.lineNo
	for _, s := range f.sites[marked] {
		if s.line == i {
			num = m.st.lineNoMarked
			break
		}
	}
	return bloom.Text(num.Render(fmt.Sprintf("%*d  ", gutter-2, f.line+i)) + m.st.line(f, i, marked)).
		Truncate().
		On(on.ClickAt(func(e on.Event) tea.Cmd {
			callee, ok := f.siteAt(i, byteAt(f.src[i], e.X-gutter))
			if !ok {
				return nil
			}
			return m.goCallee(slices.Index(f.calls, callee))
		}))
}

// searchView is the search dialog: a query and the functions matching it.
func (m *model) searchView() bloom.Child {
	rows := min(max(len(m.results), 1), 12)
	w := min(72, m.w-4) // as the layer is
	q := m.query
	input := bloom.Row(
		bloom.Text("/ ").Style(m.st.accent),
		bloom.Fill(bloom.Text(q).Style(m.st.bright).Cursor(tea.NewCursor(ansi.StringWidth(q), 0))),
		bloom.Text(strconv.Itoa(len(m.results))).Style(m.st.muted),
	)
	var list bloom.Child = bloom.Text("no matches").Style(m.st.muted)
	if len(m.results) > 0 {
		list = bloom.List(len(m.results), func(i int) bloom.Child {
			f := m.results[i]
			st := m.st.row.Padding(0)
			if i == m.resultSel {
				st = m.st.rowSel.Padding(0)
			}
			icon, iconSt := m.st.icon(f)
			return bloom.Box(st, bloom.Row(
				bloom.Text(" "+icon+" ").Style(iconSt),
				bloom.Fill(bloom.Text(f.name).Truncate()),
				bloom.Text(" "+f.pkgPath+" ").Style(m.st.muted),
			)).HoverStyle(st.Background(bgHover)).On(on.Click(func() tea.Cmd {
				return tea.Batch(m.search.Hide(), m.jump(f))
			}))
		}).Keep(m.resultSel)
	}
	return bloom.Box(m.st.dialog, bloom.Column(
		bloom.Fixed(1, input),
		bloom.Fixed(1, bloom.Text(strings.Repeat("─", w-m.st.dialog.GetHorizontalFrameSize())).Style(m.st.faint)),
		bloom.Fixed(rows, list),
	))
}

// byteAt returns the byte of s at cell x.
func byteAt(s string, x int) int {
	at := 0
	for i, r := range s {
		if at >= x {
			return i
		}
		at += max(ansi.StringWidth(string(r)), 1)
	}
	return len(s)
}
