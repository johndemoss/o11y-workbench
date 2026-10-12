<script lang="ts">
  import type { Stage, ProcessorConfig, PipelinesBySignal, SignalType } from './types'
  import { SIGNALS } from './types'
  import StageEditor from './StageEditor.svelte'

  let {
    selection = null,
    pipelines,
    pipelineYaml = '',
    onupdate,
    onupdateProcessor,
    onMove,
    onRemove,
  }: {
    selection:
      | { kind: 'stage'; stage: Stage; processor: ProcessorConfig; signal: SignalType }
      | { kind: 'receiver'; name: string }
      | null
    pipelines: PipelinesBySignal
    pipelineYaml?: string
    onupdate?: (s: Stage) => void
    onupdateProcessor?: (config: ProcessorConfig) => void
    onMove?: (direction: 'left' | 'right') => void
    onRemove?: () => void
  } = $props()

  let usedBy = $derived.by(() => {
    if (!selection || selection.kind !== 'receiver') return [] as SignalType[]
    return SIGNALS.filter((sig) => (pipelines[sig]?.receivers ?? []).includes(selection.name))
  })

  let sharedCount = $derived(usedBy.length)

  let heading = $derived(
    selection?.kind === 'receiver'
      ? 'Receiver configuration'
      : selection?.kind === 'stage'
        ? 'Stage configuration'
        : 'Configuration',
  )

  let typeHint = $derived.by(() => {
    if (selection?.kind === 'receiver') return 'Receiver · shared'
    if (selection?.kind === 'stage') {
      const idx = pipelines[selection.signal]?.stages.findIndex((s) => s.id === selection.stage.id) ?? -1
      const n = idx >= 0 ? String(idx + 1).padStart(2, '0') : '··'
      const kind = selection.stage.type === 'transform' ? 'Transform' : 'Filter'
      return `${n} · ${kind}`
    }
    return 'No selection'
  })
</script>

<aside class="panel inspector" data-od-id="persistent-stage-inspector">
  <div class="panel-head">
    <h2>{heading}</h2>
    <span class="tiny muted" id="stage-type">{typeHint}</span>
  </div>
  <div class="panel-body">
    {#if selection?.kind === 'receiver'}
      <section data-od-id="shared-receiver-inspector">
        <h3 class="mono-title">{selection.name}</h3>
        <p class="tiny muted">
          One receiver definition · {sharedCount} pipeline {sharedCount === 1 ? 'reference' : 'references'}
        </p>
        <div class="status">
          Changes apply to every pipeline using this receiver. Selecting this card highlights every
          pipeline reference to the same receiver.
        </div>
        <h4 class="section-label">Configuration</h4>
        <p class="tiny muted">
          Export uses Collector defaults for <code>receivers.{selection.name}.protocols</code>
          (gRPC and HTTP). Protocol toggles are unavailable until the pipeline model persists
          receiver configuration objects — not invented here.
        </p>
        <h4 class="section-label">Used by</h4>
        <ul class="used-by" data-od-id="receiver-used-by">
          {#each usedBy as sig}
            <li>{sig}</li>
          {:else}
            <li class="muted">No pipeline references</li>
          {/each}
        </ul>
      </section>
    {:else if selection?.kind === 'stage'}
      <StageEditor
        stage={selection.stage}
        processor={selection.processor}
        signal={selection.signal}
        {pipelineYaml}
        onupdate={(s) => onupdate?.(s)}
        onupdateProcessor={(c) => onupdateProcessor?.(c)}
      />
      <div class="toolbar stage-actions" role="group" aria-label="Stage order">
        <button type="button" data-od-id="move-stage-left" onclick={() => onMove?.('left')}>←</button>
        <button type="button" data-od-id="move-stage-right" onclick={() => onMove?.('right')}>→</button>
        <button type="button" class="ghost danger" data-od-id="remove-stage" onclick={() => onRemove?.()}>Remove</button>
      </div>
    {:else}
      <p class="tiny muted">Select a stage or shared receiver in the graph to edit configuration.</p>
    {/if}
  </div>
</aside>

<style>
  .inspector {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    min-width: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border);
    min-height: 58px;
    flex-shrink: 0;
  }

  .panel-head h2 { font-size: 14px; margin: 0; }
  .panel-body { padding: 12px 14px; overflow: auto; flex: 1; min-height: 0; }
  .panel-head .tiny {
    font: 12px var(--font-mono);
  }
  .tiny { font-size: 12px; }
  .muted { color: var(--color-text-secondary); }
  .mono-title {
    font: 14px var(--font-mono);
    margin: 0 0 8px;
    overflow-wrap: anywhere;
  }
  .status {
    padding: 12px;
    margin-top: 12px;
    font-size: 12px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }
  .section-label {
    margin: 16px 0 8px;
    font-size: 12px;
    font-weight: 600;
  }
  .used-by {
    padding-left: 20px;
    font: 12px/1.8 var(--font-mono);
    margin: 0;
  }
  code { font-family: var(--font-mono); font-size: 11px; }
  .toolbar {
    display: grid;
    grid-template-columns: 36px 36px 1fr;
    gap: 6px;
    margin-top: 8px;
  }
  .toolbar button {
    font-size: 12px;
    min-height: 32px;
    padding: 4px 8px;
  }
  .ghost { background: transparent; }
  .danger { color: var(--color-error-text); }
</style>
