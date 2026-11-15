// @ts-check
const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  testMatch: '**/run-list-detail.spec.js',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  timeout: 60000,
  use: {
    baseURL: 'http://127.0.0.1:18080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'go run ./cmd/o11y-workbench-web/ -addr :18080 -pipeline-prototyper',
    url: 'http://127.0.0.1:18080/api/health',
    timeout: 120000,
    reuseExistingServer: false,
    cwd: __dirname,
  },
});
