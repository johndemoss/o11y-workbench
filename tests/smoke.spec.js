const { test, expect } = require('@playwright/test');

test.describe('Smoke Tests', () => {

  test('page loads and shows logo', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.logo-img')).toBeVisible();
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

  test.describe('Brand Assets', () => {

    test('dashboard has favicon link', async ({ page }) => {
      await page.goto('/');
      const favicon = page.locator('link[rel="icon"]');
      await expect(favicon).toHaveAttribute('href', '/assets/favicon.svg');
    });

    test('dashboard has logo image in header', async ({ page }) => {
      await page.goto('/');
      const logo = page.locator('.logo-img');
      await expect(logo).toBeVisible();
      await expect(logo).toHaveAttribute('src', '/assets/logo.svg');
      await expect(logo).toHaveAttribute('alt', 'o11y workbench');
    });

    test('pipeline UI has favicon link', async ({ page }) => {
      await page.goto('/pipeline/');
      const favicon = page.locator('link[rel="icon"]');
      await expect(favicon).toHaveAttribute('href', '/pipeline/favicon.svg');
    });

    test('pipeline UI has logo image in nav', async ({ page }) => {
      await page.goto('/pipeline/');
      const logo = page.locator('.app-logo');
      await expect(logo).toBeVisible();
      await expect(logo).toHaveAttribute('src', '/pipeline/logo.svg');
      await expect(logo).toHaveAttribute('alt', 'o11y workbench');
    });

    test('empty state shows otter illustration', async ({ page }) => {
      await page.goto('/');
      await page.waitForTimeout(1500);
      // Clear any runs from previous tests and stop polling so otter SVG empty state stays visible
      await page.evaluate(() => { clearInterval(pollTimer); pollTimer = null; runs = {}; renderRunList(); });
      const empty = page.locator('.otter-empty-svg');
      await expect(empty).toBeVisible();
    });

    test('footer has otter icon', async ({ page }) => {
      await page.goto('/');
      await expect(page.locator('.app-footer')).toBeVisible();
      await expect(page.locator('.footer-otter')).toBeVisible();
    });

    test('header has signal EQ bars', async ({ page }) => {
      await page.goto('/');
      const eq = page.locator('.signal-eq');
      await expect(eq).toBeVisible();
      const bars = await page.locator('.eq-bar').count();
      expect(bars).toBe(3);
    });

    test('config section has signal border', async ({ page }) => {
      await page.goto('/');
      const section = page.locator('#config-section');
      await expect(section).toBeVisible();
      const borderColor = await section.evaluate(el => getComputedStyle(el).borderLeftColor);
      expect(borderColor).toBe('rgb(59, 130, 246)');
    });

  });

});
