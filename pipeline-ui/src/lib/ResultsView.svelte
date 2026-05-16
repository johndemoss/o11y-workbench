<script lang="ts">
  import type { PipelineResult } from './types'

  let { result, error }: {
    result: PipelineResult | null
    error?: string
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
            <span class="stat-badge">{stage.input_span_count} spans</span>
            <span class="stat-arrow">→</span>
            <span class="stat-badge">{stage.output_span_count} spans</span>
          </div>

          {#if stage.errors && stage.errors.length > 0}
            <div class="stage-errors">
              {#each stage.errors as err}
                <div class="stage-error">{err}</div>
              {/each}
            </div>
          {/if}

          {#if expandedStages.has(stage.stage_index) && stage.intermediate_data !== undefined}
            <div class="stage-data">
              <pre>{JSON.stringify(stage.intermediate_data, null, 2)}</pre>
            </div>
          {/if}
        </div>
      {/each}
    </div>

    {#if result.final_data !== undefined}
      <div class="final-data-section">
        <button class="final-header" onclick={() => expandedFinal = !expandedFinal}>
          <span>Final Result Data</span>
          <span class="expand-icon">{expandedFinal ? '▾' : '▸'}</span>
        </button>
        {#if expandedFinal}
          <div class="final-data">
            <pre>{JSON.stringify(result.final_data, null, 2)}</pre>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .results-view {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    font-size: 0.875rem;
  }
  .error-box {
    background: #fef2f2;
    color: #991b1b;
    border: 1px solid #fecaca;
    padding: 0.75rem 1rem;
    border-radius: 0.375rem;
    font-weight: 500;
  }
  .empty-state {
    color: #6b7280;
    text-align: center;
    padding: 2rem;
    font-style: italic;
  }
  .summary-card {
    display: flex;
    gap: 1.5rem;
    background: #f9fafb;
    border: 1px solid #e5e7eb;
    border-radius: 0.5rem;
    padding: 1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .summary-stat {
    display: flex;
    flex-direction: column;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: #111827;
  }
  .stat-label {
    font-size: 0.75rem;
    color: #6b7280;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .stages {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }
  .stage-card {
    border: 1px solid #e5e7eb;
    border-radius: 0.5rem;
    overflow: hidden;
    background: #fff;
  }
  .stage-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.625rem 0.875rem;
    background: #f9fafb;
    border: none;
    border-bottom: 1px solid #e5e7eb;
    cursor: pointer;
    text-align: left;
    font-size: 0.875rem;
  }
  .stage-header:hover {
    background: #f3f4f6;
  }
  .stage-title {
    font-weight: 600;
    color: #111827;
    flex: 1;
  }
  .stage-duration {
    font-size: 0.75rem;
    color: #6b7280;
    font-variant-numeric: tabular-nums;
  }
  .expand-icon {
    color: #9ca3af;
    font-size: 0.75rem;
  }
  .stage-stats {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.875rem;
  }
  .stat-badge {
    font-size: 0.75rem;
    background: #e5e7eb;
    color: #374151;
    padding: 0.125rem 0.5rem;
    border-radius: 0.25rem;
    font-weight: 500;
  }
  .stat-arrow {
    color: #9ca3af;
    font-size: 0.875rem;
  }
  .stage-errors {
    padding: 0.25rem 0.875rem 0.5rem;
  }
  .stage-error {
    font-size: 0.75rem;
    color: #991b1b;
    background: #fef2f2;
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    margin-bottom: 0.25rem;
  }
  .stage-data {
    border-top: 1px solid #e5e7eb;
    padding: 0.75rem 0.875rem;
  }
  .stage-data pre {
    font-size: 0.75rem;
    line-height: 1.4;
    overflow-x: auto;
    max-height: 300px;
    overflow-y: auto;
    margin: 0;
  }
  .final-data-section {
    border: 1px solid #e5e7eb;
    border-radius: 0.5rem;
    overflow: hidden;
    background: #fff;
  }
  .final-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.625rem 0.875rem;
    background: #f9fafb;
    border: none;
    cursor: pointer;
    text-align: left;
    font-size: 0.875rem;
    font-weight: 600;
    color: #111827;
  }
  .final-header:hover {
    background: #f3f4f6;
  }
  .final-data {
    border-top: 1px solid #e5e7eb;
    padding: 0.75rem 0.875rem;
  }
  .final-data pre {
    font-size: 0.75rem;
    line-height: 1.4;
    overflow-x: auto;
    max-height: 400px;
    overflow-y: auto;
    margin: 0;
  }
</style>
