// @ts-check
const { defineConfig } = require('@playwright/test');
const { tmpdir } = require('node:os');
const { join } = require('node:path');

const rawPort = process.env.O11Y_BROWSER_PORT || '18080';
const port = Number(rawPort);
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error(`O11Y_BROWSER_PORT must be a port number, got ${JSON.stringify(rawPort)}`);
}
const baseURL = `http://127.0.0.1:${port}`;

module.exports = defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { outputFolder: 'playwright-report' }]],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    // Never accept an arbitrary stale daemon or bypass the approved build path.
    // Foreground launch leaves the operator's PID file alone; Playwright owns/join-
    // terminates this process and its workers, including when a browser test fails.
    command: `./scripts/serve.sh --foreground --addr 127.0.0.1:${port}`,
    url: `${baseURL}/api/health`,
    timeout: 180000,
    reuseExistingServer: false,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 10000 },
    env: {
      O11Y_WORKBENCH_EVALUATION_DIR: join(tmpdir(), `o11y-workbench-browser-${process.pid}`),
    },
    cwd: __dirname,
  },
});
