/** Where the site is published, with a trailing slash. Pages are served from its root. */
export const SITE_URL = "https://bloomfx.dev/";

/** Prefixes a root-relative path with the base path, for plain links to files in public/. */
export function withBase(path: string) {
    return import.meta.env.BASE_URL.replace(/\/$/, "") + path;
}
