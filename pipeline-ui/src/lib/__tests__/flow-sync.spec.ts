import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  pipelineToFlow,
  flowToPipeline,
  topologicalSort,
  wouldCreateCycle,
  mergeResults,
  pipelinesToFlow,
  flowToPipelines,
  reorderStage,
  type StageNodeData,
} from '../flow-sync'
import type { Edge, Node } from '@xyflow/svelte'
import type { Pipeline, PipelinesBySignal, Stage, StageResult } from '../types'

// ----- Smart fetch mock -----
// Implements Kahn's algorithm locally so tests can exercise topologicalSort
// and wouldCreateCycle without requiring a running Go backend.

function mockTopoSortResponse(body: {
  nodes: { id: string }[]
  edges: { source: string; target: string }[]
}): Response {
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
    return new Response(
      JSON.stringify({ sorted_ids: [], error: 'Cycle detected in pipeline graph' }),
      { headers: { 'Content-Type': 'application/json' } },
    )
  }

  return new Response(JSON.stringify({ sorted_ids: sorted }), {
    headers: { 'Content-Type': 'application/json' },
  })
}

function mockWouldCycleResponse(body: {
  edges: { source: string; target: string }[]
  new_edge: { source: string; target: string }
}): Response {
  const testEdges = [...body.edges, body.new_edge]
  const nodeIds = new Set<string>()
  for (const e of testEdges) {
    nodeIds.add(e.source)
    nodeIds.add(e.target)
  }

  const adj = new Map<string, string[]>()
  const indeg = new Map<string, number>()
  for (const id of nodeIds) {
    adj.set(id, [])
    indeg.set(id, 0)
  }
  for (const e of testEdges) {
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

  return new Response(JSON.stringify({ would_cycle: sorted.length !== nodeIds.size }), {
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse((init?.body as string) ?? '{}')

      if (url === '/api/ottl/pipeline/topological-sort') {
        return mockTopoSortResponse(body)
      }
      if (url === '/api/ottl/pipeline/would-cycle') {
        return mockWouldCycleResponse(body)
      }

      return new Response(JSON.stringify({ error: 'unexpected request' }), {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// ----- Helpers -----

function makeStage(id: string, processorRef?: string): Stage {
  return {
    id,
    type: 'transform',
    processor_ref: processorRef ?? `proc-${id}`,
    error_mode: 'propagate',
  }
}

function makeResult(
  stageId: string,
  added = 1,
  removed = 0,
  modified = 0,
): StageResult {
  return {
    stage_index: 0,
    stage_id: stageId,
    input_span_count: 5,
    output_span_count: 5,
    statements_parsed: 1,
    duration_ns: 1000,
    intermediate_data: { added, removed, modified },
  }
}

function makeNode(id: string, data?: Partial<StageNodeData>): Node<StageNodeData> {
  return {
    id,
    type: 'stage',
    position: { x: 200, y: 0 },
    data: {
      stage: makeStage(id),
      processor: {},
      stageIndex: 0,
      ...data,
    },
  }
}

function makeEdge(source: string, target: string): Edge {
  return { id: `e-${source}-${target}`, source, target }
}

// ===== pipelineToFlow =====

describe('pipelineToFlow', () => {
  it('converts empty pipeline to empty nodes and edges', () => {
    const pipeline: Pipeline = { stages: [], processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.nodes).toHaveLength(0)
    expect(result.edges).toHaveLength(0)
  })

  it('converts single stage to one node with no edges', () => {
    const stage1 = makeStage('stage-1')
    const pipeline: Pipeline = { stages: [stage1], processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.nodes).toHaveLength(1)
    expect(result.edges).toHaveLength(0)
    expect(result.nodes[0].id).toBe('stage-1')
    expect(result.nodes[0].type).toBe('stage')
  })

  it('creates correct positions for multiple stages', () => {
    const stages = [makeStage('a'), makeStage('b'), makeStage('c')]
    const pipeline: Pipeline = { stages, processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.nodes).toHaveLength(3)
    expect(result.nodes[0].position).toEqual({ x: 200, y: 0 })
    expect(result.nodes[1].position).toEqual({ x: 200, y: 150 })
    expect(result.nodes[2].position).toEqual({ x: 200, y: 300 })
  })

  it('connects sequential stages with straight edges', () => {
    const stages = [makeStage('a'), makeStage('b'), makeStage('c')]
    const pipeline: Pipeline = { stages, processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.edges).toHaveLength(2)
    expect(result.edges[0].source).toBe('a')
    expect(result.edges[0].target).toBe('b')
    expect(result.edges[0].type).toBe('straight')
    expect(result.edges[1].source).toBe('b')
    expect(result.edges[1].target).toBe('c')
  })

  it('includes processor config in node data', () => {
    const stage1 = makeStage('s1', 'my-proc')
    const pipeline: Pipeline = {
      stages: [stage1],
      processors: { 'my-proc': { someKey: 'someValue' } },
    }
    const result = pipelineToFlow(pipeline)
    expect(result.nodes[0].data.processor).toEqual({ someKey: 'someValue' })
    expect(result.nodes[0].data.stageIndex).toBe(0)
  })

  it('uses empty object for missing processor ref', () => {
    const stage1 = makeStage('s1', 'missing-ref')
    const pipeline: Pipeline = { stages: [stage1], processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.nodes[0].data.processor).toEqual({})
  })
})

// ===== flowToPipeline =====

describe('flowToPipeline', () => {
  it('converts single node to single-stage pipeline', async () => {
    const nodes = [makeNode('a')]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = await flowToPipeline(nodes, edges, currentPipeline)
    expect(result.stages).toHaveLength(1)
    expect(result.stages[0].id).toBe('a')
  })

  it('orders stages by topological sort', async () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = await flowToPipeline(nodes, edges, currentPipeline)
    expect(result.stages.map((s) => s.id)).toEqual(['a', 'b', 'c'])
  })

  it('preserves existing processor configs from currentPipeline', async () => {
    const s1 = makeStage('a', 'proc-a')
    const nodes = [makeNode('a', { stage: s1, processor: { foo: 1 } })]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = {
      stages: [s1],
      processors: { 'proc-a': { existingKey: 'existingValue' } },
    }
    const result = await flowToPipeline(nodes, edges, currentPipeline)
    expect(result.processors['proc-a']).toEqual({ existingKey: 'existingValue' })
  })

  it('merges new processor refs from nodes', async () => {
    const s1 = makeStage('a', 'proc-a')
    const nodes = [
      makeNode('a', { stage: s1, processor: { newKey: 'newVal' } }),
    ]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = await flowToPipeline(nodes, edges, currentPipeline)
    // processor_ref from node should be added since it's not in currentPipeline
    expect(result.processors['proc-a']).toEqual({ newKey: 'newVal' })
  })

  it('throws on cyclic pipeline graph', async () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    await expect(
      flowToPipeline(nodes, edges, currentPipeline),
    ).rejects.toThrow('Cycle detected in pipeline graph')
  })
})

// ===== topologicalSort =====

describe('topologicalSort', () => {
  it('returns single node for one-node graph', async () => {
    const nodes = [makeNode('a')]
    await expect(topologicalSort(nodes, [])).resolves.toEqual(['a'])
  })

  it('sorts linear graph correctly', async () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    await expect(topologicalSort(nodes, edges)).resolves.toEqual(['a', 'b', 'c'])
  })

  it('handles branched graph (a→b, a→c)', async () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('a', 'c')]
    const result = await topologicalSort(nodes, edges)
    expect(result[0]).toBe('a')
    // b and c can be in either order
    expect(result.slice(1).sort()).toEqual(['b', 'c'])
  })

  it('handles disconnected subgraphs', async () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const result = await topologicalSort(nodes, [])
    expect(result).toContain('a')
    expect(result).toContain('b')
    expect(result).toHaveLength(2)
  })

  it('throws on direct cycle (a→b, b→a)', async () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    await expect(topologicalSort(nodes, edges)).rejects.toThrow(
      'Cycle detected in pipeline graph',
    )
  })

  it('throws on indirect cycle (a→b, b→c, c→a)', async () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c'), makeEdge('c', 'a')]
    await expect(topologicalSort(nodes, edges)).rejects.toThrow(
      'Cycle detected in pipeline graph',
    )
  })
})

// ===== wouldCreateCycle =====

describe('wouldCreateCycle', () => {
  it('returns false for acyclic edge addition', async () => {
    const edges = [makeEdge('a', 'b')]
    await expect(
      wouldCreateCycle(edges, { source: 'b', target: 'c' }),
    ).resolves.toBe(false)
  })

  it('returns true for direct cycle (b→a when a→b exists)', async () => {
    const edges = [makeEdge('a', 'b')]
    await expect(
      wouldCreateCycle(edges, { source: 'b', target: 'a' }),
    ).resolves.toBe(true)
  })

  it('returns true for indirect cycle (c→a when a→b, b→c)', async () => {
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    await expect(
      wouldCreateCycle(edges, { source: 'c', target: 'a' }),
    ).resolves.toBe(true)
  })

  it('returns true for self-loop', async () => {
    const edges: Edge[] = []
    await expect(
      wouldCreateCycle(edges, { source: 'a', target: 'a' }),
    ).resolves.toBe(true)
  })

  it('returns true for duplicate edge', async () => {
    const edges = [makeEdge('a', 'b')]
    await expect(
      wouldCreateCycle(edges, { source: 'a', target: 'b' }),
    ).resolves.toBe(true)
  })

  it('does not mutate the input edges array', async () => {
    const edges = [makeEdge('a', 'b')]
    const edgesCopy = [...edges]
    await wouldCreateCycle(edges, { source: 'b', target: 'c' })
    expect(edges).toEqual(edgesCopy)
  })
})

// ===== mergeResults =====

describe('mergeResults', () => {
  it('adds result to matching node by id', () => {
    const nodes = [makeNode('a')]
    const results = [makeResult('a', 3, 1, 2)]
    const merged = mergeResults(nodes, results)
    expect(merged[0].data.results?.intermediate_data).toEqual({
      added: 3,
      removed: 1,
      modified: 2,
    })
  })

  it('sets results to undefined when no match found', () => {
    const nodes = [makeNode('a')]
    const results: StageResult[] = [makeResult('other', 1, 0, 0)]
    const merged = mergeResults(nodes, results)
    expect(merged[0].data.results).toBeUndefined()
  })

  it('handles partial matches across multiple nodes', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const results = [makeResult('a', 1, 0, 0), makeResult('c', 0, 2, 0)]
    const merged = mergeResults(nodes, results)
    expect(merged[0].data.results?.intermediate_data).toEqual({
      added: 1,
      removed: 0,
      modified: 0,
    })
    expect(merged[1].data.results).toBeUndefined()
    expect(merged[2].data.results?.intermediate_data).toEqual({
      added: 0,
      removed: 2,
      modified: 0,
    })
  })

  it('does not mutate the input nodes array', () => {
    const nodes = [makeNode('a')]
    const results = [makeResult('a', 1, 0, 0)]
    const original = nodes[0].data.results
    mergeResults(nodes, results)
    expect(nodes[0].data.results).toBe(original)
  })
})

// ===== pipelinesToFlow (multi-swimlane) =====

describe('pipelinesToFlow', () => {
  it('creates swimlane parent nodes for each signal', () => {
    const p: PipelinesBySignal = {
      traces: { stages: [{ id: 't1', type: 'transform', processor_ref: 'p1' }], processors: { p1: {} } },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes } = pipelinesToFlow(p)
    const swimlanes = nodes.filter(n => n.type === 'swimlane')
    expect(swimlanes.length).toBe(3)
    expect(swimlanes.map(s => s.id)).toContain('swimlane-traces')
    expect(swimlanes.map(s => s.id)).toContain('swimlane-logs')
    expect(swimlanes.map(s => s.id)).toContain('swimlane-metrics')
  })

  it('creates stage nodes with correct parentId', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 't1', type: 'transform', processor_ref: 'p1' },
          { id: 't2', type: 'filter', processor_ref: 'p2' },
        ],
        processors: { p1: {}, p2: {} },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes } = pipelinesToFlow(p)
    const stages = nodes.filter(n => n.type === 'stage')
    expect(stages).toHaveLength(2)
    for (const s of stages) {
      expect(s.parentId).toBe('swimlane-traces')
    }
  })

  it('connects sequential stages within each swimlane', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 'a', type: 'transform', processor_ref: 'p1' },
          { id: 'b', type: 'filter', processor_ref: 'p2' },
          { id: 'c', type: 'transform', processor_ref: 'p3' },
        ],
        processors: { p1: {}, p2: {}, p3: {} },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { edges } = pipelinesToFlow(p)
    const traceEdges = edges.filter(e => e.source.startsWith('traces') || ['a', 'b', 'c'].includes(e.source))
    expect(traceEdges).toHaveLength(2)
    expect(traceEdges[0].source).toBe('a')
    expect(traceEdges[0].target).toBe('b')
    expect(traceEdges[1].source).toBe('b')
    expect(traceEdges[1].target).toBe('c')
  })

  it('minimal swimlane width for few stages', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 'x', type: 'transform', processor_ref: 'p1' },
        ],
        processors: { p1: {} },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes } = pipelinesToFlow(p)
    const swimlane = nodes.find(n => n.id === 'swimlane-traces')
    expect(swimlane!.width).toBeGreaterThanOrEqual(640)
  })

  it('infra nodes for receivers and exporters', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 'x', type: 'transform', processor_ref: 'p1' },
        ],
        processors: { p1: {} },
        receivers: ['otlp', 'zipkin'],
        exporters: ['debug', 'otlp'],
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes } = pipelinesToFlow(p)
    const infra = nodes.filter(n => n.type === 'infra')
    expect(infra.length).toBeGreaterThanOrEqual(2)
    const rNode = infra.find(n => n.id.includes('receiver'))
    const eNode = infra.find(n => n.id.includes('exporter'))
    expect(rNode).toBeDefined()
    expect(eNode).toBeDefined()
  })
})

// ===== flowToPipelines (multi-swimlane) =====

describe('flowToPipelines', () => {
  it('reconstructs PipelinesBySignal from swimlane nodes and edges', async () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 't1', type: 'transform', processor_ref: 'p1' },
        ],
        processors: { p1: { some: 'config' } },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const result = await flowToPipelines(nodes, edges, p)
    expect(result.traces.stages.length).toBe(1)
    expect(result.traces.stages[0].id).toBe('t1')
    expect(result.traces.processors.p1).toEqual({ some: 'config' })
  })

  it('preserves empty swimlanes', async () => {
    const p: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const result = await flowToPipelines(nodes, edges, p)
    expect(result.traces.stages).toHaveLength(0)
    expect(result.logs.stages).toHaveLength(0)
    expect(result.metrics.stages).toHaveLength(0)
  })
})

// ===== reorderStage =====

describe('reorderStage', () => {
  it('changes stage position when dragged horizontally within swimlane', async () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 't1', type: 'transform', processor_ref: 'p1' },
          { id: 't2', type: 'filter', processor_ref: 'p2' },
          { id: 't3', type: 'transform', processor_ref: 'p3' },
        ],
        processors: { p1: {}, p2: {}, p3: {} },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const result = reorderStage(nodes, edges, 't1', 400)
    const reconstructed = await flowToPipelines(result.nodes, result.edges, p)
    expect(reconstructed.traces.stages[0].id).toBe('t2')
    expect(reconstructed.traces.stages[1].id).toBe('t1')
  })

  it('does not mutate the input nodes and edges arrays', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [
          { id: 't1', type: 'transform', processor_ref: 'p1' },
          { id: 't2', type: 'filter', processor_ref: 'p2' },
        ],
        processors: { p1: {}, p2: {} },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const nodesCopy = [...nodes]
    const edgesCopy = [...edges]
    reorderStage(nodes, edges, 't1', 400)
    expect(nodes).toEqual(nodesCopy)
    expect(edges).toEqual(edgesCopy)
  })
})
