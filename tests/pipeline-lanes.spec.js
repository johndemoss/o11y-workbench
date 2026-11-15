const { test, expect } = require('@playwright/test');
const fs = require('node:fs/promises');

const signals = {
  traces: { label: 'Traces', context: 'span', transformKey: 'trace_statements', filterKey: 'span' },
  logs: { label: 'Logs', context: 'log', transformKey: 'log_statements', filterKey: 'log_record' },
  metrics: { label: 'Metrics', context: 'datapoint', transformKey: 'metric_statements', filterKey: 'datapoint' },
};

async function openGraph(page, width, height) {
  await page.setViewportSize({ width, height });
  await page.goto('/pipeline/');
  await expect(page.getByRole('heading', { name: 'Pipeline flow' })).toBeVisible();
}

function lane(page, signal) {
  return page.getByRole('region', { name: `${signals[signal].label} pipeline` });
}

async function expectInLane(page, card, signal) {
  await expect(card).toHaveCount(1);
  await expect.poll(async () => {
    const stageBox = await card.boundingBox();
    const laneBox = await lane(page, signal).boundingBox();
    if (!stageBox || !laneBox) return false;
    const x = stageBox.x + stageBox.width / 2;
    const y = stageBox.y + stageBox.height / 2;
    return x >= laneBox.x && x <= laneBox.x + laneBox.width &&
      y >= laneBox.y && y <= laneBox.y + laneBox.height;
  }).toBe(true);
}

async function assertInspector(page, signal, type, statement) {
  const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
  await expect(inspector.locator('#stage-context')).toHaveText(`${signals[signal].label} / ${signals[signal].context} context`);
  await expect(inspector.getByLabel(type === 'transform' ? 'OTTL statements' : 'OTTL drop conditions')).toHaveValue(statement);
}

async function exportedPipelines(page, request) {
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export YAML', exact: true }).click();
  const download = await downloadPromise;
  const yaml = await fs.readFile(await download.path(), 'utf8');
  const validation = await request.post('/api/ottl/pipeline/validate', {
    headers: { 'Content-Type': 'application/x-yaml' },
    data: yaml,
  });
  expect(validation.ok(), await validation.text()).toBeTruthy();
  const result = await validation.json();
  expect(result.valid, JSON.stringify(result)).toBe(true);
  // Parse the actual downloaded Collector YAML with the real import endpoint.
  const response = await request.post('/api/ottl/pipeline/from-yaml', {
    data: { config_yaml: yaml },
  });
  expect(response.ok(), `Importing exported YAML failed (${response.status()}): ${await response.text()}\n${yaml}`).toBeTruthy();
  return response.json();
}

test('blank stages stay in their selected lanes and export signal-specific processors', async ({ page, request }) => {
  await openGraph(page, 1365, 768);
  const palette = page.locator('[data-od-id="stage-palette"]');
  await expect(page.getByRole('heading', { name: 'Pipeline YAML' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Execute & Results' })).toBeVisible();
  await expect(palette.getByRole('button', { name: 'Add Transform blank stage' })).toBeVisible();
  await expect(page.locator('[data-od-id="signal-graph"] .svelte-flow')).toBeVisible();
  const executionSignal = page.getByRole('radiogroup', { name: 'Signal', exact: true });
  await expect(executionSignal.getByRole('radio', { name: 'traces' })).toHaveAttribute('aria-checked', 'true');

  const created = [];
  for (const [signal, info] of Object.entries(signals)) {
    await palette.getByRole('radiogroup', { name: 'Lane' }).getByRole('radio', { name: info.label }).click();
    for (const type of ['transform', 'filter']) {
      await palette.getByRole('button', { name: `Add ${type === 'transform' ? 'Transform' : 'Filter'} blank stage` }).click();
      const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
      await expect(inspector.locator('#stage-context')).toHaveText(`${info.label} / ${info.context} context`);
      await expect(inspector.locator('#stage-name')).toHaveText(new RegExp(`^${type}/`));
      const ref = await inspector.locator('#stage-name').innerText();
      const card = page.locator('.svelte-flow__node-stage .node-card').filter({ has: page.locator('.node-name', { hasText: ref.split('/').slice(1).join('/') }) });
      await expectInLane(page, card, signal);
      const statement = type === 'transform'
        ? `set(attributes["lane"], "${signal}")`
        : `attributes["lane"] == "${signal}"`;
      await inspector.getByLabel(type === 'transform' ? 'OTTL statements' : 'OTTL drop conditions').fill(statement);
      await assertInspector(page, signal, type, statement);
      created.push({ signal, type, statement, ref, card });
    }
  }

  // Re-select each graph card: editing must not depend on the execution picker.
  for (const stage of created) {
    await stage.card.click();
    await assertInspector(page, stage.signal, stage.type, stage.statement);
  }
  await expect(executionSignal.getByRole('radio', { name: 'traces' })).toHaveAttribute('aria-checked', 'true');

  const pipelines = await exportedPipelines(page, request);
  for (const [signal, info] of Object.entries(signals)) {
    const expected = created.filter(stage => stage.signal === signal);
    expect(pipelines[signal].stages.map(stage => stage.processor_ref)).toEqual(expected.map(stage => stage.ref));
    for (const stage of expected) {
      const processor = pipelines[signal].processors[stage.ref];
      if (stage.type === 'transform') {
        expect(processor[info.transformKey]).toEqual([{ context: info.context, statements: [stage.statement] }]);
        for (const other of Object.values(signals).filter(value => value !== info)) {
          expect(processor[other.transformKey]).toBeUndefined();
        }
      } else {
        expect(processor[signal][info.filterKey]).toEqual([stage.statement]);
        for (const other of Object.keys(signals).filter(value => value !== signal)) {
          expect(processor[other]).toBeUndefined();
        }
      }
    }
  }
});

test('narrow viewport still permits adding and editing a logs stage', async ({ page }) => {
  await openGraph(page, 800, 700);
  const palette = page.locator('[data-od-id="stage-palette"]');
  await palette.getByRole('radiogroup', { name: 'Lane' }).getByRole('radio', { name: 'Logs' }).click();
  await palette.getByRole('button', { name: 'Add Filter blank stage' }).click();
  const statement = 'attributes["narrow"] == "logs"';
  const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
  await expect(inspector.locator('#stage-context')).toHaveText('Logs / log context');
  await inspector.getByLabel('OTTL drop conditions').fill(statement);
  await assertInspector(page, 'logs', 'filter', statement);
  const ref = await inspector.locator('#stage-name').innerText();
  const card = page.locator('.svelte-flow__node-stage .node-card').filter({ has: page.locator('.node-name', { hasText: ref.split('/')[1] }) });
  await expectInLane(page, card, 'logs');
});

test('dropping onto logs and metrics uses the visible lane after zooming', async ({ page, request }) => {
  await openGraph(page, 1365, 768);
  await page.getByRole('button', { name: 'Hide pipeline YAML' }).click();
  await page.getByRole('button', { name: 'Collapse', exact: true }).click();
  const graph = page.locator('[data-od-id="signal-graph"]');
  const palette = page.locator('[data-od-id="stage-palette"]');
  // Keep the palette target at Traces: the drop location must determine the lane.
  for (const signal of ['logs', 'metrics']) {
    await graph.getByRole('button', { name: /fit view/i }).click();
    await graph.getByRole('button', { name: /zoom out/i }).click();
    const target = lane(page, signal);
    const box = await target.boundingBox();
    expect(box).not.toBeNull();
    await palette.getByRole('button', { name: 'Add Transform blank stage' }).dragTo(target, {
      targetPosition: { x: box.width * 0.7, y: box.height * 0.7 },
    });
    const card = graph.locator('.svelte-flow__node-stage .node-card.transform')
      .filter({ hasText: `${signals[signal].context} ·` });
    await expectInLane(page, card, signal);
    await card.click();
    const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
    await expect(inspector.locator('#stage-context')).toHaveText(`${signals[signal].label} / ${signals[signal].context} context`);
    await inspector.getByLabel('OTTL statements').fill(`set(attributes["lane"], "${signal}")`);
  }
  const pipelines = await exportedPipelines(page, request);
  expect(pipelines.logs.stages).toHaveLength(1);
  expect(pipelines.metrics.stages).toHaveLength(1);
  expect(pipelines.traces?.stages ?? []).toHaveLength(0);
});

function favorite(page, name) {
  return page.getByRole('group', { name: `${name} library entry` });
}

async function selectedStage(page, signal, statement) {
  const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
  await assertInspector(page, signal, 'transform', statement);
  const ref = await inspector.locator('#stage-name').innerText();
  const card = page.locator('.svelte-flow__node-stage .node-card')
    .filter({ has: page.locator('.node-name', { hasText: ref.split('/').slice(1).join('/') }) });
  await expectInLane(page, card, signal);
  return { ref, card };
}

function expectTransform(pipelines, signal, ref, statement, context = signals[signal].context) {
  const processor = pipelines[signal].processors[ref];
  expect(processor[signals[signal].transformKey]).toEqual([{ context, statements: [statement] }]);
  for (const other of Object.values(signals).filter(info => info !== signals[signal])) {
    expect(processor[other.transformKey]).toBeUndefined();
  }
}

for (const signal of ['logs', 'metrics']) {
  test(`favorite add and preview target ${signal} and preserve resource context`, async ({ page, request }) => {
    await openGraph(page, 1365, 768);
    const palette = page.locator('[data-od-id="stage-palette"]');
    await palette.getByRole('radiogroup', { name: 'Lane' }).getByRole('radio', { name: signals[signal].label }).click();
    const statement = 'set(attributes["env"], "prod")';
    const entry = favorite(page, 'Set environment attribute');
    await entry.getByRole('button', { name: 'Add', exact: true }).click();
    const added = await selectedStage(page, signal, statement);
    await entry.getByRole('button', { name: 'Preview' }).click();
    const preview = page.getByRole('dialog').filter({ has: page.getByRole('heading', { name: 'Set environment attribute' }) });
    await expect(preview).toContainText(signals[signal].transformKey);
    await preview.getByRole('button', { name: 'Add to lane' }).click();
    const previewed = await selectedStage(page, signal, statement);
    const resourceStatement = 'set(attributes["service.version"], "1.0.0")';
    await favorite(page, 'Add service version').getByRole('button', { name: 'Add', exact: true }).click();
    const resource = await selectedStage(page, signal, resourceStatement);
    const pipelines = await exportedPipelines(page, request);
    expect(pipelines.traces?.stages ?? []).toHaveLength(0);
    expectTransform(pipelines, signal, added.ref, statement);
    expectTransform(pipelines, signal, previewed.ref, statement);
    expectTransform(pipelines, signal, resource.ref, resourceStatement, 'resource');
  });

  test(`favorite drag from traces to ${signal} uses the drop lane after zooming`, async ({ page, request }) => {
    await openGraph(page, 1365, 768);
    await page.getByRole('button', { name: 'Hide pipeline YAML' }).click();
    await page.getByRole('button', { name: 'Collapse', exact: true }).click();
    const graph = page.locator('[data-od-id="signal-graph"]');
    await graph.getByRole('button', { name: /fit view/i }).click();
    await graph.getByRole('button', { name: /zoom out/i }).click();
    const target = lane(page, signal);
    const box = await target.boundingBox();
    expect(box).not.toBeNull();
    await favorite(page, 'Set environment attribute').dragTo(target, {
      targetPosition: { x: box.width * 0.7, y: box.height * 0.7 },
    });
    const card = graph.locator('.svelte-flow__node-stage .node-card.transform');
    await expectInLane(page, card, signal);
    await card.click();
    const statement = 'set(attributes["env"], "prod")';
    const stage = await selectedStage(page, signal, statement);
    const pipelines = await exportedPipelines(page, request);
    expect(pipelines.traces?.stages ?? []).toHaveLength(0);
    expectTransform(pipelines, signal, stage.ref, statement);
  });

  test(`trace-only favorites are unavailable in ${signal}`, async ({ page }) => {
    await openGraph(page, 1365, 768);
    const lanePicker = page.locator('[data-od-id="stage-palette"]').getByRole('radiogroup', { name: 'Lane' });
    const selected = lanePicker.getByRole('radio', { name: signals[signal].label });
    await selected.click();
    for (const name of ['Drop internal spans', 'Drop spans with OK status']) {
      const entry = favorite(page, name);
      await expect(entry.getByRole('button', { name: 'Add', exact: true })).toBeDisabled();
      const previewButton = entry.getByRole('button', { name: 'Preview' });
      if (await previewButton.isEnabled()) {
        await previewButton.click();
        const preview = page.getByRole('dialog').filter({ has: page.getByRole('heading', { name }) });
        await expect(preview.getByRole('button', { name: 'Add to lane' })).toBeDisabled();
        await preview.getByRole('button', { name: 'Cancel' }).click();
      }
      await expect(selected).toHaveAttribute('aria-checked', 'true');
    }
    await expect(page.locator('.svelte-flow__node-stage')).toHaveCount(0);
  });

  test(`selecting a ${signal} node synchronizes additions without changing execution signal`, async ({ page, request }) => {
    await openGraph(page, 1365, 768);
    const palette = page.locator('[data-od-id="stage-palette"]');
    const lanePicker = palette.getByRole('radiogroup', { name: 'Lane' });
    const signalRadio = lanePicker.getByRole('radio', { name: signals[signal].label });
    await signalRadio.click();
    await palette.getByRole('button', { name: 'Add Transform blank stage' }).click();
    const statement = `set(attributes["selected"], "${signal}")`;
    const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
    await inspector.getByLabel('OTTL statements').fill(statement);
    const existing = await selectedStage(page, signal, statement);
    await lanePicker.getByRole('radio', { name: 'Traces' }).click();
    await existing.card.click();
    await expect(signalRadio).toHaveAttribute('aria-checked', 'true');
    await favorite(page, 'Set environment attribute').getByRole('button', { name: 'Add', exact: true }).click();
    const favoriteStage = await selectedStage(page, signal, 'set(attributes["env"], "prod")');
    await palette.getByRole('button', { name: 'Add Transform blank stage' }).click();
    await inspector.getByLabel('OTTL statements').fill(statement);
    await selectedStage(page, signal, statement);
    await expect(page.getByRole('radiogroup', { name: 'Signal', exact: true }).getByRole('radio', { name: 'traces' })).toHaveAttribute('aria-checked', 'true');
    const pipelines = await exportedPipelines(page, request);
    expect(pipelines[signal].stages).toHaveLength(3);
    expect(pipelines.traces?.stages ?? []).toHaveLength(0);
    expectTransform(pipelines, signal, favoriteStage.ref, 'set(attributes["env"], "prod")');
  });

  test(`Pipeline tab edits ${signal} stages with native configuration`, async ({ page, request }) => {
    await openGraph(page, 1365, 768);
    await page.getByRole('navigation', { name: 'Pipeline views' }).getByRole('button', { name: 'Pipeline', exact: true }).click();
    await page.locator('#signal-select').selectOption(signal);
    await page.getByRole('button', { name: '+ Add Stage' }).click();
    const editor = page.locator('[data-od-id="stage-editor-body"]');
    await expect(editor.locator('#stage-context')).toHaveText(`${signals[signal].label} / ${signals[signal].context} context`);
    const statement = `set(attributes["pipeline_tab"], "${signal}")`;
    await editor.getByLabel('OTTL statements').fill(statement);
    const ref = await editor.locator('#stage-name').innerText();
    await page.getByRole('navigation', { name: 'Pipeline views' }).getByRole('button', { name: 'Graph', exact: true }).click();
    const pipelines = await exportedPipelines(page, request);
    expect(pipelines[signal].stages).toHaveLength(1);
    expect(pipelines.traces?.stages ?? []).toHaveLength(0);
    expectTransform(pipelines, signal, ref, statement);
  });
}
