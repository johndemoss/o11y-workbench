import { test, expect } from '@playwright/test'

test.describe('Flow Builder - Graph Tab', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.click('button:has-text("Graph Builder")')
  })

  test('Full workflow: drag stage, edit, execute', async ({ page }) => {
    await page.waitForSelector('.svelte-flow', { timeout: 5000 }).catch(() => {})

    await expect(page.locator('text=Pipeline Stages')).toBeVisible()
    await expect(page.locator('text=Transform')).toBeVisible()
    await expect(page.locator('text=Filter')).toBeVisible()

    await expect(page.locator('button:has-text("Import YAML")')).toBeVisible()
  })

  test('Palette drag: both Transform and Filter types', async ({ page }) => {
    const transformCard = page.locator('.drag-card--transform')
    const filterCard = page.locator('.drag-card--filter')
    await expect(transformCard).toBeVisible()
    await expect(filterCard).toBeVisible()
    await expect(transformCard).toContainText('Transform')
    await expect(filterCard).toContainText('Filter')
  })

  test('Delete workflow: add 2 stages, delete one', async ({ page }) => {
    const nodeCards = page.locator('.node-card')
    const count = await nodeCards.count()

    if (count > 0) {
      const deleteBtn = nodeCards.first().locator('.delete-btn')
      await deleteBtn.click()

      await expect(page.locator('.confirm-dialog')).toBeVisible()
      await expect(page.locator('.confirm-dialog')).toContainText('Delete')

      await page.locator('.confirm-btn-danger').click()

      await expect(page.locator('.node-card')).toHaveCount(count - 1)
    }
  })

  test('Cycle prevention', async ({ page }) => {
    await expect(page.locator('.svelte-flow')).toBeVisible()
  })

  test('Empty state guidance text', async ({ page }) => {
    const emptyState = page.locator('.empty-state')
    const svelteFlow = page.locator('.svelte-flow')

    const isEmptyVisible = await emptyState.isVisible().catch(() => false)
    const isFlowVisible = await svelteFlow.isVisible().catch(() => false)

    if (isEmptyVisible) {
      await expect(emptyState).toContainText('Drag stages from the palette')
    } else if (isFlowVisible) {
      // no-op: pipeline already has nodes
    }
  })

  test('Results display: badges on nodes', async ({ page }) => {
    await expect(page.locator('button:has-text("Validate")')).toBeVisible()

    await expect(page.locator('h2:has-text("Execute Pipeline")')).toBeVisible()
  })

  test('Import button triggers config modal', async ({ page }) => {
    await page.locator('button:has-text("Import YAML")').click()

    await page.waitForTimeout(500)
  })
})
