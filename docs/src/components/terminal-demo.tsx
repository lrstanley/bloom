"use client";

import { useState, type CSSProperties, type ReactNode } from "react";

// A terminal drawn in HTML, playing the notes app from the tutorials once: a delete confirmation irises open
// over a dimmed screen, focus moves to Delete, the dialog closes, the note goes, and a toast slides in. Timing
// and the static frame shown with reduced motion are in app.css (.td-*).
//
// Boxes are sized in cells (ch by rows) and their borders drawn with CSS halfway through the outer cells, where a
// terminal draws box-drawing characters. Web fonts rarely include those characters, and a fallback font's don't
// join up.

const pad = (s: string, n: number) => s + " ".repeat(Math.max(n - [...s].length, 0));

function Box({ cols, rows, className = "", children }: { cols: number; rows: number; className?: string; children: ReactNode }) {
    return (
        <div className={`td-box ${className}`} style={{ "--cols": cols, "--rows": rows } as CSSProperties}>
            {children}
        </div>
    );
}

const Row = ({ children, className = "" }: { children?: ReactNode; className?: string }) => (
    <div className={`td-row ${className}`}>{children ?? " "}</div>
);

export function TerminalDemo() {
    const [run, setRun] = useState(0);

    return (
        <figure className="td not-prose" aria-label="A Bubble Tea program built with bloom, showing a confirmation dialog and a toast notification">
            <div className="td-window" key={run}>
                <div className="td-chrome" aria-hidden="true">
                    <span className="td-dots">
                        <i />
                        <i />
                        <i />
                    </span>
                    <span className="td-title">notes</span>
                    <span className="td-keys">
                        <kbd className="td-key td-key-d">d</kbd>
                        <kbd className="td-key td-key-tab">tab</kbd>
                        <kbd className="td-key td-key-enter">enter</kbd>
                    </span>
                </div>

                <div className="td-screen" aria-hidden="true">
                    <div className="td-base">
                        <div className="td-panes">
                            <Box cols={22} rows={12} className="td-pane">
                                <Row className="td-collapse">
                                    <span className="td-sel">{pad(" Groceries", 20)}</span>
                                </Row>
                                <Row>
                                    <span className="td-next">{pad(" Ideas", 20)}</span>
                                </Row>
                                <Row> Reading</Row>
                            </Box>
                            <Box cols={44} rows={12} className="td-pane">
                                <div className="td-body">
                                    <div className="td-body-a">
                                        <Row>
                                            <b className="td-heading"> Groceries</b>
                                        </Row>
                                        <Row> Oat milk, lemons, basil, and a loaf of</Row>
                                        <Row> rye.</Row>
                                    </div>
                                    <div className="td-body-b">
                                        <Row>
                                            <b className="td-heading"> Ideas</b>
                                        </Row>
                                        <Row> A terminal garden that grows while you</Row>
                                        <Row> type.</Row>
                                    </div>
                                </div>
                            </Box>
                        </div>
                        <Row className="td-status"> ↑/↓ choose, d delete, q quit</Row>
                    </div>

                    <Box cols={32} rows={8} className="td-dialog">
                        <Row />
                        <Row>
                            <b className="td-heading">{' Delete "Groceries"?'}</b>
                        </Row>
                        <Row> This can't be undone.</Row>
                        <Row />
                        <Row>
                            {"     "}
                            <span className="td-btn td-cancel">{"  Cancel  "}</span>
                            {"  "}
                            <span className="td-btn td-delete">{"  Delete  "}</span>
                        </Row>
                    </Box>

                    <Box cols={22} rows={3} className="td-toast">
                        <Row>
                            {" "}
                            <span className="td-type">✓ Deleted Groceries</span>
                        </Row>
                    </Box>
                </div>
            </div>
            <figcaption className="td-caption">
                <span>The notes app from the tutorials: a modal dialog over a dimmed screen, then a toast.</span>
                <button type="button" className="td-replay" onClick={() => setRun((n) => n + 1)}>
                    Replay
                </button>
            </figcaption>
        </figure>
    );
}
