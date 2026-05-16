import { describe, it, expect } from 'vitest'
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

function makeNode(id: string, data?: Partial<StageNodeData>): Node {
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

  it('connects sequential stages with smoothstep edges', () => {
    const stages = [makeStage('a'), makeStage('b'), makeStage('c')]
    const pipeline: Pipeline = { stages, processors: {} }
    const result = pipelineToFlow(pipeline)
    expect(result.edges).toHaveLength(2)
    expect(result.edges[0].source).toBe('a')
    expect(result.edges[0].target).toBe('b')
    expect(result.edges[0].type).toBe('smoothstep')
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
  it('converts single node to single-stage pipeline', () => {
    const nodes = [makeNode('a')]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = flowToPipeline(nodes, edges, currentPipeline)
    expect(result.stages).toHaveLength(1)
    expect(result.stages[0].id).toBe('a')
  })

  it('orders stages by topological sort', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = flowToPipeline(nodes, edges, currentPipeline)
    expect(result.stages.map((s) => s.id)).toEqual(['a', 'b', 'c'])
  })

  it('preserves existing processor configs from currentPipeline', () => {
    const s1 = makeStage('a', 'proc-a')
    const nodes = [makeNode('a', { stage: s1, processor: { foo: 1 } })]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = {
      stages: [s1],
      processors: { 'proc-a': { existingKey: 'existingValue' } },
    }
    const result = flowToPipeline(nodes, edges, currentPipeline)
    expect(result.processors['proc-a']).toEqual({ existingKey: 'existingValue' })
  })

  it('merges new processor refs from nodes', () => {
    const s1 = makeStage('a', 'proc-a')
    const nodes = [
      makeNode('a', { stage: s1, processor: { newKey: 'newVal' } }),
    ]
    const edges: Edge[] = []
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    const result = flowToPipeline(nodes, edges, currentPipeline)
    // processor_ref from node should be added since it's not in currentPipeline
    expect(result.processors['proc-a']).toEqual({ newKey: 'newVal' })
  })

  it('throws on cyclic pipeline graph', () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    const currentPipeline: Pipeline = { stages: [], processors: {} }
    expect(() => flowToPipeline(nodes, edges, currentPipeline)).toThrow(
      'Cycle detected in pipeline graph',
    )
  })
})

// ===== topologicalSort =====

describe('topologicalSort', () => {
  it('returns single node for one-node graph', () => {
    const nodes = [makeNode('a')]
    expect(topologicalSort(nodes, [])).toEqual(['a'])
  })

  it('sorts linear graph correctly', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    expect(topologicalSort(nodes, edges)).toEqual(['a', 'b', 'c'])
  })

  it('handles branched graph (a→b, a→c)', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('a', 'c')]
    const result = topologicalSort(nodes, edges)
    expect(result[0]).toBe('a')
    // b and c can be in either order
    expect(result.slice(1).sort()).toEqual(['b', 'c'])
  })

  it('handles disconnected subgraphs', () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const result = topologicalSort(nodes, [])
    expect(result).toContain('a')
    expect(result).toContain('b')
    expect(result).toHaveLength(2)
  })

  it('throws on direct cycle (a→b, b→a)', () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    expect(() => topologicalSort(nodes, edges)).toThrow(
      'Cycle detected in pipeline graph',
    )
  })

  it('throws on indirect cycle (a→b, b→c, c→a)', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c'), makeEdge('c', 'a')]
    expect(() => topologicalSort(nodes, edges)).toThrow(
      'Cycle detected in pipeline graph',
    )
  })
})

// ===== wouldCreateCycle =====

describe('wouldCreateCycle', () => {
  it('returns false for acyclic edge addition', () => {
    const edges = [makeEdge('a', 'b')]
    expect(wouldCreateCycle(edges, { source: 'b', target: 'c' })).toBe(false)
  })

  it('returns true for direct cycle (b→a when a→b exists)', () => {
    const edges = [makeEdge('a', 'b')]
    expect(wouldCreateCycle(edges, { source: 'b', target: 'a' })).toBe(true)
  })

  it('returns true for indirect cycle (c→a when a→b, b→c)', () => {
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    expect(wouldCreateCycle(edges, { source: 'c', target: 'a' })).toBe(true)
  })

  it('returns true for self-loop', () => {
    const edges: Edge[] = []
    expect(wouldCreateCycle(edges, { source: 'a', target: 'a' })).toBe(true)
  })

  it('returns true for duplicate edge', () => {
    const edges = [makeEdge('a', 'b')]
    expect(wouldCreateCycle(edges, { source: 'a', target: 'b' })).toBe(true)
  })

  it('does not mutate the input edges array', () => {
    const edges = [makeEdge('a', 'b')]
    const edgesCopy = [...edges]
    wouldCreateCycle(edges, { source: 'b', target: 'c' })
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

  it('creates child stage nodes inside each swimlane', () => {
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
    const stageNodes = nodes.filter(n => n.type === 'stage')
    expect(stageNodes.length).toBe(2)
    expect(stageNodes[0].parentId).toBe('swimlane-traces')
    expect(stageNodes[1].parentId).toBe('swimlane-traces')
  })

  it('creates edges between consecutive stages in the same swimlane', () => {
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
    const { edges } = pipelinesToFlow(p)
    expect(edges.length).toBe(1)
    expect(edges[0].source).toBe('t1')
    expect(edges[0].target).toBe('t2')
  })

  it('spreads multiple signals across separate swimlanes', () => {
    const p: PipelinesBySignal = {
      traces: {
        stages: [{ id: 't1', type: 'transform', processor_ref: 'p1' }],
        processors: { p1: {} },
      },
      logs: {
        stages: [{ id: 'l1', type: 'filter', processor_ref: 'p2' }],
        processors: { p2: {} },
      },
      metrics: {
        stages: [{ id: 'm1', type: 'transform', processor_ref: 'p3' }],
        processors: { p3: {} },
      },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const tNode = nodes.find(n => n.id === 't1')
    const lNode = nodes.find(n => n.id === 'l1')
    const mNode = nodes.find(n => n.id === 'm1')
    expect(tNode?.parentId).toBe('swimlane-traces')
    expect(lNode?.parentId).toBe('swimlane-logs')
    expect(mNode?.parentId).toBe('swimlane-metrics')
    expect(edges).toHaveLength(0)
  })
})

// ===== flowToPipelines (multi-swimlane) =====

describe('flowToPipelines', () => {
  it('reconstructs PipelinesBySignal from swimlane nodes and edges', () => {
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
    const result = flowToPipelines(nodes, edges, p)
    expect(result.traces.stages.length).toBe(1)
    expect(result.traces.stages[0].id).toBe('t1')
    expect(result.traces.processors.p1).toEqual({ some: 'config' })
  })

  it('preserves empty swimlanes', () => {
    const p: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const { nodes, edges } = pipelinesToFlow(p)
    const result = flowToPipelines(nodes, edges, p)
    expect(result.traces.stages).toHaveLength(0)
    expect(result.logs.stages).toHaveLength(0)
    expect(result.metrics.stages).toHaveLength(0)
  })
})

// ===== reorderStage =====

describe('reorderStage', () => {
  it('changes stage position when dragged horizontally within swimlane', () => {
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
    const reconstructed = flowToPipelines(result.nodes, result.edges, p)
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
    const originalNodePositions = nodes.map(n => ({ ...n.position }))
    reorderStage(nodes, edges, 't1', 400)
    expect(nodes.map(n => n.position)).toEqual(originalNodePositions)
  })
})
