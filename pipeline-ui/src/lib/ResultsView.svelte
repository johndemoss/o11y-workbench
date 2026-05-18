<script lang="ts">
  import { computeAllStageDiffs, computeUnifiedDiff, base64ToHex, isZeroId, convertTraceSpanIds } from './diff'
  import type { UnifiedDiffLine } from './diff'
  import type { PipelineResult, StageDiff } from './types'

  let { result, error, originalData }: {
    result: PipelineResult | null
    error?: string
    originalData?: unknown
  } = $props()

  let expandedStages: Set<number> = $state(new Set())
  let expandedFinal = $state(false)

  function toggleStage(index: number) {
    const next = new Set(expandedStages)
    if (next.has(index)) next.delete(index)
    else next.add(index)
    expandedStages = next
  }

  function formatDuration(ns: number): string {
    if (ns < 1000) return `${ns}ns`
    if (ns < 1_000_000) return `${(ns / 1000).toFixed(2)}µs`
    if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)}ms`
    return `${(ns / 1_000_000_000).toFixed(3)}s`
  }

  function inferStageType(id: string): string {
    if (id.includes('transform')) return 'transform'
    if (id.includes('filter')) return 'filter'
    return 'unknown'
  }

  let stageDiffs: StageDiff[] = $derived(
    result ? computeAllStageDiffs(result.stages, originalData) : []
  )

  let expandedDiffs: Set<number> = $state(new Set())
  let expandedRaw: Set<number> = $state(new Set())

  function toggleDiff(index: number) {
    const next = new Set(expandedDiffs)
    if (next.has(index)) next.delete(index)
    else next.add(index)
    expandedDiffs = next
  }

  function toggleRaw(index: number) {
    const next = new Set(expandedRaw)
    if (next.has(index)) next.delete(index)
    else next.add(index)
    expandedRaw = next
  }

  let diffView: 'inline' | 'unified' = $state('inline')

  function toggleView() {
    diffView = diffView === 'inline' ? 'unified' : 'inline'
  }

  let stageUnifiedDiffs: UnifiedDiffLine[][] = $derived(
    result ? result.stages.map((stage, i) => {
      const before = i === 0 ? originalData ?? null : result.stages[i - 1].intermediate_data
      const after = stage.intermediate_data
      return computeUnifiedDiff(convertTraceSpanIds(before), convertTraceSpanIds(after))
    }) : []
  )

  function downloadJSON(data: unknown, filename: string) {
    const json = JSON.stringify(convertTraceSpanIds(data), null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  }

  function formatValue(val: unknown): string {
    if (val === null || val === undefined) return 'null'
    if (typeof val === 'string') return `"${val}"`
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
  }

  function formatJSON(val: unknown): string {
    return JSON.stringify(convertTraceSpanIds(val), null, 2)
  }

  function highlightJSON(str: string): string {
    const escaped = str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    return escaped.replace(
      /("(?:[^"\\]|\\.)*")\s*:|("(?:[^"\\]|\\.)*")|(\btrue\b|\bfalse\b)|(\bnull\b)|(-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b)/g,
      (_m, key, str, bool, nullVal, num) => {
        if (key) return `<span class="json-key">${key}</span>:`
        if (str) return `<span class="json-string">${str}</span>`
        if (bool) return `<span class="json-bool">${bool}</span>`
        if (nullVal) return `<span class="json-null">${nullVal}</span>`
        if (num) return `<span class="json-number">${num}</span>`
        return _m
      }
    )
  }
</script>

<div class="results-view">
  {#if error}
    <div class="error-box">{error}</div>
  {:else if !result}
    <div class="empty-state">Execute a pipeline to see results</div>
  {:else}
    <div class="summary-card">
      <div class="summary-stat">
        <span class="stat-value">{result.final_span_count}</span>
        <span class="stat-label">Final Span Count</span>
      </div>
      <div class="summary-stat">
        <span class="stat-value">{result.stages.length}</span>
        <span class="stat-label">Stages</span>
      </div>
    </div>

    <div class="stages">
      {#each result.stages as stage}
        <div class="stage-card">
          <button class="stage-header" onclick={() => toggleStage(stage.stage_index)}>
            <span class="stage-title">Stage {stage.stage_index}: {stage.stage_id} ({inferStageType(stage.stage_id)})</span>
            <span class="stage-duration">{formatDuration(stage.duration_ns)}</span>
            <span class="expand-icon">{expandedStages.has(stage.stage_index) ? '▾' : '▸'}</span>
          </button>

          <div class="stage-stats">
            <span class="stat-badge input-badge">Input: {stage.input_span_count} spans</span>
            <span class="stat-arrow">→</span>
            <span class="stat-badge output-badge">Output: {stage.output_span_count} spans</span>
          </div>

          {#if stageDiffs[stage.stage_index]}
            <div class="stage-diff">
              <button class="diff-toggle" onclick={() => toggleDiff(stage.stage_index)}>
                <span class="expand-icon">{expandedDiffs.has(stage.stage_index) ? '▾' : '▸'}</span>
                <span>Show Diff</span>
                <span class="diff-counts">
                  <span class="count-added">+{stageDiffs[stage.stage_index].added.length}</span>
                  <span class="count-removed">-{stageDiffs[stage.stage_index].removed.length}</span>
                  <span class="count-modified">~{stageDiffs[stage.stage_index].modified.length}</span>
                </span>
              </button>
              {#if expandedDiffs.has(stage.stage_index)}
                <div class="diff-content">
                  <div class="diff-view-toggle">
                    <button class:active={diffView === 'inline'} onclick={toggleView}>Inline</button>
                    <button class:active={diffView === 'unified'} onclick={toggleView}>Unified Diff</button>
                  </div>

                  {#if diffView === 'inline'}
                    {#each stageDiffs[stage.stage_index].modified as span}
                      <div class="diff-item diff-modified">
                        <span class="diff-badge">MODIFIED</span>
                        <code class="diff-span-id" class:zero-id={isZeroId(span.span_id)}>{base64ToHex(span.span_id)}</code>
                        {#if span.changes && span.changes.length > 0}
                          <div class="diff-changes">
                            {#each span.changes as change}
                              <div class="diff-change diff-change-{change.type}">
                                <span class="change-icon">
                                  {#if change.type === 'added'}+{:else if change.type === 'removed'}-{:else}~{/if}
                                </span>
                                <span class="change-field">{change.field}</span>
                                {#if change.type === 'changed'}
                                  <span class="change-value-before">{formatValue(change.before)}</span>
                                  <span class="change-arrow">→</span>
                                  <span class="change-value-after">{formatValue(change.after)}</span>
                                {:else if change.type === 'added'}
                                  <span class="change-value-after">{formatValue(change.after)}</span>
                                {:else}
                                  <span class="change-value-before">{formatValue(change.before)}</span>
                                {/if}
                              </div>
                            {/each}
                          </div>
                        {/if}
                      </div>
                    {/each}
                    {#each stageDiffs[stage.stage_index].added as span}
                      <div class="diff-item diff-added">
                        <span class="diff-badge">ADDED</span>
                        <code class="diff-span-id" class:zero-id={isZeroId(span.span_id)}>{base64ToHex(span.span_id)}</code>
                      </div>
                    {/each}
                    {#each stageDiffs[stage.stage_index].removed as span}
                      <div class="diff-item diff-removed">
                        <span class="diff-badge">REMOVED</span>
                        <code class="diff-span-id" class:zero-id={isZeroId(span.span_id)}>{base64ToHex(span.span_id)}</code>
                      </div>
                    {/each}
                    {#if stageDiffs[stage.stage_index].added.length === 0 && stageDiffs[stage.stage_index].removed.length === 0 && stageDiffs[stage.stage_index].modified.length === 0}
                      <div class="diff-empty">No changes detected</div>
                    {/if}
                  {:else}
                    <div class="unified-diff">
                      {#if stageUnifiedDiffs[stage.stage_index] && stageUnifiedDiffs[stage.stage_index].length > 0}
                        {#each stageUnifiedDiffs[stage.stage_index] as line}
                          <div class="diff-line diff-line-{line.type}">
                            <span class="diff-prefix">{line.type === 'context' ? ' ' : line.type === 'add' ? '+' : '-'}</span>
                            <span class="diff-text">{@html highlightJSON(line.text)}</span>
                          </div>
                        {/each}
                      {:else}
                        <div class="diff-empty">No changes detected</div>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/if}

          {#if stage.errors && stage.errors.length > 0}
            <div class="stage-errors">
              {#each stage.errors as err}
                <div class="stage-error">{err}</div>
              {/each}
            </div>
          {/if}

          <button class="raw-toggle" onclick={() => toggleRaw(stage.stage_index)}>
            <span class="expand-icon">{expandedRaw.has(stage.stage_index) ? '▾' : '▸'}</span>
            Show Raw Data
          </button>
          {#if expandedRaw.has(stage.stage_index) && stage.intermediate_data !== undefined}
            <pre class="raw-data">{@html highlightJSON(formatJSON(stage.intermediate_data))}</pre>
          {/if}
        </div>
      {/each}
    </div>

    {#if result.final_data !== undefined}
      <div class="final-data-section">
        <div class="final-header-row">
          <button class="final-header" onclick={() => expandedFinal = !expandedFinal}>
            <span>Final Result Data</span>
            <span class="expand-icon">{expandedFinal ? '▾' : '▸'}</span>
          </button>
          <button class="export-btn" onclick={() => downloadJSON(result.final_data, 'pipeline-result.json')} title="Export as JSON">
            Export JSON
          </button>
        </div>
        {#if expandedFinal}
          <div class="final-data">
            <pre>{@html highlightJSON(formatJSON(result.final_data))}</pre>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .results-view {
    font-family: var(--font-family, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
    font-size: 0.875rem;
  }
  .error-box {
    background: var(--color-error-bg, #7f1d1d);
    color: var(--color-error, #ef4444);
    border: 1px solid var(--color-error, #ef4444);
    padding: 0.75rem var(--space-md, 1rem);
    border-radius: var(--radius-md, 6px);
    font-weight: 500;
  }
  .empty-state {
    color: var(--color-text-secondary, #94a3b8);
    text-align: center;
    padding: var(--space-xl, 2rem);
    font-style: italic;
  }
  .summary-card {
    display: flex;
    gap: var(--space-lg, 1.5rem);
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    padding: var(--space-md, 1rem) 1.25rem;
    margin-bottom: var(--space-md, 1rem);
  }
  .summary-stat {
    display: flex;
    flex-direction: column;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--color-text, #f1f5f9);
  }
  .stat-label {
    font-size: 0.75rem;
    color: var(--color-text-secondary, #94a3b8);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .stages {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: var(--space-md, 1rem);
  }
  .stage-card {
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    overflow: hidden;
    background: var(--color-surface, #1e293b);
  }
  .stage-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.625rem 0.875rem;
    background: var(--color-surface, #1e293b);
    border: none;
    border-bottom: 1px solid var(--color-border, #334155);
    cursor: pointer;
    text-align: left;
    font-size: 0.875rem;
  }
  .stage-header:hover {
    background: var(--color-surface-hover, #334155);
  }
  .stage-title {
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
    flex: 1;
  }
  .stage-duration {
    font-size: 0.75rem;
    color: var(--color-text-secondary, #94a3b8);
    font-variant-numeric: tabular-nums;
  }
  .expand-icon {
    color: var(--color-text-secondary, #94a3b8);
    font-size: 0.75rem;
  }
  .stage-stats {
    display: flex;
    align-items: center;
    gap: var(--space-sm, 0.5rem);
    padding: var(--space-sm, 0.5rem) 0.875rem;
  }
  .stat-badge {
    font-size: 0.75rem;
    background: var(--color-border, #334155);
    color: var(--color-text, #f1f5f9);
    padding: 0.125rem var(--space-sm, 0.5rem);
    border-radius: var(--radius-sm, 4px);
    font-weight: 500;
  }
  .stat-arrow {
    color: var(--color-text-secondary, #94a3b8);
    font-size: 0.875rem;
  }
  .stage-errors {
    padding: 0.25rem 0.875rem var(--space-sm, 0.5rem);
  }
  .stage-error {
    font-size: 0.75rem;
    color: var(--color-error, #ef4444);
    background: var(--color-error-bg, #7f1d1d);
    padding: 0.25rem var(--space-sm, 0.5rem);
    border-radius: var(--radius-sm, 4px);
    margin-bottom: var(--space-xs, 0.25rem);
  }
  .stage-data {
    border-top: 1px solid var(--color-border, #334155);
    padding: 0.75rem 0.875rem;
  }
  .final-data-section {
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    overflow: hidden;
    background: var(--color-surface, #1e293b);
  }
  .final-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.625rem 0.875rem;
    background: var(--color-surface, #1e293b);
    border: none;
    cursor: pointer;
    text-align: left;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
  }
  .final-header:hover {
    background: var(--color-surface-hover, #334155);
  }
  .final-data {
    border-top: 1px solid var(--color-border, #334155);
    padding: 0.75rem 0.875rem;
  }
  .input-badge { color: var(--color-text-secondary, #94a3b8); }
  .output-badge { color: var(--color-text, #f1f5f9); }
  .stage-diff {
    margin-top: 0.5rem;
    border-top: 1px solid var(--color-border, #334155);
    padding-top: 0.5rem;
  }
  .diff-toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.375rem 0.5rem;
    border: none;
    background: var(--color-surface, #1e293b);
    color: var(--color-text, #f1f5f9);
    cursor: pointer;
    font-size: 0.85rem;
    border-radius: var(--radius-sm, 4px);
  }
  .diff-toggle:hover {
    background: var(--color-surface-hover, #334155);
  }
  .diff-counts {
    margin-left: auto;
    display: flex;
    gap: 0.5rem;
    font-size: 0.8rem;
  }
  .count-added { color: var(--color-success, #22c55e); }
  .count-removed { color: var(--color-error, #ef4444); }
  .count-modified { color: var(--color-warning, #f59e0b); }
  .diff-content {
    padding: var(--space-sm, 0.5rem);
  }
  .diff-item {
    display: flex;
    align-items: center;
    gap: var(--space-sm, 0.5rem);
    padding: var(--space-xs, 0.25rem) 0;
    font-size: 0.85rem;
  }
  .diff-added { color: var(--color-success, #22c55e); }
  .diff-removed { color: var(--color-error, #ef4444); }
  .diff-modified { color: var(--color-warning, #f59e0b); }
  .diff-badge {
    font-size: 0.7rem;
    font-weight: 600;
    padding: 1px 4px;
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface-hover, #334155);
  }
  .diff-span-id {
    font-family: var(--font-mono, monospace);
    font-size: 0.8rem;
  }
  .diff-span-id.zero-id {
    color: var(--color-error, #ef4444);
    font-weight: bold;
    text-decoration: underline wavy var(--color-error, #ef4444);
  }
  .diff-empty {
    color: var(--color-text-secondary, #94a3b8);
    font-style: italic;
    font-size: 0.85rem;
  }
  .raw-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-xs, 0.25rem);
    padding: var(--space-xs, 0.25rem) var(--space-sm, 0.5rem);
    border: none;
    background: transparent;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    font-size: 0.8rem;
    margin-top: var(--space-xs, 0.25rem);
  }
  .raw-toggle:hover {
    color: var(--color-text, #f1f5f9);
  }
  .raw-data {
    background: var(--color-bg, #0f172a);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    padding: var(--space-sm, 0.5rem);
    font-family: var(--font-mono, monospace);
    font-size: 0.75rem;
    overflow-x: auto;
    max-height: 300px;
    overflow-y: auto;
    color: var(--color-text-secondary, #94a3b8);
  }
  .diff-view-toggle {
    display: flex;
    gap: var(--space-xs, 0.25rem);
    margin-bottom: var(--space-sm, 0.5rem);
  }
  .diff-view-toggle button {
    padding: 0.2rem 0.6rem;
    font-size: 0.75rem;
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface, #1e293b);
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
  }
  .diff-view-toggle button.active {
    background: var(--color-primary, #3b82f6);
    color: white;
    border-color: var(--color-primary, #3b82f6);
  }
  .diff-changes {
    margin-top: 0.25rem;
    margin-left: 1.5rem;
    font-size: 0.8rem;
  }
  .diff-change {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0;
    font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace);
  }
  .diff-change-added { color: var(--color-success, #22c55e); }
  .diff-change-removed { color: var(--color-error, #ef4444); }
  .diff-change-changed { color: var(--color-warning, #eab308); }
  .change-icon { font-weight: bold; width: 1rem; text-align: center; }
  .change-field { font-weight: 500; }
  .change-arrow { color: var(--color-text-secondary, #94a3b8); }
  .change-value-before { color: var(--color-error, #ef4444); text-decoration: line-through; }
  .change-value-after { color: var(--color-success, #22c55e); }
  .unified-diff {
    font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace);
    font-size: 0.75rem;
    line-height: 1.5;
    background: var(--color-bg, #0f172a);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    overflow-x: auto;
    max-height: 400px;
    overflow-y: auto;
  }
  .diff-line {
    display: flex;
    padding: 0 0.5rem;
    min-height: 1.25rem;
  }
  .diff-line-context { background: transparent; }
  .diff-line-add { background: rgba(34, 197, 94, 0.1); }
  .diff-line-remove { background: rgba(239, 68, 68, 0.1); }
  .diff-prefix {
    width: 1.2rem;
    flex-shrink: 0;
    color: var(--color-text-secondary, #64748b);
    text-align: center;
    user-select: none;
  }
  .diff-line-add .diff-prefix { color: var(--color-success, #22c55e); }
  .diff-line-remove .diff-prefix { color: var(--color-error, #ef4444); }
  .diff-text { white-space: pre; }

  /* JSON syntax highlighting
     Note: :global() needed because highlightJSON() injects spans via {@html},
     which bypasses Svelte's scoped CSS. */
  :global(.json-key) { color: #93c5fd; }
  :global(.json-string) { color: #86efac; }
  :global(.json-number) { color: #fca5a5; }
  :global(.json-bool) { color: #c084fc; }
  :global(.json-null) { color: #94a3b8; font-style: italic; }

  /* Final data section header row with export button */
  .final-header-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--color-border, #334155);
  }
  .final-header-row .final-header {
    border-bottom: none;
  }
  .export-btn {
    margin-right: 0.5rem;
    padding: 0.4rem 0.9rem;
    border: none;
    border-radius: var(--radius-md, 6px);
    background: var(--color-primary, #3b82f6);
    color: var(--color-text, #f1f5f9);
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
    white-space: nowrap;
  }
  .export-btn:hover {
    background: var(--color-primary-hover, #2563eb);
  }
</style>
