<script lang="ts">
  import { untrack } from 'svelte'
  import { MarkerType } from '@xyflow/svelte'
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
    ProcessorConfig,
    PipelineResult,
    SnippetTemplate,
  } from './types'
  import { getPipelinesHash, STAGE_WIDTH, MIN_GAP, SWIMLANE_MIN_WIDTH, SWIMLANE_EXTRA } from './svg-utils'

  // ─── Props ────────────────────────────────────────────────────────────────
  let {
    pipelines = {
      traces: { stages: [], processors: {} },
    } as PipelinesBySignal,
  onImport,
  onExport,
  onNodeEdit,
  onPipelinesChange,
  onNodeSelected,
  onResultClear,
  selectedNodeId = $bindable(''),
  result = null,
}: {
  pipelines: PipelinesBySignal
    onImport?: () => void
    onExport?: () => void
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig }) => void
    onPipelinesChange?: (p: PipelinesBySignal) => void
    onNodeSelected?: (nodeId: string | null) => void
    result?: PipelineResult | null
    onResultClear?: () => void
  } = $props()

  // ─── State ────────────────────────────────────────────────────────────────
  let nodes = $state.raw<Node<StageNodeData>[]>([])
  let edges = $state.raw<Edge[]>([])
  let nodeIdCounter = $state(1)

  let lastPipelinesHash = $state('')

  let skipNextFlowToPipelines = false

  let pipelineVersion = $state(0)

  // ─── Pipelines → Flow (external pipeline changes) ─────────────────────────
  $effect(() => {
    const p = pipelines
    const hash = getPipelinesHash(p)
    if (hash === lastPipelinesHash) return
    lastPipelinesHash = hash

    const initial = pipelinesToFlow(p)
    for (const node of initial.nodes) {
      if (node.type === 'swimlane') {
        node.draggable = false
        node.selectable = false
      }
    }
    untrack(() => {
      skipNextFlowToPipelines = true
      pipelineVersion++
      nodes = initial.nodes as Node<StageNodeData>[]
      edges = initial.edges
    })
  })

  // ─── Flow → Pipelines (user edits to the graph) ───────────────────────────
  let lastFlowHash = $state('')

  $effect(() => {
    const flowHash = JSON.stringify({
      nodeIds: nodes.map((n) => n.id),
      stagePositions: nodes
        .filter((n) => n.type === 'stage')
        .map((n) => `${Math.round(n.position.x)},${Math.round(n.position.y)}`),
      edgeIds: edges.map((e) => `${e.source}->${e.target}`),
    })
    if (flowHash === lastFlowHash) return
    lastFlowHash = flowHash

    untrack(() => {
      try {
        if (skipNextFlowToPipelines) {
          skipNextFlowToPipelines = false
          return
        }
        const updated = flowToPipelines(nodes as any, edges, pipelines)
        onPipelinesChange?.(updated)
      } catch {
        // Cycle errors during editing are swallowed; handled during connect
      }
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

  // ─── Drag-over (allow drop) ───────────────────────────────────────────────
  function handleDragOver(event: DragEvent) {
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  }

  // ─── Drop (add new stage node into target swimlane) ───────────────────────
  function handleDrop(event: DragEvent) {
    event.preventDefault()
    onResultClear?.()
    const stageType = event.dataTransfer?.getData('application/svelteflow') || undefined
    const snippetData = event.dataTransfer?.getData('application/snippet')
    if (!stageType && !snippetData) return

    const type = (stageType || (snippetData ? JSON.parse(snippetData).type : 'transform')) as StageType

    const id = `stage-${nodeIdCounter++}`
    const procName = `${type}/${type}-${id}`

    const canvasEl = event.currentTarget as HTMLElement
    const rect = canvasEl.getBoundingClientRect()
    const flowPos = { x: event.clientX - rect.left, y: event.clientY - rect.top }

    let currentNodes = [...nodes]
    if (currentNodes.filter((n) => n.type === 'swimlane').length === 0) {
      const defaultPipelines = {
        traces: { stages: [], processors: {} },
        logs: { stages: [], processors: {} },
        metrics: { stages: [], processors: {} },
      }
      const initial = pipelinesToFlow(defaultPipelines)
      currentNodes = [...initial.nodes.filter((n) => n.type === 'swimlane'), ...currentNodes]
    }

    const swimlaneNodes = currentNodes.filter((n) => n.type === 'swimlane')
    let parentId: string | undefined
    let parentOrigin = { x: 0, y: 0 }
    for (const sn of swimlaneNodes) {
      const sx = sn.position.x
      const sy = sn.position.y
      const sw = ((sn.style as any)?.width as number) || 400
      const sh = ((sn.style as any)?.height as number) || 150
      if (flowPos.x >= sx && flowPos.x <= sx + sw && flowPos.y >= sy && flowPos.y <= sy + sh) {
        parentId = sn.id
        parentOrigin = { x: sx, y: sy }
        if (sn.data?.disabled) return
        break
      }
    }
    if (!parentId && swimlaneNodes.length > 0) {
      parentId = swimlaneNodes[0].id
      parentOrigin = {
        x: swimlaneNodes[0].position.x,
        y: swimlaneNodes[0].position.y,
      }
    }

    let siblings: Node<StageNodeData>[] = []
    if (parentId) {
      siblings = currentNodes.filter(
        (n) => n.parentId === parentId && n.type === 'stage',
      )
    }

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
      y: 35,
    }

    const newNode: Node<StageNodeData> = {
      id,
      type: 'stage',
      parentId,
      position,
      zIndex: 10,
      draggable: false,
      data: {
        stage: { id, type, processor_ref: procName },
        processor: snippetData ? JSON.parse(snippetData).processor ?? {} : {},
        stageIndex: siblings.length,
      },
    }

    const newNodes = [...currentNodes, newNode]
    const newEdges = [...edges]

    if (siblings.length > 0) {
      const lastSibling = siblings[siblings.length - 1]
      newEdges.push({
        id: `e-${lastSibling.id}-${id}`,
        source: lastSibling.id,
        target: id,
        type: 'straight',
        markerEnd: { type: MarkerType.ArrowClosed },
      })
    }

    const swimlaneNode = newNodes.find(n => n.id === (parentId || 'swimlane-traces'))
    if (swimlaneNode) {
      const siblingsAfter = newNodes.filter(n => n.parentId === swimlaneNode.id && n.type === 'stage')
      const newWidth = Math.max(SWIMLANE_MIN_WIDTH, siblingsAfter.length * (STAGE_WIDTH + MIN_GAP) + SWIMLANE_EXTRA)
      swimlaneNode.width = newWidth
      swimlaneNode.style = { ...(swimlaneNode.style || {}), width: `${newWidth}px` }

      const stageCount = siblingsAfter.length
      if (stageCount > 1) {
        const totalStageWidth = stageCount * STAGE_WIDTH
        const gap = Math.max(MIN_GAP, (newWidth - 200 - MIN_GAP - firstStageX - totalStageWidth) / (stageCount - 1))
        for (let i = 0; i < stageCount; i++) {
          siblingsAfter[i].position.x = firstStageX + i * (STAGE_WIDTH + gap)
        }
      }
    }

    nodes = newNodes
    edges = newEdges
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flow-container" onkeydown={(e) => e.key === 'Escape' && handleCancelDelete()}>
  <header class="builder-header">
    <h1>Pipeline Graph</h1>
  </header>

  <div class="flow-layout">
    <StagePalette {onImport} onSnippetView={handleSnippetView} />
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
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
    color: var(--color-text, #f1f5f9);
  }

  .flow-layout {
    display: flex;
    flex: 1;
    overflow: hidden;
  }
</style>
