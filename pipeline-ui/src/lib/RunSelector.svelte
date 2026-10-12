<script lang="ts">
  import { onMount } from 'svelte'
  import type { PipelineResult, SignalType } from './types'
  import { withCollectorVersion } from './collector-version'

  interface Run {
    id: string
    status: string
    span_count: number
    has_trace_data?: boolean
    has_log_data?: boolean
    has_metric_data?: boolean
  }

  let {
    pipelineYaml = '',
    signal = $bindable('traces' as SignalType),
    onresult,
    onrunselect,
  }: {
    pipelineYaml: string
    signal?: SignalType
    onresult?: (result: PipelineResult | null, error?: string, runId?: string, originalData?: unknown) => void
    onrunselect?: (runId: string, data: unknown, signal: SignalType) => void
  } = $props()

  let runs: Run[] = $state([])
  let selectedRunId: string = $state('')
  let selectedRunPayload: unknown = $state(null)
  let payloadFetch: Promise<void> | null = null
  let loading: boolean = $state(false)
  let error: string = $state('')

  const signals: SignalType[] = ['traces', 'logs', 'metrics']

  function payloadKey(sig: SignalType): string {
    switch (sig) {
      case 'logs':
        return 'log_data'
      case 'metrics':
        return 'metric_data'
      default:
        return 'trace_data'
    }
  }

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

  // Get-by-id once on select/signal change; cache payload for execute→originalData
  $effect(() => {
    const id = selectedRunId
    const sig = signal
    if (!id) {
      selectedRunPayload = null
      payloadFetch = null
      onrunselect?.('', null, sig)
      return
    }
    let cancelled = false
    const p = (async () => {
      try {
        const res = await fetch(`/api/runs/${encodeURIComponent(id)}`)
        if (!res.ok || cancelled) return
        const data = await res.json()
        if (cancelled) return
        selectedRunPayload = data[payloadKey(sig)] ?? null
        onrunselect?.(id, selectedRunPayload, sig)
      } catch {
        if (!cancelled) {
          selectedRunPayload = null
          onrunselect?.(id, null, sig)
        }
      }
    })()
    payloadFetch = p
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
      if (payloadFetch) await payloadFetch
      const res = await fetch(
        withCollectorVersion(
          `/api/ottl/pipeline/execute?run_id=${encodeURIComponent(selectedRunId)}&signal=${encodeURIComponent(signal)}`,
        ),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-yaml' },
          body: pipelineYaml,
        },
      )
      const data = await res.json()
      if (!res.ok) {
        onresult?.(null, data.error || 'Execution failed')
      } else {
        onresult?.(data as PipelineResult, undefined, selectedRunId, selectedRunPayload)
      }
    } catch (e: unknown) {
      onresult?.(null, e instanceof Error ? e.message : 'Network error')
    } finally {
      loading = false
    }
  }

  let selectedRun = $derived(runs.find((r) => r.id === selectedRunId) ?? null)
</script>

<div class="run-selector" data-od-id="execution-controls">
  {#if error}
    <div class="error" role="alert">{error}</div>
  {/if}

  <div
    class="pill-toggle"
    role="radiogroup"
    aria-label="Signal"
    title={selectedRun ? `${selectedRun.status} · ${selectedRun.span_count} spans` : 'Signal'}
  >
    {#each signals as sig}
      <button
        type="button"
        role="radio"
        aria-checked={signal === sig}
        class:active={signal === sig}
        disabled={loading}
        onclick={() => signal = sig}
      >{sig}</button>
    {/each}
  </div>

  <div class="run-controls">
    <select id="execute-run" aria-label="Run" bind:value={selectedRunId} disabled={loading || runs.length === 0}>
      {#if runs.length === 0}
        <option value="">No runs available</option>
      {/if}
      {#each runs as run (run.id)}
        <option value={run.id}>
          {run.id} — {run.status} ({run.span_count})
        </option>
      {/each}
    </select>
    <button type="button" class="ghost-icon" onclick={refreshRuns} disabled={loading} title="Refresh runs" aria-label="Refresh runs">↻</button>
  </div>

  <button type="button" class="execute-btn" onclick={execute} disabled={loading || !selectedRunId}>
    {#if loading}
      <span class="spinner" aria-hidden="true"></span>
      Executing…
    {:else}
      Execute
    {/if}
  </button>
</div>

<style>
  .run-selector {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
  }

  .error {
    color: var(--color-error-text, #ef4444);
    background: var(--color-error-bg, transparent);
    border: 1px solid var(--color-error, #ef4444);
    border-radius: var(--radius-sm, 4px);
    font-size: 12px;
    padding: 8px;
  }

  .pill-toggle {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 2px;
    padding: 3px;
    border-radius: 999px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
  }

  .pill-toggle button {
    min-height: 28px;
    padding: 2px 4px;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--color-text-secondary);
    font-size: 11px;
    font-weight: 500;
    text-transform: capitalize;
  }

  .pill-toggle button.active,
  .pill-toggle button[aria-checked='true'] {
    background: var(--action-bg);
    color: var(--action-fg);
  }

  .pill-toggle button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .run-controls {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 36px;
    gap: 6px;
    align-items: center;
  }

  select {
    width: 100%;
    min-height: 36px;
    padding: 4px 8px;
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-bg, transparent);
    color: var(--color-text, inherit);
    font-size: 12px;
  }

  .ghost-icon {
    width: 36px;
    height: 36px;
    min-height: 36px;
    padding: 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-bg);
    color: var(--color-text-secondary);
    cursor: pointer;
  }

  .ghost-icon:hover:not(:disabled) {
    color: var(--color-text);
  }

  .execute-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    width: 100%;
    min-height: 36px;
    padding: 6px 12px;
    background: var(--action-bg, var(--color-primary, #3b82f6));
    color: var(--action-fg, var(--color-text, #f1f5f9));
    border: none;
    border-radius: var(--radius-sm, 4px);
    font-size: 13px;
    cursor: pointer;
    white-space: nowrap;
  }

  .execute-btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .spinner {
    display: inline-block;
    width: 0.875rem;
    height: 0.875rem;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
