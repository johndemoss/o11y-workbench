<script lang="ts">
  import type { Pipeline, PipelineResult, Stage, ProcessorConfig, PipelinesBySignal, SignalType } from './lib/types'
  import { SIGNALS } from './lib/types'
  import Layout from './lib/Layout.svelte'
  import PipelineBuilder from './lib/PipelineBuilder.svelte'
  import SwimlaneFlowBuilder from './lib/SwimlaneFlowBuilder.svelte'
  import EditorPanel from './lib/EditorPanel.svelte'
  import StageEditor from './lib/StageEditor.svelte'
  import StageInspector from './lib/StageInspector.svelte'
  import RunSelector from './lib/RunSelector.svelte'
  import ResultsView from './lib/ResultsView.svelte'
  import ConfigImport from './lib/ConfigImport.svelte'
  import QuickTab from './lib/QuickTab.svelte'
  import { exportPipelinesToYaml, exportPipelinesToYamlLocal, downloadYaml } from './lib/export-yaml'
  import { debounce } from './lib/debounce'
  import { withCollectorVersion } from './lib/collector-version'
  import { dump } from 'js-yaml'
  import { onDestroy } from 'svelte'

  let pipelinesBySignal: PipelinesBySignal = $state({
    traces: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    logs: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    metrics: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }
  })
  let selectedSignal: SignalType = $state('traces')
  let pipeline = $derived(pipelinesBySignal[selectedSignal] || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] })
  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let originalData: unknown = $state(null)
  let selectedRunPayload: unknown = $state(null)
  let showImport: boolean = $state(false)
  let pipelineYaml: string = $state('')
  let activeTab: 'quick' | 'pipeline' | 'graph' = $state('graph')
  /** Modal editor for Quick/Pipeline tabs only — Graph uses persistent StageInspector. */
  let editingStage: { stage: Stage; processor: ProcessorConfig; signal: SignalType } | null = $state(null)
  let selectedNodeId: string | null = $state(null)
  let yamlOpen = $state(true)
  let panelHeight = $state(320)
  let isCollapsed = $state(false)
  let savedHeight = $state(300)
  let resultsDialog: HTMLDialogElement | undefined = $state()
  let viewportOpen = $state(false)
  /** YAML fingerprint captured at last successful execute — config edits mark results stale. */
  let executedYaml = $state<string | null>(null)

  type InspectorSelection =
    | { kind: 'stage'; stage: Stage; processor: ProcessorConfig; signal: SignalType }
    | { kind: 'receiver'; name: string }
    | null

  let inspectorSelection = $state<InspectorSelection>(null)
  let highlightedReceiver = $derived.by(() => {
    const sel = inspectorSelection
    return sel?.kind === 'receiver' ? sel.name : null
  })
  let resultsStale = $derived(!!result && executedYaml !== null && pipelineYaml !== executedYaml)
  let selectedResultStageId = $derived.by(() => {
    const sel = inspectorSelection
    return sel?.kind === 'stage' ? sel.stage.id : null
  })

  function toggleCollapse() {
    if (!isCollapsed) { savedHeight = panelHeight; isCollapsed = true }
    else { isCollapsed = false; panelHeight = savedHeight }
  }

  function openResultsViewport() {
    isCollapsed = false
    viewportOpen = true
  }

  function closeResultsViewport() {
    if (resultsDialog?.open) resultsDialog.close()
    else viewportOpen = false
  }

  function onResultsDialogClose() {
    viewportOpen = false
  }

  $effect(() => {
    if (!viewportOpen) return
    const dialog = resultsDialog
    if (!dialog) return
    if (!dialog.open) dialog.showModal()
  })

  function handleResizeStart(e: MouseEvent) {
    e.preventDefault()
    savedHeight = panelHeight
    if (isCollapsed) isCollapsed = false
    const startY = e.clientY
    const startHeight = panelHeight
    const graphShell = (e.target as HTMLElement).closest('.graph-shell') as HTMLElement | null
    function onMouseMove(e: MouseEvent) {
      if (!graphShell) return
      const delta = startY - e.clientY
      const newHeight = Math.max(120, Math.min(graphShell.clientHeight - 120, startHeight + delta))
      if (newHeight <= 120) { if (!isCollapsed) { savedHeight = panelHeight; isCollapsed = true } }
      else { if (isCollapsed) isCollapsed = false; panelHeight = newHeight }
    }
    function onMouseUp() { window.removeEventListener('mousemove', onMouseMove); window.removeEventListener('mouseup', onMouseUp) }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }

  function clearResults() {
    result = null
    resultError = undefined
    executedYaml = null
  }

  function handleResultStageSelect(stageId: string | null) {
    if (!stageId || !result) return
    for (const sig of SIGNALS) {
      const stage = pipelinesBySignal[sig].stages.find((s) => s.id === stageId)
      if (stage) {
        inspectorSelection = {
          kind: 'stage',
          stage,
          processor: pipelinesBySignal[sig].processors[stage.processor_ref] ?? {},
          signal: sig,
        }
        selectedNodeId = stage.id
        return
      }
    }
  }

  function handlePipelinesChange(p: PipelinesBySignal) {
    pipelinesBySignal = p
    // Keep inspector bound to current stage identity after graph sync
    if (inspectorSelection?.kind === 'stage') {
      const selected = inspectorSelection
      const sig = selected.signal
      const stage = pipelinesBySignal[sig].stages.find((s) => s.id === selected.stage.id)
      if (!stage) {
        inspectorSelection = null
        return
      }
      inspectorSelection = {
        kind: 'stage',
        stage,
        processor: pipelinesBySignal[sig].processors[stage.processor_ref] ?? {},
        signal: sig,
      }
    }
  }

  function handleNodeSelected(nodeId: string | null) {
    selectedNodeId = nodeId
    if (!nodeId) {
      inspectorSelection = null
      return
    }

    // Infra receiver ids: infra-{signal}-receiver-{name}
    const recvMatch = /^infra-[^-]+-receiver-(.+)$/.exec(nodeId)
    if (recvMatch || nodeId.includes('-receiver-')) {
      const name = recvMatch?.[1] ?? resolveReceiverName(nodeId)
      inspectorSelection = { kind: 'receiver', name }
      return
    }

    for (const sig of SIGNALS) {
      const p = pipelinesBySignal[sig]
      const stage = p.stages.find((s) => s.id === nodeId || s.processor_ref === nodeId)
      if (stage) {
        inspectorSelection = {
          kind: 'stage',
          stage,
          processor: p.processors[stage.processor_ref] ?? {},
          signal: sig,
        }
        return
      }
    }
  }

  function resolveReceiverName(nodeId: string): string {
    // Prefer an actual shared receiver name present across pipelines
    const names = new Set<string>()
    for (const sig of SIGNALS) {
      for (const r of pipelinesBySignal[sig]?.receivers ?? []) names.add(r)
    }
    for (const n of names) {
      if (nodeId.includes(n)) return n
    }
    return [...names][0] ?? 'otlp'
  }

  async function handleExport() { downloadYaml('pipeline-config.yaml', await exportPipelinesToYaml(pipelinesBySignal)) }
  function handleValidate(p: Pipeline) {
    const body = activeTab === 'graph' ? pipelineYaml : yamlStringify(p)
    const url = activeTab === 'graph'
      ? withCollectorVersion('/api/ottl/pipeline/validate')
      : withCollectorVersion(`/api/ottl/pipeline/validate?signal=${encodeURIComponent(selectedSignal)}`)
    fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-yaml' }, body })
      .then(r => r.json()).then(d => alert(d.valid ? 'Pipeline is valid!' : 'Validation errors:\n' + (d.errors || []).join('\n')))
      .catch(e => alert('Validation failed: ' + e.message))
  }
  function handleRunSelect(_runId: string, payload: unknown, sig?: SignalType) {
    selectedRunPayload = payload
    if (sig) selectedSignal = sig
  }
  function handleExecuteResult(r: PipelineResult | null, err?: string, _runId?: string, originalPayload?: unknown) {
    result = r
    resultError = err
    if (!r) {
      executedYaml = null
      return
    }
    originalData = originalPayload !== undefined ? originalPayload : selectedRunPayload
    executedYaml = pipelineYaml
  }
  function handleImport(p: PipelinesBySignal) {
    pipelinesBySignal = {
      traces: p.traces || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
      logs: p.logs || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
      metrics: p.metrics || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    }
    const withStages = (['traces', 'logs', 'metrics'] as SignalType[]).find(s => (pipelinesBySignal[s]?.stages?.length ?? 0) > 0)
    if (withStages) selectedSignal = withStages
    showImport = false
    if (activeTab !== 'graph') activeTab = 'graph'
  }
  function handleNodeEdit(data: { stage: Stage; processor: ProcessorConfig; signal: SignalType }) {
    // Graph: persistent inspector. Other tabs: modal.
    if (activeTab === 'graph') {
      const stage = pipelinesBySignal[data.signal].stages.find((item) => item.id === data.stage.id) ?? data.stage
      const processor = pipelinesBySignal[data.signal].processors[stage.processor_ref] ?? data.processor
      inspectorSelection = { kind: 'stage', stage, processor, signal: data.signal }
      selectedNodeId = data.stage.id
      return
    }
    editingStage = data
  }
  function handleInspectorStageUpdate(s: Stage) {
    if (inspectorSelection?.kind !== 'stage') return
    const sig = inspectorSelection.signal
    const prev = inspectorSelection.stage
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: {
        ...pipelinesBySignal[sig],
        stages: pipelinesBySignal[sig].stages.map(st => st.id === prev.id ? s : st),
      },
    }
    inspectorSelection = {
      kind: 'stage',
      stage: s,
      processor: pipelinesBySignal[sig].processors[s.processor_ref] ?? inspectorSelection.processor,
      signal: sig,
    }
  }
  function handleInspectorProcessorUpdate(c: ProcessorConfig) {
    if (inspectorSelection?.kind !== 'stage') return
    const sig = inspectorSelection.signal
    const ref = inspectorSelection.stage.processor_ref
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: {
        ...pipelinesBySignal[sig],
        processors: { ...pipelinesBySignal[sig].processors, [ref]: c },
      },
    }
    inspectorSelection = { ...inspectorSelection, processor: c }
  }

  function handleInspectorMove(direction: 'left' | 'right') {
    const sel = inspectorSelection
    if (sel?.kind !== 'stage') return
    const sig = sel.signal
    const stages = [...pipelinesBySignal[sig].stages]
    const idx = stages.findIndex((s) => s.id === sel.stage.id)
    if (idx < 0) return
    const swapWith = direction === 'left' ? idx - 1 : idx + 1
    if (swapWith < 0 || swapWith >= stages.length) return
    ;[stages[idx], stages[swapWith]] = [stages[swapWith], stages[idx]]
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: { ...pipelinesBySignal[sig], stages },
    }
  }

  function handleInspectorRemove() {
    const sel = inspectorSelection
    if (sel?.kind !== 'stage') return
    const sig = sel.signal
    const id = sel.stage.id
    const ref = sel.stage.processor_ref
    const stages = pipelinesBySignal[sig].stages.filter((s) => s.id !== id)
    const processors = { ...pipelinesBySignal[sig].processors }
    const stillUsed = stages.some((s) => s.processor_ref === ref)
    if (!stillUsed) delete processors[ref]
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: { ...pipelinesBySignal[sig], stages, processors },
    }
    inspectorSelection = null
    selectedNodeId = null
    clearResults()
  }
  function handleStageUpdate(s: Stage) {
    const sig = editingStage!.signal
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: { ...pipelinesBySignal[sig], stages: pipelinesBySignal[sig].stages.map(st => st.id === editingStage!.stage.id ? s : st) }
    }
    editingStage = { ...editingStage!, stage: s }
  }
  function handleStageProcessorUpdate(c: ProcessorConfig) {
    const sig = editingStage!.signal
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: { ...pipelinesBySignal[sig], processors: { ...pipelinesBySignal[sig].processors, [editingStage!.stage.processor_ref]: c } }
    }
    editingStage = { ...editingStage!, processor: c }
  }
  function yamlStringify(p: Pipeline): string {
    let out = 'stages:\n'
    for (const s of p.stages) out += `  - id: ${s.id}\n    type: ${s.type}\n    processor_ref: ${s.processor_ref}\n${s.error_mode ? `    error_mode: ${s.error_mode}\n` : ''}`
    out += 'processors:\n'
    for (const [k, v] of Object.entries(p.processors)) {
      if (v && typeof v === 'object' && Object.keys(v).length > 0) {
        const lines = dump(v, { indent: 2, noRefs: true, sortKeys: false }).trimEnd().split('\n')
        out += `  ${k}:\n`
        for (const l of lines) out += `    ${l}\n`
      } else out += `  ${k}: {}\n`
    }
    return out
  }

  let yamlVersion = 0

  const debouncedFetchExportYaml = debounce(
    (ver: number, tab: string, pipelines: PipelinesBySignal, sig: SignalType, singlePipeline: Pipeline) => {
      void fetchExportYaml(ver, tab, pipelines, sig, singlePipeline)
    },
    400,
  )
  onDestroy(() => debouncedFetchExportYaml.cancel())

  $effect(() => {
    const tab = activeTab
    const pipelines = pipelinesBySignal
    const sig = selectedSignal
    const singlePipeline = pipeline

    const ver = ++yamlVersion

    if (tab === 'graph') {
      pipelineYaml = exportPipelinesToYamlLocal(pipelines)
    } else {
      pipelineYaml = yamlStringify(singlePipeline)
    }

    debouncedFetchExportYaml(ver, tab, pipelines, sig, singlePipeline)
  })

  async function fetchExportYaml(
    ver: number,
    tab: string,
    pipelines: PipelinesBySignal,
    sig: SignalType,
    singlePipeline: Pipeline
  ) {
    const empty = { stages: [], processors: {} as Record<string, ProcessorConfig>, receivers: ['otlp'], exporters: ['debug'] }
    const payload = tab === 'graph'
      ? pipelines
      : {
          traces: sig === 'traces' ? singlePipeline : empty,
          logs: sig === 'logs' ? singlePipeline : empty,
          metrics: sig === 'metrics' ? singlePipeline : empty,
        }

    try {
      const res = await fetch('/api/ottl/pipeline/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })
      if (res.ok) {
        const text = await res.text()
        if (ver === yamlVersion) {
          pipelineYaml = text
        }
      }
    } catch {
      // Keep the client-side fallback value already set by the effect
    }
  }
</script>

<Layout {activeTab} onTabChange={(t) => activeTab = t}>
  {#if activeTab === 'quick'}
    <QuickTab onresult={handleExecuteResult} />
  {:else if activeTab === 'graph'}
    <div class="graph-shell">
      <div class="layout" class:yaml-closed={!yamlOpen} id="layout">
        <EditorPanel
          pipelines={pipelinesBySignal}
          yaml={pipelineYaml}
          {selectedNodeId}
          bind:open={yamlOpen}
          onPipelinesChange={handlePipelinesChange}
        />
        <div class="workspace">
          <div class="canvas-wrapper">
            <SwimlaneFlowBuilder
              pipelines={pipelinesBySignal}
              {result}
              {highlightedReceiver}
              {yamlOpen}
              onShowYaml={() => yamlOpen = true}
              onImport={() => showImport = true}
              onExport={handleExport}
              onNodeEdit={handleNodeEdit}
              onPipelinesChange={handlePipelinesChange}
              onNodeSelected={handleNodeSelected}
              onResultClear={clearResults}
            />
          </div>
        </div>
        <StageInspector
          selection={inspectorSelection}
          pipelines={pipelinesBySignal}
          pipelineYaml={pipelineYaml}
          onupdate={handleInspectorStageUpdate}
          onupdateProcessor={handleInspectorProcessorUpdate}
          onMove={handleInspectorMove}
          onRemove={handleInspectorRemove}
        />
      </div>

      {#snippet executeResultsBody()}
        <div class="bottom-content" id="bottom-content">
          <section class="execute" data-od-id="execution-panel" aria-label="Execute">
            <h3 title="Captured run → current YAML">Execute</h3>
            <RunSelector {pipelineYaml} bind:signal={selectedSignal} onresult={handleExecuteResult} onrunselect={handleRunSelect} />
          </section>
          <section class="results" data-od-id="stage-results">
            <ResultsView
              {result}
              error={resultError}
              {originalData}
              stale={resultsStale}
              selectedStageId={selectedResultStageId}
              onSelectStage={handleResultStageSelect}
            />
          </section>
        </div>
      {/snippet}

      {#if viewportOpen}
        <dialog
          class="results-dialog"
          id="results-dialog"
          bind:this={resultsDialog}
          aria-label="Execute and results workspace"
          data-od-id="viewport-results"
          onclose={onResultsDialogClose}
        >
          <section class="panel bottom dialog-panel" data-od-id="execute-and-results">
            <div class="panel-head">
              <h2>Execute & Results</h2>
              <div class="toolbar">
                <button
                  type="button"
                  class="panel-max-btn"
                  onclick={closeResultsViewport}
                  data-od-id="maximize-results"
                >Exit viewport</button>
              </div>
            </div>
            {@render executeResultsBody()}
          </section>
        </dialog>
      {:else}
        <section
          class="panel bottom"
          class:collapsed={isCollapsed}
          style={isCollapsed ? undefined : `flex:0 0 auto;height:${panelHeight}px`}
          data-od-id="execute-and-results"
        >
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="resize-handle" onmousedown={handleResizeStart}></div>
          <div class="panel-head">
            <h2>Execute & Results</h2>
            <div class="toolbar">
              <button
                type="button"
                class="panel-max-btn"
                onclick={openResultsViewport}
                title="Fill viewport"
                data-od-id="maximize-results"
                aria-haspopup="dialog"
              >Fill viewport</button>
              <button
                type="button"
                class="ghost"
                onclick={toggleCollapse}
                aria-expanded={!isCollapsed}
                aria-controls="bottom-content"
                data-od-id="toggle-results"
              >{isCollapsed ? 'Expand' : 'Collapse'}</button>
            </div>
          </div>
          {#if !isCollapsed}
            {@render executeResultsBody()}
          {/if}
        </section>
      {/if}
    </div>
  {:else}
    <div class="signal-bar">
      <label for="signal-select">Signal</label>
      <select id="signal-select" bind:value={selectedSignal}>
        <option value="traces">traces</option>
        <option value="logs">logs</option>
        <option value="metrics">metrics</option>
      </select>
    </div>
    <PipelineBuilder {pipeline} signal={selectedSignal} onValidate={handleValidate} onImport={() => showImport = true} />
    <section class="execution-section"><h2>Execute</h2><RunSelector {pipelineYaml} bind:signal={selectedSignal} onresult={handleExecuteResult} onrunselect={handleRunSelect} /></section>
  {/if}
  {#if activeTab !== 'graph'}
    <section class="results-section"><h2>Results</h2><ResultsView {result} error={resultError} {originalData} stale={resultsStale} /></section>
  {/if}
  {#if showImport}
    <div
      class="modal-overlay"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onclick={(e) => { if (e.target === e.currentTarget) showImport = false }}
      onkeydown={(e) => e.key === 'Escape' && (showImport = false)}
    >
      <div class="modal"><ConfigImport onimport={handleImport} /><button class="btn-close" onclick={() => showImport = false}>Close</button></div>
    </div>
  {/if}
  {#if editingStage !== null && activeTab !== 'graph'}
    <div
      class="modal-overlay"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onclick={(e) => { if (e.target === e.currentTarget) editingStage = null }}
      onkeydown={(e) => e.key === 'Escape' && (editingStage = null)}
    >
      <div class="modal">
        <StageEditor stage={editingStage.stage} processor={editingStage.processor} signal={editingStage.signal} onupdate={handleStageUpdate} onupdateProcessor={handleStageProcessorUpdate} />
        <button class="btn-close" onclick={() => editingStage = null}>Close</button>
      </div>
    </div>
  {/if}
</Layout>

<style>
  .graph-shell {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    overflow: auto;
  }

  .layout {
    display: grid;
    grid-template-columns: 360px minmax(0, 1fr) 290px;
    gap: 12px;
    align-items: stretch;
    flex: 1 0 400px;
    min-height: 400px;
  }

  .layout.yaml-closed {
    grid-template-columns: minmax(0, 1fr) 290px;
  }

  .workspace {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .canvas-wrapper {
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .panel.bottom {
    position: relative;
    display: flex;
    flex-direction: column;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    overflow: hidden;
    min-height: 0;
  }

  .panel.bottom:not(.dialog-panel):not(.collapsed) {
    max-height: max(160px, 60vh);
  }

  .panel.bottom.collapsed {
    flex: 0 0 auto;
    height: 58px !important;
  }

  .results-dialog {
    margin: 0;
    padding: 0;
    width: 100vw;
    max-width: 100vw;
    height: 100dvh;
    max-height: 100dvh;
    background: var(--color-bg);
    color: var(--color-text);
    border: 0;
  }

  .results-dialog::backdrop {
    background: var(--color-bg);
  }

  .results-dialog .dialog-panel {
    margin: 0;
    border-radius: 0;
    min-height: 100%;
    height: 100%;
    border: 0;
  }

  .results-dialog .bottom-content {
    min-height: calc(100dvh - 72px);
    flex: 1;
  }

  .results-dialog .results {
    padding: 24px;
  }

  :global(body:has(.results-dialog[open])) {
    overflow: hidden;
  }

  .panel.bottom .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border);
    min-height: 58px;
    flex-shrink: 0;
  }

  .panel.bottom .panel-head h2 {
    font-size: 14px;
    margin: 0;
  }

  .panel.bottom .toolbar {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .panel.bottom .toolbar button,
  .panel-max-btn {
    font-size: 12px;
    min-height: 44px;
  }

  .ghost {
    background: transparent;
  }

  .resize-handle {
    height: 8px;
    cursor: row-resize;
    flex-shrink: 0;
    position: relative;
    background: transparent;
  }

  .resize-handle:hover::after,
  .resize-handle:active::after {
    content: '';
    position: absolute;
    top: 3px;
    left: 50%;
    transform: translateX(-50%);
    width: 40px;
    height: 2px;
    background: var(--color-primary);
    border-radius: 1px;
    opacity: .6;
  }

  .bottom-content {
    display: grid;
    grid-template-columns: minmax(220px, 250px) minmax(0, 1fr);
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .execute {
    padding: 12px;
    border-right: 1px solid var(--color-border);
    overflow: auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .execute h3 {
    margin: 0;
    font-size: 13px;
  }

  .results h3 {
    margin: 0 0 16px;
    font-size: 14px;
  }

  .results {
    padding: 16px;
    min-width: 0;
    overflow: auto;
  }

  .signal-bar {
    display: flex;
    align-items: center;
    gap: .5rem;
    margin-bottom: 1rem;
    color: var(--color-text);
  }

  .signal-bar select {
    padding: .35rem .6rem;
    border-radius: 6px;
    border: 1px solid var(--color-border);
    background: var(--color-surface);
    color: inherit;
    min-height: 44px;
  }

  .execution-section,
  .results-section {
    margin-top: var(--space-xl);
    padding-top: var(--space-md);
    border-top: 1px solid var(--color-border);
  }

  .execution-section h2,
  .results-section h2 {
    font-size: var(--font-size-lg);
    font-weight: 600;
    margin: 0 0 .75rem 0;
    color: var(--color-text);
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .modal {
    background: var(--color-surface);
    border-radius: var(--radius-lg);
    padding: var(--space-lg);
    max-width: 700px;
    width: 90%;
    max-height: 80vh;
    overflow-y: auto;
  }

  .btn-close {
    margin-top: .75rem;
    padding: .4rem .9rem;
    background: var(--color-surface-hover);
    color: var(--color-text);
    border: none;
    border-radius: var(--radius-md);
    cursor: pointer;
    min-height: 44px;
  }

  @media (max-width: 1450px) {
    .layout {
      grid-template-columns: minmax(0, 1fr) 290px;
      flex-basis: 612px;
      grid-template-rows: 200px minmax(400px, 1fr);
    }
    .layout.yaml-closed {
      flex-basis: 400px;
      grid-template-rows: minmax(400px, 1fr);
    }
    .layout :global(#yaml-panel) {
      grid-column: 1 / -1;
      width: 100% !important;
      min-height: 0;
      max-height: 200px;
    }
  }

  @media (max-width: 960px) {
    .layout,
    .layout.yaml-closed {
      grid-template-columns: 1fr;
      flex-basis: 1114px;
      grid-template-rows: 200px 650px 240px;
    }
    .layout.yaml-closed {
      flex-basis: 902px;
      grid-template-rows: 650px 240px;
    }
    .layout :global(.graph-workspace) {
      grid-template-rows: 200px minmax(360px, 1fr);
    }
    .layout :global(.palette) {
      width: 100%;
      min-height: 0;
    }
    .bottom-content {
      grid-template-columns: 1fr;
      grid-template-rows: minmax(180px, auto) minmax(180px, 1fr);
      overflow: auto;
    }
    .execute {
      border-right: 0;
      border-bottom: 1px solid var(--color-border);
    }
  }
</style>
