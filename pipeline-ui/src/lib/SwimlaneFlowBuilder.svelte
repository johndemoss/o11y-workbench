<script lang="ts">
  import { untrack } from 'svelte'
  import type { Node, Edge } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import {
    pipelinesToFlow,
    flowToPipelines,
    mergeResults,
  } from './flow-sync'
  import StageFlowNode from './StageFlowNode.svelte'
  import SwimlaneNode from './SwimlaneNode.svelte'
  import InfraNode from './InfraNode.svelte'
  import SwimlaneRow from './SwimlaneRow.svelte'
  import StagePalette from './StagePalette.svelte'
  import type {
    PipelinesBySignal,
    Stage,
    StageType,
    SignalType,
    ProcessorConfig,
    PipelineResult,
    SnippetTemplate,
  } from './types'
  import { signalEdgeProps } from './types'
  import {
    getPipelinesTopologyHash,
    STAGE_WIDTH,
    MIN_GAP,
    FIRST_STAGE_X,
    NODE_Y,
    swimlaneWidthForStageCount,
    exporterXForStageCount,
  } from './svg-utils'

  function packSwimlane(flowNodes: Node<StageNodeData>[], parentId: string) {
    const stages = flowNodes
      .filter((n) => n.parentId === parentId && n.type === 'stage')
      .sort((a, b) => a.position.x - b.position.x)
    for (let i = 0; i < stages.length; i++) {
      stages[i].position = { x: FIRST_STAGE_X + i * (STAGE_WIDTH + MIN_GAP), y: NODE_Y }
    }
    const expX = exporterXForStageCount(stages.length)
    for (const n of flowNodes) {
      if (n.parentId !== parentId || n.type !== 'infra') continue
      const kind = (n.data as { type?: string })?.type
      if (kind === 'exporter') n.position = { ...n.position, x: expX, y: NODE_Y }
      if (kind === 'receiver') n.position = { ...n.position, y: NODE_Y }
    }
    const swimlaneNode = flowNodes.find((n) => n.id === parentId)
    if (swimlaneNode) {
      const w = swimlaneWidthForStageCount(stages.length)
      swimlaneNode.width = w
      swimlaneNode.style = `width: ${w}px`
    }
  }

  function laneEdges(flowNodes: Node<StageNodeData>[], flowEdges: Edge[], parentId: string, signal: SignalType): Edge[] {
    const laneNodes = flowNodes.filter((node) => node.parentId === parentId)
    const stages = laneNodes.filter((node) => node.type === 'stage').sort((a, b) => a.position.x - b.position.x)
    const stageIds = new Set(stages.map((node) => node.id))
    const infra = laneNodes.filter((node) => node.type === 'infra')
    const infraIds = new Set(infra.map((node) => node.id))
    const result = flowEdges.filter((edge) =>
      !(stageIds.has(edge.source) && (stageIds.has(edge.target) || infraIds.has(edge.target))) &&
      !(infraIds.has(edge.source) && stageIds.has(edge.target)),
    )
    if (stages.length === 0) return result
    const connect = (source: string, target: string) => {
      result.push({ id: `e-${source}-${target}`, source, target, type: 'straight', ...signalEdgeProps(signal) })
    }
    for (const node of infra) {
      if (node.data.type === 'receiver') connect(node.id, stages[0].id)
      if (node.data.type === 'exporter') connect(stages[stages.length - 1].id, node.id)
    }
    for (let i = 1; i < stages.length; i++) connect(stages[i - 1].id, stages[i].id)
    return result
  }

  // ─── Props ────────────────────────────────────────────────────────────────
  let {
    pipelines = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    } as PipelinesBySignal,
  onImport,
  onExport,
  onNodeEdit,
  onPipelinesChange,
  onNodeSelected,
  onResultClear,
  onShowYaml,
  yamlOpen = true,
  selectedNodeId = $bindable(''),
  highlightedReceiver = null,
  result = null,
}: {
  pipelines: PipelinesBySignal
    onImport?: () => void
    onExport?: () => void
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig; signal: SignalType }) => void
    onPipelinesChange?: (p: PipelinesBySignal) => void
    onNodeSelected?: (nodeId: string | null) => void
    onResultClear?: () => void
    /** Reveal Pipeline YAML after hide (graph header, design pattern). */
    onShowYaml?: () => void
    yamlOpen?: boolean
    selectedNodeId?: string
    highlightedReceiver?: string | null
    result?: PipelineResult | null
  } = $props()

  // ─── State ────────────────────────────────────────────────────────────────
  let nodes = $state.raw<Node<StageNodeData>[]>([])
  let edges = $state.raw<Edge[]>([])
  let nodeIdCounter = $state(1)

  let lastPipelinesHash = ''
  let lastTopologyHash = ''
  let lastFlowHash = ''
  let flowRequest = 0

  let skipNextFlowToPipelines = false

  let pipelineVersion = $state(0)

  // ─── Pipelines → Flow (external pipeline changes) ─────────────────────────
  $effect(() => {
    const p = pipelines
    const hash = JSON.stringify(p)
    if (hash === lastPipelinesHash) return
    const topologyHash = getPipelinesTopologyHash(p)
    const topologyChanged = topologyHash !== lastTopologyHash
    lastPipelinesHash = hash
    lastTopologyHash = topologyHash

    const initial = pipelinesToFlow(p)
    untrack(() => {
      if (topologyChanged) {
        flowRequest++
        skipNextFlowToPipelines = true
        pipelineVersion++
        nodes = initial.nodes as Node<StageNodeData>[]
        edges = initial.edges
      } else {
        // Update model data without disturbing graph positions, selection, or viewport.
        const fresh = new Map(initial.nodes.map((node) => [node.id, node]))
        nodes = nodes.map((node) => {
          const updated = fresh.get(node.id)
          return updated ? { ...node, data: { ...node.data, ...updated.data } } : node
        })
      }
    })
  })

  // ─── Flow → Pipelines (user edits to the graph) ───────────────────────────
  function getFlowHash(flowNodes: Node<StageNodeData>[], flowEdges: Edge[]): string {
    return JSON.stringify({
      nodes: flowNodes.map((node) => [node.id, node.parentId, node.position.x, node.position.y]),
      edges: flowEdges.map((edge) => [edge.source, edge.target]),
    })
  }

  $effect(() => {
    const flowNodes = nodes
    const flowEdges = edges
    const flowHash = getFlowHash(flowNodes, flowEdges)
    const p = pipelines
    const pipelinesHash = JSON.stringify(p)

    untrack(() => {
      if (skipNextFlowToPipelines) {
        skipNextFlowToPipelines = false
        lastFlowHash = flowHash
        return
      }
      if (flowHash === lastFlowHash) return

      const request = ++flowRequest
      void flowToPipelines(flowNodes as any, flowEdges, p)
        .then((updated) => {
          if (request !== flowRequest ||
              pipelinesHash !== JSON.stringify(pipelines) ||
              flowHash !== getFlowHash(nodes, edges)) return
          lastFlowHash = flowHash
          onPipelinesChange?.(updated)
        })
        .catch(() => {
          // Cycle errors during editing are swallowed; handled during connect
        })
    })
  })

  // ─── Results integration (badges + animated edges) ──────────────────────────
  let mergedResultRef: PipelineResult | null | undefined = undefined

  $effect(() => {
    if (result?.stages && mergedResultRef !== result) {
      mergedResultRef = result
      untrack(() => {
        nodes = mergeResults(nodes, result.stages)
        edges = edges.map((e) => ({ ...e, animated: true }))
      })
    }
  })

  $effect(() => {
    if (!result) {
      untrack(() => {
        edges = edges.map((e) => ({ ...e, animated: false }))
      })
    }
  })

  // ─── Snippet view (bridges StagePalette → SwimlaneRow snippet modal) ────
  let snippetConfig = $state<SnippetTemplate | null>(null)
  let snippetModal = $state(false)

  function handleSnippetView(snippet: SnippetTemplate) {
    snippetConfig = snippet
    snippetModal = true
  }

  function addStageToSignal(opts: {
    type: StageType
    signal: import('./types').SignalType
    snippet?: SnippetTemplate
  }) {
    onResultClear?.()
    const type = opts.type
    const id = `stage-${nodeIdCounter++}`
    const procName = opts.snippet
      ? `${type}/${opts.snippet.id}-${nodeIdCounter}`
      : `${type}/${type}-${id}`
    const parentId = `swimlane-${opts.signal}`
    let currentNodes = [...nodes]
    if (!currentNodes.some((n) => n.id === parentId)) {
      const initial = pipelinesToFlow(pipelines)
      currentNodes = initial.nodes as Node<StageNodeData>[]
      edges = initial.edges
    }
    const siblings = currentNodes
      .filter((n) => n.parentId === parentId && n.type === 'stage')
      .sort((a, b) => a.position.x - b.position.x)
    const receiverNodes = currentNodes.filter(
      (n) => n.parentId === parentId && n.type === 'infra' && (n.data as any)?.type === 'receiver',
    )
    const receiverRight = receiverNodes.length > 0
      ? receiverNodes[0].position.x + (receiverNodes[0].width || STAGE_WIDTH)
      : 180
    const firstStageX = receiverRight + MIN_GAP
    const position = {
      x: siblings.length > 0
        ? siblings[siblings.length - 1].position.x + STAGE_WIDTH + MIN_GAP
        : firstStageX,
      y: NODE_Y,
    }
    const processor = (opts.snippet?.processor ?? {}) as ProcessorConfig
    const newNode: Node<StageNodeData> = {
      id,
      type: 'stage',
      parentId,
      position,
      zIndex: 10,
      draggable: false,
      data: {
        stage: { id, type, processor_ref: procName, error_mode: (processor.error_mode as any) || 'ignore' },
        processor,
        stageIndex: siblings.length,
        signal: opts.signal,
      },
    }
    const newNodes = [...currentNodes, newNode]
    packSwimlane(newNodes, parentId)
    nodes = newNodes
    edges = laneEdges(newNodes, edges, parentId, opts.signal)
    onNodeSelected?.(id)
    onNodeEdit?.({ stage: newNode.data.stage, processor, signal: opts.signal })
  }

  // Highlight all shared receiver references when one is selected
  function classStr(value: unknown): string {
    if (typeof value === 'string') return value
    if (Array.isArray(value)) return value.filter((x) => typeof x === 'string').join(' ')
    return ''
  }

  $effect(() => {
    const name = highlightedReceiver
    const selected = selectedNodeId
    untrack(() => {
      nodes = nodes.map((n) => {
        const base = classStr(n.class).replace(/\bshared-highlight\b/g, '').trim()
        if (n.type !== 'infra' || (n.data as any)?.type !== 'receiver') {
          return { ...n, class: base }
        }
        const label = String((n.data as any)?.label ?? '')
        const on = !!name && label === name
        return {
          ...n,
          selected: on || n.id === selected,
          class: on ? `${base} shared-highlight`.trim() : base,
        }
      })
    })
  })

  // ─── Drag-over (allow drop) ───────────────────────────────────────────────
  function handleDragOver(event: DragEvent) {
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  }

  // ─── Drop (add new stage node into target swimlane) ───────────────────────
  function handleDrop(event: DragEvent) {
    event.preventDefault()
    const stageType = event.dataTransfer?.getData('application/svelteflow')
    const snippetData = event.dataTransfer?.getData('application/snippet')
    if (!stageType && !snippetData) return

    // Svelte Flow applies pan and zoom to these elements, so their screen
    // rectangles are the authoritative drop targets in client coordinates.
    const canvas = event.currentTarget as HTMLElement
    const laneElements = canvas.querySelectorAll<HTMLElement>('.svelte-flow__node-swimlane')
    const laneElement = Array.from(laneElements).find((element) => {
      const rect = element.getBoundingClientRect()
      return event.clientX >= rect.left && event.clientX <= rect.right &&
        event.clientY >= rect.top && event.clientY <= rect.bottom
    })
    const lane = nodes.find((node) => node.type === 'swimlane' && node.id === laneElement?.dataset.id)
    const signal = lane?.data.signal
    if (!lane || lane.data.disabled || (signal !== 'traces' && signal !== 'logs' && signal !== 'metrics')) return

    const snippet = snippetData ? JSON.parse(snippetData) as SnippetTemplate : undefined
    const type = (stageType || snippet?.type) as StageType
    const siblings = nodes
      .filter((node) => node.parentId === lane.id && node.type === 'stage')
      .sort((a, b) => a.position.x - b.position.x)
    const stageElements = Array.from(canvas.querySelectorAll<HTMLElement>('.svelte-flow__node-stage'))
    const index = siblings.findIndex((node) => {
      const element = stageElements.find((candidate) => candidate.dataset.id === node.id)
      if (!element) return false
      const rect = element.getBoundingClientRect()
      return event.clientX < rect.left + rect.width / 2
    })
    const insertionIndex = index < 0 ? siblings.length : index
    const id = `stage-${nodeIdCounter++}`
    const processor = (snippet?.processor ?? {}) as ProcessorConfig
    const newNode: Node<StageNodeData> = {
      id,
      type: 'stage',
      parentId: lane.id,
      position: { x: FIRST_STAGE_X + insertionIndex * (STAGE_WIDTH + MIN_GAP), y: NODE_Y },
      zIndex: 10,
      draggable: false,
      data: {
        stage: { id, type, processor_ref: `${type}/${type}-${id}` },
        processor,
        stageIndex: insertionIndex,
        signal,
      },
    }

    const ordered = [...siblings.slice(0, insertionIndex), newNode, ...siblings.slice(insertionIndex)]
    for (let i = 0; i < ordered.length; i++) {
      ordered[i].position = { x: FIRST_STAGE_X + i * (STAGE_WIDTH + MIN_GAP), y: NODE_Y }
    }
    const newNodes = [...nodes, newNode]
    packSwimlane(newNodes, lane.id)
    onResultClear?.()
    nodes = newNodes
    edges = laneEdges(newNodes, edges, lane.id, signal)
  }
</script>

<section class="flow-container panel" data-od-id="signal-graph">
  <div class="panel-head builder-header">
    <h2>Pipeline flow</h2>
    <div class="toolbar">
      {#if !yamlOpen}
        <button
          type="button"
          id="show-yaml"
          data-od-id="show-yaml"
          onclick={() => onShowYaml?.()}
        >Show YAML</button>
      {/if}
      {#if onExport}
        <button type="button" id="export-yaml" data-od-id="export-yaml" onclick={() => onExport?.()}>Export YAML</button>
      {/if}
      {#if onImport}
        <button type="button" class="ghost" onclick={() => onImport?.()}>Import YAML</button>
      {/if}
    </div>
  </div>

  <div class="graph-workspace flow-layout">
    <StagePalette {onImport} {onExport} onSnippetView={handleSnippetView} onAddStage={addStageToSignal} />
    <div class="graph-area">
      <p class="graph-help tiny muted">Drag cards to reorder · drop between nodes to insert · select to edit</p>
      <SwimlaneRow
        bind:nodes
        bind:edges
        bind:snippetConfig
        bind:snippetModal
        nodeTypes={{ stage: StageFlowNode, swimlane: SwimlaneNode, infra: InfraNode }}
        {pipelineVersion}
        hasStages={nodes.some(n => n.type === 'stage')}
        {onNodeEdit}
        {onNodeSelected}
        {onResultClear}
        ondragover={handleDragOver}
        ondrop={handleDrop}
      />
      <p class="graph-feedback tiny muted" role="status" aria-live="polite">
        Animated connections show direction only. No telemetry is running until you execute.
      </p>
    </div>
  </div>
</section>

<style>
  .flow-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border);
    min-height: 58px;
    flex-shrink: 0;
  }

  .builder-header h2 {
    font-size: 14px;
    font-weight: 600;
    margin: 0;
    color: var(--color-text);
  }

  .toolbar {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .toolbar button {
    font-size: 12px;
    min-height: 44px;
  }

  .toolbar .ghost {
    background: transparent;
  }

  .graph-workspace,
  .flow-layout {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .graph-area {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .graph-help,
  .graph-feedback {
    padding: 12px;
    margin: 0;
    flex-shrink: 0;
  }

  .tiny {
    font-size: 12px;
  }

  .muted {
    color: var(--color-text-secondary);
  }

  .graph-area :global(.swimlane-row),
  .graph-area :global(.svelte-flow) {
    flex: 1;
    min-height: 0;
  }

  @media (max-width: 960px) {
    .graph-workspace,
    .flow-layout {
      grid-template-columns: 1fr;
    }
  }
</style>
