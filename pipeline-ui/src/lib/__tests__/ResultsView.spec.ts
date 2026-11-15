import { test, expect } from '@playwright/experimental-ct-svelte'
import type { Page, Route } from '@playwright/test'
import ResultsView from '../ResultsView.svelte'
import App from '../../App.svelte'

const runs = [
  { id: 'A', status: 'completed', span_count: 1 },
  { id: 'B', status: 'completed', span_count: 1 },
]
const execution = {
  stages: [{
    stage_index: 0, stage_id: 'test-stage', input_span_count: 1,
    output_span_count: 1, statements_parsed: 1, duration_ns: 1000,
    intermediate_data: { marker: 'after' },
  }],
  final_span_count: 1,
  final_data: { marker: 'after' },
}

async function mockAppApi(page: Page, execute: (route: Route) => Promise<void>, baselines: unknown[]) {
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/runs') return route.fulfill({ json: runs })
    if (url.pathname.startsWith('/api/runs/')) {
      return route.fulfill({ json: { trace_data: { marker: url.pathname.split('/').pop() }, log_data: { marker: 'logs' } } })
    }
    if (url.pathname === '/api/ottl/pipeline/execute') return execute(route)
    if (url.pathname === '/api/ottl/pipeline/diff') {
      baselines.push(route.request().postDataJSON().before_data)
      return route.fulfill({ json: { stage_index: 0, changes: [] } })
    }
    return route.fulfill({ json: {} })
  })
}

test('renders empty state', async ({ mount }) => {
  const component = await mount(ResultsView, { props: { result: null } })
  await expect(component).toContainText('Execute a pipeline to see Collector results')
})

test('shows stale freshness without implying auto re-execution', async ({ mount }) => {
  const component = await mount(ResultsView, {
    props: {
      stale: true,
      result: {
        stages: [{
          stage_index: 0,
          stage_id: 'transform/set-env',
          input_span_count: 1,
          output_span_count: 1,
          statements_parsed: 1,
          duration_ns: 1000,
        }],
        final_span_count: 1,
      },
    },
  })
  await expect(component).toContainText('Draft changed · results out of date')
  await expect(component).toContainText('Configuration edits do not produce new runtime results')
})

test('evidence follows the selected run and signal without losing its baseline', async ({ mount, page }) => {
  const baselines: unknown[] = []
  await mockAppApi(page, route => route.fulfill({ json: execution }), baselines)
  const app = await mount(App)
  await app.getByRole('button', { name: 'Pipeline', exact: true }).click()
  const run = app.getByLabel('Run', { exact: true })
  await expect(run).toHaveValue('A')
  await app.getByRole('button', { name: 'Execute', exact: true }).click()
  await expect(app.locator('#freshness')).toHaveText('Executed · evidence current')
  await expect.poll(() => baselines.length).toBeGreaterThan(0)
  await run.selectOption('B')
  await expect(app.locator('#freshness')).toContainText('results out of date')
  await run.selectOption('A')
  await expect(app.locator('#freshness')).toHaveText('Executed · evidence current')
  await app.getByRole('radio', { name: 'logs' }).click()
  await expect(app.locator('#freshness')).toContainText('results out of date')
  await app.getByRole('radio', { name: 'traces' }).click()
  await expect(app.locator('#freshness')).toHaveText('Executed · evidence current')
  expect(baselines.every(value => JSON.stringify(value) === JSON.stringify({ marker: 'A' }))).toBe(true)
})

test('late Quick execution remains stale for Pipeline selection and retains Quick baseline', async ({ mount, page }) => {
  const baselines: unknown[] = []
  const requestStarted = Promise.withResolvers<void>()
  const responseReady = Promise.withResolvers<void>()
  await mockAppApi(page, async route => {
    requestStarted.resolve()
    await responseReady.promise
    await route.fulfill({ json: execution })
  }, baselines)
  const app = await mount(App)
  await app.getByRole('button', { name: 'Quick', exact: true }).click()
  await expect(app.getByLabel('Run', { exact: true })).toHaveValue('A')
  await app.getByRole('button', { name: 'Execute', exact: true }).click()
  await requestStarted.promise
  await app.getByRole('button', { name: 'Pipeline', exact: true }).click()
  const run = app.getByLabel('Run', { exact: true })
  await expect(run).toHaveValue('A')
  await run.selectOption('B')
  responseReady.resolve()
  await expect(app.locator('#freshness')).toContainText('results out of date')
  await expect.poll(() => baselines.length).toBeGreaterThan(0)
  expect(baselines.at(-1)).toEqual({ marker: 'A' })
})
