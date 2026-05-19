<script lang="ts">
  import type { Pipeline, PipelineResult, Stage, ProcessorConfig, PipelinesBySignal } from './lib/types'
  import Layout from './lib/Layout.svelte'
  import PipelineBuilder from './lib/PipelineBuilder.svelte'
  import SwimlaneFlowBuilder from './lib/SwimlaneFlowBuilder.svelte'
  import EditorPanel from './lib/EditorPanel.svelte'
  import StageEditor from './lib/StageEditor.svelte'
  import RunSelector from './lib/RunSelector.svelte'
  import ResultsView from './lib/ResultsView.svelte'
  import ConfigImport from './lib/ConfigImport.svelte'
  import QuickTab from './lib/QuickTab.svelte'
  import { exportPipelinesToYaml, downloadYaml } from './lib/export-yaml'
  import yaml from 'js-yaml'

  let pipeline: Pipeline = $state({ stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] })
  let pipelinesBySignal: PipelinesBySignal = $state({
    traces: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    logs: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    metrics: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }
  })
  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let originalData: unknown = $state(null)
  let showImport: boolean = $state(false)
  let pipelineYaml: string = $state('')
  let activeTab: 'quick' | 'pipeline' | 'graph' = $state('graph')
  let editingStage: { stage: Stage; processor: ProcessorConfig } | null = $state(null)
  let selectedNodeId: string | null = $state(null)
  let panelHeight = $state(300)
  let isCollapsed = $state(false)
  let isMaximized = $state(false)
  let savedHeight = $state(300)
  let currentPipeline = $derived(pipelinesBySignal.traces || { stages: [], processors: {}, receivers: [], exporters: [] })

  function toggleCollapse() {
    if (!isCollapsed) { savedHeight = isMaximized ? savedHeight : panelHeight; isCollapsed = true; if (isMaximized) isMaximized = false }
    else { isCollapsed = false; panelHeight = savedHeight }
  }
  function toggleMaximize() {
    if (!isMaximized) { savedHeight = isCollapsed ? savedHeight : panelHeight; isMaximized = true; if (isCollapsed) isCollapsed = false }
    else { isMaximized = false; panelHeight = savedHeight }
  }
  function handleResizeStart(e: MouseEvent) {
    e.preventDefault()
    savedHeight = panelHeight
    if (isCollapsed) isCollapsed = false
    if (isMaximized) isMaximized = false
    const startY = e.clientY
    const startHeight = panelHeight
    const graphMain = (e.target as HTMLElement).closest('.graph-main') as HTMLElement | null
    function onMouseMove(e: MouseEvent) {
      if (!graphMain) return
      const delta = startY - e.clientY
      const newHeight = startHeight + delta
      const maxHeight = graphMain.clientHeight - 60
      if (newHeight <= 100) { if (!isCollapsed) { savedHeight = panelHeight; isCollapsed = true } }
      else if (newHeight >= maxHeight) { if (!isMaximized) { savedHeight = panelHeight; isMaximized = true } }
      else { if (isCollapsed) isCollapsed = false; if (isMaximized) isMaximized = false; panelHeight = newHeight }
    }
    function onMouseUp() { window.removeEventListener('mousemove', onMouseMove); window.removeEventListener('mouseup', onMouseUp) }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }

  function handlePipelinesChange(p: PipelinesBySignal) { pipelinesBySignal = p }
  function handleNodeSelected(nodeId: string | null) { selectedNodeId = nodeId }
  function handleExport() { downloadYaml('pipeline-config.yaml', exportPipelinesToYaml(pipelinesBySignal)) }
  function handleValidate(p: Pipeline) {
    pipelineYaml = yamlStringify(p)
    fetch('/api/ottl/pipeline/validate', { method: 'POST', headers: { 'Content-Type': 'application/x-yaml' }, body: pipelineYaml })
      .then(r => r.json()).then(d => alert(d.valid ? 'Pipeline is valid!' : 'Validation errors:\n' + (d.errors || []).join('\n')))
      .catch(e => alert('Validation failed: ' + e.message))
  }
  async function handleExecuteResult(r: PipelineResult | null, err?: string, runId?: string) {
    result = r; resultError = err
    if (r && runId) { try { const res = await fetch(`/api/runs/${runId}`); if (res.ok) originalData = (await res.json()).trace_data ?? null } catch {} }
  }
  function handleImport(p: PipelinesBySignal) {
    pipelinesBySignal = p; pipeline = p.traces || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }
    showImport = false; if (activeTab !== 'graph') activeTab = 'pipeline'
  }
  function handleNodeEdit(data: { stage: Stage; processor: ProcessorConfig }) { editingStage = data }
  function yamlStringify(p: Pipeline): string {
    let out = 'stages:\n'
    for (const s of p.stages) out += `  - id: ${s.id}\n    type: ${s.type}\n    processor_ref: ${s.processor_ref}\n${s.error_mode ? `    error_mode: ${s.error_mode}\n` : ''}`
    out += 'processors:\n'
    for (const [k, v] of Object.entries(p.processors)) {
      if (v && typeof v === 'object' && Object.keys(v).length > 0) {
        const lines = yaml.dump(v, { indent: 2, noRefs: true, sortKeys: false }).trimEnd().split('\n')
        out += `  ${k}:\n`
        for (const l of lines) out += `    ${l}\n`
      } else out += `  ${k}: {}\n`
    }
    return out
  }
</script>

<Layout {activeTab} onTabChange={(t) => activeTab = t}>
  {#if activeTab === 'quick'}
    <QuickTab onresult={handleExecuteResult} />
  {:else if activeTab === 'graph'}
    <div class="graph-fullscreen">
      <EditorPanel pipelines={pipelinesBySignal} {selectedNodeId} onPipelinesChange={handlePipelinesChange} />
      <div class="graph-main">
        <div class="canvas-wrapper" class:maximized={isMaximized}>
          <SwimlaneFlowBuilder pipelines={pipelinesBySignal} {result} onImport={() => showImport = true} onExport={handleExport} onNodeEdit={handleNodeEdit} onPipelinesChange={handlePipelinesChange} onNodeSelected={handleNodeSelected} onResultClear={() => { result = null; resultError = undefined }} />
        </div>
        <div class="graph-bottom-panels" class:collapsed={isCollapsed} class:maximized={isMaximized} style={isCollapsed ? '' : isMaximized ? 'flex:1;min-height:0' : `flex:0 0 auto;height:${panelHeight}px`}>
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="resize-handle" onmousedown={handleResizeStart}></div>
          <div class="panel-bar">
            <button class="panel-toggle" onclick={toggleCollapse}>{isCollapsed ? '▶' : '▼'} Execute & Results</button>
            <button class="panel-max-btn" onclick={toggleMaximize} title={isMaximized ? 'Restore' : 'Maximize'}>{isMaximized ? '⤡' : '⛶'}</button>
          </div>
          {#if !isCollapsed}
            <div class="panel-content">
              <section class="execution-section"><h2>Execute</h2><RunSelector pipelineYaml={yamlStringify(pipelinesBySignal.traces || { stages: [], processors: {} })} onresult={handleExecuteResult} /></section>
              <section class="results-section"><h2>Results</h2><ResultsView {result} error={resultError} {originalData} /></section>
            </div>
          {/if}
        </div>
      </div>
    </div>
  {:else}
    <PipelineBuilder {pipeline} onValidate={handleValidate} onImport={() => showImport = true} />
    <section class="execution-section"><h2>Execute</h2><RunSelector pipelineYaml={yamlStringify(pipeline)} onresult={handleExecuteResult} /></section>
  {/if}
  {#if activeTab !== 'graph'}
    <section class="results-section"><h2>Results</h2><ResultsView {result} error={resultError} {originalData} /></section>
  {/if}
  {#if showImport}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="modal-overlay" onclick={() => showImport = false} onkeydown={(e) => e.key === 'Escape' && (showImport = false)}>
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="modal" onclick={(e) => e.stopPropagation()}><ConfigImport onimport={handleImport} /><button class="btn-close" onclick={() => showImport = false}>Close</button></div>
    </div>
  {/if}
  {#if editingStage !== null}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="modal-overlay" onclick={() => editingStage = null} onkeydown={(e) => e.key === 'Escape' && (editingStage = null)}>
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="modal" onclick={(e) => e.stopPropagation()}>
        <StageEditor stage={editingStage.stage} processor={editingStage.processor} onupdate={(s: Stage) => pipeline.stages = pipeline.stages.map(st => st.id === editingStage!.stage.id ? s : st)} onupdateProcessor={(c: ProcessorConfig) => pipeline.processors = { ...pipeline.processors, [editingStage!.stage.processor_ref]: c }} />
        <button class="btn-close" onclick={() => editingStage = null}>Close</button>
      </div>
    </div>
  {/if}
</Layout>

<style>
  .execution-section, .results-section { margin-top: var(--space-xl,2rem); padding-top: var(--space-md,1rem); border-top: 1px solid var(--color-border,#334155); }
  .execution-section h2, .results-section h2 { font-size: var(--font-size-lg,1.1rem); font-weight: 600; margin: 0 0 .75rem 0; color: var(--color-text,#f1f5f9); }
  .modal-overlay { position: fixed; inset: 0; background: rgba(0,0,0,.4); display: flex; align-items: center; justify-content: center; z-index: 100; }
  .modal { background: var(--color-surface,#1e293b); border-radius: var(--radius-lg,8px); padding: var(--space-lg,1.5rem); max-width: 700px; width: 90%; max-height: 80vh; overflow-y: auto; }
  .btn-close { margin-top: .75rem; padding: .4rem .9rem; background: var(--color-surface-hover,#334155); color: var(--color-text,#f1f5f9); border: none; border-radius: var(--radius-md,6px); cursor: pointer; }
  .graph-fullscreen { flex: 1; min-height: 0; display: flex; flex-direction: row; overflow: hidden; }
  .graph-main { display: flex; flex-direction: column; flex: 1; overflow: hidden; }
  .canvas-wrapper { flex: 1; min-height: 0; overflow: hidden; }
  .canvas-wrapper.maximized { flex: 0 0 40px; }
  .graph-bottom-panels { position: relative; display: flex; flex-direction: column; border-top: 1px solid var(--color-border,#334155); overflow: hidden; background: var(--color-bg,#0f172a); }
  .graph-bottom-panels.collapsed { flex: 0 0 auto; height: 36px !important; }
  .panel-bar { flex-shrink: 0; display: flex; align-items: center; justify-content: space-between; padding: 4px 8px; background: var(--color-surface,#1e293b); border-bottom: 1px solid var(--color-border,#334155); min-height: 32px; }
  .panel-toggle { background: none; border: none; color: var(--color-text-secondary,#94a3b8); cursor: pointer; font-size: .85rem; padding: 4px 8px; border-radius: 4px; }
  .panel-toggle:hover { color: var(--color-text,#f1f5f9); background: var(--color-surface-hover,#334155); }
  .panel-max-btn { background: none; border: none; color: var(--color-text-secondary,#94a3b8); cursor: pointer; font-size: .8rem; padding: 2px 6px; border-radius: 3px; line-height: 1; }
  .panel-max-btn:hover { color: var(--color-text,#f1f5f9); background: var(--color-surface-hover,#334155); }
  .resize-handle { height: 8px; cursor: row-resize; flex-shrink: 0; position: relative; background: transparent; }
  .resize-handle:hover::after, .resize-handle:active::after { content: ''; position: absolute; top: 3px; left: 50%; transform: translateX(-50%); width: 40px; height: 2px; background: var(--color-primary,#3b82f6); border-radius: 1px; opacity: .6; }
  .panel-content { flex: 1; overflow-y: auto; padding: var(--space-md,1rem); }
  .graph-bottom-panels .execution-section, .graph-bottom-panels .results-section { margin-top: var(--space-md,1rem); padding-top: var(--space-md,1rem); border-top: 1px solid var(--color-border,#334155); }
  .graph-bottom-panels .execution-section h2, .graph-bottom-panels .results-section h2 { font-size: var(--font-size-lg,1.1rem); font-weight: 600; margin: 0 0 .75rem 0; color: var(--color-text,#f1f5f9); }
</style>
