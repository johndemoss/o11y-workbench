import { test, expect } from '@playwright/experimental-ct-svelte'
import ResultsView from '../ResultsView.svelte'

test('renders empty state', async ({ mount }) => {
  const component = await mount(ResultsView, { props: { result: null } })
  await expect(component).toContainText('Execute a pipeline to see results')
})
