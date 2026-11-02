// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package bloom

import (
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lrstanley/bloom/fx"
)

// Debug lays out c in a w×h frame, as [Solve] does, and returns its tree of
// nodes as text, one node per line: what it is, how it is sized, its ID,
// where it was laid out as x,y and width x height, and anything else that
// affects it. A node its container collapsed, or a layer whose anchor is not
// on screen, is marked hidden; one partly on screen is marked with the part
// that is, and one not on screen at all is marked offscreen. Components are
// shown by their type, and those that build their own layout, such as a
// [Layouter], with the tree they built.
//
// Debug is meant for finding out why a layout looks the way it does, such as
// by writing it to a log from View. Its format may change between versions.
//
//	Column 0,0 30x5
//	├─ Row 0,0 30x3
//	│  ├─ Fixed(10) id="sidebar" 0,0 10x3 collapsible=1
//	│  │  └─ Box 0,0 10x3
//	│  │     └─ Text "Inbox" 1,1 8x1
//	│  └─ Fill id="reader" 10,0 20x3 min=12
//	│     └─ main.reader 10,0 20x3
//	└─ Text "ready" 0,3 30x1 truncate
func Debug(c Child, w, h int) string {
	r, _ := renderers.Get().(*renderer)
	defer renderers.Put(r)
	root := r.layout(c, w, h)
	fw, fh := r.size(root, w, h)
	r.cache.EndFrame(fw * fh)
	if root < 0 {
		return ""
	}
	var sb strings.Builder
	r.debugTree(&sb, root, r.area, "", "", false)
	return sb.String()
}

// debugTree writes box i and the boxes in it, clipped to clip. lead is drawn
// before the box, and indent before the boxes in it. hidden is set inside a
// hidden box, as boxes in one are not laid out.
func (e *engine) debugTree(sb *strings.Builder, i int, clip image.Rectangle, lead, indent string, hidden bool) {
	b := &e.boxes[i]
	kind, attrs := debugNode(b.child)
	sb.WriteString(lead + kind)
	if b.meta.id != nil {
		sb.WriteString(" id=" + debugID(b.meta.id))
	}

	hidden = hidden || b.hidden
	if hidden {
		sb.WriteString(" hidden")
	} else {
		sb.WriteString(" " + debugRect(b.rect))
		vis := b.rect.Intersect(clip)
		switch {
		case b.rect.Empty():
		case vis.Empty():
			sb.WriteString(" offscreen")
		case vis != b.rect:
			sb.WriteString(" visible=" + debugRect(vis))
		}
		clip = vis
	}

	if b.cramped {
		attrs = append(attrs, "cramped")
	}
	if b.modal {
		attrs = append(attrs, "modal")
	}
	if b.meta.focusable {
		attrs = append(attrs, "focusable")
	}
	if b.meta.scope {
		attrs = append(attrs, "scope")
	}
	if b.meta.cursor != nil {
		attrs = append(attrs, "cursor")
	}
	for _, a := range attrs {
		sb.WriteString(" " + a)
	}
	sb.WriteByte('\n')

	kids := e.kidsOf(i)
	if b.expanded >= 0 {
		kids = append(slices.Clip(kids), b.expanded)
	}
	for j, k := range kids {
		if j == len(kids)-1 {
			e.debugTree(sb, k, clip, indent+"└─ ", indent+"   ", hidden)
		} else {
			e.debugTree(sb, k, clip, indent+"├─ ", indent+"│  ", hidden)
		}
	}
}

// debugNode returns what c is, and the settings of it that are not their
// defaults.
func debugNode(c Child) (kind string, attrs []string) {
	switch n := c.(type) {
	case TextNode:
		if n.styled {
			attrs = append(attrs, "styled")
		}
		if n.truncate {
			attrs = append(attrs, "truncate")
		}
		return "Text " + debugText(n.s), attrs
	case SlotNode:
		return debugSlot(n)
	case FlexNode:
		kind = "Row"
		if n.vertical {
			kind = "Column"
		}
		if n.gap > 0 {
			attrs = append(attrs, fmt.Sprintf("gap=%d", n.gap))
		}
		if n.justify != 0 {
			attrs = append(attrs, "justify="+debugName(justifyNames, n.justify))
		}
		if n.align != Stretch {
			attrs = append(attrs, "align="+debugName(alignNames, n.align))
		}
		return kind, attrs
	case GridNode:
		return fmt.Sprintf("Grid(%d)", n.cols), debugGap(n.gapX, n.gapY)
	case WrapNode:
		return "Wrap", debugGap(n.gapX, n.gapY)
	case BoxNode:
		return "Box", nil
	case wrapNode:
		switch n.kind {
		case wrapScroll:
			return "Scroll", []string{fmt.Sprintf("offset=%d", n.offset)}
		case wrapTransition:
			return "Transition", []string{fmt.Sprintf("t=%.2f", n.t), debugEffects(n.effects)}
		case wrapCenter:
		}
		return "Centered", nil
	case OverlayNode:
		return "Overlay", nil
	case LayerNode:
		return "Layer", debugLayer(n)
	case drawableNode:
		return fmt.Sprintf("Drawable(%T)", n.d), nil
	default:
		return fmt.Sprintf("%T", c), nil
	}
}

func debugSlot(n SlotNode) (kind string, attrs []string) {
	switch {
	case n.sizing == sizeFixed:
		kind = fmt.Sprintf("Fixed(%d)", n.n)
	case n.sizing == sizeFill && n.n == 1:
		kind = "Fill"
	case n.sizing == sizeFill:
		kind = fmt.Sprintf("Grow(%d)", n.n)
	case n.sizing == sizePercent:
		kind = fmt.Sprintf("Percent(%d)", n.n)
	default:
		kind = "Auto"
	}
	if n.min > 0 {
		attrs = append(attrs, fmt.Sprintf("min=%d", n.min))
	}
	if n.max > 0 {
		attrs = append(attrs, fmt.Sprintf("max=%d", n.max))
	}
	if n.collapse > 0 {
		attrs = append(attrs, fmt.Sprintf("collapsible=%d", n.collapse))
	}
	if n.aligned {
		attrs = append(attrs, "align="+debugName(alignNames, n.align))
	}
	if n.animation != nil {
		attrs = append(attrs, "animate")
	}
	if len(n.moves) > 0 {
		attrs = append(attrs, "moveWith="+strings.TrimPrefix(debugEffects(n.moves), "effects="))
	}
	return kind, attrs
}

func debugLayer(n LayerNode) (attrs []string) {
	switch {
	case n.anchor != nil:
		attrs = append(attrs, "anchorTo="+debugID(n.anchor)+"/"+debugName(directionNames, n.side))
	case n.x != Center || n.y != Center:
		attrs = append(attrs, "anchor="+debugName(alignNames, n.x)+","+debugName(alignNames, n.y))
	}
	if n.offsetX != 0 || n.offsetY != 0 {
		attrs = append(attrs, fmt.Sprintf("offset=%d,%d", n.offsetX, n.offsetY))
	}
	if n.margin > 0 {
		attrs = append(attrs, fmt.Sprintf("margin=%d", n.margin))
	}
	if n.w > 0 || n.h > 0 {
		attrs = append(attrs, fmt.Sprintf("size=%dx%d", n.w, n.h))
	}
	if n.t < 1 || len(n.effects) > 0 {
		attrs = append(attrs, fmt.Sprintf("t=%.2f", n.t), debugEffects(n.effects))
	}
	if n.animation != nil {
		attrs = append(attrs, "animate")
	}
	if len(n.moves) > 0 {
		attrs = append(attrs, "moveWith="+strings.TrimPrefix(debugEffects(n.moves), "effects="))
	}
	return attrs
}

var (
	alignNames   = []string{"stretch", "start", "center", "end"}
	justifyNames = []string{"start", "center", "end", "space-between", "space-around", "space-evenly"}
	// The zero value of a layer's side is the default, Bottom.
	directionNames = []string{"bottom", "bottom", "top", "left", "right", "top-left", "top-right", "bottom-left", "bottom-right"}
)

// debugName returns the name of v in names, or its number if it has none.
func debugName[T ~uint8](names []string, v T) string {
	if int(v) < len(names) {
		return names[v]
	}
	return strconv.Itoa(int(v))
}

func debugGap(x, y int) []string {
	if x == 0 && y == 0 {
		return nil
	}
	return []string{fmt.Sprintf("gap=%d,%d", x, y)}
}

func debugEffects(effects []fx.Effect) string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = fmt.Sprintf("%T", e)
	}
	return "effects=" + strings.Join(names, ",")
}

func debugRect(r image.Rectangle) string {
	return fmt.Sprintf("%d,%d %dx%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

// debugID returns id quoted if it is a string, or with its type otherwise,
// since IDs of different types are different IDs, kept to one line.
func debugID(id any) string {
	if s, ok := id.(string); ok {
		return strconv.Quote(s)
	}
	return lineBreaks.Replace(fmt.Sprintf("%T(%v)", id, id))
}

// debugText returns the start of s as one quoted line, without its styles.
func debugText(s string) string {
	s = lineBreaks.Replace(ansi.Strip(s))
	return strconv.Quote(ansi.Truncate(s, 24, "…"))
}
