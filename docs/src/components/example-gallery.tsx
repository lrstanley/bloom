"use client";

import { useEffect, useRef, useState } from "react";
import { Link } from "fumapress/client";
import { X } from "lucide-react";
import { withBase } from "../lib/site";

export type Example = {
    name: string;
    src: string;
    alt: string;
    body: string;
    width: number;
    height: number;
    /** The example's section on the examples page. */
    docs: string;
    /** Its source on GitHub. */
    source: string;
};

const ease = "cubic-bezier(0.2, 0.8, 0.2, 1)";

const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// The transform that puts the image drawn at `to` over the thumbnail frame `from`, so it can grow from where it was
// clicked. The frame is one shape for every recording, and each is drawn inside it at its own shape (object-fit:
// contain), so the image is fitted inside the frame and centered, keeping its shape.
function flip(from: DOMRect, to: DOMRect) {
    const scale = Math.min(from.width / to.width, from.height / to.height);
    const dx = from.left + from.width / 2 - (to.left + to.width / 2);
    const dy = from.top + from.height / 2 - (to.top + to.height / 2);
    return `translate(${dx}px, ${dy}px) scale(${scale})`;
}

/**
 * A grid of example recordings. Clicking one zooms it from where it sits to fill the screen, in a modal dialog that
 * closes with escape, a click outside it, or the close button, shrinking back into its place.
 */
export function ExampleGallery({ examples }: { examples: Example[] }) {
    const dialog = useRef<HTMLDialogElement>(null);
    const image = useRef<HTMLImageElement>(null);
    const closeButton = useRef<HTMLButtonElement>(null);
    // The zoomed example, and the thumbnail it grows from, shrinks back into, and gives focus back to.
    const [open, setOpen] = useState<{ example: Example; trigger: HTMLButtonElement } | null>(null);
    const closing = useRef(false);

    const current = open?.example;

    // Opens once the dialog has the clicked example's image in it, so it's measured at its own size.
    useEffect(() => {
        const d = dialog.current;
        const img = image.current;
        if (!open || !d || !img || d.open) return;
        const thumb = open.trigger;
        d.showModal();
        closeButton.current?.focus();
        if (reducedMotion()) return;
        img.animate([{ transform: flip(thumb.getBoundingClientRect(), img.getBoundingClientRect()) }, { transform: "none" }], {
            duration: 320,
            easing: ease,
        });
        d.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, pseudoElement: "::backdrop" });
        d.querySelector(".bloom-zoom-caption")?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, delay: 140, fill: "backwards" });
    }, [open]);

    const close = async () => {
        const d = dialog.current;
        const img = image.current;
        const thumb = open?.trigger;
        if (!d || closing.current) return;
        closing.current = true;
        if (img && thumb && !reducedMotion()) {
            // Closed while still opening: carry on from where each part has got to, and measure the image without the
            // opening's transform.
            const caption = d.querySelector(".bloom-zoom-caption");
            const from = getComputedStyle(img).transform;
            const backdrop = getComputedStyle(d, "::backdrop").opacity;
            const text = caption ? getComputedStyle(caption).opacity : "0";
            d.getAnimations({ subtree: true }).forEach((a) => a.cancel());
            const to = flip(thumb.getBoundingClientRect(), img.getBoundingClientRect());
            caption?.animate([{ opacity: text }, { opacity: 0 }], { duration: 120, fill: "forwards" });
            d.animate([{ opacity: backdrop }, { opacity: 0 }], { duration: 260, pseudoElement: "::backdrop", fill: "forwards" });
            await img.animate([{ transform: from }, { transform: to }], { duration: 260, easing: ease, fill: "forwards" }).finished;
        }
        d.getAnimations({ subtree: true }).forEach((a) => a.cancel());
        d.close();
        closing.current = false;
        thumb?.focus({ preventScroll: true });
        setOpen(null);
    };

    return (
        <>
            <ul className="bloom-gallery">
                {examples.map((e) => (
                    <li key={e.name}>
                        <button
                            type="button"
                            className="bloom-gallery-thumb"
                            onClick={(ev) => setOpen({ example: e, trigger: ev.currentTarget })}
                            aria-label={`Enlarge the ${e.name} example`}
                            aria-haspopup="dialog"
                            // Hidden while it's zoomed, but not with visibility, so it can still take focus back.
                            style={{ opacity: open?.example === e ? 0 : undefined }}
                        >
                            <img src={withBase(e.src)} alt={e.alt} width={e.width} height={e.height} loading="lazy" />
                        </button>
                        <div className="bloom-gallery-text">
                            <h3>
                                <Link href={e.docs}>{e.name}</Link>
                            </h3>
                            <p>{e.body}</p>
                        </div>
                    </li>
                ))}
            </ul>

            <dialog
                ref={dialog}
                className="bloom-zoom"
                aria-label={current ? `The ${current.name} example` : undefined}
                onCancel={(ev) => {
                    ev.preventDefault();
                    void close();
                }}
                onClick={(ev) => {
                    if (ev.target === ev.currentTarget || ev.target === image.current) void close();
                }}
            >
                {current && (
                    <>
                        <img ref={image} src={withBase(current.src)} alt={current.alt} width={current.width} height={current.height} />
                        <div className="bloom-zoom-caption">
                            <p>
                                <strong>{current.name}</strong> {current.body}
                            </p>
                            <a href={current.source} target="_blank" rel="noreferrer">
                                Source
                            </a>
                            <button type="button" ref={closeButton} className="bloom-zoom-close" onClick={() => void close()} aria-label="Close">
                                <X aria-hidden />
                            </button>
                        </div>
                    </>
                )}
            </dialog>
        </>
    );
}
