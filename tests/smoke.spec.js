const { test, expect } = require('@playwright/test');

test.describe('Smoke Tests', () => {

  test('page loads and shows title', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.logo-text')).toHaveText('otel-workbench');
  });

  test('no console errors on load', async ({ page }) => {
    const errors = [];
    page.on('pageerror', err => errors.push(err.message));
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    await page.goto('/');
    await page.waitForTimeout(1000);

    expect(errors).toEqual([]);
  });

  test('Run button exists and is enabled', async ({ page }) => {
    await page.goto('/');
    const btn = page.locator('#run-btn');
    await expect(btn).toBeVisible();
    await expect(btn).toBeEnabled();
  });

  test('Run generation creates a run with default config', async ({ page }) => {
    await page.goto('/');

    // Enable traces and disable collapse
    await page.evaluate(() => {
      const cb = document.querySelector('#signal-checkboxes input[value="traces"]');
      if (cb) cb.checked = true;
      const area = document.getElementById('trace-config-area');
      if (area) area.classList.remove('collapsed');
      const cnt = document.getElementById('trace-count');
      if (cnt) cnt.value = 1;
    });

    await page.click('#run-btn');
    await page.waitForTimeout(3000);

    // Verify run was created
    const runList = page.locator('#run-list');
    await expect(runList).not.toContainText('No runs yet');
    const cards = await page.locator('.run-card').count();
    expect(cards).toBeGreaterThan(0);
  });

});
