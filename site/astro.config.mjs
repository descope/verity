import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

const viteCacheDir = process.env.VITE_CACHE_DIR?.trim() || 'node_modules/.vite';

export default defineConfig({
  site: 'https://verity.supply',
  output: 'static',
  integrations: [sitemap()],
  vite: {
    cacheDir: viteCacheDir,
    plugins: [tailwindcss()],
  },
});
