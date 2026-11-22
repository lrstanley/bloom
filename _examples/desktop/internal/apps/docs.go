// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package apps

import (
	"slices"
	"strings"
)

// names are the files in the files window, in order.
var names = func() []string {
	n := make([]string, 0, len(files))
	for f := range files {
		n = append(n, f)
	}
	slices.Sort(n)
	return n
}()

// files are the text of each file the files window lists, a line each.
var files = map[string][]string{
	"README.md": {
		"# desktop",
		"",
		"Every window here is a layer of one bloom overlay. Its place and size come from a rectangle in the model, given to the layer with Place, so a window stays wherever it is dropped: the tree is built again from the model on every frame, and nothing else remembers where anything is.",
		"",
		"Drag a window by its title bar to move it. Drag it against the left or right edge of the screen to snap it to that half, or against the top to fill the desktop. Drag the ◢ in its corner to resize it, and double click its title to maximize it.",
		"",
		"A click anywhere on a window brings it to the front, and the order of the layers is the order the windows stack in. The taskbar shows every window: click one to bring it forward, or the one in front to minimize it.",
		"",
		"Click ❀ bloom for the start menu, to open apps or log out. Settings changes the theme.",
		"",
		"The wheel scrolls this text, and the scrollbar on the right edge can be dragged.",
	},
	"todo.txt": {
		"[x] drag windows by their title bars",
		"[x] keep them where they are dropped",
		"[x] resize from the corner",
		"[x] snap to the edges of the screen",
		"[x] minimize to the taskbar",
		"[ ] water the plants",
		"[ ] write a window manager in a terminal",
	},
	"haiku.txt": {
		"windows drift and stack",
		"a title bar, held and moved",
		"the cells remember",
		"",
		"dropped against the edge",
		"half the screen is suddenly",
		"all that it needs",
	},
	"lorem.txt": strings.Split(strings.Repeat(
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.\n", 24), "\n"),
}
