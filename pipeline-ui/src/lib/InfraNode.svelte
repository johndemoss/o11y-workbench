<script lang="ts">
  import type { NodeProps, Node } from '@xyflow/svelte'
  import { Position, Handle } from '@xyflow/svelte'
  import { STAGE_COLORS } from './types'

  interface InfraData {
    label: string
    type: 'receiver' | 'exporter' | 'connector'
    disabled?: boolean
    sharedCount?: number
    [key: string]: unknown
  }

  let { data, selected }: NodeProps<Node<InfraData>> = $props()

  let border = $derived(STAGE_COLORS[data.type]?.border ?? '#64748b')
  let sharedCount = $derived(
    data.type === 'receiver' && typeof data.sharedCount === 'number' ? data.sharedCount : null,
  )
</script>

<div
  class="node-card node {data.type}"
  class:disabled={data.disabled}
  class:selected
  class:receiver={data.type === 'receiver'}
  style="border-top-color: {border}"
  aria-pressed={selected}
  data-receiver={data.type === 'receiver' ? data.label : undefined}
>
  {#if data.type === 'receiver'}
    <Handle type="source" position={Position.Right} style="top: 40px" />
  {/if}
  {#if data.type === 'exporter'}
    <Handle type="target" position={Position.Left} style="top: 40px" />
  {/if}

  <div class="title-row">
    <small class="kind">{data.type}</small>
    {#if sharedCount !== null}
      <span
        class="shared-badge"
        title="Referenced by {sharedCount} pipelines"
        aria-label="Shared by {sharedCount} pipelines"
      >
        <span class="shared-rings" aria-hidden="true"></span>
        <span class="shared-count">{sharedCount}</span>
      </span>
    {/if}
  </div>
  <strong class="node-name">{data.label}</strong>
  {#if sharedCount !== null}
    <small class="meta">shared</small>
  {:else if data.type === 'exporter'}
    <small class="meta">export</small>
  {:else}
    <small class="meta">&nbsp;</small>
  {/if}
</div>

<style>
  .node-card {
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    gap: 0;
    width: 110px;
    min-height: 80px;
    height: 80px;
    padding: 10px 8px;
    border-radius: 8px;
    border: 1px solid var(--color-border);
    border-top-width: 3px;
    background: var(--color-surface);
    box-sizing: border-box;
    text-align: left;
    box-shadow: var(--shadow-sm);
    font-family: var(--font-family);
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

  .title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
    min-width: 0;
  }

  .kind,
  .meta {
    display: block;
    font-size: 11px;
    line-height: 1.4;
    color: var(--color-text-secondary);
    min-width: 0;
  }

  .shared-badge {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-height: 18px;
    padding: 1px 6px 1px 4px;
    border-radius: 999px;
    border: 1px solid color-mix(in oklab, var(--focus) 50%, var(--color-border));
    background: color-mix(in oklab, var(--focus) 16%, var(--color-bg));
    color: var(--color-info-text, var(--color-text));
  }

  /* Two overlapping rings — compact "shared/link" mark without emoji. */
  .shared-rings {
    position: relative;
    width: 10px;
    height: 8px;
    flex-shrink: 0;
  }

  .shared-rings::before,
  .shared-rings::after {
    content: '';
    position: absolute;
    top: 0;
    width: 7px;
    height: 7px;
    border: 1.5px solid currentColor;
    border-radius: 50%;
    box-sizing: border-box;
  }

  .shared-rings::before { left: 0; opacity: 0.85; }
  .shared-rings::after { left: 3px; opacity: 1; }

  .shared-count {
    font: 600 10px/1 var(--font-mono);
  }

  .node-name {
    font: 12px/1.4 var(--font-mono);
    color: var(--color-text);
    overflow-wrap: anywhere;
    font-weight: 600;
    margin: 4px 0;
    display: block;
  }

  .node-card.disabled {
    opacity: 0.4;
    filter: grayscale(0.7);
  }
</style>
