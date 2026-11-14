const { test, expect } = require('@playwright/test');

function isExactListRunsUrl(url) {
  try {
    const u = new URL(url);
    return u.pathname === '/api/runs' || u.pathname.endsWith('/api/runs');
  } catch {
    return false;
  }
}

async function safeEval(page, fn, arg) {
  if (page.isClosed()) return undefined;
  try {
    return arg === undefined ? await page.evaluate(fn) : await page.evaluate(fn, arg);
  } catch (e) {
    if (page.isClosed() || /Target page|closed|destroyed|Execution context/i.test(String(e))) {
      return undefined;
    }
    throw e;
  }
}

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

    // Task C: pollTimer non-null while a run is active.
    await expect
      .poll(async () => safeEval(page, () => pollTimer !== null), { timeout: 5000 })
      .toBe(true);

    await expect(page.locator('.run-card.completed').first()).toBeVisible({ timeout: 20000 });

    // After completion, polling should stop.
    await expect
      .poll(async () => safeEval(page, () => pollTimer), { timeout: 5000 })
      .toBeNull();

    // Task C done-when: idle list GETs ≈ 0 over ~3s (beyond initial load / active poll).
    let idleListGets = 0;
    const onIdleReq = (req) => {
      if (req.method() === 'GET' && isExactListRunsUrl(req.url())) idleListGets++;
    };
    page.on('request', onIdleReq);
    await page.waitForTimeout(3000);
    page.off('request', onIdleReq);
    expect(idleListGets).toBe(0);

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

    // Task B: Jobs List signal filter uses has_trace_data when list omits trace_data.
    const filterResult = await safeEval(page, () => {
      const candidates = Object.values(runs).filter(
        (r) => r.status === 'completed' && !!r.has_trace_data
      );
      if (!candidates.length) return { ok: false, reason: 'no completed has_trace_data run' };

      // Ensure we exercise the has_* path, not embedded payloads.
      for (const r of candidates) {
        delete r.trace_data;
        delete r.log_data;
        delete r.metric_data;
      }
      const target = candidates[0];

      document.querySelectorAll('#run-filter-signals input[type="checkbox"]').forEach((c) => {
        c.checked = c.value === 'traces';
      });
      renderRunList();
      const htmlTraces = document.getElementById('run-list').innerHTML;
      const visibleTraces = htmlTraces.includes(target.id);

      document.querySelectorAll('#run-filter-signals input[type="checkbox"]').forEach((c) => {
        c.checked = c.value === 'logs';
      });
      renderRunList();
      const htmlLogs = document.getElementById('run-list').innerHTML;
      const visibleLogs = htmlLogs.includes(target.id);
      const expectHiddenOnLogs =
        !target.has_log_data && !target.log_count && !target.log_count_completed;

      document.querySelectorAll('#run-filter-signals input[type="checkbox"]').forEach((c) => {
        c.checked = true;
      });
      renderRunList();

      return {
        ok: visibleTraces && (!expectHiddenOnLogs || !visibleLogs),
        visibleTraces,
        visibleLogs,
        expectHiddenOnLogs,
        id: target.id,
      };
    });
    expect(filterResult).toBeTruthy();
    expect(filterResult.ok).toBe(true);
    expect(filterResult.visibleTraces).toBe(true);

    const completedId = await page.locator('.run-card.completed').first().evaluate((el) => {
      const idEl = el.querySelector('.id');
      return idEl ? idEl.textContent.split('·')[0].trim() : null;
    });
    expect(completedId).toBeTruthy();

    const detailResponsePromise = page.waitForResponse(
      (res) => res.url().includes('/api/runs/' + completedId) && res.request().method() === 'GET'
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
