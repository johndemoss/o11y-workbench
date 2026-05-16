<script lang="ts">
  import { setContext, untrack } from 'svelte'
  import '@xyflow/svelte/dist/style.css'
  import {
    SvelteFlow,
    Controls,
    Background,
    BackgroundVariant,
    MiniMap,
    MarkerType,
  } from '@xyflow/svelte'
  import type { Node, Edge, Connection } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import {
    pipelinesToFlow,
    flowToPipelines,
    wouldCreateCycle,
    reorderStage,
    mergeResults,
  } from './flow-sync'
  import StageFlowNode from './StageFlowNode.svelte'
  import SwimlaneNode from './SwimlaneNode.svelte'
  import InfraNode from './InfraNode.svelte'
  import StagePalette from './StagePalette.svelte'
  import type {
    PipelinesBySignal,
    Stage,
    StageType,
    ProcessorConfig,
    PipelineResult,
    SnippetTemplate,
  } from './types'

  // ─── Props ────────────────────────────────────────────────────────────────
  let {
    pipelines = {
      traces: { stages: [], processors: {} },
    } as PipelinesBySignal,
    onValidate,
    onImport,
    onExport,
    onNodeEdit,
    onPipelinesChange,
    onNodeSelected,
    result,
    onResultClear,
  }: {
    pipelines: PipelinesBySignal
    onValidate?: (p: PipelinesBySignal) => void
    onImport?: () => void
    onExport?: () => void
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig }) => void
    onPipelinesChange?: (p: PipelinesBySignal) => void
    onNodeSelected?: (nodeId: string | null) => void
    result?: PipelineResult | null
    onResultClear?: () => void
  } = $props()

  // ─── Node types ───────────────────────────────────────────────────────────
  const nodeTypes = { stage: StageFlowNode, swimlane: SwimlaneNode, infra: InfraNode }

  // ─── State ────────────────────────────────────────────────────────────────
  let nodes = $state.raw<Node<StageNodeData>[]>([])
  let edges = $state.raw<Edge[]>([])
  let nodeIdCounter = $state(1)
  let deleteConfirmNodeId: string | null = $state(null)
  let connectError: string | null = $state(null)

  /** Serialised hash of pipelines that the flow currently reflects. */
  let lastPipelinesHash = $state('')

  function getPipelinesHash(p: PipelinesBySignal): string {
    return JSON.stringify({
      traces: p.traces?.stages?.map((s) => s.id) ?? [],
    })
  }

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

  // ─── Delete confirmation via context ─────────────────────────────────────
  setContext('requestDelete', (nodeId: string) => {
    deleteConfirmNodeId = nodeId
  })

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

    // Calculate position relative to canvas area (always use fallback bounding rect)
    const canvasEl = event.currentTarget as HTMLElement
    const rect = canvasEl.getBoundingClientRect()
    const flowPos = { x: event.clientX - rect.left, y: event.clientY - rect.top }

    // If no swimlane nodes exist yet, create default swimlanes first
    let currentNodes = [...nodes]
    if (currentNodes.filter((n) => n.type === 'swimlane').length === 0) {
      const defaultPipelines = {
        traces: { stages: [], processors: {} },
        logs: { stages: [], processors: {} },
        metrics: { stages: [], processors: {} },
      }
      const initial = pipelinesToFlow(defaultPipelines)
      // Merge swimlane nodes in front of any existing nodes
      currentNodes = [...initial.nodes.filter((n) => n.type === 'swimlane'), ...currentNodes]
    }

    // Determine target swimlane by checking which swimlane rect contains the drop
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
        break
      }
    }
    // Default to first swimlane if position doesn't intersect any
    if (!parentId && swimlaneNodes.length > 0) {
      parentId = swimlaneNodes[0].id
      parentOrigin = {
        x: swimlaneNodes[0].position.x,
        y: swimlaneNodes[0].position.y,
      }
    }

    // Find siblings for end-of-chain positioning and auto-connect
    let siblings: Node<StageNodeData>[] = []
    if (parentId) {
      siblings = currentNodes.filter(
        (n) => n.parentId === parentId && n.type === 'stage',
      )
    }

    // Node position is at the end of the existing chain
    const position = {
      x: siblings.length > 0 ? siblings[siblings.length - 1].position.x + 220 : 20,
      y: 35,
    }

    const newNode: Node<StageNodeData> = {
      id,
      type: 'stage',
      parentId,
      position,
      data: {
        stage: { id, type, processor_ref: procName },
        processor: snippetData ? JSON.parse(snippetData).processor ?? {} : {},
        stageIndex: siblings.length,
      },
    }

    const newNodes = [...currentNodes, newNode]
    const newEdges = [...edges]

    // Auto-connect from the last stage in the same swimlane to the new node
    if (siblings.length > 0) {
      const lastSibling = siblings[siblings.length - 1]
      newEdges.push({
        id: `e-${lastSibling.id}-${id}`,
        source: lastSibling.id,
        target: id,
        type: 'smoothstep',
        markerEnd: { type: MarkerType.ArrowClosed },
      })
    }

    // Expand swimlane to fit new node
    const swimlaneNode = newNodes.find(n => n.id === (parentId || 'swimlane-traces'))
    if (swimlaneNode) {
      const siblingsAfter = newNodes.filter(n => n.parentId === swimlaneNode.id && n.type === 'stage')
      const newWidth = Math.max(400, siblingsAfter.length * 220 + 100)
      swimlaneNode.width = newWidth
    }

    nodes = newNodes
    edges = newEdges
  }

  // ─── Connect (user drags between handles) ────────────────────────────────
  function handleConnect(connection: Connection) {
    onResultClear?.()
    // Reject incomplete connections
    if (!connection.source || !connection.target) return

    // Prevent self-connections
    if (connection.source === connection.target) {
      connectError = 'Cannot connect a stage to itself'
      setTimeout(() => {
        connectError = null
      }, 2500)
      return
    }

    // Prevent duplicate connections
    const duplicate = edges.some(
      (e) => e.source === connection.source && e.target === connection.target,
    )
    if (duplicate) {
      connectError = 'Connection already exists'
      setTimeout(() => {
        connectError = null
      }, 2500)
      return
    }

    // Prevent cycle-causing connections
    if (wouldCreateCycle(edges, connection)) {
      connectError = 'Cannot create a cycle in the pipeline'
      setTimeout(() => {
        connectError = null
      }, 2500)
      return
    }

    // Prevent cross-swimlane connections
    const sourceNode = nodes.find((n) => n.id === connection.source)
    const targetNode = nodes.find((n) => n.id === connection.target)
    if (
      sourceNode &&
      targetNode &&
      sourceNode.parentId &&
      targetNode.parentId &&
      sourceNode.parentId !== targetNode.parentId
    ) {
      connectError = 'Cannot connect stages across different signals'
      setTimeout(() => {
        connectError = null
      }, 2500)
      return
    }

    // Valid connection — add the edge
    edges = [
      ...edges,
      {
        id: `e-${connection.source}-${connection.target}`,
        source: connection.source,
        target: connection.target,
        type: 'smoothstep',
        markerEnd: { type: MarkerType.ArrowClosed },
      },
    ]
  }

  // ─── Node click → open StageEditor ──────────────────────────────────────
  function handleNodeClick({
    node,
  }: {
    node: Node<StageNodeData>
    event: MouseEvent | TouchEvent
  }) {
    // Ignore clicks on swimlane parent nodes
    if (node.type !== 'stage') return
    onNodeSelected?.(node.id)
    onNodeEdit?.({ stage: node.data.stage, processor: node.data.processor })
  }

  // ─── Node drag stop → reorder within swimlane ───────────────────────────
  function handleNodeDragStop({
    targetNode,
  }: {
    targetNode: Node | null
    nodes: Node[]
    event: MouseEvent | TouchEvent
  }) {
    if (!targetNode || targetNode.type !== 'stage') return

    const result = reorderStage(nodes as any, edges, targetNode.id, targetNode.position.x)
    untrack(() => {
      nodes = result.nodes as Node<StageNodeData>[]
      edges = result.edges
    })
  }

  // ─── Validate ─────────────────────────────────────────────────────────────
  function handleValidate() {
    onValidate?.(flowToPipelines(nodes as any, edges, pipelines))
  }

  // ─── Delete handlers ─────────────────────────────────────────────────────
  function handleNodeDelete(nodeId: string) {
    onResultClear?.()
    nodes = nodes.filter((n) => n.id !== nodeId)
    edges = edges.filter((e) => e.source !== nodeId && e.target !== nodeId)
  }

  function handleConfirmDelete() {
    if (deleteConfirmNodeId) {
      handleNodeDelete(deleteConfirmNodeId)
      deleteConfirmNodeId = null
    }
  }

  function handleCancelDelete() {
    deleteConfirmNodeId = null
  }

  // ─── Snippet modal ─────────────────────────────────────────────────────────
  let snippetModal = $state(false)
  let snippetConfig = $state<SnippetTemplate | null>(null)

  function handleSnippetView(snippet: SnippetTemplate) {
    snippetConfig = snippet
    snippetModal = true
  }

  function handleSnippetApply() {
    if (!snippetConfig) return
    const snippet = snippetConfig
    const id = `snippet-${nodeIdCounter++}`
    const type = snippet.type
    const procName = `${type}/${type}-${id}`

    const tracesSwimlane = nodes.find(n => n.id === 'swimlane-traces')
    if (!tracesSwimlane) return

    let processor = snippet.processor
    try {
      const configTextarea = document.getElementById('snippet-config-textarea') as HTMLTextAreaElement
      if (configTextarea && configTextarea.value) {
        const parsed = JSON.parse(configTextarea.value)
        processor = parsed
      }
    } catch {
      // Invalid JSON — use original
    }

    const siblings = nodes.filter(n => n.parentId === 'swimlane-traces' && n.type === 'stage')
    const newNode: Node<StageNodeData> = {
      id, type: 'stage', parentId: 'swimlane-traces',
      position: {
        x: siblings.length > 0 ? siblings[siblings.length - 1].position.x + 220 : 20,
        y: 35,
      },
      width: 160,
      data: {
        stage: { id, type: type as StageType, processor_ref: procName },
        processor,
        stageIndex: siblings.length,
      } as any,
    }

    let newNodes = [...nodes, newNode]
    let newEdges = [...edges]
    if (siblings.length > 0) {
      const last = siblings[siblings.length - 1]
      newEdges.push({ id: `e-${last.id}-${id}`, source: last.id, target: id, type: 'smoothstep', markerEnd: { type: MarkerType.ArrowClosed } })
    }
    // Expand swimlane
    const swimlaneNode = newNodes.find(n => n.id === 'swimlane-traces')
    if (swimlaneNode) {
      const siblingsAfter = newNodes.filter(n => n.parentId === 'swimlane-traces' && n.type === 'stage')
      swimlaneNode.width = Math.max(400, siblingsAfter.length * 220 + 100)
    }
    nodes = newNodes; edges = newEdges
    snippetModal = false
    snippetConfig = null
  }

  let showMiniMap = $state(true)

  // ─── Derived: has any stage nodes ─────────────────────────────────────────
  let hasStages = $derived(nodes.some((n) => n.type === 'stage'))
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flow-container" onkeydown={(e) => e.key === 'Escape' && handleCancelDelete()}>
  <header class="builder-header">
    <h1>Pipeline Graph</h1>
  <div class="actions">
    <button onclick={handleValidate} class="btn btn-primary">Validate</button>
  </div>
  </header>

  <div class="flow-layout">
    <StagePalette {onImport} onSnippetView={handleSnippetView} />
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="canvas-area" ondragover={handleDragOver} ondrop={handleDrop}>
      {#if connectError}
        <div class="toast-error" role="alert">
          {connectError}
        </div>
      {/if}
      {#if !hasStages}
        <div class="empty-state">
          <p>Drag stages from the palette to begin building your pipeline.</p>
          <p class="hint">Or click "Import YAML" to load an existing pipeline.</p>
        </div>
      {/if}
      <div class="svelte-flow-wrapper" class:empty={!hasStages}>
        <SvelteFlow
          {nodeTypes}
          bind:nodes
          bind:edges
          fitView
          minZoom={0.15}
          fitViewOptions={{ padding: 0.5 }}
          onconnect={handleConnect}
          onnodeclick={handleNodeClick}
          onnodedragstop={handleNodeDragStop}
          onpaneclick={() => onNodeSelected?.(null)}
        >
          <Controls />
          <Background variant={BackgroundVariant.Dots} />
          {#if showMiniMap}
            <MiniMap pannable zoomable nodeStrokeColor="#334155" nodeColor="#1e293b" maskColor="rgba(0,0,0,0.7)" style={{ width: 120, height: 80 }} />
          {/if}
          <button class="minimap-toggle nodrag nopan" onclick={() => showMiniMap = !showMiniMap} title="Toggle minimap">🗺</button>
        </SvelteFlow>
      </div>
      {#if deleteConfirmNodeId}
        <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
        <div class="confirm-overlay" onclick={handleCancelDelete} role="dialog" aria-modal="true">
          <div class="confirm-dialog" onclick={(e) => e.stopPropagation()}>
            <p>Delete {deleteConfirmNodeId}?</p>
            <div class="confirm-actions">
              <button class="confirm-btn confirm-btn-secondary" onclick={handleCancelDelete}>No</button>
              <button class="confirm-btn confirm-btn-danger" onclick={handleConfirmDelete}>Yes</button>
            </div>
          </div>
        </div>
      {/if}

      {#if snippetModal && snippetConfig}
        <div class="modal-overlay" onclick={() => { snippetModal = false; snippetConfig = null }} role="dialog">
          <div class="snippet-modal" onclick={(e) => e.stopPropagation()}>
            <h3>{snippetConfig.name}</h3>
            <p class="modal-hint">Type: <strong>{snippetConfig.type}</strong></p>
            <p class="modal-hint">{snippetConfig.description}</p>
            <label class="modal-label" for="snippet-config-textarea">Processor Configuration (edit before applying):</label>
            <textarea id="snippet-config-textarea" class="modal-textarea" rows="10">{JSON.stringify(snippetConfig.processor, null, 2)}</textarea>
            <div class="modal-actions">
              <button class="confirm-btn confirm-btn-secondary" onclick={() => { snippetModal = false; snippetConfig = null }}>Cancel</button>
              <button class="confirm-btn confirm-btn-primary" onclick={handleSnippetApply}>Apply to Pipeline</button>
            </div>
          </div>
        </div>
      {/if}
    </div>
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

  .canvas-area {
    flex: 1;
    position: relative;
    overflow: hidden;
  }

  .canvas-area :global(.svelte-flow) {
    font-family: var(--font-family, inherit);
    background: #0f172a;
  }

  .canvas-area :global(.svelte-flow__renderer) {
    background: #0f172a;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--color-text-secondary, #94a3b8);
    text-align: center;
    padding: var(--space-xl, 2rem);
  }

  .empty-state p {
    margin: var(--space-xs, 0.25rem) 0;
    font-size: var(--font-size-md, 0.9rem);
  }

  .empty-state .hint {
    font-size: var(--font-size-sm, 0.8rem);
    opacity: 0.7;
  }

  .svelte-flow-wrapper {
    position: absolute;
    inset: 0;
    z-index: 1;
  }
  .svelte-flow-wrapper.empty {
    opacity: 0.3;
    pointer-events: all;
  }

  .confirm-overlay {
    position: absolute;
    inset: 0;
    z-index: 100;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .confirm-dialog {
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    padding: var(--space-lg, 1rem);
    min-width: 200px;
    color: var(--color-text, #f1f5f9);
  }

  .confirm-dialog p {
    margin: 0 0 var(--space-md, 0.75rem) 0;
    font-size: 0.9rem;
  }

  .confirm-actions {
    display: flex;
    gap: var(--space-sm, 0.5rem);
    justify-content: flex-end;
  }

  .confirm-btn {
    padding: 0.4rem 0.9rem;
    border: none;
    border-radius: var(--radius-md, 6px);
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
  }

  .confirm-btn-secondary {
    background: var(--color-surface-hover, #334155);
    color: var(--color-text, #f1f5f9);
  }

  .confirm-btn-primary {
    background: var(--color-primary, #3b82f6);
    color: #fff;
  }

  .confirm-btn-primary:hover {
    background: var(--color-primary-hover, #2563eb);
  }

  .confirm-btn-danger {
    background: var(--color-error, #ef4444);
    color: #fff;
  }

  .modal-overlay {
    position: absolute;
    inset: 0;
    z-index: 200;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .snippet-modal {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 1.5rem;
    max-width: 500px;
    width: 90%;
    color: #f1f5f9;
  }

  .snippet-modal h3 {
    margin: 0 0 0.3rem;
    font-size: 1rem;
  }

  .modal-hint {
    font-size: 0.8rem;
    color: #94a3b8;
    margin: 0.2rem 0;
  }

  .modal-label {
    font-size: 0.8rem;
    display: block;
    margin: 0.5rem 0 0.3rem;
    color: #cbd5e1;
  }

  .modal-textarea {
    width: 100%;
    background: #0f172a;
    color: #e2e8f0;
    border: 1px solid #334155;
    border-radius: 4px;
    padding: 0.5rem;
    font-family: monospace;
    font-size: 0.8rem;
    resize: vertical;
    box-sizing: border-box;
  }

  .modal-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.75rem;
    justify-content: flex-end;
  }

  .toast-error {
    position: absolute;
    top: var(--space-md, 0.75rem);
    left: 50%;
    transform: translateX(-50%);
    z-index: 200;
    background: var(--color-error, #ef4444);
    color: #fff;
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    border-radius: var(--radius-md, 6px);
    font-size: 0.85rem;
    font-weight: 500;
    box-shadow: var(--shadow-lg, 0 8px 16px rgba(0, 0, 0, 0.3));
    pointer-events: none;
    animation: toast-in 0.2s ease-out;
  }

  .minimap-toggle {
    position: absolute;
    bottom: 8px;
    right: 8px;
    z-index: 10;
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: 4px;
    padding: 4px 8px;
    cursor: pointer;
    font-size: 0.8rem;
    color: var(--color-text, #f1f5f9);
    line-height: 1;
  }

  .minimap-toggle:hover {
    background: var(--color-surface-hover, #334155);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(-8px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%) translateY(0);
    }
  }
</style>
