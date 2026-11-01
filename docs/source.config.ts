import { defineConfig } from "fumadocs-mdx/config";
import { rehypeCodeDefaultOptions } from "fumadocs-core/mdx-plugins/rehype-code";
import { remarkMdxMermaid } from "fumadocs-core/mdx-plugins/remark-mdx-mermaid";
import { transformerMetaHighlight, transformerMetaWordHighlight } from "@shikijs/transformers";

export default defineConfig({
  mdxOptions: {
    remarkPlugins: (v) => [...v, remarkMdxMermaid],
    rehypeCodeOptions: {
      // Rosé Pine sits close to bloom's plum and rose palette.
      themes: { light: "rose-pine-dawn", dark: "rose-pine-moon" },
      transformers: [
        ...(rehypeCodeDefaultOptions.transformers ?? []),
        // supports ```go {1,3-5} and ```go /word/ meta strings.
        transformerMetaHighlight(),
        transformerMetaWordHighlight(),
      ],
    },
  },
});
