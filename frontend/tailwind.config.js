/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        positive: '#10b981',
        negative: '#ef4444',
        primary: {
          50: '#fef2f2',
          100: '#fee2e2',
          200: '#fecaca',
          300: '#fca5a5',
          400: '#f87171',
          500: '#ef4444',
          600: '#dc2626',
          700: '#b91c1c',
          800: '#991b1b',
          900: '#7f1d1d',
        },
      },
      animation: {
        'blink-up': 'blink-up 5s ease-in-out',
        'blink-down': 'blink-down 5s ease-in-out',
      },
      keyframes: {
        'blink-up': {
          '0%': { color: 'rgb(17, 24, 39)' },
          '20%': { color: 'rgb(34, 197, 94)' },
          '80%': { color: 'rgb(34, 197, 94)' },
          '100%': { color: 'rgb(17, 24, 39)' },
        },
        'blink-down': {
          '0%': { color: 'rgb(17, 24, 39)' },
          '20%': { color: 'rgb(239, 68, 68)' },
          '80%': { color: 'rgb(239, 68, 68)' },
          '100%': { color: 'rgb(17, 24, 39)' },
        },
      },
    },
  },
  plugins: [],
}
