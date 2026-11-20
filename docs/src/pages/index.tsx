import type { ReactNode } from "react";
import { Link } from "fumapress/client";
import { asMarkdown, md } from "fumapress/markdown";
import { BrandCode } from "../components/brand-code";
import { CopyInstall } from "../components/copy-install";
import { ExampleGallery, type Example } from "../components/example-gallery";
import { LlmsLinks } from "../components/llms-links";
import { Logo } from "../components/logo";
import { TerminalDemo } from "../components/terminal-demo";
import { withBase } from "../lib/site";

const install = "go get bloomfx.dev/bloom@latest";

const requirements = "Needs Go 1.27+, Bubble Tea v2, and Lip Gloss v2.";

const tagline = "Layout, motion, and effects for Bubble Tea.";

const summary =
    "bloom lays out, layers, animates, and routes input for Bubble Tea programs. Describe your view as a tree of rows, columns, and layers built from your model, and bloom handles the sizing, the dialogs and toasts on top, the effects, and which node the mouse is over.";

const viewCode = `func (m *model) View() tea.View {
	return m.ui.View(m.width, m.height, bloom.Overlay(
		bloom.Column(
			bloom.Fill(bloom.Row(
				bloom.Fixed(24, sidebar).Collapsible(1),
				bloom.Fill(reader).Min(30),
			)),
			statusbar,
		),
		bloom.Layer(dialog).
			Modal().
			Backdrop(fx.Dim{Amount: 0.6}).
			Presence(m.confirm, fx.Iris{}),
		bloom.Layer(toast).
			Anchor(bloom.End, bloom.End).
			Presence(m.toast,
				fx.Slide{From: bloom.Right}, fx.Typewriter{}),
	))
}`;

const showcase = {
    href: "https://github.com/lrstanley/bloom/tree/master/_examples/showcase",
    src: "/demos/showcase.webp",
    alt: "The showcase example: panes waving in with each bubbletint theme, a toast decoding the theme's name, a pane dragged across the others, which glide out of its way, a double click zooming a pane, a tour spotlighting each part, and panes collapsing as the terminal narrows",
    body: "switches between bubbletint themes, waving the panes back in with each one's colors while a toast decodes its name. Drag a pane by its title and the others glide out of its way, double click it to zoom, narrow the terminal to collapse panes by priority, and take a tour that spotlights each part. It's a few hundred lines.",
};

// Smaller recordings under the showcase. Sizes are the recordings' own, from "task docs:demos".
const gallery: Example[] = [
    {
        name: "desktop",
        width: 1200,
        height: 887,
        alt: "The desktop example: windows dragged by their title bars, one snapped to the left edge, a calculator resized from its corner, minimized to the taskbar and restored, and the theme changed from the start menu",
        body: "Windows dragged by their title bars, snapped to an edge, resized from a corner, and minimized to a taskbar.",
    },
    {
        name: "golens",
        width: 1310,
        height: 887,
        alt: "The golens example: walking a Go project's call graph as a tree that opens to the right, and a function's code opening between its callers and callees",
        body: "A call graph explorer for any Go project, as a tree whose branches glide open beside highlighted code.",
    },
    {
        name: "editor",
        width: 1200,
        height: 787,
        alt: "The editor example: typing, selecting words and cutting them, pasting, undo and redo, and wrapping turned off and on, in a million lines",
        body: "A text editor on a million lines, with selection, cut and paste, undo and redo, and wrapping.",
    },
    {
        name: "forms",
        width: 1200,
        height: 787,
        alt: "The forms example: a form with an autocomplete dropdown, then two stacked modal dialogs over a recolored backdrop",
        body: "Focusable inputs with an autocomplete dropdown, then stacked modal dialogs over a recolored backdrop.",
    },
    {
        name: "dashboard",
        width: 1420,
        height: 887,
        alt: "The dashboard example: selecting services, a restart dialog over a dimmed backdrop, toasts, the sidebar springing shut and open, and panels collapsing as the terminal narrows",
        body: "Services, a restart dialog, toasts, and a sidebar that springs shut, drawn at 120 frames a second.",
    },
    {
        name: "logs",
        width: 1310,
        height: 737,
        alt: "The logs example: a quarter of a million log lines followed as they grow, scrolled sideways, searched with matches marked, and a line selected and copied",
        body: "A quarter of a million log lines followed as they grow, searched, scrolled sideways, and copied.",
    },
].map((e) => ({
    ...e,
    src: `/demos/${e.name}.webp`,
    docs: `/resources/examples#${e.name}`,
    source: `https://github.com/lrstanley/bloom/tree/master/_examples/${e.name}`,
}));

// What a Bubble Tea program otherwise does by hand, next to the bloom code that replaces it.
const chores = [
    {
        title: "Splitting the screen into panes",
        without: "Measure rendered strings, subtract borders and padding, and redo the math on every resize.",
        code: "bloom.Row(bloom.Fixed(24, nav), bloom.Fill(main))",
        href: "/layout/slots",
    },
    {
        title: "Drawing a dialog over the screen",
        without: "Strings can be joined side by side or stacked, not layered, so you splice lines by hand.",
        code: "bloom.Overlay(screen, bloom.Layer(dialog).Modal())",
        href: "/layers/dialogs",
    },
    {
        title: "Dimming what's behind it",
        without: "Re-render everything underneath in muted colors, and undo it when the dialog closes.",
        code: ".Backdrop(fx.Dim{Amount: 0.6})",
        href: "/layers/dialogs#dim-whats-beneath",
    },
    {
        title: "Showing a toast that hides itself",
        without: "A timer message, a flag to track it, and a tick loop to slide it in and out.",
        code: "m.toast.ShowFor(2 * time.Second)",
        href: "/layers/toasts",
    },
    {
        title: "Showing data that's still loading",
        without: "Placeholder strings sized to match the content by hand, and a tick loop to shimmer them.",
        code: "bloom.Skeleton(8, 0, 5, 11).Shimmer(t)",
        href: "/motion/feedback-effects#skeleton-loading-placeholders",
    },
    {
        title: "Knowing what was clicked",
        without: "Turn an X and Y back into whichever list item or button you drew there.",
        code: "bloom.Text(note.Title).On(on.Click(m.open))",
        href: "/input/mouse",
    },
    {
        title: "Explaining an icon button",
        without: "Follow the pointer, wait for it to rest, find room beside the button, and clear it on the next key.",
        code: 'bloom.Text("⟳").Tooltip("Refresh")',
        href: "/input/tooltips",
    },
    {
        title: "Scrolling a huge log",
        without: "Slice out the lines in sight, wrap them again on every resize, and keep the same line at the top.",
        code: "bloom.Viewport(m.log).At(m.pos).Follow()",
        href: "/content/viewports",
    },
    {
        title: "Selecting and copying text",
        without: "Turning on the mouse turns off the terminal's own selection, so text can't be copied out.",
        code: "bloom.Viewport(m.log).Selectable()",
        href: "/input/selection",
    },
    {
        title: "Keeping focus inside a dialog",
        without: "Track tab order yourself, and remember where focus was before the dialog opened.",
        code: "m.focus.Push(confirmDialog)",
        href: "/input/focus",
    },
    {
        title: "Animating anything",
        without: "Tick messages, easing math, and a little state machine for every moving part.",
        code: "anim.NewPresence(anim.Spring(9, 0.35))",
        href: "/motion/animating-values",
    },
    {
        title: "Fitting a narrow terminal",
        without: "Decide which panels to drop as the window shrinks, and bring them back as it grows.",
        code: "bloom.Fixed(30, details).Collapsible(1)",
        href: "/layout/responsive",
    },
];

const effects = [
    {
        label: "Bring things in",
        names: ["Fade", "Slide", "Wipe", "Iris", "Unfold", "Dissolve", "Typewriter", "Scramble", "Decode", "Shade", "Blinds", "Wave", "Glitch"],
    },
    { label: "Set them apart", names: ["Dim", "Desaturate", "Tint", "Recolor", "Spotlight"] },
    { label: "Respond to events", names: ["Shake", "Shimmer", "Roll", "Burst"] },
    { label: "Combine them", names: ["Stagger", "Delay", "Reverse", "Ease"] },
];

const paths = [
    {
        title: "New to Bubble Tea",
        body: "Build a small notes app from an empty file: panes, clicks, then a dialog and a toast.",
        links: [
            { text: "Build your first layout", href: "/getting-started/first-layout" },
            { text: "Add a dialog and a toast", href: "/getting-started/dialogs-and-toasts" },
        ],
    },
    {
        title: "Bringing bloom to an existing app",
        body: "Your model and components stay as they are. Anything with a View() string drops into a layout.",
        links: [
            { text: "Add bloom to an existing program", href: "/getting-started/existing-app" },
            { text: "Using Bubbles components", href: "/bubble-tea/bubbles-components" },
            { text: "How layout works", href: "/concepts/how-layout-works" },
        ],
    },
    {
        title: "Looking something up",
        body: "Every effect, node, and option on one page each, and the full API on pkg.go.dev.",
        links: [
            { text: "Effects", href: "/reference/effects" },
            { text: "Nodes and options", href: "/reference/options" },
            { text: "API reference", href: "https://pkg.go.dev/bloomfx.dev/bloom" },
        ],
    },
];

function SmartLink({ href, className, children }: { href: string; className?: string; children: ReactNode }) {
    if (href.startsWith("http")) {
        return (
            <a href={href} className={className} target="_blank" rel="noreferrer">
                {children}
            </a>
        );
    }
    return (
        <Link href={href} className={className}>
            {children}
        </Link>
    );
}

export default function Page() {
    if (asMarkdown()) {
        return md`
# bloom

${tagline}

${summary}

> **Warning**
>
> bloom is pre-1.0. Its API may change in breaking ways until v1.0.0.

\`\`\`console
${install}
\`\`\`

${requirements} See [Installation](/getting-started/installation#requirements).

## A few programs built with bloom

The [showcase example](${showcase.href}) ${showcase.body}

![${showcase.alt}](${showcase.src})

${gallery.map((e) => `- [${e.name}](${e.docs}): ${e.body}`).join("\n")}

See [all the examples](/resources/examples).

## What bloom takes off your hands

${chores.map((c) => `- **${c.title}** ([guide](${c.href})): without bloom, ${c.without.charAt(0).toLowerCase()}${c.without.slice(1)} With bloom: \`${c.code}\``).join("\n")}

## A whole screen in one View

\`\`\`go
${viewCode}
\`\`\`

## Effects

${effects.map((g) => `- ${g.label}: ${g.names.map((n) => `\`fx.${n}\``).join(", ")}`).join("\n")}

See the [effects reference](/reference/effects).

## Where to start

${paths.map((p) => `- **${p.title}**: ${p.body} ${p.links.map((l) => `[${l.text}](${l.href})`).join(", ")}`).join("\n")}

## Links

- [Source](https://github.com/lrstanley/bloom)
- [API reference](https://pkg.go.dev/bloomfx.dev/bloom)
- [Related libraries](/resources/related-libraries), such as bubbletint for color themes
`;
    }

    return (
        <>
            <title>bloom: layout, motion, and effects for Bubble Tea</title>
            <meta name="description" content={summary} />
            <meta property="og:title" content="bloom" />
            <meta property="og:description" content={tagline} />

            <main className="bloom-home">
                <section className="bloom-hero">
                    <div className="bloom-hero-text">
                        <h1 className="bloom-glow">
                            <Logo className="text-[clamp(3.25rem,8vw,5.75rem)] leading-none" markClassName="size-[0.9em]" />
                        </h1>
                        <p className="bloom-tagline">{tagline}</p>
                        <p className="bloom-lede">
                            Describe your{" "}
                            <a href="https://github.com/charmbracelet/bubbletea" target="_blank" rel="noreferrer">
                                Bubble Tea
                            </a>{" "}
                            view as a tree of rows, columns, and layers, built from your model every frame. bloom sizes
                            it to the terminal, draws dialogs and toasts on top, animates what changes, and tells you
                            what the mouse is over.
                        </p>
                        <div className="bloom-actions">
                            <Link href="/getting-started/introduction" className="bloom-button bloom-button-primary">
                                Read the docs
                            </Link>
                            <a href="https://github.com/lrstanley/bloom" className="bloom-button" target="_blank" rel="noreferrer">
                                View on GitHub
                            </a>
                        </div>
                        <CopyInstall text={install} />
                        <p className="bloom-notice">
                            <span className="bloom-chip">Pre-1.0</span>
                            Expect breaking changes until v1.0.0. {requirements}
                        </p>
                    </div>
                    <TerminalDemo />
                </section>

                <section className="bloom-section">
                    <h2>A few programs built with bloom</h2>
                    <p className="bloom-section-lede bloom-section-lede-wide">
                        The{" "}
                        <a href={showcase.href} target="_blank" rel="noreferrer">
                            showcase example
                        </a>{" "}
                        {showcase.body}
                    </p>
                    <img className="bloom-showcase" src={withBase(showcase.src)} alt={showcase.alt} width={1238} height={578} loading="lazy" />
                    <ExampleGallery examples={gallery} />
                    <Link href="/resources/examples" className="bloom-gallery-more">
                        See all the examples <span aria-hidden>→</span>
                    </Link>
                </section>

                <section className="bloom-section">
                    <h2>What bloom takes off your hands</h2>
                    <p className="bloom-section-lede">
                        Bubble Tea gives you a model, messages, and a view that returns a string. Most of what makes a
                        terminal app feel finished happens between those, and it's code you'd otherwise write for every
                        program.
                    </p>
                    <dl className="bloom-chores">
                        {chores.map((c) => (
                            <div key={c.title} className="bloom-chore">
                                <dt>
                                    <SmartLink href={c.href}>{c.title}</SmartLink>
                                </dt>
                                <dd className="bloom-chore-without">{c.without}</dd>
                                <dd className="bloom-chore-with">
                                    <BrandCode code={c.code} />
                                </dd>
                            </div>
                        ))}
                    </dl>
                </section>

                <section className="bloom-section bloom-split">
                    <div>
                        <h2>One view, the whole screen</h2>
                        <p>
                            This is the <code>View</code> behind the terminal above. The layout, the dialog with its
                            dimmed backdrop and iris entrance, and the toast in the corner are all described here, from
                            model state. There's no separate place where layers are registered or animations are kept
                            running.
                        </p>
                        <ul className="bloom-points">
                            <li>
                                <strong>Slots</strong> decide how panes share space: fixed, filling, by weight, or by
                                percentage, with minimums and panels that collapse.
                            </li>
                            <li>
                                <strong>Layers</strong> sit over the screen, anchored to a corner or to another node, with
                                backdrops, shadows, and modal input.
                            </li>
                            <li>
                                <strong>Presences</strong> are animated values in your model. Show one, and the layer
                                plays its effects in; hide it, and they play back out.
                            </li>
                        </ul>
                        <p>
                            Components you already have, from{" "}
                            <a href="https://github.com/charmbracelet/bubbles" target="_blank" rel="noreferrer">
                                Bubbles
                            </a>{" "}
                            or your own, go anywhere in the tree as they are.
                        </p>
                    </div>
                    <BrandCode code={viewCode} className="bloom-code-panel" />
                </section>

                <section className="bloom-section">
                    <h2>Effects are values</h2>
                    <p className="bloom-section-lede">
                        Every effect takes a progress from 0 to 1. The same effect is a transition on a node, a backdrop
                        behind a dialog, or a reaction to something happening, and effects combine in a list.
                    </p>
                    <div className="bloom-effects">
                        {effects.map((g) => (
                            <div key={g.label} className="bloom-effect-group">
                                <h3>{g.label}</h3>
                                <p>
                                    {g.names.map((n) => (
                                        <Link key={n} href="/reference/effects" className="bloom-effect">
                                            {n}
                                        </Link>
                                    ))}
                                </p>
                            </div>
                        ))}
                    </div>
                </section>

                <section className="bloom-section">
                    <h2>Where to start</h2>
                    <div className="bloom-paths">
                        {paths.map((p) => (
                            <div key={p.title} className="bloom-path">
                                <h3>{p.title}</h3>
                                <p>{p.body}</p>
                                <ul>
                                    {p.links.map((l) => (
                                        <li key={l.href}>
                                            <SmartLink href={l.href}>{l.text}</SmartLink>
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        ))}
                    </div>
                </section>

                <footer className="bloom-footer">
                    <div>
                        <span>© Liam Stanley.</span>
                        <span className="bloom-footer-links">
                            <SmartLink href="https://github.com/lrstanley/bloom">GitHub</SmartLink>
                            <SmartLink href="https://pkg.go.dev/bloomfx.dev/bloom">pkg.go.dev</SmartLink>
                            <SmartLink href="/resources/related-libraries">Related libraries</SmartLink>
                        </span>
                        <LlmsLinks />
                    </div>
                </footer>
            </main>
        </>
    );
}
