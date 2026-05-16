<script lang="ts">
  import RunSelector from './RunSelector.svelte'
  import type { PipelineResult } from './types'

  let {
    pipelineYaml = '',
    onresult
  }: {
    pipelineYaml?: string
    onresult?: (result: PipelineResult | null, error?: string) => void
  } = $props()

  let quickType: 'transform' | 'filter' = $state('transform')
  let statements: string = $state('')

  let yaml = $derived.by(() => {
    const lines: string[] = []
    lines.push('stages:')
    lines.push('  - id: quick-stage')
    lines.push(`    type: ${quickType}`)
    lines.push('    processor_ref: proc-quick')
    lines.push('processors:')
    lines.push('  proc-quick:')
    lines.push('    statements:')
    const stmts = statements.split('\n').filter(s => s.trim())
    for (const s of stmts) {
      lines.push(`      - ${s.trim()}`)
    }
    return lines.join('\n')
  })
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

  <RunSelector pipelineYaml={yaml} {onresult} />
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
