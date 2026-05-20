import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

export default defineConfig({
  site: 'https://verity.supply',
  output: 'static',
  integrations: [sitemap()],
  vite: {
    cacheDir: process.env.VITE_CACHE_DIR ?? 'node_modules/.vite',
    plugins: [tailwindcss()],
  },
});
