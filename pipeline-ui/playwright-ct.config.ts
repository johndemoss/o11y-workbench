import { defineConfig, devices } from '@playwright/experimental-ct-svelte'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  testDir: './src/lib/__tests__',
  testMatch: ['**/*.spec.ts', '**/*.spec.ct.ts'],
  timeout: 10000,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  use: {
    trace: 'on-first-retry',
    ctViteConfig: {
      plugins: [svelte()]
    }
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
