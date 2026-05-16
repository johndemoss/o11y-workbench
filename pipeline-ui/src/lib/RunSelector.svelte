<script lang="ts">
  import { onMount } from 'svelte'
  import type { PipelineResult } from './types'

  interface Run {
    id: string
    status: string
    span_count: number
  }

  let { pipelineYaml = '', onresult }: {
    pipelineYaml: string
    onresult?: (result: PipelineResult | null, error?: string) => void
  } = $props()

  let runs: Run[] = $state([])
  let selectedRunId: string = $state('')
  let loading: boolean = $state(false)
  let error: string = $state('')

  onMount(async () => {
    try {
      const res = await fetch('/api/runs')
      if (!res.ok) throw new Error('Failed to fetch runs')
      const data = await res.json()
      runs = Array.isArray(data) ? data : (data.runs || [])
      if (data.length > 0) {
        selectedRunId = data[0].id
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : 'Failed to load runs'
    }
  })

  async function execute(): Promise<void> {
    loading = true
    error = ''
    try {
      const res = await fetch(`/api/ottl/pipeline/execute?run_id=${encodeURIComponent(selectedRunId)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: pipelineYaml
      })
      const data = await res.json()
      if (!res.ok) {
        onresult?.(null, data.error || 'Execution failed')
      } else {
        onresult?.(data as PipelineResult)
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
  </div>
</div>

<style>
  .run-selector {
    margin-bottom: 1rem;
  }
  h3 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }
  .error {
    color: #e53e3e;
    font-size: 0.875rem;
    margin-bottom: 0.5rem;
  }
  .controls {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  select {
    flex: 1;
    padding: 0.5rem;
    border: 1px solid #ccc;
    border-radius: 4px;
    font-size: 0.875rem;
  }
  button {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.5rem 1rem;
    background: #3182ce;
    color: #fff;
    border: none;
    border-radius: 4px;
    font-size: 0.875rem;
    cursor: pointer;
    white-space: nowrap;
  }
  button:disabled {
    background: #a0aec0;
    cursor: not-allowed;
  }
  .spinner {
    display: inline-block;
    width: 0.875rem;
    height: 0.875rem;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
