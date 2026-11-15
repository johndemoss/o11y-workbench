const { test, expect } = require('@playwright/test');

const viewports = [
  { name: 'desktop', width: 1365, height: 768 },
  { name: 'narrow', width: 390, height: 640 },
  { name: 'short', width: 844, height: 390 },
];

function entry(page, name) {
  return page.getByRole('group', { name: `${name} library entry` });
}

async function assertInViewport(locator, page) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(box.x).toBeGreaterThanOrEqual(8);
  expect(box.y).toBeGreaterThanOrEqual(8);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width - 8);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height - 8);
}

async function assertPreviewBounds(dialog, page) {
  await assertInViewport(dialog, page);
  await assertInViewport(dialog.getByRole('button', { name: 'Cancel' }), page);
  await assertInViewport(dialog.getByRole('button', { name: 'Add to lane' }), page);
}

for (const viewport of viewports) {
  for (const name of ['Keep production tags (lambda)', 'Drop internal spans']) {
    test(`${name} preview fits the ${viewport.name} viewport and restores focus`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await page.goto('/pipeline/');
      await expect(page.getByRole('heading', { name: 'Pipeline flow' })).toBeVisible();

      const previewButton = entry(page, name).getByRole('button', { name: 'Preview' });
      await previewButton.click();
      const dialog = page.getByRole('dialog', { name, exact: true });
      await expect(dialog).toHaveAttribute('data-od-id', 'template-preview');
      await assertPreviewBounds(dialog, page);

      // The underlying library cannot receive focus while the modal is open.
      await previewButton.evaluate(button => button.focus());
      await expect(previewButton).not.toBeFocused();
      await dialog.getByRole('button', { name: 'Cancel' }).focus();
      await page.keyboard.press('Tab');
      await expect.poll(() => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);

      await page.keyboard.press('Escape');
      await expect(dialog).not.toBeVisible();
      await expect(previewButton).toBeFocused();
      await previewButton.click();
      await assertPreviewBounds(dialog, page);
      await dialog.getByRole('button', { name: 'Cancel' }).click();
      await expect(dialog).not.toBeVisible();
      await expect(previewButton).toBeFocused();
    });
  }
}

test('long preview scrolls while its actions stay visible', async ({ page }) => {
  await page.setViewportSize({ width: 844, height: 390 });
  await page.goto('/pipeline/');
  await entry(page, 'Rename attribute').getByRole('button', { name: 'Preview' }).click();
  const dialog = page.getByRole('dialog', { name: 'Rename attribute', exact: true });
  await assertPreviewBounds(dialog, page);

  // Find the user-scrollable region, without depending on its styling class.
  const scroller = dialog.locator('*').filter({ hasText: 'delete_key' });
  const scrollTarget = await scroller.evaluateAll(elements => {
    const element = elements.find(candidate => {
      const style = getComputedStyle(candidate);
      return /(auto|scroll)/.test(style.overflowY) && candidate.scrollHeight > candidate.clientHeight + 1;
    });
    if (!element) return null;
    const rect = element.getBoundingClientRect();
    return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
  });
  expect(scrollTarget).not.toBeNull();
  await page.mouse.move(scrollTarget.x, scrollTarget.y);
  await page.mouse.wheel(0, 500);
  await expect.poll(() => scroller.evaluateAll(elements => elements.some(element => element.scrollTop > 0))).toBe(true);
  await assertPreviewBounds(dialog, page);
});

test('clicking the backdrop dismisses without adding a stage', async ({ page }) => {
  await page.setViewportSize({ width: 1365, height: 768 });
  await page.goto('/pipeline/');
  const previewButton = entry(page, 'Drop internal spans').getByRole('button', { name: 'Preview' });
  await previewButton.click();
  const dialog = page.getByRole('dialog', { name: 'Drop internal spans', exact: true });
  await expect(dialog).toBeVisible();
  await page.mouse.click(2, 2);
  await expect(dialog).not.toBeVisible();
  await expect(previewButton).toBeFocused();
  await expect(page.locator('.svelte-flow__node-stage')).toHaveCount(0);
});

test('preview adds a transform to the selected Logs lane', async ({ page }) => {
  await page.setViewportSize({ width: 1365, height: 768 });
  await page.goto('/pipeline/');
  await page.locator('[data-od-id="stage-palette"]').getByRole('radiogroup', { name: 'Lane' })
    .getByRole('radio', { name: 'Logs' }).click();
  await entry(page, 'Drop internal spans').getByRole('button', { name: 'Preview' }).click();
  const unsupported = page.getByRole('dialog', { name: 'Drop internal spans', exact: true });
  await expect(unsupported.getByRole('button', { name: 'Add to lane' })).toBeDisabled();
  await unsupported.getByRole('button', { name: 'Cancel' }).click();
  await entry(page, 'Rename attribute').getByRole('button', { name: 'Preview' }).click();
  const dialog = page.getByRole('dialog', { name: 'Rename attribute', exact: true });
  await dialog.getByRole('button', { name: 'Add to lane' }).click();
  await expect(dialog).not.toBeVisible();

  const inspector = page.locator('[data-od-id="persistent-stage-inspector"]');
  await expect(inspector.locator('#stage-context')).toHaveText('Logs / log context');
  await expect(inspector.getByLabel('OTTL statements')).toHaveValue(
    'set(attributes["new_name"], attributes["old_name"]) where attributes["old_name"] != nil\ndelete_key(attributes, "old_name")',
  );
  const card = page.locator('.svelte-flow__node-stage .node-card.transform');
  await expect(card).toHaveCount(1);
  await expect.poll(async () => {
    const node = await card.boundingBox();
    const lane = await page.getByRole('region', { name: 'Logs pipeline' }).boundingBox();
    if (!node || !lane) return false;
    const x = node.x + node.width / 2;
    const y = node.y + node.height / 2;
    return x >= lane.x && x <= lane.x + lane.width && y >= lane.y && y <= lane.y + lane.height;
  }).toBe(true);
});
