/** Where the site is published, with a trailing slash. Pages are served from its root. */
export const SITE_URL = "https://bloomfx.dev/";

/** Prefixes a root-relative path with the base path, for plain links to files in public/. */
export function withBase(path: string) {
    return import.meta.env.BASE_URL.replace(/\/$/, "") + path;
}

/** The repository the Go module is fetched from. */
export const REPO_URL = "https://github.com/lrstanley/bloom";

/**
 * The vanity import path the Go module lives under. Every page, the 404 page included, carries these meta tags, which
 * is all `go get` needs: it reads only the <head>, even of a 404, so any path under the prefix resolves to the
 * repository. postbuild.ts fails the build if a page is missing them.
 */
export const GO_IMPORT_PATH = "bloomfx.dev/bloom";
export const GO_IMPORT = `${GO_IMPORT_PATH} git ${REPO_URL}`;
export const GO_SOURCE = `${GO_IMPORT_PATH} ${REPO_URL} ${REPO_URL}/tree/master{/dir} ${REPO_URL}/blob/master{/dir}/{file}#L{line}`;
