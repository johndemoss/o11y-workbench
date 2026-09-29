<script lang="ts">
  import { onMount } from 'svelte'
  import type { PipelineResult } from './types'

  interface Run {
    id: string
    status: string
    span_count: number
  }

  let { pipelineYaml = '', onresult, onrunselect }: {
    pipelineYaml: string
    onresult?: (result: PipelineResult | null, error?: string, runId?: string, originalTrace?: unknown) => void
    onrunselect?: (runId: string, traceData: unknown) => void
  } = $props()

  let runs: Run[] = $state([])
  let selectedRunId: string = $state('')
  let selectedRunTraceData: unknown = $state(null)
  let traceFetch: Promise<void> | null = null
  let loading: boolean = $state(false)
  let error: string = $state('')

  onMount(async () => {
    try {
      const res = await fetch('/api/runs')
      if (!res.ok) throw new Error('Failed to fetch runs')
      const data = await res.json()
      runs = Array.isArray(data) ? data : (data.runs || [])
      if (runs.length > 0) {
        selectedRunId = runs[0].id
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : 'Failed to load runs'
    }
  })

  // Get-by-id once on select; cache trace_data for execute→originalData (Item 5B)
  $effect(() => {
    const id = selectedRunId
    if (!id) {
      selectedRunTraceData = null
      traceFetch = null
      onrunselect?.('', null)
      return
    }
    let cancelled = false
    const p = (async () => {
      try {
        const res = await fetch(`/api/runs/${encodeURIComponent(id)}`)
        if (!res.ok || cancelled) return
        const data = await res.json()
        if (cancelled) return
        selectedRunTraceData = data.trace_data ?? null
        onrunselect?.(id, selectedRunTraceData)
      } catch {
        if (!cancelled) {
          selectedRunTraceData = null
          onrunselect?.(id, null)
        }
      }
    })()
    traceFetch = p
    return () => { cancelled = true }
  })

  async function refreshRuns(): Promise<void> {
    try {
      const res = await fetch('/api/runs')
      if (!res.ok) throw new Error('Failed to fetch runs')
      const data = await res.json()
      const newRuns: Run[] = Array.isArray(data) ? data : (data.runs || [])
      const prevId = selectedRunId
      runs = newRuns
      // Preserve selected run if still in list
      if (prevId && newRuns.some(r => r.id === prevId)) {
        selectedRunId = prevId
      } else if (newRuns.length > 0) {
        selectedRunId = newRuns[0].id
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : 'Failed to refresh runs'
    }
  }

  async function execute(): Promise<void> {
    loading = true
    error = ''
    try {
      // Ensure select-time Get finished so stage-0 baseline is available
      if (traceFetch) await traceFetch
      const res = await fetch(`/api/ottl/pipeline/execute?run_id=${encodeURIComponent(selectedRunId)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: pipelineYaml
      })
      const data = await res.json()
      if (!res.ok) {
        onresult?.(null, data.error || 'Execution failed')
      } else {
        onresult?.(data as PipelineResult, undefined, selectedRunId, selectedRunTraceData)
      }
    } catch (e: unknown) {
      onresult?.(null, e instanceof Error ? e.message : 'Network error')
    } finally {
      loading = false
    }
  }
</script>

<div class="run-selector">
  <h3>Select Run</h3>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  <div class="controls">
    <select bind:value={selectedRunId} disabled={loading}>
      {#each runs as run (run.id)}
        <option value={run.id}>
          {run.id} — {run.status} ({run.span_count} spans)
        </option>
      {/each}
    </select>

    <button onclick={execute} disabled={loading || !selectedRunId}>
      {#if loading}
        <span class="spinner"></span>
        Executing...
      {:else}
        Execute Pipeline
      {/if}
    </button>

    <button onclick={refreshRuns} disabled={loading} title="Refresh runs">
      ↻
    </button>
  </div>
</div>

<style>
  .run-selector {
    margin-bottom: var(--space-md, 1rem);
  }
  h3 {
    font-size: var(--font-size-md, 0.9rem);
    font-weight: 600;
    margin-bottom: var(--space-sm, 0.5rem);
  }
  .error {
    color: var(--color-error, #ef4444);
    font-size: 0.875rem;
    margin-bottom: var(--space-sm, 0.5rem);
  }
  .controls {
    display: flex;
    gap: var(--space-sm, 0.5rem);
    align-items: center;
  }
  select {
    flex: 1;
    padding: var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    font-size: 0.875rem;
  }
  button {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    background: var(--color-primary, #3b82f6);
    color: var(--color-text, #f1f5f9);
    border: none;
    border-radius: var(--radius-sm, 4px);
    font-size: 0.875rem;
    cursor: pointer;
    white-space: nowrap;
  }
  button:disabled {
    background: var(--color-text-secondary, #94a3b8);
    cursor: not-allowed;
  }
  .spinner {
    display: inline-block;
    width: 0.875rem;
    height: 0.875rem;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: var(--color-text, #f1f5f9);
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
