<script lang="ts">
  import { getContext } from 'svelte'
  import { Handle, Position, type NodeProps, type Node } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import { STAGE_COLORS } from './types'

  let { data }: NodeProps<Node<StageNodeData>> = $props()

  let requestDelete = getContext<(id: string) => void>('requestDelete')

  function handleDelete() {
    requestDelete(data.stage.id)
  }
</script>

<div
  class="node-card"
  class:disabled={data.disabled}
  style="border-color: {STAGE_COLORS[data.stage.type]?.border ?? '#334155'}"
>
  <Handle type="target" position={Position.Left} />

  <div
    class="card-header"
    style="background: {STAGE_COLORS[data.stage.type]?.bg ?? '#1e293b'}; border-bottom: 2px solid {STAGE_COLORS[data.stage.type]?.border ?? '#334155'}"
  >
    <span class="type-label">{data.stage.type}</span>
  </div>

  <div class="card-body">
    <span class="node-name">{data.stage.id}</span>
  </div>

  <button class="delete-btn nodrag" onclick={handleDelete} aria-label="Delete stage">×</button>

  <Handle type="source" position={Position.Right} />
</div>

<style>
  .node-card {
    display: flex;
    flex-direction: column;
    width: 160px;
    height: 80px;
    border-radius: 8px;
    border: 1px solid;
    overflow: hidden;
    position: relative;
    box-shadow: var(--shadow-md, 0 4px 6px rgba(0, 0, 0, 0.4));
    font-family: var(--font-family, inherit);
    transition: box-shadow 150ms ease, transform 150ms ease;
  }

  .node-card:hover {
    box-shadow: var(--shadow-lg, 0 8px 12px rgba(0, 0, 0, 0.5));
    transform: translateY(-1px);
  }

  .card-header {
    height: 25%;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2px 4px;
  }

  .type-label {
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #fff;
  }

  .card-body {
    height: 75%;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 4px;
    background: rgba(30, 41, 59, 0.95);
  }

  .node-name {
    font-size: 0.75rem;
    font-weight: 600;
    color: #f1f5f9;
    text-align: center;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    font-size: 0.8rem;
    padding: 1px 5px;
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

  .node-card.disabled {
    opacity: 0.4;
    filter: grayscale(0.7);
  }
</style>
