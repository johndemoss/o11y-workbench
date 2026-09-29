<script lang="ts">
  import { buildStageUnifiedDiffsLazy, formatJSON, highlightJSON } from './diff'
  import type { PipelineResult, StageDiff } from './types'
  import StatsBar from './StatsBar.svelte'
  import DiffPanel from './DiffPanel.svelte'

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

  let stageDiffs: StageDiff[] = $state([])
  let loadingDiff = $state(false)
  let diffError = $state<string | null>(null)

  $effect(() => {
    if (!result) {
      stageDiffs = []
      diffError = null
      return
    }
    const currentResult = result

    let cancelled = false

    async function fetchDiffs() {
      loadingDiff = true
      diffError = null

      try {
        const res = await fetch('/api/ottl/pipeline/diff', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            stages: currentResult.stages,
            original_data: originalData ?? null
          })
        })

        if (cancelled) return

        if (!res.ok) {
          const errBody = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
          throw new Error(errBody.error || `Request failed with status ${res.status}`)
        }

        stageDiffs = await res.json()
      } catch (e) {
        if (cancelled) return
        diffError = e instanceof Error ? e.message : 'Failed to compute stage diffs'
        stageDiffs = []
      } finally {
        if (!cancelled) loadingDiff = false
      }
    }

    fetchDiffs()

    return () => { cancelled = true }
  })

  let expandedRaw: Set<number> = $state(new Set())

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

  const unifiedDiffCache = buildStageUnifiedDiffsLazy()

  $effect(() => {
    void result
    void originalData
    unifiedDiffCache.clear()
  })

  function getUnifiedDiff(stageIndex: number) {
    if (!result) return []
    const before = stageIndex === 0
      ? originalData ?? null
      : result.stages[stageIndex - 1]?.intermediate_data
    const after = result.stages[stageIndex]?.intermediate_data
    return unifiedDiffCache.get(stageIndex, before, after, {
      expanded: expandedStages.has(stageIndex),
      unified: diffView === 'unified',
    })
  }

  function downloadJSON(data: unknown, filename: string) {
    const json = JSON.stringify(data, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  }
</script>

<div class="results-view">
  {#if error}
    <div class="error-box">{error}</div>
  {:else if !result}
    <div class="empty-state">Execute a pipeline to see results</div>
  {:else}
    <StatsBar finalSpanCount={result.final_span_count} stageCount={result.stages.length} />

    {#if loadingDiff}
      <div class="diff-loading">Computing stage diffs…</div>
    {/if}

    {#if diffError}
      <div class="diff-error-box">
        <strong>Diff computation failed:</strong> {diffError}
      </div>
    {/if}

    <div class="stages">
      {#each result.stages as stage}
        <div class="stage-card">
          <button class="stage-header" onclick={() => toggleStage(stage.stage_index)}>
            <span class="stage-title">Stage {stage.stage_index}: {stage.stage_id}</span>
            <span class="stage-duration">{formatDuration(stage.duration_ns)}</span>
            <span class="expand-icon">{expandedStages.has(stage.stage_index) ? '▾' : '▸'}</span>
          </button>

          <div class="stage-stats">
            <span class="stat-badge input-badge">Input: {stage.input_span_count} spans</span>
            <span class="stat-arrow">→</span>
            <span class="stat-badge output-badge">Output: {stage.output_span_count} spans</span>
          </div>

          {#if stageDiffs[stage.stage_index]}
            <DiffPanel
              stageDiff={stageDiffs[stage.stage_index]}
              unifiedDiff={getUnifiedDiff(stage.stage_index)}
              stageIndex={stage.stage_index}
              {diffView}
              onToggleView={toggleView}
            />
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
  .diff-loading {
    text-align: center;
    padding: var(--space-md, 1rem);
    color: var(--color-text-secondary, #94a3b8);
    font-style: italic;
  }
  .diff-error-box {
    background: var(--color-error-bg, #7f1d1d);
    color: var(--color-error, #ef4444);
    border: 1px solid var(--color-error, #ef4444);
    padding: 0.75rem var(--space-md, 1rem);
    border-radius: var(--radius-md, 6px);
    font-size: 0.875rem;
    margin-bottom: var(--space-md, 1rem);
  }
  .diff-error-box strong {
    font-weight: 600;
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
