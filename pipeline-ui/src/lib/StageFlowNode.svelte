<script lang="ts">
  import { getContext } from 'svelte'
  import { Handle, Position, type NodeProps, type Node } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'

  let { data }: NodeProps<Node<StageNodeData>> = $props()

  let requestDelete = getContext<(id: string) => void>('requestDelete')

  const typeColors: Record<string, string> = {
    transform: '#7c3aed',
    filter: '#d97706',
  }

  let badgeColor = $derived(typeColors[data.stage.type] ?? '#64748b')

  function handleDelete() {
    requestDelete(data.stage.id)
  }
</script>

<div class="node-card">
  <Handle type="target" position={Position.Left} />

  <div class="card-content">
    <span class="drag-grip" aria-hidden="true">
      <span class="dot"></span><span class="dot"></span><span class="dot"></span>
      <span class="dot"></span><span class="dot"></span><span class="dot"></span>
    </span>

    <span class="stage-name">{data.stage.id}</span>

    <span class="type-badge" style="background: {badgeColor}">
      {data.stage.type}
    </span>
  </div>

  <button class="delete-btn nodrag" onclick={handleDelete} aria-label="Delete stage">×</button>

  <Handle type="source" position={Position.Right} />
</div>

<style>
  .node-card {
    display: flex;
    align-items: center;
    position: relative;
    width: 180px;
    height: 80px;
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    box-shadow: var(--shadow-md, 0 4px 6px rgba(0, 0, 0, 0.4));
    font-family: var(--font-family, inherit);
    transition: box-shadow 150ms ease, transform 150ms ease;
  }

  .node-card:hover {
    box-shadow: var(--shadow-lg, 0 8px 12px rgba(0, 0, 0, 0.5));
    transform: translateY(-1px);
  }

  .card-content {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 10px;
    flex: 1;
    min-width: 0;
  }

  /* ── 6-dot grip icon ── */
  .drag-grip {
    display: grid;
    grid-template-columns: 3px 3px;
    gap: 2px;
    flex-shrink: 0;
    opacity: 0.35;
    cursor: grab;
  }

  .dot {
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--color-text-secondary, #94a3b8);
  }

  .stage-name {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
    min-width: 0;
  }

  .type-badge {
    font-size: 0.6rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #fff;
    padding: 2px 6px;
    border-radius: 4px;
    flex-shrink: 0;
  }

  /* ── Delete button (hidden by default, visible on hover) ── */
  .delete-btn {
    position: absolute;
    top: 2px;
    right: 2px;
    z-index: 2;
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.4);
    cursor: pointer;
    font-size: 0.75rem;
    padding: 2px 5px;
    border-radius: var(--radius-sm, 4px);
    line-height: 1;
    opacity: 0;
    transition: opacity 150ms ease, background 150ms ease, color 150ms ease;
  }

  .node-card:hover .delete-btn {
    opacity: 1;
  }

  .delete-btn:hover {
    background: rgba(0, 0, 0, 0.25);
    color: #fff;
  }
</style>
