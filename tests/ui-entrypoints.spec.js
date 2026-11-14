const { test, expect } = require('@playwright/test');

for (const { route, shell } of [
  { route: '/pipeline/', shell: '.app-logo' },
]) {
  test(`${route} executes its embedded bundle and rejects absent assets`, async ({ page, request }) => {
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('response', response => {
      if (['script', 'stylesheet'].includes(response.request().resourceType())) {
        if (!response.ok() || response.headers()['content-type']?.includes('text/html')) {
          errors.push(`invalid bundle response: ${response.status()} ${new URL(response.url()).pathname}`);
        }
      }
    });
    const response = await page.goto(route);
    expect(response.status()).toBe(200);
    // This markup is produced by Svelte, not the source index.html shell.
    await expect(page.locator(shell)).toBeVisible();
    expect(errors).toEqual([]);
    const missing = await request.get(`${route}assets/not-a-real-bundle.js`);
    expect(missing.status()).toBe(404);
    expect(missing.headers()['content-type']).not.toContain('text/html');
  });
}
