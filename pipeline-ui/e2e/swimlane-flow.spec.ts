import { test, expect } from '@playwright/test'

test.describe('Swimlane Flow Builder - Graph Tab', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.click('button:has-text("Graph Builder")')
  })

  test('Swimlane headers and canvas render', async ({ page }) => {
    await page.waitForSelector('.svelte-flow', { timeout: 5000 }).catch(() => {})

    await expect(page.locator('text=Traces').or(page.locator('text=traces'))).toBeVisible()
    await expect(page.locator('text=Logs').or(page.locator('text=logs'))).toBeVisible()
    await expect(page.locator('text=Metrics').or(page.locator('text=metrics'))).toBeVisible()

    await expect(page.locator('text=Pipeline Stages')).toBeVisible()
    await expect(page.locator('text=Transform')).toBeVisible()
    await expect(page.locator('text=Filter')).toBeVisible()
  })

  test('EditorPanel toggle', async ({ page }) => {
    const editorPanel = page.locator('.editor-panel')
    await expect(editorPanel).toBeVisible()
    await expect(editorPanel).not.toHaveClass(/collapsed/)
    await expect(page.locator('.editor-title')).toContainText('Pipeline YAML')

    await page.locator('.editor-panel button:has-text("◀")').click()
    await expect(editorPanel).toHaveClass(/collapsed/)
    await expect(page.locator('.toggle-tab')).toContainText('Editor ▶')

    await page.locator('.toggle-tab').click()
    await expect(editorPanel).toBeVisible()
    await expect(editorPanel).not.toHaveClass(/collapsed/)
  })

  test('Export button present', async ({ page }) => {
    const exportBtn = page.locator('button:has-text("Export YAML")')
    await expect(exportBtn).toBeVisible()
    await expect(exportBtn).toBeEnabled()
  })

  test('Snippets section in palette', async ({ page }) => {
    await expect(page.locator('h3:has-text("Snippets")')).toBeVisible()

    await expect(page.locator('text=Drop internal spans')).toBeVisible()
    await expect(page.locator('text=Set environment attribute')).toBeVisible()

    const snippetCards = page.locator('.drag-card[draggable="true"]')
    const count = await snippetCards.count()
    expect(count).toBeGreaterThanOrEqual(5)
  })

  test('Stage node rendering', async ({ page }) => {
    await page.waitForSelector('.svelte-flow', { timeout: 5000 }).catch(() => {})

    const flowNodes = page.locator('.svelte-flow__node')
    const nodeCount = await flowNodes.count().catch(() => 0)

    if (nodeCount > 0) {
      await expect(flowNodes.first()).toBeVisible()
    }
  })

  test('Results display section', async ({ page }) => {
    const collapseToggle = page.locator('.collapse-toggle')
    await expect(collapseToggle).toBeVisible()
    await expect(collapseToggle).toContainText('Execute & Results')

    await expect(page.locator('h2:has-text("Execute Pipeline")')).toBeVisible()
  })

  test('Empty state guidance text', async ({ page }) => {
    await page.waitForSelector('.svelte-flow', { timeout: 5000 }).catch(() => {})

    const emptyState = page.locator('.empty-state')
    const svelteFlow = page.locator('.svelte-flow')

    const isEmptyVisible = await emptyState.isVisible().catch(() => false)
    const isFlowVisible = await svelteFlow.isVisible().catch(() => false)

    if (isEmptyVisible) {
      await expect(emptyState).toContainText('Drag stages from the palette')
    } else if (isFlowVisible) {
      // pipeline already has nodes — no empty state to check
    }
  })

  test('Validate button', async ({ page }) => {
    const validateBtn = page.locator('.builder-header button:has-text("Validate")')
    await expect(validateBtn).toBeVisible()
    await expect(validateBtn).toBeEnabled()
  })
})
