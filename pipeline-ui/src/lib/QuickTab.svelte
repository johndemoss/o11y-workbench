<script lang="ts">
  import RunSelector from './RunSelector.svelte'
  import type { PipelineResult, SignalType } from './types'
  import { dump } from 'js-yaml'

  let {
    onresult,
    ondraft,
    onrunselect
  }: {
    onresult?: (result: PipelineResult | null, error?: string, runId?: string, originalTrace?: unknown, submittedYaml?: string, signal?: SignalType) => void
    ondraft?: (yaml: string) => void
    onrunselect?: (runId: string, data: unknown, signal: SignalType) => void
  } = $props()

  let quickType: 'transform' | 'filter' = $state('transform')
  let statements: string = $state('')
  let signal: SignalType = $state('traces')

  let yaml = $derived.by(() => {
    const lines = statements.split('\n').map(s => s.trim()).filter(Boolean)
    const field = signal === 'traces' ? 'span' : signal === 'logs' ? 'log_record' : 'datapoint'
    const processor = quickType === 'filter'
      ? { [signal]: { [field]: lines } }
      : { statements: lines }
    return dump({
      stages: [{ id: 'quick-stage', type: quickType, processor_ref: `${quickType}/quick` }],
      processors: { [`${quickType}/quick`]: processor },
    }, { noRefs: true, lineWidth: -1 })
  })
  $effect(() => { ondraft?.(yaml) })
</script>

<div class="quick-tab">
  <h2>Quick {quickType === 'transform' ? 'Transform' : 'Filter'}</h2>

  <div class="type-toggle">
    <label class="toggle-option {quickType === 'transform' ? 'active' : ''}">
      <input type="radio" name="quickType" value="transform" bind:group={quickType} />
      Transform
    </label>
    <label class="toggle-option {quickType === 'filter' ? 'active' : ''}">
      <input type="radio" name="quickType" value="filter" bind:group={quickType} />
      Filter
    </label>
  </div>

  <div class="statements-section">
    <label for="statements">OTTL Statements</label>
    <textarea
      id="statements"
      bind:value={statements}
      placeholder='set(attributes["service"], "demo")'
      rows="5"
    ></textarea>
  </div>

  <RunSelector pipelineYaml={yaml} bind:signal {onresult} {onrunselect} />
</div>

<style>
  .quick-tab {
    padding: var(--space-md, 1rem);
  }
  h2 {
    font-size: var(--font-size-lg, 1.1rem);
    margin-bottom: var(--space-md, 1rem);
    color: var(--color-text, #f1f5f9);
  }
  .type-toggle {
    display: flex;
    gap: var(--space-sm, 0.5rem);
    margin-bottom: var(--space-md, 1rem);
  }
  .toggle-option {
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-md, 6px);
    cursor: pointer;
    font-size: var(--font-size-sm, 0.8rem);
    color: var(--color-text-secondary, #94a3b8);
    background: var(--color-surface, #1e293b);
  }
  .toggle-option.active {
    background: var(--color-primary, #3b82f6);
    color: white;
    border-color: var(--color-primary, #3b82f6);
  }
  .toggle-option input { display: none; }
  .statements-section {
    margin-bottom: var(--space-md, 1rem);
  }
  .statements-section label {
    display: block;
    margin-bottom: var(--space-xs, 0.25rem);
    font-size: var(--font-size-sm, 0.8rem);
    color: var(--color-text-secondary, #94a3b8);
  }
  textarea {
    width: 100%;
    padding: var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-md, 6px);
    background: var(--color-bg, #0f172a);
    color: var(--color-text, #f1f5f9);
    font-family: var(--font-mono, monospace);
    font-size: var(--font-size-sm, 0.8rem);
    resize: vertical;
  }
  textarea:focus {
    outline: none;
    border-color: var(--color-primary, #3b82f6);
  }
</style>
