/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,ts}'],
  theme: {
    extend: {
      colors: {
        verity: {
          nucleus: '#00D4B8',
          orbit: '#0099A8',
          teal: {
            light: '#00CCAA',
            DEFAULT: '#00D4B8',
            dark: '#0099A8',
          },
          void: '#0B1419',
          surface: '#111B22',
          border: '#1F2B35',
          text: {
            primary: '#C5D5DD',
            secondary: '#7A8E9C',
            muted: '#5A6E7C',
          },
        },
      },
      fontFamily: {
        mono: ['"Share Tech Mono"', '"Courier New"', 'monospace'],
      },
    },
  },
  plugins: [],
};
