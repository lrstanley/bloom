import { defineConfig } from "fumapress";
import { fumadocsMdx } from "fumapress/adapters/mdx";
import { metaSchema, pageSchema } from "fumapress/adapters/mdx/schema";
import { createDocsLayoutPage } from "fumapress/layouts/docs";
import { createHomeLayout } from "fumapress/layouts/home";
import { flexsearchPlugin } from "fumapress/plugins/flexsearch";
import { llmsPlugin } from "fumapress/plugins/llms.txt";
import { defineDocs } from "fumadocs-mdx/macro";
import favicon from "./src/assets/favicon.svg";
import { DiscordIcon } from "./src/components/discord-icon";
import { LlmsLinks } from "./src/components/llms-links";
import { Logo } from "./src/components/logo";
import { getMdxComponents } from "./src/components/mdx";
import { GO_IMPORT, GO_SOURCE, REPO_URL, SITE_URL, withBase } from "./src/lib/site";

const docs = defineDocs({
    dir: "content",
    docs: {
        async: true,
        schema: pageSchema,
        lastModified: true,
        postprocess: {
            includeProcessedMarkdown: { output: "function" },
        },
    },
    meta: {
        schema: metaSchema,
    },
});

const config = defineConfig({
    content: docs.toFumadocsSource(),
    mode: "static",
    site: {
        name: "bloom",
        baseUrl: SITE_URL,
        git: {
            user: "lrstanley",
            repo: "bloom",
            branch: "master",
            rootDir: "..",
        },
    },
    defaultLayoutProps: {
        nav: {
            title: <Logo />,
        },
        githubUrl: REPO_URL,
        links: [
            {
                text: "pkg.go.dev",
                url: "https://pkg.go.dev/bloomfx.dev/bloom",
                external: true,
            },
            {
                text: "Bubble Tea",
                url: "https://github.com/charmbracelet/bubbletea",
                external: true,
            },
            {
                type: "icon",
                text: "Discord",
                label: "Discord",
                url: "https://liam.sh/chat",
                icon: <DiscordIcon />,
                external: true,
            },
        ],
    },
    renderPage: createDocsLayoutPage({
        render() {
            return {
                layoutProps: { sidebar: { footer: <LlmsLinks className="mt-3 px-1" /> } },
            };
        },
    }),
    meta: {
        root() {
            return (
                <>
                    <meta name="go-import" content={GO_IMPORT} />
                    <meta name="go-source" content={GO_SOURCE} />
                    <meta name="author" content="Liam Stanley" />
                    <meta name="copyright" content="© Liam Stanley" />
                    <meta name="theme-color" content="#0d0a12" />
                    <meta name="darkreader-lock" />
                    <meta property="og:image" content={`${SITE_URL}og.png`} />
                    <meta property="og:image:width" content="1280" />
                    <meta property="og:image:height" content="640" />
                    <meta name="twitter:card" content="summary_large_image" />
                    <link rel="icon" href={favicon} type="image/svg+xml" />
                    <link rel="apple-touch-icon" href={withBase("/apple-touch-icon.png")} />
                    <link rel="preconnect" href="https://fonts.googleapis.com" />
                    <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
                    <link
                        href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=Hanken+Grotesk:ital,wght@0,300..800;1,300..800&family=Martian+Mono:wdth,wght@75..112.5,100..800&display=swap"
                        rel="stylesheet"
                    />
                </>
            );
        },
    },
})
    .adapters(fumadocsMdx({ getMdxComponents }))
    .plugins(llmsPlugin({ routes: "all" }), flexsearchPlugin());

export default config;
export const HomeLayout = createHomeLayout<typeof config.$context>();
