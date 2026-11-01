<!-- template:define:options
{
  "nodescription": true
}
-->
![banner](https://github.com/user-attachments/assets/aabf5a70-8c5c-4de0-8dd1-805767dd2290)

<!-- template:begin:header -->
<!-- do not edit anything in this "template" block, its auto-generated -->

<p align="center">
  <a href="https://github.com/lrstanley/bloom/tags">
    <img title="Latest Semver Tag" src="https://img.shields.io/github/v/tag/lrstanley/bloom?style=flat-square">
  </a>
  <a href="https://github.com/lrstanley/bloom/commits/master">
    <img title="Last commit" src="https://img.shields.io/github/last-commit/lrstanley/bloom?style=flat-square">
  </a>




  <a href="https://github.com/lrstanley/bloom/actions?query=workflow%3Atest+event%3Apush">
    <img title="GitHub Workflow Status (test @ master)" src="https://img.shields.io/github/actions/workflow/status/lrstanley/bloom/test.yml?branch=master&label=test&style=flat-square">
  </a>


  <a href="https://codecov.io/gh/lrstanley/bloom">
    <img title="Code Coverage" src="https://img.shields.io/codecov/c/github/lrstanley/bloom/master?style=flat-square">
  </a>

  <a href="https://pkg.go.dev/github.com/lrstanley/bloom">
    <img title="Go Documentation" src="https://pkg.go.dev/badge/github.com/lrstanley/bloom?style=flat-square">
  </a>
  <a href="https://goreportcard.com/report/github.com/lrstanley/bloom">
    <img title="Go Report Card" src="https://goreportcard.com/badge/github.com/lrstanley/bloom?style=flat-square">
  </a>
</p>
<p align="center">
  <a href="https://github.com/lrstanley/bloom/issues?q=is:open+is:issue+label:bug">
    <img title="Bug reports" src="https://img.shields.io/github/issues/lrstanley/bloom/bug?label=issues&style=flat-square">
  </a>
  <a href="https://github.com/lrstanley/bloom/issues?q=is:open+is:issue+label:enhancement">
    <img title="Feature requests" src="https://img.shields.io/github/issues/lrstanley/bloom/enhancement?label=feature%20requests&style=flat-square">
  </a>
  <a href="https://github.com/lrstanley/bloom/pulls">
    <img title="Open Pull Requests" src="https://img.shields.io/github/issues-pr/lrstanley/bloom?label=prs&style=flat-square">
  </a>
  <a href="https://github.com/lrstanley/bloom/discussions/new?category=q-a">
    <img title="Ask a Question" src="https://img.shields.io/badge/support-ask_a_question!-blue?style=flat-square">
  </a>
  <a href="https://liam.sh/chat"><img src="https://img.shields.io/badge/discord-bytecord-blue.svg?style=flat-square" title="Discord Chat"></a>
</p>
<!-- template:end:header -->

<!-- template:begin:toc -->
<!-- do not edit anything in this "template" block, its auto-generated -->
## :link: Table of Contents

  - [Features](#sparkles-features)
  - [Usage](#gear-usage)
    - [Related libraries](#related-libraries)
  - [Support &amp; Assistance](#raising_hand_man-support--assistance)
  - [Contributing](#handshake-contributing)
  - [License](#balance_scale-license)
<!-- template:end:toc -->

## :sparkles: Features

**bloom** is a layout, compositing, and animation engine for [Bubble Tea](https://github.com/charmbracelet/bubbletea)
programs. Describe your view as a tree of rows, columns, and layers, built from your model every frame, and bloom
sizes it to the terminal, draws dialogs and toasts on top, animates what changes, and tells you what the mouse is
over.

> [!WARNING]
> bloom is still pre-1.0. Its API may change drastically until the v1.0.0 release, so expect breaking changes
> between versions, and pin the version you use.

- :straight_ruler: **Layout without the math**: `Row`, `Column`, `Grid`, and `Wrap`, with `Fixed`, `Fill`, `Grow`,
  `Percent`, and natural-size slots, `Min`/`Max`, gaps, alignment, and justification. No more measuring rendered
  strings and subtracting borders on every resize.
- :iphone: **Responsive by priority**: panels marked `Collapsible` drop out when the terminal is too small, lowest
  priority first, and components can build a different layout for the space they're given.
- :card_index_dividers: **Real layers**: `Overlay` draws dialogs, dropdowns, popovers, and toasts over the screen,
  anchored to a corner or to another node, with backdrops, drop shadows, and modal input.
- :computer_mouse: **Mouse handlers**: clicks, double clicks, the wheel, hover, and drags go to handlers on the
  nodes they're about, `bloom.Text(name).On(on.Click(m.open))`, with hover styles and drag capture built in.
- :keyboard: **Focus**: tab order from the layout, focused styles, the real terminal cursor, and focus scopes that
  keep <kbd>tab</kbd> inside a dialog until it closes.
- :sparkles: **Animation kept in your model**: eased and spring values, presences for showing and hiding (with
  `ShowFor` for toasts that hide themselves), sequences of steps, and automatic layout animation with `Animate()`.
- :art: **25+ cell effects**: `Fade`, `Slide`, `Iris`, `Unfold`, `Decode`, `Glitch`, and more as transitions; `Dim`,
  `Desaturate`, `Tint`, and `Spotlight` as backdrops; `Shake`, `Shimmer`, `Roll`, and `Burst` for feedback; and
  combinators to stagger, delay, reverse, and ease them. Or write your own.
- :hourglass_flowing_sand: **Loading placeholders**: `Skeleton(8, 0, 5, 11)` draws shimmering bars in place of
  content that's still loading, a line for each line of the content, so nothing jumps when it arrives.
- :jigsaw: **Works with what you have**: anything with a `View() string` method, including
  [Bubbles](https://github.com/charmbracelet/bubbles) components, is a leaf in the tree.
- :zap: **Fast**: strings are parsed once, Lip Gloss renders are cached, and unchanged frames aren't composited
  again. A 200×60 dashboard animating a dialog over a dimmed backdrop takes about 0.6ms a frame.

---

## :gear: Usage

Take a look at the [official documentation](https://lrstanley.github.io/bloom/) for tutorials, guides, and
reference, starting with [building your first layout](https://lrstanley.github.io/bloom/getting-started/first-layout).
The documentation is also available for LLMs and agents as [llms.txt](https://lrstanley.github.io/bloom/llms.txt)
and [llms-full.txt](https://lrstanley.github.io/bloom/llms-full.txt), and every page has a markdown version.

```go
func (m *model) View() tea.View {
	v := m.ui.View(m.width, m.height, bloom.Overlay(
		bloom.Column(
			bloom.Fill(bloom.Row(
				bloom.Fixed(24, m.sidebar).Collapsible(1),
				bloom.Fill(m.reader).Min(40),
			)),
			m.statusbar,
		),
		bloom.Layer(m.dialog).
			Modal().
			Backdrop(fx.Dim{Amount: 0.5}).
			Presence(m.dialogAnim, fx.Fade{}, fx.Slide{From: bloom.Bottom}),
		bloom.Layer(m.toast).
			Anchor(bloom.End, bloom.End).
			Presence(m.toastAnim, fx.Slide{From: bloom.Right}),
	))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
```

Runnable programs, from a basic layout to a 120 FPS dashboard, are in [`_examples`](_examples). Run one with
`go run ./forms` from that directory.

### Related libraries

- [bubbletint](https://github.com/lrstanley/bubbletint): hundreds of terminal color themes for Bubble Tea and Lip
  Gloss, which work directly in bloom's styles and effects.
- [steep](https://github.com/lrstanley/x/tree/master/term/steep): a test harness that runs Bubble Tea programs in an
  emulated terminal, used for bloom's own end-to-end tests.

<!-- template:begin:goget -->
<!-- do not edit anything in this "template" block, its auto-generated -->
```console
go get -u github.com/lrstanley/bloom@latest
```
<!-- template:end:goget -->

---

<!-- template:begin:support -->
<!-- do not edit anything in this "template" block, its auto-generated -->
## :raising_hand_man: Support & Assistance

* :heart: Please review the [Code of Conduct](.github/CODE_OF_CONDUCT.md) for
     guidelines on ensuring everyone has the best experience interacting with
     the community.
* :raising_hand_man: Take a look at the [support](.github/SUPPORT.md) document on
     guidelines for tips on how to ask the right questions.
* :lady_beetle: For all features/bugs/issues/questions/etc, [head over here](https://github.com/lrstanley/bloom/issues/new/choose).
<!-- template:end:support -->

<!-- template:begin:contributing -->
<!-- do not edit anything in this "template" block, its auto-generated -->
## :handshake: Contributing

* :heart: Please review the [Code of Conduct](.github/CODE_OF_CONDUCT.md) for guidelines
     on ensuring everyone has the best experience interacting with the
    community.
* :clipboard: Please review the [contributing](.github/CONTRIBUTING.md) doc for submitting
     issues/a guide on submitting pull requests and helping out.
* :old_key: For anything security related, please review this repositories [security policy](https://github.com/lrstanley/bloom/security/policy).
<!-- template:end:contributing -->

<!-- template:begin:license -->
<!-- do not edit anything in this "template" block, its auto-generated -->
## :balance_scale: License

```
MIT License

Copyright (c) 2026 Liam Stanley <liam@liam.sh>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

_Also located [here](LICENSE)_
<!-- template:end:license -->
