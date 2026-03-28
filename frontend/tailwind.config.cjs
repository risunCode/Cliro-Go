/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html', './src/**/*.{svelte,ts,js}'],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Outfit', 'Segoe UI', 'Helvetica Neue', 'Arial', 'sans-serif']
      },
      colors: {
        app: 'var(--color-bg)',
        surface: 'var(--color-surface)',
        border: 'var(--color-border)',
        text: {
          primary: 'var(--color-text-primary)',
          secondary: 'var(--color-text-secondary)'
        },
        state: {
          success: 'var(--color-success)',
          error: 'var(--color-error)',
          warning: 'var(--color-warning)',
          info: 'var(--color-info)'
        }
      },
      borderRadius: {
        base: 'var(--radius-base)',
        sm: 'var(--radius-small)'
      },
      boxShadow: {
        soft: 'var(--shadow-soft)',
        medium: 'var(--shadow-medium)'
      }
    }
  },
  plugins: []
}
