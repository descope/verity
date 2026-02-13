import { defineConfig } from 'astro/config';
import tailwind from '@astrojs/tailwind';

// Use /verity/ base path in CI (GitHub Pages), / locally for dev
const base = process.env.CI ? '/verity/' : '/';

export default defineConfig({
  site: 'https://descope.github.io',
  base,
  output: 'static',
  integrations: [tailwind()],
});
