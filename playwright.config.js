// @ts-check
const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { outputFolder: 'playwright-report' }]],
  use: {
    baseURL: 'http://localhost:18080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    // Brand Assets tests hit /pipeline/, which is only registered when the
    // pipeline prototyper is enabled.
    command: 'go run ./cmd/o11y-workbench-web/ -addr :18080 -pipeline-prototyper',
    port: 18080,
    timeout: 30000,
    reuseExistingServer: !process.env.CI,
    cwd: __dirname,
  },
});
