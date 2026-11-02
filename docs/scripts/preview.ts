// Serves the built site under BASE, as GitHub Pages does, so base-path links resolve locally.
import { createReadStream, existsSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";

const BASE = "/";
const ROOT = join(import.meta.dirname, "..", "dist", "public");
const port = Number(process.env.PORT ?? 3000);

const types: Record<string, string> = {
    ".html": "text/html; charset=utf-8",
    ".js": "text/javascript",
    ".css": "text/css",
    ".svg": "image/svg+xml",
    ".png": "image/png",
    ".webp": "image/webp",
    ".json": "application/json",
    ".xml": "application/xml",
    ".md": "text/markdown; charset=utf-8",
    ".txt": "text/plain; charset=utf-8",
};

function resolve(pathname: string) {
    const rel = normalize(decodeURIComponent(pathname.slice(BASE.length))).replace(/^(\.\.(\/|$))+/, "");
    const path = join(ROOT, rel);
    if (existsSync(path) && statSync(path).isFile()) return path;
    if (existsSync(join(path, "index.html"))) return join(path, "index.html");
    return undefined;
}

createServer((req, res) => {
    const { pathname } = new URL(req.url ?? "/", "http://localhost");
    if (!pathname.startsWith(BASE)) {
        res.writeHead(302, { location: BASE }).end();
        return;
    }
    const file = resolve(pathname) ?? join(ROOT, "404.html");
    res.writeHead(file.endsWith("404.html") && !resolve(pathname) ? 404 : 200, {
        "content-type": types[extname(file)] ?? "application/octet-stream",
    });
    createReadStream(file).pipe(res);
}).listen(port, () => console.log(`serving dist/public at http://localhost:${port}${BASE}`));
