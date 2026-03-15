import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';

// Use /verity/ base path in CI (GitHub Pages), / locally for dev
const base = process.env.CI ? '/verity/' : '/';

export default defineConfig({
  site: 'https://verity-org.github.io',
  base,
  output: 'static',
  vite: {
    plugins: [tailwindcss()],
  },
});
