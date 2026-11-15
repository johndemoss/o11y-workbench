<script lang="ts">
  import { getContext } from 'svelte'
  import { Handle, Position, type NodeProps, type Node } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import { STAGE_COLORS } from './types'

  let { data, selected }: NodeProps<Node<StageNodeData>> = $props()

  let requestDelete = getContext<(id: string) => void>('requestDelete')
  let moveStage = getContext<(id: string, dir: 'up' | 'down') => void>('moveStage')

  function handleDelete(e: MouseEvent) {
    e.stopPropagation()
    requestDelete(data.stage.id)
  }

  let displayName = $derived.by(() => {
    const ref = data.stage.processor_ref
    if (!ref) return data.stage.id
    const slashIdx = ref.indexOf('/')
    if (slashIdx >= 0) return ref.slice(slashIdx + 1)
    return ref
  })

  let indexLabel = $derived(
    typeof data.stageIndex === 'number'
      ? String(data.stageIndex + 1).padStart(2, '0')
      : '··',
  )

  let contextLabel = $derived(
    data.signal === 'logs' ? 'log' : data.signal === 'metrics' ? 'datapoint' : 'span',
  )

  let border = $derived(STAGE_COLORS[data.stage.type]?.border ?? '#334155')
</script>

<div
  class="node-card node {data.stage.type}"
  class:disabled={data.disabled}
  class:selected
  style="border-top-color: {border}"
  aria-pressed={selected}
>
  <Handle type="target" position={Position.Left} style="top: 40px" />

  <div class="card-actions nodrag" aria-label="Stage actions">
    <button
      type="button"
      class="icon-btn"
      onclick={(e) => { e.stopPropagation(); moveStage(data.stage.id, 'up') }}
      title="Move left"
      aria-label="Move stage left"
    >◀</button>
    <button
      type="button"
      class="icon-btn"
      onclick={(e) => { e.stopPropagation(); moveStage(data.stage.id, 'down') }}
      title="Move right"
      aria-label="Move stage right"
    >▶</button>
    <button
      type="button"
      class="icon-btn delete"
      onclick={(e) => handleDelete(e)}
      aria-label="Delete stage"
    >×</button>
  </div>

  <small class="kind">{indexLabel} · {data.stage.type}</small>
  <strong class="node-name">{displayName}</strong>
  <small class="meta">{contextLabel} · {data.stage.error_mode ?? 'default'}</small>

  <Handle type="source" position={Position.Right} style="top: 40px" />
</div>

<style>
  .node-card {
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    gap: 0;
    width: 164px;
    min-height: 80px;
    height: 80px;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border);
    border-top-width: 3px;
    background: var(--color-surface);
    position: relative;
    box-shadow: var(--shadow-sm);
    font-family: var(--font-family);
    text-align: left;
    box-sizing: border-box;
    transition: border-color var(--transition-fast, 120ms ease), background var(--transition-fast, 120ms ease);
  }

  .node-card:hover {
    border-color: var(--color-text-secondary);
    background: var(--color-surface-hover);
  }

  .node-card.selected,
  .node-card[aria-pressed='true'] {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
    background: var(--color-surface-hover);
  }

  .kind,
  .meta {
    font-size: 11px;
    line-height: 1.4;
    color: var(--color-text-secondary);
    display: block;
  }

  .node-name {
    font: 12px/1.4 var(--font-mono);
    display: block;
    margin: 4px 0;
    overflow-wrap: anywhere;
    color: var(--color-text);
    font-weight: 600;
  }

  .card-actions {
    position: absolute;
    z-index: 2;
    top: 6px;
    right: 6px;
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 2px;
    border-radius: 6px;
    background: color-mix(in oklab, var(--color-surface) 88%, transparent);
    border: 1px solid transparent;
    opacity: 0;
    transition: opacity var(--transition-fast, 120ms ease), border-color var(--transition-fast, 120ms ease);
  }

  .node-card:hover .card-actions,
  .node-card.selected .card-actions,
  .node-card[aria-pressed='true'] .card-actions,
  .card-actions:focus-within {
    opacity: 1;
    border-color: var(--color-border);
  }

  .icon-btn {
    width: 22px;
    height: 22px;
    min-height: 22px;
    padding: 0;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--color-text-secondary);
    font-size: 11px;
    line-height: 1;
    cursor: pointer;
  }

  .icon-btn:hover {
    color: var(--color-text);
    background: var(--color-bg);
  }

  .icon-btn.delete:hover {
    color: var(--color-error-text);
    background: var(--color-error-bg);
  }

  .node-card.disabled {
    opacity: 0.45;
  }
</style>
