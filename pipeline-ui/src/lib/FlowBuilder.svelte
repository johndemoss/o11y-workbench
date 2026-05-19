<script lang="ts">
  import '@xyflow/svelte/dist/style.css'
  import type { Node, Edge } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import { pipelineToFlow, flowToPipeline } from './flow-sync'
  import PipelineCanvas from './PipelineCanvas.svelte'
  import NodePalette from './NodePalette.svelte'
  import StageFlowNode from './StageFlowNode.svelte'
  import type { Pipeline, PipelineResult, Stage, ProcessorConfig } from './types'

  // ─── Props ────────────────────────────────────────────────────────────────
  let {
    pipeline = { stages: [], processors: {} },
    onValidate,
    onImport,
    onNodeEdit,
    result,
    onResultClear,
  }: {
    pipeline: Pipeline
    onValidate?: (p: Pipeline) => void
    onImport?: () => void
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig }) => void
    result?: PipelineResult | null
    onResultClear?: () => void
  } = $props()

  const nodeTypes = { stage: StageFlowNode }

  // ─── State ────────────────────────────────────────────────────────────────
  let nodes = $state.raw<Node<StageNodeData>[]>([])
  let edges = $state.raw<Edge[]>([])
  let nodeIdCounter = $state(1)

  /** Serialised hash of the pipeline that the flow currently reflects.
   *  Used to distinguish external pipeline changes from our own syncs. */
  let lastPipelineHash = $state('')

  // ─── Helpers ──────────────────────────────────────────────────────────────
  function getPipelineHash(p: Pipeline): string {
    return JSON.stringify({
      stages: p.stages.map((s) => s.id),
      processorKeys: Object.keys(p.processors).sort(),
    })
  }

  // ─── Pipeline → Flow (external pipeline changes) ─────────────────────────
  $effect(() => {
    const hash = getPipelineHash(pipeline)
    if (hash === lastPipelineHash) return
    lastPipelineHash = hash

    const result = pipelineToFlow(pipeline)
    nodes = result.nodes
    edges = result.edges
    nodeIdCounter = pipeline.stages.length + 1
  })

  // ─── Flow → Pipeline (user deletions, reconnections) ─────────────────────
  $effect(() => {
    const currentNodes = nodes
    const currentEdges = edges

    const currentHash = getPipelineHash(pipeline)
    if (currentHash !== lastPipelineHash) return

    const flowPipeline = flowToPipeline(currentNodes, currentEdges, pipeline)
    const flowHash = getPipelineHash(flowPipeline)
    if (flowHash === currentHash) return

    lastPipelineHash = flowHash
    pipeline.stages = flowPipeline.stages
    pipeline.processors = flowPipeline.processors
    onValidate?.(flowPipeline)
  })
</script>

<div class="flow-container">
  <header class="builder-header">
    <h1>Pipeline Prototyper</h1>
    <span class="badge">Experimental</span>
    <div class="actions">
      <button onclick={() => onValidate?.(flowToPipeline(nodes, edges, pipeline))} class="btn btn-primary">Validate</button>
    </div>
  </header>

  <div class="flow-layout">
    <NodePalette {onImport} />
    <PipelineCanvas
      bind:nodes
      bind:edges
      bind:nodeIdCounter
      {nodeTypes}
      {pipeline}
      {onValidate}
      {onNodeEdit}
      {result}
      {onResultClear}
    />
  </div>
</div>

<style>
  .flow-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--color-bg, #0f172a);
  }

  .builder-header {
    display: flex;
    align-items: center;
    gap: var(--space-sm, 0.5rem);
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    background: var(--color-surface, #1e293b);
    border-bottom: 1px solid var(--color-border, #334155);
    flex-shrink: 0;
  }

  .builder-header h1 {
    font-size: var(--font-size-lg, 1.1rem);
    font-weight: 600;
    margin: 0;
    color: var(--color-text, #f1f5f9);
  }

  .badge {
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 2px 8px;
    border-radius: var(--radius-sm, 4px);
    background: var(--color-warning, #f59e0b);
    color: #000;
  }

  .actions {
    margin-left: auto;
    display: flex;
    gap: var(--space-sm, 0.5rem);
  }

  .btn {
    padding: 0.4rem 0.9rem;
    border: none;
    border-radius: var(--radius-md, 6px);
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
  }

  .btn-primary {
    background: var(--color-primary, #3b82f6);
    color: var(--color-text, #f1f5f9);
  }

  .btn-primary:hover {
    background: var(--color-primary-hover, #2563eb);
  }

  .flow-layout {
    display: flex;
    flex: 1;
    overflow: hidden;
  }
</style>
