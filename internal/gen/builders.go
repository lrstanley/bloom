// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

// nodes are the exported node types of package bloom, in the order their
// builders are written. Each embeds meta, which the builders set. Noun is
// what a builder's documentation calls the node.
//
//nolint:goconst // each type is named where something is said of it.
var nodes = []node{
	{Type: "TextNode", Noun: "node"},
	{Type: "SlotNode", Noun: "slot"},
	{Type: "FlexNode", Noun: "row or column"},
	{Type: "GridNode", Noun: "grid"},
	{Type: "WrapNode", Noun: "flow"},
	{Type: "BoxNode", Noun: "box"},
	{Type: "CenterNode", Noun: "node"},
	{Type: "ScrollNode", Noun: "scroll"},
	{Type: "TransitionNode", Noun: "transition"},
	{Type: "DrawableNode", Noun: "node"},
	{Type: "OverlayNode", Noun: "overlay"},
	{Type: "LayerNode", Noun: "layer"},
	{Type: "ListNode", Noun: "list"},
	{Type: "ViewportNode", Noun: "viewport"},
	{Type: "MarqueeNode", Noun: "marquee"},
	{Type: "SkeletonNode", Noun: "skeleton"},
	{Type: "MinSizeNode", Noun: "node"},
	{Type: "ScrollbarNode", Noun: "scrollbar"},
	{Type: "TooltipSinkNode", Noun: "sink"},
}

// builders are the builder methods the node types share, in the order they
// are written for each type. Every type has a builder unless Only names the
// types that have it, or Except the types that do not.
//
// Doc is the documentation of the builder, in paragraphs split by blank
// lines, wrapped when written. It is a text/template given the node, so
// {{.Noun}} is what the node is called. Docs replaces it for the types it
// names, for what holds only for them.
var builders = []builder{
	{
		Name:   "ID",
		Params: "[K comparable](id K)",
		Body:   "n.id = id",
		Doc:    "ID sets the ID that focus and [Frame.Rect] refer to the {{.Noun}} by.",
		Docs: map[string]string{
			"SlotNode":      "ID sets the ID that focus, layout animation, and [Frame.Rect] refer to the slot by.",
			"LayerNode":     "ID sets the ID that focus, layout animation, anchoring, and [Frame.Rect] refer to the layer by.",
			"ListNode":      "ID sets the ID that focus, [Frame.Rect], [Frame.List], and [ScrollbarOf] refer to the list by.",
			"ViewportNode":  "ID sets the ID that focus, [Frame.Rect], [Frame.Viewport], and [ScrollbarOf] refer to the viewport by.",
			"ScrollbarNode": "ID sets the ID that focus and [Frame.Rect] refer to the scrollbar by. It is in the frame only while it is drawn.",
		},
	},
	{
		Name:    "On",
		Params:  "(handlers ...on.Handler)",
		Body:    "n.handlers = with(n.handlers, handlers)",
		Imports: []string{"bloomfx.dev/bloom/on"},
		Doc:     "On adds mouse handlers to the {{.Noun}}, made by package [on].",
		Docs: map[string]string{
			"FlexNode": "On adds mouse handlers to the node, made by package [on]. They take events on the gaps between children too.",
			"BoxNode": "On adds mouse handlers to the box, made by package [on]. The box takes events on its frame as well as inside it, " +
				"and the positions handlers are given are relative to its outer corner.",
			"ListNode":      "On adds mouse handlers to the list, made by package [on]. They take events on the gaps between items too, such as the wheel.",
			"ScrollbarNode": "On adds mouse handlers to the scrollbar, made by package [on], beside the one of [ScrollbarNode.OnScroll]. They take events only while it is drawn.",
		},
	},
	{
		Name: "Focusable",
		Body: "n.focusable = true",
		Doc:  "Focusable makes the {{.Noun}} one that [Focus] can move to.",
		Docs: map[string]string{
			"ScrollbarNode": "Focusable makes the scrollbar one that [Focus] can move to while it is drawn.",
		},
	},
	{
		Name: "Selectable",
		Body: "n.selectable = true",
		// A drawable and a scrollbar have no text to select, and a sink's
		// is not known until the frame is drawn.
		Except: []string{"DrawableNode", "ScrollbarNode", "TooltipSinkNode"},
		Doc: "Selectable lets the left button select text in the {{.Noun}} by dragging over it, as a terminal selects text, " +
			"but kept inside the {{.Noun}}. Holding alt as the drag starts selects a rectangle of cells instead. " +
			"The innermost selectable node under the pointer takes the drag, unless a node inside it has an [on.Drag] handler. " +
			"See [Root.Selection] for the text selected.",
		Docs: map[string]string{
			"TextNode": "Selectable lets the left button select text in the node by dragging over it, as a terminal selects text, " +
				"but kept inside the node. Holding alt as the drag starts selects a rectangle of cells instead. " +
				"The innermost selectable node under the pointer takes the drag, unless a node inside it has an [on.Drag] handler. " +
				"See [Root.Selection] for the text selected. Text scrolled out of sight is selected as well.",
			"BoxNode": "Selectable lets the left button select text in the box by dragging over it, as a terminal selects text, " +
				"but kept inside the box. Holding alt as the drag starts selects a rectangle of cells instead. " +
				"The innermost selectable node under the pointer takes the drag, unless a node inside it has an [on.Drag] handler. " +
				"See [Root.Selection] for the text selected. Only what is inside the box's frame is selected.",
			"ListNode": "Selectable lets the left button select text in the list by dragging over it, as a terminal selects text, " +
				"but kept inside the list. Holding alt as the drag starts selects a rectangle of cells instead. " +
				"The innermost selectable node under the pointer takes the drag, unless a node inside it has an [on.Drag] handler. " +
				"See [Root.Selection] for the text selected.\n\n" +
				"A selection is kept in the items it starts and ends in, by their index, so it stays as they scroll out of sight and back, " +
				"and reaches the items between them, which the list does not build: dragging past the list's edge, " +
				"when its [on.Scroll] handler moves its offset, takes the items that scroll into sight. " +
				"The text selected is that of every item it covers, built or not. When the list's items change, " +
				"the selection keeps to the same indexes, and ends when an index it is in is past the count, or the list leaves the frame. " +
				"A rectangle selected with alt held is of the cells in sight: nothing is selected while a corner of it is scrolled away.",
			"ViewportNode": "Selectable lets the left button select text in the viewport by dragging over it, as a terminal selects text, " +
				"but kept inside the viewport. Holding alt as the drag starts selects a rectangle of cells instead, " +
				"and a triple click selects the whole line, on all the rows it wraps to. " +
				"See [Root.Selection] for the text selected, which takes the lines scrolled out of sight as well, " +
				"with the rows of a wrapped line joined again, and without the gutters.\n\n" +
				"The selection is kept on the buffer's text, rather than on the rows drawn, so that it stays on the same text " +
				"as the viewport scrolls and wraps its lines differently, and as lines are appended, and the buffer is edited, " +
				"such as to take out the first lines of a capped log. Setting the buffer's content again, as [Buffer.SetContent] does, clears it.",
		},
	},
	{
		Name:   "Tooltip",
		Params: "(text string, opts ...TipOption)",
		Body:   "n.tip = newTooltip(text, opts)",
		Doc: "Tooltip gives the {{.Noun}} a tooltip showing text, which a [Root] with tooltips on (see [WithTooltips]) " +
			"draws beside it once the pointer has rested on it, or once a key press has moved the focus onto it. " +
			"Without them it does nothing. A node without a tooltip of its own shows that of the nearest node it is inside that has one. " +
			"On a [TextNode.Truncate] node, it shows in place of the full text the node was cut from. " +
			"Empty text gives the {{.Noun}} no tooltip. The text is drawn without the escape sequences that [TextNode.Trusted] keeps.",
	},
	{
		Name: "NoTooltip",
		Body: "n.noTip = true",
		Doc: "NoTooltip keeps the [TextNode.Truncate] nodes inside the {{.Noun}} that are cut to fit from showing their full text " +
			"as a tooltip, such as where the app shows it some other way. A tooltip given with Tooltip, " +
			"to the {{.Noun}} or to a node inside it, still shows, and so does the full text of a node pinned open with TooltipOpen.",
		Docs: map[string]string{
			"TextNode": "NoTooltip keeps the node's full text from showing as its tooltip when it is cut to fit with [TextNode.Truncate], " +
				"such as where the app shows it some other way. A tooltip given with Tooltip still shows, " +
				"and so does the full text while the node is pinned open with TooltipOpen.",
		},
	},
	{
		Name:   "TooltipOpen",
		Params: "(open bool)",
		Body:   "n.tipOpen = open",
		Doc: "TooltipOpen pins the {{.Noun}}'s tooltip open while open is true: a [Root] with tooltips on (see [WithTooltips]) " +
			"shows it at once, without the pointer or the focus, and keys and the pointer do not close it. " +
			"Several can be pinned at once, each placed where it covers none of the others or their nodes, " +
			"and one that fits nowhere is left out of the frame. " +
			"It shows the tooltip given with Tooltip, or for a [TextNode.Truncate] node cut to fit, its full text, " +
			"even inside a node with NoTooltip, as the app asked for it. Text that fits, and a {{.Noun}} without a tooltip, show nothing. " +
			"Use it to show the full text of a row the app has selected, which bloom does not know of.",
	},
	{
		Name:    "HoverStyle",
		Params:  "(style lipgloss.Style)",
		Body:    "n.hover = &style",
		Imports: []string{"charm.land/lipgloss/v2"},
		// The types that draw themselves with a style of their own.
		Only: []string{"TextNode", "BoxNode"},
		Docs: map[string]string{
			"TextNode": "HoverStyle renders the text with style instead of its own while the pointer is over it, " +
				"which needs no state in the model. It should take the same space as the node's style, " +
				"changing only how it looks. See [on.Hover] for when the pointer is followed.",
			"BoxNode": "HoverStyle draws the box's frame with style instead of its own while the pointer is over it. " +
				"It should take the same space as the box's style, changing only how it looks. " +
				"See [on.Hover] for when the pointer is followed.",
		},
	},
	{
		Name:    "Cursor",
		Params:  "(c *tea.Cursor)",
		Body:    "n.cursor = c",
		Imports: []string{"charm.land/bubbletea/v2"},
		Only:    []string{"TextNode"},
		Doc: "Cursor shows the terminal cursor at c, relative to the {{.Noun}}'s top-left cell, " +
			"while that cell is on screen and not covered by a later layer. A terminal has one cursor: " +
			"with [WithFocus], a cursor inside a focusable node whose ID is of the focus's type is shown " +
			"only while the nearest such node, which may be the {{.Noun}} itself, is focused, and is shown " +
			"over one in no such node. Otherwise the cursor drawn last is shown.",
	},
	{
		Name: "Scope",
		Body: "n.scope = true",
		// A modal layer is a scope too; see LayerNode.Modal.
		Only: []string{"SlotNode"},
		Doc: "Scope makes the {{.Noun}} a focus scope: [Focus.Within] reports whether focus is inside it, " +
			"and [Focus.Push] confines focus to it. A {{.Noun}} needs an ID to be a scope.",
	},
}
