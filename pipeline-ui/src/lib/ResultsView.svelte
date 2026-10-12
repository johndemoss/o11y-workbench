<script lang="ts">
  import {
    buildStageUnifiedDiffsLazy,
    computeUnifiedDiff,
    formatJSON,
    highlightJSON,
  } from './diff'
  import type { PipelineResult, StageDiff } from './types'
  import DiffPanel from './DiffPanel.svelte'

  let {
    result,
    error,
    originalData,
    stale = false,
    selectedStageId = null,
    onSelectStage,
  }: {
    result: PipelineResult | null
    error?: string
    originalData?: unknown
    stale?: boolean
    selectedStageId?: string | null
    onSelectStage?: (stageId: string | null) => void
  } = $props()

  let resultView: 'records' | 'payload' | 'final' = $state('records')
  let compareMode: 'split' | 'changes' = $state('changes')
  let payloadScope: 'stage' | 'pipeline' = $state('stage')
  let diffView: 'inline' | 'unified' = $state('inline')
  let diffExpandedStages: Set<number> = $state(new Set())
  let selectedIndex = $state(0)

  let stageDiffs: StageDiff[] = $state([])
  let loadingDiff = $state(false)
  let diffError = $state<string | null>(null)

  $effect(() => {
    if (!result?.stages?.length) {
      selectedIndex = 0
      return
    }
    if (selectedStageId) {
      const idx = result.stages.findIndex((s) => s.stage_id === selectedStageId)
      if (idx >= 0) selectedIndex = idx
    }
  })

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
            original_data: originalData ?? null,
          }),
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

  const unifiedDiffCache = buildStageUnifiedDiffsLazy()

  $effect(() => {
    void result
    void originalData
    unifiedDiffCache.clear()
    diffExpandedStages = new Set()
  })

  function onDiffExpandChange(stageIndex: number, expanded: boolean) {
    const next = new Set(diffExpandedStages)
    if (expanded) next.add(stageIndex)
    else next.delete(stageIndex)
    diffExpandedStages = next
  }

  function toggleView() {
    diffView = diffView === 'inline' ? 'unified' : 'inline'
  }

  function getUnifiedDiff(stageIndex: number) {
    if (!result) return []
    const before =
      stageIndex === 0 ? originalData ?? null : result.stages[stageIndex - 1]?.intermediate_data
    const after = result.stages[stageIndex]?.intermediate_data
    return unifiedDiffCache.get(stageIndex, before, after, {
      expanded: diffExpandedStages.has(stageIndex),
      unified: diffView === 'unified',
    })
  }

  function stageBefore(index: number): unknown | undefined {
    if (!result) return undefined
    if (index === 0) return originalData
    return result.stages[index - 1]?.intermediate_data
  }

  function stageAfter(index: number): unknown | undefined {
    return result?.stages[index]?.intermediate_data
  }

  let selectedStage = $derived(result?.stages[selectedIndex] ?? null)
  let selectedDiff = $derived(
    selectedStage ? stageDiffs[selectedStage.stage_index] : undefined,
  )

  let payloadBefore = $derived.by(() => {
    if (!result) return undefined
    if (payloadScope === 'pipeline') return originalData
    return stageBefore(selectedIndex)
  })

  let payloadAfter = $derived.by(() => {
    if (!result) return undefined
    if (payloadScope === 'pipeline') return result.final_data
    return stageAfter(selectedIndex)
  })

  let payloadLines = $derived.by(() => {
    if (payloadBefore === undefined || payloadAfter === undefined) return null
    return computeUnifiedDiff(payloadBefore, payloadAfter)
  })

  function selectStage(index: number) {
    selectedIndex = index
    const id = result?.stages[index]?.stage_id ?? null
    onSelectStage?.(id)
  }

  function setResultView(next: 'records' | 'payload' | 'final') {
    resultView = next
  }

  function onViewKey(e: KeyboardEvent, key: 'records' | 'payload' | 'final') {
    const keys: Array<'records' | 'payload' | 'final'> = ['records', 'payload', 'final']
    let idx = keys.indexOf(key)
    if (e.key === 'ArrowRight') idx = (idx + 1) % 3
    else if (e.key === 'ArrowLeft') idx = (idx + 2) % 3
    else if (e.key === 'Home') idx = 0
    else if (e.key === 'End') idx = 2
    else return
    e.preventDefault()
    setResultView(keys[idx])
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

  function formatDuration(ns: number): string {
    if (ns < 1000) return `${ns}ns`
    if (ns < 1_000_000) return `${(ns / 1000).toFixed(2)}µs`
    if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)}ms`
    return `${(ns / 1_000_000_000).toFixed(3)}s`
  }
</script>

<div class="results-view" data-od-id="stage-results">
  <div class="result-top">
    <h3>Results explorer</h3>
    <span
      class="badge"
      class:warn={stale || !!error}
      class:ok={!!result && !stale && !error}
      id="freshness"
      role="status"
      aria-live="polite"
    >
      {#if error}
        Execution error
      {:else if !result}
        No execution evidence
      {:else if stale}
        Draft changed · results out of date
      {:else}
        Executed · evidence current
      {/if}
    </span>
  </div>

  {#if error}
    <div class="error-box">{error}</div>
  {:else if !result}
    <div class="empty-state">Execute a pipeline to see Collector results. No mock evidence is shown.</div>
  {:else}
    {#if stale}
      <div class="status warn" role="status">
        Configuration edits do not produce new runtime results. Re-execute to refresh evidence.
      </div>
    {/if}

    <div class="stage-tabs" aria-label="Select stage evidence">
      {#each result.stages as stage, i}
        <button
          type="button"
          aria-pressed={selectedIndex === i}
          onclick={() => selectStage(i)}
        >
          {String(i + 1).padStart(2, '0')} · {stage.stage_id}
        </button>
      {/each}
    </div>

    <div class="result-views" role="tablist" aria-label="Result views" data-od-id="result-view-tabs">
      <button
        type="button"
        id="view-records"
        role="tab"
        aria-selected={resultView === 'records'}
        tabindex={resultView === 'records' ? 0 : -1}
        aria-controls="records-panel"
        data-od-id="records-tab"
        onclick={() => setResultView('records')}
        onkeydown={(e) => onViewKey(e, 'records')}
      >Per-record changes</button>
      <button
        type="button"
        id="view-payload"
        role="tab"
        aria-selected={resultView === 'payload'}
        tabindex={resultView === 'payload' ? 0 : -1}
        aria-controls="payload-panel"
        data-od-id="payload-diff-tab"
        onclick={() => setResultView('payload')}
        onkeydown={(e) => onViewKey(e, 'payload')}
      >Full-payload diff</button>
      <button
        type="button"
        id="view-final"
        role="tab"
        aria-selected={resultView === 'final'}
        tabindex={resultView === 'final' ? 0 : -1}
        aria-controls="final-panel"
        data-od-id="final-result-tab"
        onclick={() => setResultView('final')}
        onkeydown={(e) => onViewKey(e, 'final')}
      >Final result</button>
    </div>

    {#if loadingDiff}
      <div class="diff-loading">Computing stage diffs…</div>
    {/if}
    {#if diffError}
      <div class="diff-error-box"><strong>Diff computation failed:</strong> {diffError}</div>
    {/if}

    {#if resultView === 'records'}
      <div id="records-panel" role="tabpanel" aria-labelledby="view-records" data-od-id="record-comparison">
        {#if !selectedStage}
          <p class="unavailable">No stage selected.</p>
        {:else}
          <p class="record-label">
            Stage {selectedStage.stage_index} · {selectedStage.stage_id} ·
            {formatDuration(selectedStage.duration_ns)} ·
            {selectedStage.input_span_count} → {selectedStage.output_span_count} spans
          </p>
          <div class="comparison-bar">
            <h3>{selectedStage.stage_id}</h3>
            <div class="mode" aria-label="Comparison view">
              <button
                type="button"
                aria-pressed={compareMode === 'split'}
                data-od-id="before-after-mode"
                onclick={() => compareMode = 'split'}
              >Before / after</button>
              <button
                type="button"
                aria-pressed={compareMode === 'changes'}
                data-od-id="changes-only-mode"
                onclick={() => compareMode = 'changes'}
              >Inline changes</button>
            </div>
          </div>

          {#if compareMode === 'split'}
            {#if stageBefore(selectedIndex) === undefined || stageAfter(selectedIndex) === undefined}
              <div class="unavailable" role="status">
                Stage before/after payloads are unavailable. The backend did not supply
                intermediate_data{selectedIndex === 0 ? ' and/or original input' : ''} for this stage.
              </div>
            {:else}
              <div class="compare">
                <div>
                  <h4>Before · stage input</h4>
                  <pre class="code">{@html highlightJSON(formatJSON(stageBefore(selectedIndex)))}</pre>
                </div>
                <div>
                  <h4>After · stage output</h4>
                  <pre class="code">{@html highlightJSON(formatJSON(stageAfter(selectedIndex)))}</pre>
                </div>
              </div>
            {/if}
          {:else if selectedDiff}
            <DiffPanel
              stageDiff={selectedDiff}
              unifiedDiff={getUnifiedDiff(selectedStage.stage_index)}
              stageIndex={selectedStage.stage_index}
              {diffView}
              onToggleView={toggleView}
              onExpandChange={(expanded) => onDiffExpandChange(selectedStage.stage_index, expanded)}
            />
          {:else}
            <div class="unavailable" role="status">
              Per-record change evidence is unavailable for this stage.
            </div>
          {/if}

          {#if selectedStage.errors?.length}
            <div class="stage-errors">
              {#each selectedStage.errors as err}
                <div class="stage-error">{err}</div>
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    {:else if resultView === 'payload'}
      <div id="payload-panel" role="tabpanel" aria-labelledby="view-payload" data-od-id="full-payload-diff">
        <div class="scope-control">
          <label for="diff-scope">Compare</label>
          <select id="diff-scope" bind:value={payloadScope} data-od-id="payload-diff-scope">
            <option value="stage">Selected stage input → output</option>
            <option value="pipeline">Pipeline input → final result</option>
          </select>
        </div>
        {#if payloadLines === null}
          <div class="unavailable" role="status">
            Full-payload unified diff is unavailable for this scope. Missing
            {payloadScope === 'pipeline' ? 'original input and/or final_data' : 'stage intermediate_data / original input'}.
          </div>
        {:else}
          <p class="record-label" id="payload-caption">
            {payloadScope === 'pipeline'
              ? 'Pipeline input → final output · complete payload'
              : `${selectedStage?.stage_id ?? 'stage'} · complete stage input → output`}
          </p>
          <pre class="code payload-code" aria-label="Unified full payload diff">
{#each payloadLines as line}<span class="diff-line" class:diff-added={line.type === 'add'} class:diff-removed={line.type === 'remove'}><span class="diff-prefix">{line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}</span><span class="diff-text">{line.text}</span>
</span>{/each}</pre>
          <p class="provenance">Full OTLP JSON envelope · + added / − removed · unchanged context retained.</p>
        {/if}
      </div>
    {:else}
      <div id="final-panel" role="tabpanel" aria-labelledby="view-final" data-od-id="final-result">
        <div class="comparison-bar">
          <h3>Final Result Data</h3>
          {#if result.final_data !== undefined}
            <button
              type="button"
              id="export-result"
              data-od-id="export-final-result"
              onclick={() => downloadJSON(result.final_data, 'pipeline-result.json')}
            >Export JSON</button>
          {/if}
        </div>
        {#if result.final_data === undefined}
          <div class="unavailable" role="status">
            Final output is unavailable — execute did not return final_data.
          </div>
        {:else}
          <p class="record-label">
            Pipeline output · {result.final_span_count} spans · {result.stages.length} stages
          </p>
          <pre class="code payload-code">{@html highlightJSON(formatJSON(result.final_data))}</pre>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .results-view {
    font-family: var(--font-family);
    font-size: 13px;
    min-width: 0;
  }
  .result-top {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    flex-wrap: wrap;
  }
  .result-top h3 {
    margin: 0;
    font-size: 14px;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    padding: 4px 8px;
    border-radius: 999px;
    font: 12px var(--font-mono);
    background: var(--color-info-bg);
    color: var(--color-info-text);
  }
  .badge.warn {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }
  .badge.ok {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }
  .status.warn {
    margin: 12px 0;
    padding: 12px;
    border-radius: var(--radius-md);
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
    font-size: 12px;
  }
  .error-box {
    background: var(--color-error-bg);
    color: var(--color-error-text);
    border: 1px solid var(--color-error);
    padding: 12px;
    border-radius: var(--radius-md);
    margin-top: 12px;
  }
  .empty-state,
  .unavailable,
  .diff-loading {
    color: var(--color-text-secondary);
    padding: 16px 0;
    font-size: 12px;
  }
  .unavailable {
    border: 1px dashed var(--color-border);
    border-radius: var(--radius-md);
    padding: 12px;
    margin-top: 12px;
  }
  .diff-error-box {
    background: var(--color-error-bg);
    color: var(--color-error-text);
    border: 1px solid var(--color-error);
    padding: 12px;
    border-radius: var(--radius-md);
    margin: 12px 0;
  }
  .stage-tabs,
  .result-views,
  .mode {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin: 16px 0;
  }
  .result-views {
    border-bottom: 1px solid var(--color-border);
    gap: 4px;
  }
  .stage-tabs button,
  .mode button {
    font-size: 12px;
    min-height: 44px;
    background: var(--color-bg);
  }
  .stage-tabs button[aria-pressed='true'],
  .mode button[aria-pressed='true'] {
    border-color: var(--focus);
    background: var(--color-surface-hover);
  }
  .mode button[aria-pressed='true'] {
    background: var(--action-bg);
    border-color: var(--action-bg);
    color: var(--action-fg);
  }
  .result-views button {
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    font-size: 12px;
    min-height: 44px;
  }
  .result-views button[aria-selected='true'] {
    border-bottom-color: var(--focus);
    color: var(--color-info-text);
  }
  .comparison-bar,
  .scope-control {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .comparison-bar h3 {
    font: 13px var(--font-mono);
    margin: 0;
  }
  .scope-control label { margin: 0; }
  .scope-control select { width: auto; max-width: 100%; min-height: 44px; }
  .record-label {
    margin: 0 0 12px;
    font-size: 12px;
    color: var(--color-text-secondary);
  }
  .compare {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .compare h4 {
    font-size: 12px;
    font-weight: 500;
    color: var(--color-text-secondary);
    margin: 0 0 8px;
  }
  .code {
    padding: 12px;
    font: 12px/1.7 var(--font-mono);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    min-height: 160px;
    overflow: auto;
    margin: 0;
    white-space: pre-wrap;
  }
  .payload-code {
    white-space: pre;
    max-height: 540px;
  }
  .diff-line { display: flex; min-height: 1.5em; }
  .diff-prefix {
    width: 24px;
    flex-shrink: 0;
    color: var(--color-text-secondary);
    user-select: none;
  }
  .diff-added { background: color-mix(in oklab, var(--color-success) 12%, transparent); }
  .diff-removed { background: color-mix(in oklab, var(--color-error) 12%, transparent); }
  .diff-added .diff-prefix { color: var(--color-success-text); }
  .diff-removed .diff-prefix { color: var(--color-error-text); }
  .diff-text { white-space: pre; }
  .provenance {
    margin-top: 16px;
    font-size: 12px;
    color: var(--color-text-secondary);
  }
  .stage-errors { margin-top: 12px; }
  .stage-error {
    color: var(--color-error-text);
    background: var(--color-error-bg);
    padding: 8px;
    border-radius: 4px;
    margin-bottom: 4px;
    font-size: 12px;
  }
  @media (max-width: 960px) {
    .compare { grid-template-columns: 1fr; }
  }
</style>
