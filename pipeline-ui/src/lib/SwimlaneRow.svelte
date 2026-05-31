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
  import { wouldCreateCycle, reorderStage } from './flow-sync'
  import type {
    StageType,
    ProcessorConfig,
    PipelineResult,
    SnippetTemplate,
  } from './types'
  import {
    STAGE_WIDTH,
    MIN_GAP,
    STAGE_GAP_TOTAL,
    FIRST_STAGE_X,
    SWIMLANE_MIN_WIDTH,
    SWIMLANE_EXTRA,
  } from './svg-utils'

  // ─── Props (from SwimlaneFlowBuilder) ────────────────────────────────────
  let {
    nodes = $bindable<Node<StageNodeData>[]>([]),
    edges = $bindable<Edge[]>([]),
    nodeTypes = {},
    pipelineVersion = 0,
    hasStages = false,
    snippetModal = $bindable(false),
    snippetConfig = $bindable<SnippetTemplate | null>(null),
    onNodeEdit,
    onNodeSelected,
    onResultClear,
    ondragover,
    ondrop,
  }: {
    nodes: Node<StageNodeData>[]
    edges: Edge[]
    nodeTypes: Record<string, any>
    pipelineVersion: number
    hasStages: boolean
    snippetModal: boolean
    snippetConfig: SnippetTemplate | null
    onNodeEdit?: (data: { stage: any; processor: ProcessorConfig }) => void
    onNodeSelected?: (nodeId: string | null) => void
    onResultClear?: () => void
    ondragover?: (event: DragEvent) => void
    ondrop?: (event: DragEvent) => void
  } = $props()

  // ─── Internal UI state ─────────────────────────────────────────────────
  let connectError = $state<string | null>(null)
  let deleteConfirmNodeId = $state<string | null>(null)
  let showMiniMap = $state(false)

  // ─── Context: delete confirmation (consumed by StageFlowNode) ────────────
  setContext('requestDelete', (nodeId: string) => {
    deleteConfirmNodeId = nodeId
  })

  // ─── Context: move stage within swimlane (consumed by StageFlowNode) ────
  setContext('moveStage', (nodeId: string, direction: 'up' | 'down') => {
    const node = nodes.find(n => n.id === nodeId)
    if (!node) return
    const parentId = node.parentId
    const allStages = nodes
      .filter(n => n.type === 'stage' && n.parentId === parentId)
      .sort((a, b) => a.position.x - b.position.x)
    const idx = allStages.findIndex(n => n.id === nodeId)
    if (idx < 0) return
    if (direction === 'up' && idx > 0) {
      const result = reorderStage(nodes as any, edges, nodeId, allStages[idx - 1].position.x - 1)
      untrack(() => {
        nodes = result.nodes as Node<StageNodeData>[]
        edges = result.edges
      })
      onNodeSelected?.(nodeId)
    } else if (direction === 'down' && idx < allStages.length - 1) {
      const result = reorderStage(nodes as any, edges, nodeId, allStages[idx + 1].position.x + 1)
      untrack(() => {
        nodes = result.nodes as Node<StageNodeData>[]
        edges = result.edges
      })
      onNodeSelected?.(nodeId)
    }
  })

  // ─── Connect (user drags between handles) ────────────────────────────────
  async function handleConnect(connection: Connection) {
    onResultClear?.()
    if (!connection.source || !connection.target) return
    if (connection.source === connection.target) {
      connectError = 'Cannot connect a stage to itself'
      setTimeout(() => { connectError = null }, 2500)
      return
    }
    const duplicate = edges.some(
      (e) => e.source === connection.source && e.target === connection.target,
    )
    if (duplicate) {
      connectError = 'Connection already exists'
      setTimeout(() => { connectError = null }, 2500)
      return
    }
    if (await wouldCreateCycle(edges, connection)) {
      connectError = 'Cannot create a cycle in the pipeline'
      setTimeout(() => { connectError = null }, 2500)
      return
    }
    const sourceNode = nodes.find((n) => n.id === connection.source)
    const targetNode = nodes.find((n) => n.id === connection.target)
    if (
      sourceNode && targetNode &&
      sourceNode.parentId && targetNode.parentId &&
      sourceNode.parentId !== targetNode.parentId
    ) {
      connectError = 'Cannot connect stages across different signals'
      setTimeout(() => { connectError = null }, 2500)
      return
    }
    edges = [
      ...edges,
      {
        id: `e-${connection.source}-${connection.target}`,
        source: connection.source,
        target: connection.target,
        type: 'straight',
        markerEnd: { type: MarkerType.ArrowClosed },
      },
    ]
  }

  // ─── Node click → open StageEditor ──────────────────────────────────────
  function handleNodeClick({ node, event }: { node: Node<StageNodeData>; event: MouseEvent | TouchEvent }) {
    if (node.type !== 'stage') return
    onNodeSelected?.(node.id)
    onNodeEdit?.({ stage: node.data.stage, processor: node.data.processor })
  }

  // ─── Node drag stop → reorder within swimlane ───────────────────────────
  function handleNodeDragStop({ targetNode, nodes: flowNodes }: {
    targetNode: Node | null
    nodes: Node[]
    event: MouseEvent | TouchEvent
  }) {
    if (!targetNode || targetNode.type !== 'stage') return
    const result = reorderStage(flowNodes as any, edges, targetNode.id, targetNode.position.x)
    untrack(() => {
      nodes = result.nodes as Node<StageNodeData>[]
      edges = result.edges
    })
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
  function handleSnippetView(snippet: SnippetTemplate) {
    snippetConfig = snippet
    snippetModal = true
  }

  function handleSnippetApply() {
    if (!snippetConfig) return
    const snippet = snippetConfig
    const id = `snippet-${Date.now()}`
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
        x: siblings.length > 0 ? siblings[siblings.length - 1].position.x + STAGE_GAP_TOTAL : FIRST_STAGE_X,
        y: 35,
      },
      width: STAGE_WIDTH,
      draggable: false,
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
      newEdges.push({
        id: `e-${last.id}-${id}`, source: last.id, target: id,
        type: 'straight', markerEnd: { type: MarkerType.ArrowClosed },
      })
    }
    const swimlaneNode = newNodes.find(n => n.id === 'swimlane-traces')
    if (swimlaneNode) {
      const siblingsAfter = newNodes.filter(n => n.parentId === 'swimlane-traces' && n.type === 'stage')
      swimlaneNode.width = Math.max(SWIMLANE_MIN_WIDTH, siblingsAfter.length * STAGE_GAP_TOTAL + SWIMLANE_EXTRA)
    }
    nodes = newNodes; edges = newEdges
    snippetModal = false
    snippetConfig = null
  }

  // ─── Escape key handler ──────────────────────────────────────────────────
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') handleCancelDelete()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="canvas-area" ondragover={ondragover} ondrop={ondrop} onkeydown={handleKeydown}>
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
    {#key pipelineVersion}
    <SvelteFlow
      {nodeTypes}
      bind:nodes
      bind:edges
      fitView
      minZoom={0.10}
      fitViewOptions={{ padding: 0.3 }}
      onconnect={handleConnect}
      onnodeclick={handleNodeClick}
      onnodedragstop={handleNodeDragStop}
      onpaneclick={() => onNodeSelected?.(null)}
    >
      <Controls />
      <Background variant={BackgroundVariant.Dots} />
      {#if showMiniMap}
        <MiniMap pannable zoomable nodeStrokeColor="#334155" nodeColor="#1e293b" maskColor="rgba(0,0,0,0.7)" style="width: 120px; height: 80px;" />
      {/if}
      <button class="minimap-toggle nodrag nopan" onclick={() => showMiniMap = !showMiniMap} title="Toggle minimap">🗺</button>
    </SvelteFlow>
    {/key}
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

<style>
  .canvas-area {
    flex: 1;
    position: relative;
    overflow: hidden;
  }

  .empty-state {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    text-align: center;
    z-index: 5;
    pointer-events: none;
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
    --background-color: var(--color-bg, #0f172a);
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

  /* ── Zoom controls (dark theme override) ── */
  :global(.svelte-flow__controls) {
    background: #1e293b !important;
    border: 1px solid #475569 !important;
    border-radius: 8px !important;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4) !important;
  }

  :global(.svelte-flow__controls-button) {
    background: #1e293b !important;
    border-bottom: 1px solid #334155 !important;
    fill: #e2e8f0 !important;
    width: 32px !important;
    height: 32px !important;
  }

  :global(.svelte-flow__controls-button:hover) {
    background: #334155 !important;
  }

  :global(.svelte-flow__controls-button svg) {
    fill: #e2e8f0 !important;
  }

  :global(.svelte-flow__controls-button svg path) {
    fill: #e2e8f0 !important;
  }

  /* ── Minimap dark theme override ── */
  :global(.svelte-flow__minimap) {
    background: #1e293b !important;
    border: 1px solid #475569 !important;
    border-radius: 8px !important;
  }

  :global(.svelte-flow__minimap svg) {
    background: #1e293b !important;
  }

  :global(.svelte-flow__minimap svg rect:first-child) {
    fill: #1e293b !important;
  }
</style>
