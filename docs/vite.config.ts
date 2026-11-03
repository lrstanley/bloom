import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import press from "fumapress/vite";
import { fumadocsMdx } from "fumadocs-mdx/vite";

// Served from Cloudflare (see wrangler.jsonc) at https://bloomfx.dev/.
export default defineConfig({
  plugins: [press({ basePath: "/" }), fumadocsMdx(), tailwindcss()],
});
