<script lang="ts">
  import { setContext, untrack } from 'svelte'
  import {
    SvelteFlow,
    Controls,
    Background,
    BackgroundVariant,
    MiniMap,
    MarkerType,
    useSvelteFlow,
  } from '@xyflow/svelte'
  import type { Node, Edge, Connection } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import { flowToPipeline, wouldCreateCycle } from './flow-sync'
  import { mergeResults } from './flow-sync'
  import type { Pipeline, PipelineResult, Stage, StageType, ProcessorConfig } from './types'

  // ─── Props ────────────────────────────────────────────────────────────────
  let {
    nodes = $bindable([]),
    edges = $bindable([]),
    nodeIdCounter = $bindable(1),
    nodeTypes = {},
    pipeline,
    onValidate,
    onNodeEdit,
    result,
    onResultClear,
  }: {
    nodes: Node<StageNodeData>[]
    edges: Edge[]
    nodeIdCounter: number
    nodeTypes: Record<string, any>
    pipeline: Pipeline
    onValidate?: (p: Pipeline) => void
    onNodeEdit?: (data: { stage: Stage; processor: ProcessorConfig }) => void
    result?: PipelineResult | null
    onResultClear?: () => void
  } = $props()

  // Try to get screenToFlowPosition from SvelteFlow context (needed for proper drop positioning)
  let screenToFlowPosition: ((pos: { x: number; y: number }) => { x: number; y: number }) | undefined
  try {
    ({ screenToFlowPosition } = useSvelteFlow())
  } catch {
    // SvelteFlow context not yet available — drop handler will use fallback positioning
  }

  // ─── State ────────────────────────────────────────────────────────────────
  let deleteConfirmNodeId: string | null = $state(null)
  let connectError: string | null = $state(null)

  // ─── Results integration (badges + animated edges) ──────────────────────────
  // NOTE: mergedResultRef is intentionally a plain let (not $state/$state.raw)
  // so setting it DOES NOT trigger reactivity. This prevents the effect from
  // re-firing after the guard variable is updated.
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

  // ─── Drop (add new stage node at a standard position) ─────────────────────
  function handleDrop(event: DragEvent) {
    event.preventDefault()
    onResultClear?.()
    const stageType = event.dataTransfer?.getData('application/svelteflow')
    if (!stageType || (stageType !== 'transform' && stageType !== 'filter')) return

    const id = `stage-${nodeIdCounter++}`
    const procName = `proc-${id}`
    const stageIndex = nodes.length

    // Get position from drop coordinates
    let position: { x: number; y: number }
    if (screenToFlowPosition) {
      position = screenToFlowPosition({ x: event.clientX, y: event.clientY })
    } else {
      // Fallback: calculate from canvas bounds
      const canvas = event.currentTarget as HTMLElement
      const rect = canvas.getBoundingClientRect()
      position = {
        x: event.clientX - rect.left - 100,
        y: event.clientY - rect.top - 30,
      }
    }

    const newNode: Node<StageNodeData> = {
      id,
      type: 'stage',
      position,
      data: {
        stage: { id, type: stageType as StageType, processor_ref: procName },
        processor: {},
        stageIndex,
      },
    }

    const newNodes = [...nodes, newNode]
    const newEdges = [...edges]

    // Auto-connect from the last existing node to the new one
    if (nodes.length > 0) {
      const lastId = nodes[nodes.length - 1].id
      newEdges.push({
        id: `e-${lastId}-${id}`,
        source: lastId,
        target: id,
        type: 'smoothstep',
        markerEnd: { type: MarkerType.ArrowClosed },
      })
    }

    nodes = newNodes
    edges = newEdges
  }

  // ─── Connect (user drags between handles) ────────────────────────────────
  async function handleConnect(connection: Connection) {
    onResultClear?.()
    // Reject incomplete connections
    if (!connection.source || !connection.target) return

    // Prevent self-connections
    if (connection.source === connection.target) {
      connectError = 'Cannot connect a stage to itself'
      setTimeout(() => { connectError = null }, 2500)
      return
    }

    // Prevent duplicate connections
    const duplicate = edges.some(
      (e) => e.source === connection.source && e.target === connection.target,
    )
    if (duplicate) {
      connectError = 'Connection already exists'
      setTimeout(() => { connectError = null }, 2500)
      return
    }

    // Prevent cycle-causing connections
    if (await wouldCreateCycle(edges, connection)) {
      connectError = 'Cannot create a cycle in the pipeline'
      setTimeout(() => { connectError = null }, 2500)
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
    onNodeEdit?.({ stage: node.data.stage, processor: node.data.processor })
  }

  // ─── Validate ─────────────────────────────────────────────────────────────
  function handleValidate() {
    onValidate?.(flowToPipeline(nodes, edges, pipeline))
  }

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
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="canvas-area" ondragover={handleDragOver} ondrop={handleDrop} onkeydown={(e) => e.key === 'Escape' && handleCancelDelete()}>
  {#if connectError}
    <div class="toast-error" role="alert">
      {connectError}
    </div>
  {/if}
  {#if nodes.length === 0}
    <div class="empty-state">
      <p>Drag stages from the palette to begin building your pipeline.</p>
      <p class="hint">Or click "Import YAML" to load an existing pipeline.</p>
    </div>
  {:else}
    <SvelteFlow
      {nodeTypes}
      bind:nodes
      bind:edges
      fitView
      onconnect={handleConnect}
      onnodeclick={handleNodeClick}
    >
      <Controls />
      <Background variant={BackgroundVariant.Dots} />
      <MiniMap />
    </SvelteFlow>
  {/if}
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
</div>

<style>
  .canvas-area {
    flex: 1;
    position: relative;
    overflow: hidden;
  }

  .canvas-area :global(.svelte-flow) {
    font-family: var(--font-family, inherit);
    --background-color: var(--color-bg, #0f172a);
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

  .confirm-overlay {
    position: absolute; inset: 0; z-index: 100;
    background: rgba(0,0,0,0.5);
    display: flex; align-items: center; justify-content: center;
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

  .confirm-btn-danger {
    background: var(--color-error, #ef4444);
    color: #fff;
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
    box-shadow: var(--shadow-lg, 0 8px 16px rgba(0,0,0,0.3));
    pointer-events: none;
    animation: toast-in 0.2s ease-out;
  }

  @keyframes toast-in {
    from { opacity: 0; transform: translateX(-50%) translateY(-8px); }
    to { opacity: 1; transform: translateX(-50%) translateY(0); }
  }
</style>
