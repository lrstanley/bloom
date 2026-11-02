import type { ReactNode } from "react";
import { Link } from "fumapress/client";
import { asMarkdown, md } from "fumapress/markdown";
import { BrandCode } from "../components/brand-code";
import { LlmsLinks } from "../components/llms-links";
import { Logo } from "../components/logo";
import { TerminalDemo } from "../components/terminal-demo";
import { withBase } from "../lib/site";

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
        code: "id, ok := msg.Target[noteID]()",
        href: "/input/mouse",
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
            { text: "Introduction", href: "/getting-started/introduction" },
            { text: "Using Bubbles components", href: "/layout/bubbles-components" },
            { text: "How layout works", href: "/concepts/how-layout-works" },
        ],
    },
    {
        title: "Looking something up",
        body: "Every effect, node, and option on one page each, and the full API on pkg.go.dev.",
        links: [
            { text: "Effects", href: "/reference/effects" },
            { text: "Nodes and options", href: "/reference/options" },
            { text: "API reference", href: "https://pkg.go.dev/github.com/lrstanley/bloom" },
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

> **Note**
>
> bloom is pre-1.0. Its API may change in breaking ways until v1.0.0.

\`\`\`console
go get github.com/lrstanley/bloom@latest
\`\`\`

## All of it in one program

The [showcase example](${showcase.href}) ${showcase.body}

![${showcase.alt}](${showcase.src})

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
- [API reference](https://pkg.go.dev/github.com/lrstanley/bloom)
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
                        <code className="bloom-install">go get github.com/lrstanley/bloom@latest</code>
                        <p className="bloom-note">
                            Pre-1.0: expect breaking changes until v1.0.0.
                        </p>
                    </div>
                    <div className="bloom-hero-demo">
                        <TerminalDemo />
                    </div>
                </section>

                <section className="bloom-section">
                    <h2>All of it in one program</h2>
                    <p className="bloom-section-lede">
                        The{" "}
                        <a href={showcase.href} target="_blank" rel="noreferrer">
                            showcase example
                        </a>{" "}
                        {showcase.body}
                    </p>
                    <img className="bloom-showcase" src={withBase(showcase.src)} alt={showcase.alt} width={1238} height={578} loading="lazy" />
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
                    <p>
                        Theming your app? <Link href="/resources/related-libraries#bubbletint">bubbletint</Link> has
                        hundreds of terminal color themes that work in bloom's styles and effects, and{" "}
                        <Link href="/resources/related-libraries#steep">steep</Link> tests Bubble Tea programs end to
                        end.
                    </p>
                    <LlmsLinks />
                </footer>
            </main>
        </>
    );
}
