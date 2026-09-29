const { test, expect } = require('@playwright/test');

test.describe('Run list summary + detail Get-by-id', () => {
  test('list omits payloads; detail Get-by-id works; idle poll stops', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#run-btn')).toBeVisible();

    await page.evaluate(() => {
      const cb = document.querySelector('#signal-checkboxes input[value="traces"]');
      if (cb) cb.checked = true;
      const area = document.getElementById('trace-config-area');
      if (area) area.classList.remove('collapsed');
      const cnt = document.getElementById('trace-count');
      if (cnt) cnt.value = 1;
    });

    await page.click('#run-btn');
    await expect(page.locator('.run-card.completed')).toBeVisible({ timeout: 20000 });

    // After completion, polling should stop.
    await expect.poll(async () => page.evaluate(() => pollTimer), { timeout: 5000 }).toBeNull();

    const list = await page.evaluate(async () => {
      const res = await fetch('/api/runs');
      return res.json();
    });
    expect(list.runs.length).toBeGreaterThan(0);
    for (const run of list.runs) {
      expect(run.trace_data).toBeUndefined();
      expect(run.log_data).toBeUndefined();
      expect(run.metric_data).toBeUndefined();
      if (run.status === 'completed' && (run.span_count || 0) > 0) {
        expect(run.has_trace_data).toBeTruthy();
      }
    }

    const completedId = await page.locator('.run-card.completed').first().evaluate(el => {
      const idEl = el.querySelector('.id');
      return idEl ? idEl.textContent.split('·')[0].trim() : null;
    });
    expect(completedId).toBeTruthy();

    const detailResponsePromise = page.waitForResponse(
      res => res.url().includes('/api/runs/' + completedId) && res.request().method() === 'GET'
    );
    await page.locator('.run-card.completed').first().click();
    const detailRes = await detailResponsePromise;
    expect(detailRes.ok()).toBeTruthy();
    const detailBody = await detailRes.json();
    expect(detailBody.trace_data).toBeTruthy();

    await expect(page.locator('#detail-section')).toBeVisible();
    await expect(page.locator('#detail-content')).not.toContainText('Failed to load');
    await expect(page.locator('#detail-content')).not.toHaveText('');
  });
});
