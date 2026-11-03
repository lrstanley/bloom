// Post-build fixups and checks for the static site in dist/public, which is served from Cloudflare
// (https://bloomfx.dev/).
//
//  1. The markdown outputs (llms.txt, llms-full.txt, /<page>.md) are generated with root-relative links
//     (e.g. /guides/dialogs). Rewrite links to other pages to the absolute URL of their markdown version
//     (/<page>.md, /index.md for the root), per https://llmstxt.org, so agents stay on markdown. Links to
//     anything else get the absolute site URL.
//  2. Give llms.txt a title, summary, and the external references worth reading.
//  3. Validate every internal link and #fragment in the generated HTML.
import { existsSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const SITE = "https://bloomfx.dev";
const BASE = new URL(SITE).pathname.replace(/\/$/, ""); // "" when served from the root
const OUT = join(import.meta.dirname, "..", "dist", "public");

function walk(dir: string): string[] {
    return readdirSync(dir).flatMap((f) => {
        const p = join(dir, f);
        return statSync(p).isDirectory() ? walk(p) : [p];
    });
}

const files = walk(OUT);
const errors: string[] = [];

/** Maps a root-relative path (with an optional #fragment) to its absolute URL, preferring markdown. */
function toAbsolute(href: string, source: string) {
    const [path = "", fragment] = href.split("#");
    const page = path.replace(/\/$/, "") || "/index";
    const frag = fragment ? `#${fragment}` : "";
    if (existsSync(join(OUT, `${page}.md`))) return `${SITE}${page}.md${frag}`;
    if (!existsSync(join(OUT, path)) && !existsSync(join(OUT, path, "index.html"))) {
        errors.push(`${source}: no page or file for ${href}`);
    }
    return `${SITE}${path}${frag}`;
}

for (const file of files.filter((f) => f.endsWith(".md") || /llms(-full)?\.txt$/.test(f))) {
    const source = file.slice(OUT.length);
    const src = readFileSync(file, "utf8");
    const out = src
        // markdown links, [text](/path#frag)
        .replace(/\]\((\/(?!\/)[^)\s]*)\)/g, (_, href: string) => `](${toAbsolute(href, source)})`)
        // llms-full.txt page headings, "# Title (/path)"
        .replace(/^(# .+) \((\/[^)]*)\)$/gm, (_, title: string, href: string) => `${title} (${toAbsolute(href, source)})`);
    if (out !== src) writeFileSync(file, out);
}

const llmsPath = join(OUT, "llms.txt");
const llms = readFileSync(llmsPath, "utf8");
if (!llms.startsWith("# Docs")) {
    errors.push("llms.txt: expected the generated index to start with '# Docs'");
} else {
    writeFileSync(
        llmsPath,
        llms.replace(
            "# Docs",
            `# bloom

> bloom is a Go library for Bubble Tea (charm.land/bubbletea/v2) programs. A view is described each frame as a tree of nodes (rows, columns, grids, boxes, and overlays with layers such as dialogs, dropdowns, and toasts), which bloom lays out, composites, animates, and routes mouse and focus input for. Its packages are bloom (layout, layers, input), anim (springs, eased values, presences, and sequences kept in the model), and fx (cell effects used as transitions and backdrops).

Import path: github.com/lrstanley/bloom. Every page below links to its markdown version; the whole site is also available as a single file at ${SITE}/llms-full.txt.`,
        ).trimEnd() +
            `

## Optional

- [bloom API reference](https://pkg.go.dev/github.com/lrstanley/bloom): Go package documentation for bloom
- [anim API reference](https://pkg.go.dev/github.com/lrstanley/bloom/anim): Go package documentation for bloom/anim
- [fx API reference](https://pkg.go.dev/github.com/lrstanley/bloom/fx): Go package documentation for bloom/fx
- [Bubble Tea](https://github.com/charmbracelet/bubbletea): the TUI framework bloom builds on
- [Lip Gloss](https://github.com/charmbracelet/lipgloss): the styling library bloom's boxes and text use
- [Source and examples](https://github.com/lrstanley/bloom): the repository, including runnable programs in _examples
`,
    );
}

// llms-full.txt comes out in file order. Put its pages in the order of the sidebar (and llms.txt), so it reads from
// the introduction and tutorials through to the reference.
const fullPath = join(OUT, "llms-full.txt");
const order = [...readFileSync(llmsPath, "utf8").matchAll(/\]\((https:[^)]+\.md)\)/g)].map((m) => m[1] as string);
const sections = readFileSync(fullPath, "utf8")
    .split(/^(?=# .+ \(https:[^)]+\)$)/m)
    .filter((s) => s.trim());
const rank = (s: string) => {
    const url = /^# .+ \((https:[^)]+)\)$/m.exec(s)?.[1] ?? "";
    const i = order.indexOf(url);
    return i < 0 ? order.length : i;
};
writeFileSync(fullPath, sections.sort((a, b) => rank(a) - rank(b)).map((s) => s.trimEnd()).join("\n\n") + "\n");

const htmlFiles = files.filter((f) => f.endsWith(".html"));
const ids = new Map<string, Set<string>>();
const idsOf = (file: string) => {
    let set = ids.get(file);
    if (!set) {
        set = new Set([...readFileSync(file, "utf8").matchAll(/\sid="([^"]+)"/g)].map((m) => m[1] as string));
        ids.set(file, set);
    }
    return set;
};

for (const file of htmlFiles) {
    const page = "/" + file.slice(OUT.length + 1).replace(/(^|\/)index\.html$/, "");
    for (const [, href = ""] of readFileSync(file, "utf8").matchAll(/<a\s[^>]*?href="([^"]*)"/g)) {
        if (/^(https?:|mailto:|tel:|\/\/)/.test(href)) continue;

        const [path = "", fragment] = href.replace(/&amp;/g, "&").split("#");
        let target = file;
        if (path) {
            if (path !== BASE && !path.startsWith(`${BASE}/`)) {
                errors.push(`${page}: link ${href} is not under ${BASE}/`);
                continue;
            }
            const rel = path.slice(BASE.length).replace(/^\//, "").replace(/\/$/, "");
            const candidates = [join(OUT, rel, "index.html"), join(OUT, rel)];
            const found = candidates.find((c) => existsSync(c) && statSync(c).isFile());
            if (!found) {
                errors.push(`${page}: broken link ${href}`);
                continue;
            }
            target = found;
        }
        if (fragment && target.endsWith(".html") && !idsOf(target).has(decodeURIComponent(fragment))) {
            errors.push(`${page}: broken anchor ${href}`);
        }
    }
}

if (errors.length) {
    console.error(`\n${errors.length} problem(s):\n` + [...new Set(errors)].map((e) => `  - ${e}`).join("\n"));
    process.exit(1);
}
console.log(`[postbuild] checked links in ${htmlFiles.length} pages, pointed markdown links at ${SITE}`);
