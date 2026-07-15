/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./app/**/*.{ts,tsx}', './components/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // dataviz dark palette
        page: '#0d0d0d',
        surface: '#1a1a19',
        line: 'rgba(255,255,255,0.10)',
        grid: '#2c2c2a',
        ink: { DEFAULT: '#ffffff', secondary: '#c3c2b7', muted: '#898781' },
        accent: '#3987e5',
        good: '#0ca30c',
        warn: '#fab219',
        critical: '#d03b3b',
        // Agent terminali uchun (GitHub-dark uslubi)
        term: {
          bg: '#0b0f14',
          green: '#3fb950',
          yellow: '#d29922',
          red: '#f85149',
          blue: '#58a6ff',
          cyan: '#39c5cf',
          magenta: '#bc8cff',
          gray: '#8b949e',
        },
      },
    },
  },
  plugins: [],
};
