import { expect, type Page } from '@playwright/test'

export const E2E_PORT = 5199

function topologicalSortLocal(body: {
  nodes: { id: string }[]
  edges: { source: string; target: string }[]
}): string[] | { error: string } {
  const { nodes, edges } = body
  const adj = new Map<string, string[]>()
  const indeg = new Map<string, number>()
  for (const n of nodes) {
    adj.set(n.id, [])
    indeg.set(n.id, 0)
  }
  for (const e of edges) {
    adj.get(e.source)?.push(e.target)
    indeg.set(e.target, (indeg.get(e.target) ?? 0) + 1)
  }
  const q: string[] = []
  for (const [id, d] of indeg) {
    if (d === 0) q.push(id)
  }
  const sorted: string[] = []
  while (q.length) {
    const id = q.shift()!
    sorted.push(id)
    for (const nb of adj.get(id) ?? []) {
      const nd = (indeg.get(nb) ?? 1) - 1
      indeg.set(nb, nd)
      if (nd === 0) q.push(nb)
    }
  }
  if (sorted.length !== nodes.length) {
    return { error: 'Cycle detected in pipeline graph' }
  }
  return sorted
}

/** Stand in for the Go API when CI runs without a backend listener. */
export async function installGraphApiFallback(page: Page): Promise<void> {
  await page.route('**/api/ottl/pipeline/topological-sort', async (route) => {
    const body = route.request().postDataJSON() as {
      nodes: { id: string }[]
      edges: { source: string; target: string }[]
    }
    const result = topologicalSortLocal(body)
    if (Array.isArray(result)) {
      await route.fulfill({ json: { sorted_ids: result } })
      return
    }
    await route.fulfill({ json: { sorted_ids: [], error: result.error } })
  })

  await page.route('**/api/ottl/pipeline/would-cycle', async (route) => {
    const body = route.request().postDataJSON() as {
      edges: { source: string; target: string }[]
      new_edge: { source: string; target: string }
    }
    const testEdges = [...body.edges, body.new_edge]
    const nodeIds = new Set<string>()
    for (const edge of testEdges) {
      nodeIds.add(edge.source)
      nodeIds.add(edge.target)
    }
    const result = topologicalSortLocal({
      nodes: Array.from(nodeIds).map((id) => ({ id })),
      edges: testEdges,
    })
    const would_cycle = Array.isArray(result) ? result.length !== nodeIds.size : true
    await route.fulfill({ json: { would_cycle } })
  })
}

export async function openGraphBuilder(page: Page): Promise<void> {
  await page.goto('/')
  await page
    .getByRole('navigation', { name: 'Pipeline views' })
    .getByRole('button', { name: 'Graph', exact: true })
    .click()
  await expect(page.locator('[data-od-id="signal-graph"]')).toBeVisible()
  await page.locator('.svelte-flow').waitFor({ state: 'visible', timeout: 15_000 })
}

export async function addTransformStages(page: Page, count: number): Promise<void> {
  const add = page.getByRole('button', { name: 'Add Transform blank stage' })
  for (let i = 0; i < count; i++) {
    await add.click()
    await expect(page.locator('.svelte-flow__node-stage')).toHaveCount(i + 1, { timeout: 10_000 })
  }
}

export async function tracesStageOrder(page: Page): Promise<string[]> {
  return page
    .locator('[data-id^="stage-"].svelte-flow__node-stage')
    .evaluateAll((nodes) =>
      nodes
        .map((node) => ({
          id: node.getAttribute('data-id') ?? '',
          x: node.getBoundingClientRect().left,
        }))
        .sort((a, b) => a.x - b.x)
        .map((entry) => entry.id),
    )
}