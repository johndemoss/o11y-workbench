import { test, expect, type Page } from '@playwright/test'

const TEST_RUNS = [{ id: 'run-1', status: 'completed', span_count: 10 }]

const VALIDATE_RESPONSE = { valid: true }

const EMPTY_PIPELINE = { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }

const IMPORT_RESPONSE = {
  traces: {
    stages: [{ id: 'config-stage-1', type: 'transform', processor_ref: 'proc-config-1' }],
    processors: { 'proc-config-1': {} },
    receivers: ['otlp'],
    exporters: ['debug'],
  },
  logs: EMPTY_PIPELINE,
  metrics: EMPTY_PIPELINE,
}

const EXECUTE_RESPONSE = {
  stages: [
    {
      stage_index: 0,
      stage_id: 'stage-1',
      input_span_count: 10,
      output_span_count: 8,
      statements_parsed: 2,
      errors: [],
      duration_ns: 1500,
    },
  ],
  final_span_count: 8,
  final_data: { resourceSpans: [] },
}

async function openPipelineBuilder(page: Page) {
  await page.goto('/')
  await page
    .getByRole('navigation', { name: 'Pipeline views' })
    .getByRole('button', { name: 'Pipeline', exact: true })
    .click()
  await expect(page.locator('.pipeline-builder')).toBeVisible()
}

async function addStages(page: Page, count: number) {
  for (let i = 0; i < count; i++) {
    await page.locator('.btn-add-stage').click()
  }
}

async function simulateDragReorder(page: Page) {
  await page.evaluate(() => {
    const source = document.querySelector('.stage-card-wrapper:first-child')
    const target = document.querySelector('.stage-card-wrapper:nth-child(2)')
    if (!source || !target) return
    source.dispatchEvent(new DragEvent('dragstart', { bubbles: true }))
    target.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true }))
    target.dispatchEvent(new DragEvent('drop', { bubbles: true }))
  })
}

test.describe('Pipeline Prototyper', () => {
  test('Pipeline Builder loads', async ({ page }) => {
    await openPipelineBuilder(page)
    await expect(page.locator('[data-od-id="pipeline-title"]')).toHaveText('Pipeline Prototyper')
  })

  test('Add and remove stages', async ({ page }) => {
    await openPipelineBuilder(page)
    await expect(page.locator('.stage-card-wrapper')).toHaveCount(0)

    await addStages(page, 1)
    await expect(page.locator('.stage-card-wrapper')).toHaveCount(1)

    await page.locator('.btn-delete').click()
    await page.locator('button', { hasText: 'Yes' }).click()
    await expect(page.locator('.stage-card-wrapper')).toHaveCount(0)
  })

  test('Drag and reorder stages', async ({ page }) => {
    await openPipelineBuilder(page)
    await addStages(page, 2)
    await expect(page.locator('.stage-card-wrapper')).toHaveCount(2)

    const firstId = await page.locator('.stage-name').first().textContent()
    const secondId = await page.locator('.stage-name').nth(1).textContent()

    await simulateDragReorder(page)

    await expect(page.locator('\.stage-name').first()).toHaveText(secondId!)
    await expect(page.locator('\.stage-name').nth(1)).toHaveText(firstId!)
  })

  test('Validate pipeline', async ({ page }) => {
    await page.route(/\/api\/ottl\/pipeline\/validate/, async (route) => {
      await route.fulfill({ body: JSON.stringify(VALIDATE_RESPONSE) })
    })

    await openPipelineBuilder(page)
    await addStages(page, 1)

    const dialogPromise = page.waitForEvent('dialog')
    await page.locator('.btn-primary').click()
    const dialog = await dialogPromise
    expect(dialog.message()).toContain('Pipeline is valid')
    await dialog.accept()
  })

  test('Import config populates pipeline', async ({ page }) => {
    await page.route(/\/api\/ottl\/pipeline\/import-config/, async (route) => {
      await route.fulfill({ body: JSON.stringify(IMPORT_RESPONSE) })
    })

    await openPipelineBuilder(page)
    await page.locator('.btn-secondary').click()
    await expect(page.locator('.config-import')).toBeVisible()

    await page.locator('.config-import .import-btn').click()
    await expect(page.locator('.svelte-flow__node-stage')).toHaveCount(1)
  })

  test('Execute pipeline against a run', async ({ page }) => {
    await page.route(/\/api\/runs/, async (route) => {
      await route.fulfill({ body: JSON.stringify(TEST_RUNS) })
    })
    await page.route(/\/api\/ottl\/pipeline\/execute/, async (route) => {
      await route.fulfill({ body: JSON.stringify(EXECUTE_RESPONSE) })
    })

    await openPipelineBuilder(page)
    await addStages(page, 1)

    await page.locator('.execute-btn').click()
    await expect(page.locator('#execute-run option')).toHaveCount(1)

    await page.locator('.execute-btn').click()
    await page.getByRole('tab', { name: 'Final result' }).click()
    await expect(page.locator('.results-view')).toContainText('Pipeline output')
    await expect(page.locator('.results-view')).toContainText('8 spans')
  })
})
