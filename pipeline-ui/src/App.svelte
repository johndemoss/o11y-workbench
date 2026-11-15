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
  import { withCollectorVersion } from './lib/collector-version'
  import { dump } from 'js-yaml'

  function initialPipeline(): Pipeline {
    return {
      stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'],
      receiver_configs: { otlp: { protocols: { grpc: {} } } },
      exporter_configs: { debug: {} },
    }
  }
  let pipelinesBySignal: PipelinesBySignal = $state({
    traces: initialPipeline(),
    logs: initialPipeline(),
    metrics: initialPipeline()
  })
  let selectedSignal: SignalType = $state('traces')
  let pipeline = $derived(pipelinesBySignal[selectedSignal] || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] })
  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let originalData: unknown = $state(null)
  let selectedRunPayload: unknown = $state(null)
  let showImport: boolean = $state(false)
  let importGeneration = 0
  let importHandler = $state<(p: PipelinesBySignal, yaml: string) => void>(() => {})
  function openImport() {
    const generation = ++importGeneration
    importHandler = (p, yaml) => { if (generation === importGeneration) handleImport(p, yaml) }
    showImport = true
  }
  function closeImport() {
    importGeneration++
    showImport = false
  }
  let pipelineYaml: string = $state('')
  let authoredYaml = $state<string | null>(null)
  let draftReady = $state(true)
  let quickYaml = $state('')
  let executedSource = $state<'quick' | 'pipeline' | 'graph'>('graph')
  let resultSignal = $state<SignalType>('traces')
  let activeTab: 'quick' | 'pipeline' | 'graph' = $state('graph')
  let graphYaml = $derived(exportPipelinesToYamlLocal(pipelinesBySignal))
  let executableYaml = $derived(activeTab === 'graph' && authoredYaml !== null ? authoredYaml : pipelineYaml)
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
  /** Run/signal identity captured at last successful execute. */
  let executedRunId = $state<string | null>(null)
  let executedSignal = $state<SignalType | null>(null)
  let selections = $state<Record<'quick' | 'pipeline' | 'graph', { runId: string; signal: SignalType }>>({
    quick: { runId: '', signal: 'traces' },
    graph: { runId: '', signal: 'traces' },
    pipeline: { runId: '', signal: 'traces' },
  })

  type InspectorSelection =
    | { kind: 'stage'; stage: Stage; processor: ProcessorConfig; signal: SignalType }
    | { kind: 'receiver'; name: string }
    | null

  let inspectorSelection = $state<InspectorSelection>(null)
  let highlightedReceiver = $derived.by(() => {
    const sel = inspectorSelection
    return sel?.kind === 'receiver' ? sel.name : null
  })
  let resultsStale = $derived(!!result && (
    selections[activeTab].runId !== executedRunId ||
    selections[activeTab].signal !== executedSignal ||
    (executedYaml !== null && (executedSource === 'quick' ? quickYaml : executedSource === 'graph' ? authoredYaml ?? graphYaml : yamlStringify(pipelinesBySignal[resultSignal])) !== executedYaml)
  ))
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

  function normalizePipelines(p: PipelinesBySignal): PipelinesBySignal {
    const empty = (): Pipeline => ({ stages: [], processors: {}, receivers: [], exporters: [] })
    return { traces: p.traces ?? empty(), logs: p.logs ?? empty(), metrics: p.metrics ?? empty() }
  }

  function handlePipelinesChange(p: PipelinesBySignal, fromEditor = false) {
    if (!fromEditor) authoredYaml = null
    pipelinesBySignal = normalizePipelines(p)
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

  async function handleExport() {
    if (!draftReady) {
      alert('Synchronize or resync the YAML draft before downloading.')
      return
    }
    if (authoredYaml !== null) {
      downloadYaml('pipeline-config.yaml', authoredYaml)
      return
    }
    const snapshot = graphYaml
    try {
      const yaml = await exportPipelinesToYaml(pipelinesBySignal)
      if (draftReady && authoredYaml === null && snapshot === graphYaml) downloadYaml('pipeline-config.yaml', yaml)
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e))
    }
  }
  function handlePipelineChange(p: Pipeline) {
    handlePipelinesChange({ ...pipelinesBySignal, [selectedSignal]: p })
  }
  function handleValidate(p: Pipeline) {
    if (activeTab === 'graph' && !draftReady) {
      alert('Synchronize or resync the YAML draft before validation.')
      return
    }
    const body = activeTab === 'graph' ? executableYaml : yamlStringify(p)
    const url = activeTab === 'graph'
      ? withCollectorVersion('/api/ottl/pipeline/validate')
      : withCollectorVersion(`/api/ottl/pipeline/validate?signal=${encodeURIComponent(selectedSignal)}`)
    fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-yaml' }, body })
      .then(r => r.json()).then(d => alert(d.valid ? 'Pipeline is valid!' : 'Validation errors:\n' + (d.errors || []).join('\n')))
      .catch(e => alert('Validation failed: ' + e.message))
  }
  function handleRunSelect(source: 'quick' | 'pipeline' | 'graph', runId: string, payload: unknown, sig: SignalType) {
    selections[source] = { runId, signal: sig }
    selectedRunPayload = payload
    if (source !== 'quick') selectedSignal = sig
  }
  function handleExecuteResult(r: PipelineResult | null, err?: string, _runId?: string, originalPayload?: unknown, submittedYaml?: string, submittedSignal?: SignalType, source: 'quick' | 'pipeline' | 'graph' = activeTab) {
    result = r
    resultError = err
    if (!r) {
      executedYaml = null
      return
    }
    originalData = originalPayload !== undefined ? originalPayload : selectedRunPayload
    executedYaml = submittedYaml ?? null
    executedSource = source
    resultSignal = submittedSignal ?? selectedSignal
    executedRunId = _runId ?? null
    executedSignal = submittedSignal ?? selectedSignal
  }
  function handleImport(p: PipelinesBySignal, yaml: string) {
    handlePipelinesChange(p, true)
    authoredYaml = yaml
    draftReady = true
    const withStages = (['traces', 'logs', 'metrics'] as SignalType[]).find(s => (pipelinesBySignal[s]?.stages?.length ?? 0) > 0)
    if (withStages) selectedSignal = withStages
    closeImport()
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
    authoredYaml = null
    if (s.type !== prev.type) s = renameStageProcessor(sig, prev, s)
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
    authoredYaml = null
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
    authoredYaml = null
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
    authoredYaml = null
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
    authoredYaml = null
    if (s.type !== editingStage!.stage.type) s = renameStageProcessor(sig, editingStage!.stage, s)
    pipelinesBySignal = {
      ...pipelinesBySignal,
      [sig]: { ...pipelinesBySignal[sig], stages: pipelinesBySignal[sig].stages.map(st => st.id === editingStage!.stage.id ? s : st) }
    }
    editingStage = { ...editingStage!, stage: s }
  }
  function handleStageProcessorUpdate(c: ProcessorConfig) {
    const sig = editingStage!.signal
    authoredYaml = null
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

  function renameStageProcessor(sig: SignalType, previous: Stage, next: Stage): Stage {
    const used = new Set(SIGNALS.flatMap(signal => Object.keys(pipelinesBySignal[signal].processors)))
    const suffix = previous.processor_ref.includes('/') ? previous.processor_ref.slice(previous.processor_ref.indexOf('/') + 1) : previous.id
    const base = `${next.type}/${suffix}`
    let ref = base
    for (let n = 2; used.has(ref); n++) ref = `${base}_${n}`
    const lane = pipelinesBySignal[sig]
    const processors = { ...lane.processors, [ref]: lane.processors[previous.processor_ref] ?? {} }
    if (!lane.stages.some(stage => stage.id !== previous.id && stage.processor_ref === previous.processor_ref)) delete processors[previous.processor_ref]
    pipelinesBySignal = { ...pipelinesBySignal, [sig]: { ...lane, processors } }
    return { ...next, processor_ref: ref }
  }

  $effect(() => {
    pipelineYaml = activeTab === 'graph' ? graphYaml : yamlStringify(pipeline)
  })
</script>

<Layout {activeTab} onTabChange={(t) => { if (draftReady) activeTab = t; else alert('Synchronize or resync the YAML draft before switching tabs.') }}>
  {#if activeTab === 'quick'}
    <QuickTab ondraft={(yaml) => quickYaml = yaml} onresult={(r, e, id, payload, yaml, signal) => handleExecuteResult(r, e, id, payload, yaml, signal, 'quick')} />
  {:else if activeTab === 'graph'}
    <div class="graph-shell">
      <div class="layout" class:yaml-closed={!yamlOpen} id="layout">
        <EditorPanel
          pipelines={pipelinesBySignal}
          yaml={executableYaml}
          graphYaml={pipelineYaml}
          {selectedNodeId}
          bind:open={yamlOpen}
          onPipelinesChange={(p) => handlePipelinesChange(p, true)}
          onDraftChange={(text, ready) => { authoredYaml = text; draftReady = ready }}
        />
        <div class="workspace" inert={!draftReady}>
          <div class="canvas-wrapper">
            <SwimlaneFlowBuilder
              pipelines={pipelinesBySignal}
              {result}
              {highlightedReceiver}
              {yamlOpen}
              onShowYaml={() => yamlOpen = true}
              onImport={openImport}
              onExport={handleExport}
              onNodeEdit={handleNodeEdit}
              onPipelinesChange={handlePipelinesChange}
              onNodeSelected={handleNodeSelected}
              onResultClear={clearResults}
            />
          </div>
        </div>
        <div inert={!draftReady}>
        <StageInspector
          selection={inspectorSelection}
          pipelines={pipelinesBySignal}
          pipelineYaml={executableYaml}
          onupdate={handleInspectorStageUpdate}
          onupdateProcessor={handleInspectorProcessorUpdate}
          onMove={handleInspectorMove}
          onRemove={handleInspectorRemove}
        />
        </div>
      </div>

      {#snippet executeResultsBody()}
        <div class="bottom-content" id="bottom-content">
          <section class="execute" data-od-id="execution-panel" aria-label="Execute">
            <h3 title="Captured run → current YAML">Execute</h3>
            <RunSelector pipelineYaml={executableYaml} disabled={!draftReady} bind:signal={selectedSignal} onresult={(r, e, id, payload, yaml, signal) => handleExecuteResult(r, e, id, payload, yaml, signal, 'graph')} onrunselect={(id, payload, signal) => handleRunSelect('graph', id, payload, signal)} />
          </section>
          <section class="results" data-od-id="stage-results">
            <ResultsView
              {result}
              error={resultError}
              {originalData}
              signal={resultSignal}
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
    <PipelineBuilder {pipeline} signal={selectedSignal} onValidate={handleValidate} onImport={openImport} onchange={handlePipelineChange} />
    <section class="execution-section"><h2>Execute</h2><RunSelector {pipelineYaml} bind:signal={selectedSignal} onresult={(r, e, id, payload, yaml, signal) => handleExecuteResult(r, e, id, payload, yaml, signal, 'pipeline')} onrunselect={(id, payload, signal) => handleRunSelect('pipeline', id, payload, signal)} /></section>
  {/if}
  {#if activeTab !== 'graph'}
    <section class="results-section"><h2>Results</h2><ResultsView {result} error={resultError} {originalData} stale={resultsStale} signal={resultSignal} /></section>
  {/if}
  {#if showImport}
    <div
      class="modal-overlay"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onclick={(e) => { if (e.target === e.currentTarget) closeImport() }}
      onkeydown={(e) => e.key === 'Escape' && closeImport()}
    >
      <div class="modal"><ConfigImport onimport={importHandler} oncancel={closeImport} /><button class="btn-close" onclick={closeImport}>Close</button></div>
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
