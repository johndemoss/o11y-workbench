import { test, expect } from '@playwright/experimental-ct-svelte'
import ResultsView from '../ResultsView.svelte'

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
