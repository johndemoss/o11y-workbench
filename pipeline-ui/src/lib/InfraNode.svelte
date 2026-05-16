<script lang="ts">
  import type { NodeProps, Node } from '@xyflow/svelte'
  import { STAGE_COLORS } from './types'

  interface InfraData {
    label: string
    type: 'receiver' | 'exporter' | 'connector'
    disabled?: boolean
  }

  let { data }: NodeProps<Node<InfraData>> = $props()
</script>

<div
  class="node-card"
  class:disabled={data.disabled}
  style="border-color: {STAGE_COLORS[data.type]?.border ?? '#64748b'}"
>
  <div
    class="card-header"
    style="background: {STAGE_COLORS[data.type]?.bg ?? 'rgba(100,116,139,0.08)'}; border-bottom: 2px solid {STAGE_COLORS[data.type]?.border ?? '#64748b'}"
  >
    <span class="type-label">{data.type}</span>
  </div>

  <div class="card-body">
    <span class="node-name">{data.label}</span>
  </div>
</div>

<style>
  .node-card {
    display: flex;
    flex-direction: column;
    width: 160px;
    height: 80px;
    border-radius: 8px;
    border: 2px solid;
    overflow: hidden;
    background: rgba(30, 41, 59, 0.95);
    cursor: default;
    user-select: none;
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

  .node-card.disabled {
    opacity: 0.4;
    filter: grayscale(0.7);
  }
</style>
