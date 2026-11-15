const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const MALICIOUS_TEXT = '<img src=x onerror=alert(1)>';

const SCENARIO_JSON = {
  name: MALICIOUS_TEXT,
  roots: [{ service_name: 'web', name: 'request', kind: 'INTERNAL' }],
};

function scanInlineHandlerAttrs(page) {
  return page.evaluate(() => {
    const names = [];
    document.querySelectorAll('*').forEach((el) => {
      for (const attr of el.attributes) {
        if (attr.name.toLowerCase().startsWith('on')) names.push(attr.name);
      }
    });
    return names;
  });
}

async function installCspViolationCollector(page) {
  // addInitScript runs in every new document, and the count lives in
  // localStorage so it survives navigation within the origin.
  await page.addInitScript(() => {
    const KEY = 'cspViolationProbe';
    if (window.localStorage.getItem(KEY) === null) {
      window.localStorage.setItem(KEY, '0');
    }
    document.addEventListener('securitypolicyviolation', (e) => {
      const next = Number(window.localStorage.getItem(KEY) || '0') + 1;
      window.localStorage.setItem(KEY, String(next));
      window.__cspViolations = window.__cspViolations || [];
      window.__cspViolations.push({
        violatedDirective: e.violatedDirective,
        blockedURI: e.blockedURI,
        disposition: e.disposition,
      });
    });
  });
}

async function readCspViolations(page) {
  const count = await page.evaluate(() => Number(window.localStorage.getItem('cspViolationProbe') || '0'));
  return new Array(count).fill(null);
}

async function prepareTraceRun(page) {
  await page.evaluate(() => {
    document.querySelectorAll('#signal-type input[type="checkbox"]').forEach((c) => {
      c.checked = c.value === 'traces';
    });
    onSignalCheckboxChange('traces');
    const area = document.getElementById('trace-config-area');
    if (area) area.classList.remove('collapsed');
    const cnt = document.getElementById('trace-count');
    if (cnt) cnt.value = '1';
    updateInfiniteIndicator('trace-count');
    syncTraceCountDuration('trace-count');
  });
}

test.describe('Dashboard HTML injection + CSP', () => {
  test.beforeEach(async ({ page }) => {
    await installCspViolationCollector(page);
    await page.goto('/');
    await expect(page.locator('#run-btn')).toBeVisible();
  });

  test('job cards distinguish retained records from progress, including zero', async ({ page }) => {
    const stats = await page.evaluate(() => {
      const config = { signals: { traces: true, logs: true, metrics: true } };
      return [
        { id: 'completed', status: 'completed', config, log_count: 1, log_record_count: 36, metric_record_count: 0 },
        { id: 'failed', status: 'failed', config },
        { id: 'stopped', status: 'stopped', config, log_record_count: 0, metric_record_count: 7 },
        { id: 'running', status: 'running', config, log_count: 90, metric_count: 80, log_count_completed: 2 },
      ].map(run => Array.from(buildRunCard(run).querySelectorAll('.stats span'), el => el.textContent));
    });
    expect(stats[0]).toContain('36 log records');
    expect(stats[0]).toContain('0 metric records');
    expect(stats[1]).toContain('0 log records');
    expect(stats[1]).toContain('0 metric records');
    expect(stats[2]).toContain('0 log records');
    expect(stats[2]).toContain('7 metric records');
    expect(stats[3]).toContain('2 logs (progress)');
    expect(stats[3]).toContain('0 metrics (progress)');
  });

  test('scenario upload, run list, failed detail, and attrs resist injection', async ({ page, request }) => {
    const apiRes = await request.post('/api/scenarios', {
      headers: { 'Content-Type': 'application/json' },
      data: SCENARIO_JSON,
    });
    expect(apiRes.ok()).toBeTruthy();
    const body = await apiRes.json();
    expect(body.registered).toContain(MALICIOUS_TEXT);
    await page.evaluate(() => loadScenarios());

    const tmpFile = path.join(os.tmpdir(), `o11y-scenario-${Date.now()}.json`);
    fs.writeFileSync(tmpFile, JSON.stringify(SCENARIO_JSON), 'utf8');
    try {
      await page.click('#open-scenario-modal-btn');
      await page.setInputFiles('#scenario-file-input', tmpFile);
      await page.click('#scenario-upload-btn');
      await expect(page.locator('#scenario-upload-status')).toContainText(MALICIOUS_TEXT, {
        timeout: 15000,
      });
      const statusHtml = await page.locator('#scenario-upload-status').innerHTML();
      // The payload must appear as escaped text, with no element or event
      // handler attribute created from it.
      expect(statusHtml).toContain('&lt;img');
      const statusProbe = await page.evaluate(() => {
        const el = document.getElementById('scenario-upload-status');
        return {
          imgCount: el.querySelectorAll('img').length,
          handlers: Array.from(el.querySelectorAll('*')).flatMap((node) =>
            Array.from(node.attributes).map((attr) => attr.name).filter((name) => name.startsWith('on'))
          ),
        };
      });
      expect(statusProbe.imgCount).toBe(0);
      expect(statusProbe.handlers).toEqual([]);
    } finally {
      fs.unlinkSync(tmpFile);
    }

    await page.evaluate((name) => {
      const sel = document.getElementById('scenario');
      if (sel && !Array.from(sel.options).some((o) => o.value === name)) {
        const opt = document.createElement('option');
        opt.value = name;
        opt.textContent = name;
        sel.appendChild(opt);
      }
      if (sel) {
        sel.value = name;
        handleScenarioChange(sel);
      }
    }, MALICIOUS_TEXT);

    await prepareTraceRun(page);
    await page.click('#run-btn');
    await expect(page.locator('.run-card').first()).toContainText(MALICIOUS_TEXT, { timeout: 30000 });
    const listProbe = await page.evaluate(() => {
      const list = document.getElementById('run-list');
      if (!list) return { text: '', imgCount: 0, handlers: ['missing-run-list'] };
      return {
        text: list.textContent,
        imgCount: list.querySelectorAll('img').length,
        handlers: Array.from(list.querySelectorAll('*')).flatMap((node) =>
          Array.from(node.attributes).map((attr) => attr.name).filter((name) => name.startsWith('on'))
        ),
      };
    });
    expect(listProbe.text).toContain(MALICIOUS_TEXT);
    expect(listProbe.imgCount).toBe(0);
    expect(listProbe.handlers).toEqual([]);

    const failedScenario = MALICIOUS_TEXT + '-missing';
    const failedRes = await request.post('/api/runs', {
      headers: { 'Content-Type': 'application/json' },
      data: {
        seed: 42,
        output: 'file',
        format: 'json',
        signals: { traces: true, logs: false, metrics: false },
        trace_count: 1,
        rate: 1,
        duration: 10,
        topology: 'flat',
        scenario: failedScenario,
        span_count: 10,
        max_depth: 5,
        service_name: 'mock-service',
      },
    });
    expect(failedRes.ok()).toBeTruthy();
    const failedRun = await failedRes.json();
    await expect
      .poll(async () => {
        const j = await request.get(`/api/runs/${failedRun.id}`);
        if (!j.ok()) return '';
        const r = await j.json();
        return r.status;
      }, { timeout: 30000 })
      .toMatch(/failed|stopped/);

    await page.evaluate(async (id) => {
      const res = await fetch('/api/runs');
      const data = await res.json();
      (data.runs || []).forEach((r) => {
        runs[r.id] = mergeListRun(runs[r.id], r);
      });
      renderRunList();
      await showDetail(id);
    }, failedRun.id);

    const detailProbe = await page.evaluate((text) => {
      const err = document.querySelector('#detail-content .error');
      return {
        errText: err ? err.textContent : '',
        errHtmlHasMarkup: err ? /<\w+/.test(err.innerHTML) : false,
        imgCount: document.querySelector('#detail-content')?.querySelectorAll('img').length || 0,
        needle: text,
      };
    }, failedScenario);
    expect(detailProbe.errText).toContain(detailProbe.needle);
    expect(detailProbe.errHtmlHasMarkup).toBe(false);
    expect(detailProbe.imgCount).toBe(0);

    await page.evaluate(() => {
      document.querySelectorAll('#signal-type input[type="checkbox"]').forEach((c) => {
        c.checked = c.value === 'logs' || c.value === 'metrics';
      });
      onSignalCheckboxChange('logs');
      const logArea = document.getElementById('log-config-area');
      if (logArea) logArea.classList.remove('collapsed');
      const metricArea = document.getElementById('metric-config-area');
      if (metricArea) metricArea.classList.remove('collapsed');
    });

    const attrNeedleKey = '"key<>';
    const attrNeedleVal = "'val&<>";
    await page.evaluate(({ key, value }) => {
      addLogAttr(key, value);
      addMetricAttr(key, value);
    }, { key: attrNeedleKey, value: attrNeedleVal });

    await expect(page.locator('#attr-body .attr-key').last()).toHaveValue(attrNeedleKey);
    await expect(page.locator('#attr-body .attr-val').last()).toHaveValue(attrNeedleVal);
    await expect(page.locator('#metric-attr-body .attr-key').last()).toHaveValue(attrNeedleKey);
    await expect(page.locator('#metric-attr-body .attr-val').last()).toHaveValue(attrNeedleVal);

    const severity = '<img src=x onerror=alert(1)>';
    const severityProbe = await page.evaluate((text) => {
      const container = document.createElement('div');
      container.innerHTML = renderLogTable({
        resourceLogs: [{ scopeLogs: [{ logRecords: [{ severityText: text }] }] }],
      });
      return {
        text: container.querySelector('.severity-tag')?.textContent,
        images: container.querySelectorAll('img').length,
      };
    }, severity);
    expect(severityProbe.text).toBe(severity);
    expect(severityProbe.images).toBe(0);

    const inlineHandlers = await scanInlineHandlerAttrs(page);
    expect(inlineHandlers).toEqual([]);
  });

  test('ordinary interactions, pipeline execute, and CSP enforcement', async ({ page, request }) => {
    await prepareTraceRun(page);
    await page.click('#run-btn');
    await expect(page.locator('.run-card.completed').first()).toBeVisible({ timeout: 30000 });

    await page.click('#open-scenario-modal-btn');
    await expect(page.locator('#scenario-modal-overlay')).toHaveClass(/open/);
    await page.locator('#scenario-modal-overlay .modal-close').click();

    const rtConnect = page.locator('#remotetap-connect-btn');
    await rtConnect.click();
    await expect(rtConnect).toBeEnabled();
    const rtDisconnect = page.locator('#remotetap-disconnect-btn');
    if (await rtDisconnect.isVisible()) {
      await rtDisconnect.click();
    }

    const violationsBefore = await readCspViolations(page);
    // Ordinary dashboard use must not violate the policy.
    expect(violationsBefore.length).toBe(0);

    const pipelineRes = await page.goto('/pipeline/');
    expect(pipelineRes && pipelineRes.ok()).toBeTruthy();
    await expect(page.locator('.app-logo')).toBeVisible();
    await page.getByRole('button', { name: 'Quick' }).click();
    await expect(page.locator('[data-od-id="execution-controls"]')).toBeVisible({ timeout: 15000 });

    const runOptions = await page.locator('#execute-run option').allTextContents();
    expect(runOptions.some((t) => /completed/i.test(t))).toBeTruthy();
    await page.evaluate(() => {
      const sel = document.getElementById('execute-run');
      if (!sel) throw new Error('execute-run missing');
      const opt = Array.from(sel.options).find((o) => /completed/i.test(o.textContent || ''));
      if (!opt || !opt.value) throw new Error('no completed run option');
      sel.value = opt.value;
      sel.dispatchEvent(new Event('change', { bubbles: true }));
    });

    await page.locator('#statements').fill('set(attributes["pipeline"], "applied")');
    await page.locator('.execute-btn').click();
    await expect(page.locator('#freshness')).toContainText(/Executed/i, { timeout: 30000 });
    await expect(page.locator('[data-od-id="stage-results"]')).toContainText(/stage/i);
    // Loading and using the pipeline SPA must not violate the policy either.
    expect((await readCspViolations(page)).length).toBe(0);

    await page.goto('/');
    const enforcement = await page.evaluate(() => {
      window.__cspProbeRan = false;
      window.__cspInlineHit = false;
      const btn = document.createElement('button');
      btn.setAttribute('onclick', 'window.__cspInlineHit=true');
      document.body.appendChild(btn);
      btn.click();
      const script = document.createElement('script');
      script.src = 'data:text/javascript,window.__cspScriptHit=true';
      document.body.appendChild(script);
      window.__cspProbeRan = true;
      const result = {
        inlineHit: !!window.__cspInlineHit,
        scriptHit: !!window.__cspScriptHit,
      };
      btn.remove();
      script.remove();
      return result;
    });
    expect(enforcement.inlineHit).toBe(false);
    expect(enforcement.scriptHit).toBe(false);

    const violationsAfter = await readCspViolations(page);
    expect(violationsAfter.length).toBeGreaterThan(violationsBefore.length);

    const inlineHandlers = await scanInlineHandlerAttrs(page);
    expect(inlineHandlers).toEqual([]);
  });
});