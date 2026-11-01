import type { ReactNode } from "react";

// Highlights short Go snippets in the palette of the README banner's code line: bloom in rose, methods in
// apricot, and effects in cream. It's rendered on the server, so the homepage ships no highlighter.

const token =
    /(\/\/[^\n]*)|("(?:[^"\\]|\\.)*")|(?<!\.)\b(bloom|fx|anim)\b(?=\.)|(?<=\bfx\.)([A-Z]\w*)|(?<=\.)([A-Z]\w*)(?=\()|\b(func|return|if|else|var|type|range|for)\b|\b(\d+(?:\.\d+)?)\b/g;

const classes = [
    "text-fd-muted-foreground italic", // comment
    "text-[#c2255c] dark:text-salmon", // string
    "text-[#c2255c] dark:text-rose font-semibold", // package
    "text-[#7a3d00] dark:text-cream font-semibold dark:[text-shadow:0_0_10px_rgb(255_241_214/0.45)]", // effect
    "text-[#9a4a12] dark:text-apricot", // method
    "text-[#6b2a64] dark:text-[#d9a6c9]", // keyword
    "text-[#c2255c] dark:text-salmon", // number
];

function highlight(code: string): ReactNode[] {
    const out: ReactNode[] = [];
    let last = 0;
    for (const m of code.matchAll(token)) {
        const i = m.index ?? 0;
        if (i > last) out.push(code.slice(last, i));
        const group = m.slice(1).findIndex((g) => g !== undefined);
        out.push(
            <span key={i} className={classes[group]}>
                {m[0]}
            </span>,
        );
        last = i + m[0].length;
    }
    out.push(code.slice(last));
    return out;
}

export function BrandCode({ code, className = "" }: { code: string; className?: string }) {
    return (
        <pre className={`brand-code ${className}`}>
            <code>{highlight(code)}</code>
        </pre>
    );
}
