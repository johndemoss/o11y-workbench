<script lang="ts">
  import { getContext } from 'svelte'
  import { Handle, Position, type NodeProps, type Node } from '@xyflow/svelte'
  import type { StageNodeData } from './flow-sync'
  import FlowResultBadge from './FlowResultBadge.svelte'

  let { data }: NodeProps<Node<StageNodeData>> = $props()

  let requestDelete = getContext<(id: string) => void>('requestDelete')

  const typeColors: Record<string, string> = {
    transform: '#7c3aed',
    filter: '#d97706',
  }

  let headerColor = $derived(typeColors[data.stage.type] ?? '#64748b')

  let typeLabel = $derived(
    data.stage.type.charAt(0).toUpperCase() + data.stage.type.slice(1),
  )

  let resultCounts = $derived.by(() => {
    const intermediate = data.results?.intermediate_data
    if (intermediate && typeof intermediate === 'object') {
      const d = intermediate as Record<string, unknown>
      return {
        added: typeof d.added === 'number' ? d.added : 0,
        removed: typeof d.removed === 'number' ? d.removed : 0,
        modified: typeof d.modified === 'number' ? d.modified : 0,
      }
    }
    return { added: 0, removed: 0, modified: 0 }
  })

  function handleDelete() {
    requestDelete(data.stage.id)
  }
</script>

<div class="node-card">
  <Handle type="target" position={Position.Left} />

  <div class="header" style="background: {headerColor}">
    <span class="header-label">{typeLabel}</span>
    <button class="delete-btn nodrag" onclick={handleDelete} aria-label="Delete stage">
      ✕
    </button>
  </div>

  <div class="body">
    <div class="stage-id">{data.stage.id}</div>

    {#if data.stage.error_mode}
      <div class="info-row">
        <span class="info-label">Error:</span>
        <span class="info-value">{data.stage.error_mode}</span>
      </div>
    {/if}

    <div class="info-row">
      <span class="info-label">Processor:</span>
      <span class="info-value mono">{data.stage.processor_ref}</span>
    </div>

    {#if data.results}
      <div class="results-section">
        <FlowResultBadge
          added={resultCounts.added}
          removed={resultCounts.removed}
          modified={resultCounts.modified}
        />
      </div>
    {/if}
  </div>

  <Handle type="source" position={Position.Right} />
</div>

<style>
  .node-card {
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    width: 200px;
    overflow: hidden;
    font-family: var(--font-family, inherit);
    box-shadow: var(--shadow-md, 0 4px 6px rgba(0, 0, 0, 0.4));
  }

  .header {
    padding: var(--space-xs, 0.25rem) var(--space-sm, 0.5rem);
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #fff;
    min-height: 28px;
  }

  .header-label {
    opacity: 0.95;
  }

  .delete-btn {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.7);
    cursor: pointer;
    font-size: 0.75rem;
    padding: 2px 4px;
    border-radius: var(--radius-sm, 4px);
    line-height: 1;
    transition: background var(--transition-fast, 150ms ease),
                color var(--transition-fast, 150ms ease);
  }

  .delete-btn:hover {
    background: rgba(0, 0, 0, 0.25);
    color: #fff;
  }

  .body {
    padding: var(--space-sm, 0.5rem);
    display: flex;
    flex-direction: column;
    gap: var(--space-xs, 0.25rem);
  }

  .stage-id {
    font-weight: 700;
    font-size: var(--font-size-md, 0.9rem);
    color: var(--color-text, #f1f5f9);
    margin-bottom: var(--space-xs, 0.25rem);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }

  .info-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-xs, 0.25rem);
  }

  .info-label {
    font-size: var(--font-size-sm, 0.8rem);
    color: var(--color-text-secondary, #94a3b8);
    flex-shrink: 0;
  }

  .info-value {
    font-size: var(--font-size-sm, 0.8rem);
    color: var(--color-text, #f1f5f9);
  }

  .info-value.mono {
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 130px;
  }

  .results-section {
    margin-top: var(--space-xs, 0.25rem);
    padding-top: var(--space-xs, 0.25rem);
    border-top: 1px solid var(--color-border, #334155);
  }
</style>
