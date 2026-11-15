import { defineConfig } from '@playwright/test'
import { E2E_PORT } from './e2e/helpers'

const host = '127.0.0.1'
const baseURL = `http://${host}:${E2E_PORT}/pipeline/`

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  use: {
    baseURL,
    headless: true,
    trace: 'on-first-retry',
  },
  webServer: {
    command: `npm run dev -- --host ${host} --port ${E2E_PORT} --strictPort`,
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120_000,
  },
})