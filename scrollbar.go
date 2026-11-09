// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lrstanley/bloom/on"
)

// ScrollbarNode is a scrollbar, one column wide, or one row tall when
// horizontal, that shows which part of some content is in view. Build one
// with [Scrollbar] or [ScrollbarOf].
//
// It is drawn only while the content is bigger than its view, so given to
// [BoxNode.Edge] on a border, it shows over the border while there is more
// to scroll to, and takes no space. Its natural length is zero, so place it
// with [Stretch] there, or in a [Row], or a [Column] when horizontal, which
// stretches it across.
type ScrollbarNode struct {
	total, visible, offset int
	of                     any
	horizontal             bool
	onScroll               func(offset int) tea.Cmd

	thumb, track           string
	thumbStyle, trackStyle lipgloss.Style
}

// Scrollbar returns a scrollbar for content of total rows, visible of them
// in view from row offset, as a Bubbles viewport reports with
// TotalLineCount, VisibleLineCount, and YOffset. When horizontal, they are
// columns.
func Scrollbar(total, visible, offset int) ScrollbarNode {
	return ScrollbarNode{total: total, visible: visible, offset: offset}
}

// ScrollbarOf returns a scrollbar for the [Scroll] that the node with ID id
// is in, or for the [List] with ID id, in items, which follows it from its
// layout, with no state of its own. It is not drawn while there is no such
// node on screen.
func ScrollbarOf[K comparable](id K) ScrollbarNode {
	return ScrollbarNode{of: id}
}

// Horizontal makes the scrollbar one row tall, for content scrolled sideways,
// such as with [ScrollNode.X], and for a border's top or bottom.
func (n ScrollbarNode) Horizontal() ScrollbarNode { n.horizontal = true; return n }

// OnScroll runs fn with the offset to scroll to when the scrollbar is used
// with the mouse, so it needs no state in the model beyond the offset
// itself. Pressing on the thumb and moving drags it; pressing on the rest of
// the scrollbar jumps so the thumb is centered on the pointer, and holding
// the button down drags it from there.
func (n ScrollbarNode) OnScroll(fn func(offset int) tea.Cmd) ScrollbarNode {
	n.onScroll = fn
	return n
}

// Thumb sets the glyph and style of the part of the scrollbar for what is in
// view. The default glyph is "┃", or "━" when horizontal, with no style.
func (n ScrollbarNode) Thumb(glyph string, style lipgloss.Style) ScrollbarNode {
	n.thumb, n.thumbStyle = glyph, style
	return n
}

// Track sets the glyph and style of the rest of the scrollbar. The default
// is no glyph, which draws nothing there, leaving a border beneath it shown.
func (n ScrollbarNode) Track(glyph string, style lipgloss.Style) ScrollbarNode {
	n.track, n.trackStyle = glyph, style
	return n
}

// View implements [Child], drawing the node at its natural size.
func (n ScrollbarNode) View() string { return Render(n, 0, 0) }

// thumbSpan returns the cells of a track length cells long that the thumb
// covers, or an empty span when all of the content is in view.
func (n ScrollbarNode) thumbSpan(length int) (start, end int) {
	scrollable := n.total - n.visible
	if length <= 0 || n.visible <= 0 || scrollable <= 0 {
		return 0, 0
	}
	// While there is more to scroll to, the thumb leaves a cell to move in,
	// unless the track has only the one.
	size := min(max((length*n.visible+n.total/2)/n.total, 1), max(length-1, 1))
	offset := min(max(n.offset, 0), scrollable)
	start = ((length-size)*offset + scrollable/2) / scrollable
	return start, start + size
}

// offsetAt returns the offset that puts the start of a thumb size cells long
// at cell at of a track length cells long, the inverse of thumbSpan.
func (n ScrollbarNode) offsetAt(at, size, length int) int {
	room, scrollable := length-size, n.total-n.visible
	if room <= 0 || scrollable <= 0 {
		return n.offset
	}
	return (min(max(at, 0), room)*scrollable + room/2) / room
}

// moved returns the offset after the thumb, size cells long on a track
// length cells long, moves by cells from offset from. It is measured from
// where the drag started, so a drag that comes back to where it started
// gives back the offset it started from, which a cell of the track may not
// be able to show exactly.
func (n ScrollbarNode) moved(from, cells, size, length int) int {
	room, scrollable := length-size, n.total-n.visible
	if room <= 0 || scrollable <= 0 {
		return from
	}
	step := cells * scrollable
	if step < 0 {
		step -= room / 2
	} else {
		step += room / 2
	}
	return min(max(from+step/room, 0), scrollable)
}

// scrollbarDrag is the drag of a scrollbar going on: where the pointer
// pressed, and the offset it moves from, after any jump.
type scrollbarDrag struct {
	at, offset int
}

// scrollbarOf fills in n's total, visible, and offset from the layout of the
// list with n's ID, or of the [Scroll] around the node with it, reporting
// false if there is none on screen.
func (e *engine) scrollbarOf(n ScrollbarNode) (ScrollbarNode, bool) {
	i := e.shown(n.of)
	if i < 0 {
		return n, false
	}
	if _, ok := e.boxes[i].child.(ListNode); ok {
		v := e.boxes[i].list
		n.total, n.visible, n.offset = v.Count, v.Visible, v.First
		return n, true
	}
	// The scroll's child, which is laid out whole.
	for k, p := i, e.boxes[i].parent; p >= 0; k, p = p, e.boxes[p].parent {
		if _, ok := e.boxes[p].child.(ScrollNode); ok {
			view, content := e.boxes[p].rect, e.boxes[k].rect
			if n.horizontal {
				n.total, n.visible, n.offset = content.Dx(), view.Dx(), view.Min.X-content.Min.X
			} else {
				n.total, n.visible, n.offset = content.Dy(), view.Dy(), view.Min.Y-content.Min.Y
			}
			return n, true
		}
	}
	return n, false
}

// paintScrollbar adds the ops for scrollbar n, box b at index i, clipped to
// clip, and the entry its mouse handler needs, under entry parent.
func (r *renderer) paintScrollbar(i int, b *box, n ScrollbarNode, clip image.Rectangle, parent int) {
	if n.of != nil {
		var ok bool
		if n, ok = r.scrollbarOf(n); !ok {
			return
		}
	}
	// One column at the left of the box, or one row at its top, which a row
	// or column may give more room.
	rect, length := b.rect, b.rect.Dy()
	if n.horizontal {
		rect.Max.Y, length = min(rect.Max.Y, rect.Min.Y+1), rect.Dx()
	} else {
		rect.Max.X = min(rect.Max.X, rect.Min.X+1)
	}
	start, end := n.thumbSpan(length)
	if start == end {
		return
	}
	if n.onScroll != nil {
		b.meta.handlers = append(b.meta.handlers[:0:0], r.scrollbarDrag(n, length, start, end))
		r.addEntry(b, rect.Intersect(clip), clip, parent)
	}

	thumb, glyph := image.Rect(rect.Min.X, rect.Min.Y+start, rect.Max.X, rect.Min.Y+end), n.thumb
	if n.horizontal {
		thumb = image.Rect(rect.Min.X+start, rect.Min.Y, rect.Min.X+end, rect.Max.Y)
	}
	if glyph == "" {
		glyph = "┃"
		if n.horizontal {
			glyph = "━"
		}
	}
	if n.track != "" {
		r.blit(r.column(2*i, n.track, n.trackStyle, length, n.horizontal), rect, clip)
	}
	r.blit(r.column(2*i+1, glyph, n.thumbStyle, end-start, n.horizontal), thumb, clip)
}

// scrollbarDrag returns the drag handler of scrollbar n, whose track is
// length cells long with the thumb over cells start to end.
func (r *renderer) scrollbarDrag(n ScrollbarNode, length, start, end int) on.Handler {
	// Only what the handler needs is kept, not n, whose styles would make the
	// handler, made for each frame, many times bigger.
	fn, horizontal, size := n.onScroll, n.horizontal, end-start
	total, visible, offset := n.total, n.visible, n.offset
	return on.Drag(func(e on.Event) tea.Cmd {
		at := e.Y
		if horizontal {
			at = e.X
		}
		sb := ScrollbarNode{total: total, visible: visible, offset: offset}
		next := offset
		switch e.Msg.(type) {
		case tea.MouseClickMsg:
			// The drag moves from where it pressed, kept for the rest of the
			// drag, for which this handler is made again with each frame. A
			// press beside the thumb jumps there first.
			if at < start || at >= end {
				next = sb.offsetAt(at-size/2, size, length)
			}
			r.drag = scrollbarDrag{at: at, offset: next}
		case nil:
			return nil // the scrollbar left the frame
		default:
			next = sb.moved(r.drag.offset, at-r.drag.at, size, length)
		}
		if next == offset {
			return nil
		}
		return fn(next)
	})
}

// columnMemo is a scrollbar's track or thumb, as last rendered for a box.
type columnMemo struct {
	glyph      string
	style      lipgloss.Style
	n          int
	horizontal bool
	out        string
	used       uint64
}

// column returns glyph, styled, repeated n times down, or across when
// horizontal, kept at index i of the renderer's columns from frame to frame.
func (r *renderer) column(i int, glyph string, style lipgloss.Style, n int, horizontal bool) string {
	if !r.parse {
		return ""
	}
	if i >= len(r.columns) {
		r.columns = append(r.columns, make([]columnMemo, i+1-len(r.columns))...)
	}
	m := &r.columns[i]
	r.style = style
	if m.out != "" && m.glyph == glyph && m.n == n && m.horizontal == horizontal && sameStyle(&m.style, &r.style) {
		m.used = r.frames
		return m.out
	}
	var out string
	if horizontal {
		out = style.Render(strings.Repeat(glyph, n))
	} else {
		out = strings.TrimSuffix(strings.Repeat(style.Render(glyph)+"\n", n), "\n")
	}
	*m = columnMemo{glyph: glyph, style: style, n: n, horizontal: horizontal, out: out, used: r.frames}
	return out
}
