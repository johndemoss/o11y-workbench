import { test, expect } from '@playwright/test'
import { addTransformStages, installGraphApiFallback, openGraphBuilder, tracesStageOrder } from './helpers'

test.describe('Flow builder graph edits', () => {
  test.beforeEach(async ({ page }) => {
    await installGraphApiFallback(page)
    await openGraphBuilder(page)
  })

  test('delete middle stage preserves left-to-right order of survivors', async ({ page }) => {
    await addTransformStages(page, 3)
    const before = await tracesStageOrder(page)
    expect(before).toHaveLength(3)

    const middle = page.locator('.svelte-flow__node-stage').nth(1)
    await middle.getByRole('button', { name: 'Delete stage' }).click()
    await page.getByRole('button', { name: 'Yes' }).click()

    await expect(page.locator('.svelte-flow__node-stage')).toHaveCount(2)
    const after = await tracesStageOrder(page)
    expect(after).toEqual([before[0], before[2]])
  })

  test('move-right control reorders stages on the canvas', async ({ page }) => {
    await addTransformStages(page, 2)
    const before = await tracesStageOrder(page)
    await page
      .locator(`[data-id="${before[0]}"]`)
      .getByRole('button', { name: 'Move stage right' })
      .click()
    await expect
      .poll(async () => tracesStageOrder(page))
      .toEqual([before[1], before[0]])
  })

  test('horizontal drag reorders a stage within its swimlane', async ({ page }) => {
    await addTransformStages(page, 2)
    const before = await tracesStageOrder(page)
    const dragged = page.locator(`[data-id="${before[0]}"]`)
    const box = await dragged.boundingBox()
    if (!box) throw new Error('missing stage bounding box')

    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width / 2 + 240, box.y + box.height / 2, { steps: 16 })
    await page.mouse.up()

    await expect
      .poll(async () => tracesStageOrder(page))
      .toEqual([before[1], before[0]])
  })

  test('rejected cycle connections surface feedback', async ({ page }) => {
    await addTransformStages(page, 2)
    const order = await tracesStageOrder(page)
    expect(order).toHaveLength(2)

    const source = await page.locator(`[data-id="${order[1]}"] .svelte-flow__handle-right`).boundingBox()
    const target = await page.locator(`[data-id="${order[0]}"] .svelte-flow__handle-left`).boundingBox()
    if (!source || !target) throw new Error('missing connection handles')
    await page.mouse.move(source.x + source.width / 2, source.y + source.height / 2)
    await page.mouse.down()
    await page.mouse.move(source.x + source.width / 2 - 30, source.y + source.height / 2, { steps: 5 })
    await page.mouse.move(target.x + target.width / 2, target.y + target.height / 2, { steps: 12 })
    await page.mouse.up()

    await expect(page.getByRole('alert').filter({ hasText: 'Cannot' })).toBeVisible()
  })

  test('topological sort failures restore the graph and alert', async ({ page }) => {
    await page.route('**/api/ottl/pipeline/topological-sort', (route) =>
      route.fulfill({ status: 500, body: 'sort unavailable' }),
    )

    await addTransformStages(page, 2)
    const before = await tracesStageOrder(page)

    await page
      .locator(`[data-id="${before[0]}"]`)
      .getByRole('button', { name: 'Move stage right' })
      .click()

    await expect(page.getByRole('alert').filter({ hasText: 'Could not update pipeline graph' })).toBeVisible()
    await expect.poll(async () => tracesStageOrder(page)).toEqual(before)
  })

  test('network-level sort failure restores the graph and alert', async ({ page }) => {
    await addTransformStages(page, 2)
    const before = await tracesStageOrder(page)

    await page.route('**/api/ottl/pipeline/topological-sort', (route) => route.abort('failed'))

    await page
      .locator(`[data-id="${before[0]}"]`)
      .getByRole('button', { name: 'Move stage right' })
      .click()

    await expect(page.getByRole('alert').filter({ hasText: 'Could not update pipeline graph' })).toBeVisible()
    await expect.poll(async () => tracesStageOrder(page)).toEqual(before)
  })
})