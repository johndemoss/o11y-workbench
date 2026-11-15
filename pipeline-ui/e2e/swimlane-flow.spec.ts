import { test, expect } from '@playwright/test'
import { installGraphApiFallback, openGraphBuilder } from './helpers'

test.describe('Swimlane flow layout', () => {
  test.beforeEach(async ({ page }) => {
    await installGraphApiFallback(page)
    await openGraphBuilder(page)
  })

  test('swimlane regions are exposed to assistive labels', async ({ page }) => {
    await expect(page.getByRole('region', { name: 'Traces pipeline' })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Logs pipeline' })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Metrics pipeline' })).toBeVisible()
  })

  test('multiple infrastructure cards remain vertically separated', async ({ page }) => {
    await page.route('**/api/ottl/pipeline/import-config', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          traces: {
            stages: [{ id: 'imp-1', type: 'transform', processor_ref: 'transform/imp' }],
            processors: { 'transform/imp': {} },
            receivers: ['alpha', 'beta'],
            exporters: ['gamma', 'delta', 'epsilon'],
            receiver_configs: { alpha: { token: 'a' }, beta: { token: 'b' } },
            exporter_configs: { gamma: { q: 1 }, delta: {}, epsilon: { z: 3 } },
          },
          logs: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
          metrics: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
        }),
      })
    })

    await page.getByRole('button', { name: 'Import', exact: true }).first().click()
    await page.getByRole('button', { name: /^Import$/ }).click()

    await expect(page.locator('[data-id="infra-traces-receiver-alpha"]')).toBeVisible()
    await expect(page.locator('[data-id="infra-traces-receiver-beta"]')).toBeVisible()
    await expect(page.locator('[data-id="infra-traces-exporter-gamma"]')).toBeVisible()

    const tops = await page.locator('[data-id^="infra-traces-"]').evaluateAll((nodes) =>
      nodes.map((node) => Math.round(node.getBoundingClientRect().top)),
    )
    expect(new Set(tops).size).toBe(tops.length)

    const boxes = await page.locator('[data-id^="infra-traces-receiver-"]').evaluateAll((nodes) =>
      nodes.map((node) => node.getBoundingClientRect()),
    )
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        const a = boxes[i]
        const b = boxes[j]
        const overlap = !(a.bottom <= b.top || b.bottom <= a.top)
        expect(overlap).toBe(false)
      }
    }
  })

  test('stage palette remains keyboard-accessible alongside the graph', async ({ page }) => {
    await expect(page.locator('[data-od-id="stage-palette"]')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Add Transform blank stage' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Add Filter blank stage' })).toBeVisible()
    await expect(page.locator('[data-od-id="library-search"]')).toBeVisible()
  })
})