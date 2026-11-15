import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  base: '/pipeline/',
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': { target: process.env.O11Y_VITE_API_TARGET ?? 'http://127.0.0.1:18080', changeOrigin: false }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  },
  test: {
    // Vitest unit tests use *.test.ts; *.spec.ts files are Playwright
    // (component tests in src/lib/__tests__ and e2e/) run via `npm run test:ct`.
    include: ['src/**/*.test.ts']
  }
})
