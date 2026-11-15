<script lang="ts">
  import { setContext, untrack } from 'svelte'
  import '@xyflow/svelte/dist/style.css'
  import {
    SvelteFlow,
    Controls,
    Background,
    BackgroundVariant,
    MiniMap,
  } from '@xyflow/svelte'
  import HorizontalEdge from './HorizontalEdge.svelte'

  const edgeTypes = { straight: HorizontalEdge }
  import type { Node, Edge, Connection } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import { wouldCreateCycle, reorderStage } from './flow-sync'
  import type {
    Stage,
    SignalType,
    ProcessorConfig,
    SnippetTemplate,
  } from './types'
  import { signalEdgeProps } from './types'
  import { getSnippetForSignal } from './snippets'

  // ─── Props (from SwimlaneFlowBuilder) ────────────────────────────────────
  let {
    nodes = $bindable<Node<StageNodeData>[]>([]),
    edges = $bindable<Edge[]>([]),
    nodeTypes = {},
    pipelineVersion = 0,
    hasStages = false,
    snippetModal = $bindable(false),
    snippetConfig = $bindable<SnippetTemplate | null>(null),
    targetSignal = $bindable<SignalType>('traces'),
    onSnippetApply,
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
    targetSignal: SignalType
    onSnippetApply: (snippet: SnippetTemplate) => boolean
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig; signal: SignalType }) => void
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
    const signal = (sourceNode?.parentId?.replace('swimlane-', '') || 'traces') as SignalType
    edges = [
      ...edges,
      {
        id: `e-${connection.source}-${connection.target}`,
        source: connection.source,
        target: connection.target,
        type: 'straight',
        ...signalEdgeProps(signal),
      },
    ]
  }

  // ─── Node click → inspector (stage or shared receiver) ───────────────────
  function handleNodeClick({ node }: { node: Node<StageNodeData>; event: MouseEvent | TouchEvent }) {
    const signal = (node.type === 'swimlane' ? node.id : node.parentId)?.replace('swimlane-', '')
    if (signal === 'traces' || signal === 'logs' || signal === 'metrics') targetSignal = signal
    if (node.type === 'infra' && (node.data as any)?.type === 'receiver') {
      onNodeSelected?.(node.id)
      return
    }
    if (node.type !== 'stage') return
    if (signal !== 'traces' && signal !== 'logs' && signal !== 'metrics') return
    onNodeSelected?.(node.id)
    onNodeEdit?.({ stage: node.data.stage, processor: node.data.processor, signal })
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
  let resolvedSnippet = $derived(snippetConfig ? getSnippetForSignal(snippetConfig, targetSignal) : null)
  let snippetDialog = $state<HTMLDialogElement | null>(null)

  $effect(() => {
    if (!snippetModal || !snippetConfig) return
    const dialog = snippetDialog
    if (dialog && !dialog.open) dialog.showModal()
  })

  function closeSnippetDialog() {
    if (snippetDialog?.open) snippetDialog.close()
    else {
      snippetModal = false
      snippetConfig = null
    }
  }

  function handleSnippetClose() {
    snippetModal = false
    snippetConfig = null
  }

  function handleSnippetBackdropClick(event: MouseEvent) {
    const dialog = snippetDialog
    if (!dialog || event.target !== dialog) return
    const bounds = dialog.getBoundingClientRect()
    if (event.clientX < bounds.left || event.clientX > bounds.right ||
        event.clientY < bounds.top || event.clientY > bounds.bottom) dialog.close()
  }

  function isolateSnippetKeys(dialog: HTMLDialogElement) {
    const stopPropagation = (event: KeyboardEvent) => event.stopPropagation()
    dialog.addEventListener('keydown', stopPropagation)
    return { destroy: () => dialog.removeEventListener('keydown', stopPropagation) }
  }


  function handleSnippetApply() {
    if (!snippetConfig || !resolvedSnippet || !onSnippetApply(snippetConfig)) return
    closeSnippetDialog()
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
      {edgeTypes}
      bind:nodes
      bind:edges
      fitView
      minZoom={0.10}
      fitViewOptions={{ padding: 0.3 }}
      defaultEdgeOptions={{ type: 'straight' }}
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
    <div class="confirm-overlay" onclick={handleCancelDelete} role="dialog" aria-modal="true" tabindex="-1">
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
    <dialog
      class="snippet-dialog"
      bind:this={snippetDialog}
      use:isolateSnippetKeys
      aria-labelledby="snippet-preview-heading"
      data-od-id="template-preview"
      onclose={handleSnippetClose}
      onclick={handleSnippetBackdropClick}
    >
      <div class="snippet-modal">
        <h3 id="snippet-preview-heading">{snippetConfig.name}</h3>
        <div class="snippet-body">
          <p class="modal-hint">Library · <strong>{snippetConfig.type}</strong></p>
          <p class="modal-hint">{snippetConfig.description}</p>
          {#if resolvedSnippet}
            <label class="modal-label" for="snippet-config-textarea">Configuration preview for {targetSignal}</label>
            <pre id="snippet-config-textarea" class="modal-textarea preview-pre">{JSON.stringify(resolvedSnippet.processor, null, 2)}</pre>
          {:else}
            <p role="status">This favorite is not available for {targetSignal}. Available for: {snippetConfig.signals.join(', ')}.</p>
          {/if}
        </div>
        <div class="modal-actions">
          <button class="confirm-btn confirm-btn-secondary" onclick={closeSnippetDialog}>Cancel</button>
          <button class="confirm-btn confirm-btn-primary" disabled={!resolvedSnippet} title="Add to {targetSignal}" onclick={handleSnippetApply}>Add to lane</button>
        </div>
      </div>
    </dialog>
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

  .snippet-dialog {
    width: min(500px, calc(100vw - 2rem));
    max-width: calc(100vw - 2rem);
    max-height: calc(100dvh - 2rem);
    box-sizing: border-box;
    padding: 0;
    overflow: auto;
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 8px;
    color: #f1f5f9;
  }

  .snippet-dialog::backdrop {
    background: rgba(0, 0, 0, 0.6);
  }

  .snippet-modal {
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - 2rem - 2px);
    box-sizing: border-box;
    padding: 1.5rem;
    overflow: auto;
  }

  .snippet-body {
    min-height: 0;
    overflow: auto;
  }

  .snippet-modal h3 {
    flex-shrink: 0;
    overflow-wrap: anywhere;
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
    flex-shrink: 0;
    flex-wrap: wrap;
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
