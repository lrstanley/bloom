import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import press from "fumapress/vite";
import { fumadocsMdx } from "fumadocs-mdx/vite";

// Served from GitHub Pages at https://lrstanley.github.io/bloom/.
export default defineConfig({
  plugins: [press({ basePath: "/bloom/" }), fumadocsMdx(), tailwindcss()],
});
