import { defineConfig, devices } from '@playwright/experimental-ct-svelte'

export default defineConfig({
  testDir: './src/lib/__tests__',
  testMatch: ['**/*.spec.ts', '**/*.spec.ct.ts'],
  timeout: 10000,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  use: {
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
