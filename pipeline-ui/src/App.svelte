<script lang="ts">
  import type { Pipeline, PipelineResult, Stage, ProcessorConfig } from './lib/types'
  import type { PipelinesBySignal } from './lib/types'
  import PipelineBuilder from './lib/PipelineBuilder.svelte'
  import FlowBuilder from './lib/FlowBuilder.svelte'
  import SwimlaneFlowBuilder from './lib/SwimlaneFlowBuilder.svelte'
  import EditorPanel from './lib/EditorPanel.svelte'
  import StageEditor from './lib/StageEditor.svelte'
  import RunSelector from './lib/RunSelector.svelte'
  import ResultsView from './lib/ResultsView.svelte'
  import ConfigImport from './lib/ConfigImport.svelte'
  import QuickTab from './lib/QuickTab.svelte'
  import { downloadYaml } from './lib/export-yaml'
  import { exportPipelinesToYaml, importYamlToPipelines } from './lib/export-yaml'
  import yaml from 'js-yaml'

  // Single-pipeline state (kept for Pipeline tab compatibility)
  let pipeline: Pipeline = $state({ stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] })

  // Multi-signal state (for Graph tab)
  let pipelinesBySignal: PipelinesBySignal = $state({
    traces: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    logs: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    metrics: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }
  } as PipelinesBySignal)

  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let originalData: unknown = $state(null)
  let showImport: boolean = $state(false)
  let pipelineYaml: string = $state('')
  let activeTab: 'quick' | 'pipeline' | 'graph' = $state('pipeline')
  let editingStage: { stage: Stage; processor: ProcessorConfig } | null = $state(null)
  let selectedNodeId: string | null = $state(null)
  let panelHeight = $state(300)
  let isCollapsed = $state(false)
  let isMaximized = $state(false)
  let savedHeight = $state(300)

  function toggleCollapse() {
    if (!isCollapsed) {
      savedHeight = isMaximized ? savedHeight : panelHeight
      isCollapsed = true
      if (isMaximized) isMaximized = false
    } else {
      isCollapsed = false
      panelHeight = savedHeight
    }
  }

  function toggleMaximize() {
    if (!isMaximized) {
      savedHeight = isCollapsed ? savedHeight : panelHeight
      isMaximized = true
      if (isCollapsed) isCollapsed = false
    } else {
      isMaximized = false
      panelHeight = savedHeight
    }
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
      const maxHeight = graphMain.clientHeight - 60
      const delta = startY - e.clientY
      const newHeight = startHeight + delta

      if (newHeight <= 100) {
        if (!isCollapsed) {
          savedHeight = panelHeight
          isCollapsed = true
        }
      } else if (newHeight >= maxHeight) {
        if (!isMaximized) {
          savedHeight = panelHeight
          isMaximized = true
        }
      } else {
        if (isCollapsed) isCollapsed = false
        if (isMaximized) isMaximized = false
        panelHeight = newHeight
      }
    }
    function onMouseUp() {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }
  // Sync single pipeline from multi-signal for the Pipeline tab
  let currentPipeline = $derived(pipelinesBySignal.traces || { stages: [], processors: {}, receivers: [], exporters: [] })

  function handlePipelinesChange(p: PipelinesBySignal): void {
    pipelinesBySignal = p
  }

  function handleNodeSelected(nodeId: string | null): void {
    selectedNodeId = nodeId
  }

  function handleExport(): void {
    const yaml = exportPipelinesToYaml(pipelinesBySignal)
    downloadYaml('pipeline-config.yaml', yaml)
  }

  function handleValidate(p: Pipeline): void {
    pipelineYaml = yamlStringify(p)
    fetch('/api/ottl/pipeline/validate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-yaml' },
      body: pipelineYaml
    })
      .then(r => r.json())
      .then(data => {
        if (data.valid) {
          alert('Pipeline is valid!')
        } else {
          alert('Validation errors:\n' + (data.errors || []).join('\n'))
        }
      })
      .catch(e => alert('Validation failed: ' + e.message))
  }

  async function handleExecuteResult(r: PipelineResult | null, err?: string, runId?: string): void {
    result = r
    resultError = err
    if (r && runId) {
      try {
        const res = await fetch(`/api/runs/${runId}`)
        if (res.ok) {
          const runData = await res.json()
          originalData = runData.trace_data ?? null
        }
      } catch {
        // Non-critical — first-stage diff just won't be available
      }
    }
  }

  function handleImport(p: PipelinesBySignal): void {
    pipelinesBySignal = p
    pipeline = p.traces || { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] }
    showImport = false
    if (activeTab !== 'graph') {
      activeTab = 'pipeline'
    }
  }

  function handleNodeEdit(data: { stage: Stage; processor: ProcessorConfig }) {
    editingStage = data
  }

  function yamlStringify(p: Pipeline): string {
    let out = 'stages:\n'
    for (const s of p.stages) {
      out += `  - id: ${s.id}\n`
      out += `    type: ${s.type}\n`
      out += `    processor_ref: ${s.processor_ref}\n`
      if (s.error_mode) out += `    error_mode: ${s.error_mode}\n`
    }
    out += 'processors:\n'
    for (const [key, val] of Object.entries(p.processors)) {
      if (val && typeof val === 'object' && Object.keys(val).length > 0) {
        const valYaml = yaml.dump(val, { indent: 2, noRefs: true, sortKeys: false })
        const lines = valYaml.trimEnd().split('\n')
        out += `  ${key}:\n`
        for (const line of lines) {
          out += `    ${line}\n`
        }
      } else {
        out += `  ${key}: {}\n`
      }
    }
    return out
  }
</script>

<main class="app">
  <nav class="top-nav">
    <span class="app-title">Pipeline Prototyper</span>
    <a href="/" class="nav-link">Classic OTTL Prototyper →</a>
  </nav>
  <div class="tab-bar">
    <button
      class="tab-button"
      class:active={activeTab === 'quick'}
      onclick={() => activeTab = 'quick'}
    >
      Quick Transform/Filter
    </button>
    <button
      class="tab-button"
      class:active={activeTab === 'graph'}
      onclick={() => activeTab = 'graph'}
    >
      Graph Builder
    </button>
    <button
      class="tab-button"
      class:active={activeTab === 'pipeline'}
      onclick={() => activeTab = 'pipeline'}
    >
      Pipeline Builder
    </button>
  </div>

  {#if activeTab === 'quick'}
    <QuickTab onresult={handleExecuteResult} />
  {:else if activeTab === 'graph'}
    <div class="graph-fullscreen">
      <EditorPanel
        pipelines={pipelinesBySignal}
        {selectedNodeId}
        onPipelinesChange={handlePipelinesChange}
      />
      <div class="graph-main">
        <div class="canvas-wrapper" class:maximized={isMaximized}>
          <SwimlaneFlowBuilder
            pipelines={pipelinesBySignal}
            {result}
            onValidate={handleValidate}
            onImport={() => { showImport = true }}
            onExport={handleExport}
            onNodeEdit={handleNodeEdit}
            onPipelinesChange={handlePipelinesChange}
            onNodeSelected={handleNodeSelected}
            onResultClear={() => { result = null; resultError = undefined }}
          />
        </div>
        <div class="graph-bottom-panels"
          class:collapsed={isCollapsed}
          class:maximized={isMaximized}
          style={isCollapsed ? '' : isMaximized ? 'flex:1;min-height:0' : `flex:0 0 auto;height:${panelHeight}px`}
        >
          <div class="resize-handle" onmousedown={handleResizeStart}></div>
          <div class="panel-bar">
            <button class="panel-toggle" onclick={toggleCollapse}>
              {isCollapsed ? '▶' : '▼'} Execute & Results
            </button>
            <button class="panel-max-btn" onclick={toggleMaximize} title={isMaximized ? 'Restore size' : 'Maximize'}>
              {isMaximized ? '⤡' : '⛶'}
            </button>
          </div>
          {#if !isCollapsed}
            <div class="panel-content">
              <section class="execution-section">
                <h2>Execute Pipeline</h2>
                <RunSelector
                  pipelineYaml={yamlStringify(pipelinesBySignal.traces || { stages: [], processors: {} })}
                  onresult={handleExecuteResult}
                />
              </section>
              <section class="results-section">
                <h2>Results</h2>
                <ResultsView {result} error={resultError} {originalData} />
              </section>
            </div>
          {/if}
        </div>
      </div>
    </div>
  {:else}
    <PipelineBuilder
      {pipeline}
      onValidate={handleValidate}
      onImport={() => showImport = true}
    />

    <section class="execution-section">
      <h2>Execute Pipeline</h2>
      <RunSelector
        pipelineYaml={yamlStringify(pipeline)}
        onresult={handleExecuteResult}
      />
    </section>
  {/if}

  {#if activeTab !== 'graph'}
    <section class="results-section">
      <h2>Results</h2>
      <ResultsView {result} error={resultError} {originalData} />
    </section>
  {/if}

  {#if showImport}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="modal-overlay" onclick={() => showImport = false}>
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="modal" onclick={(e) => e.stopPropagation()}>
        <ConfigImport onimport={handleImport} />
        <button class="btn-close" onclick={() => showImport = false}>Close</button>
      </div>
    </div>
  {/if}

  {#if editingStage !== null}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="modal-overlay" onclick={() => editingStage = null} onkeydown={(e) => e.key === 'Escape' && (editingStage = null)}>
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="modal" onclick={(e) => e.stopPropagation()}>
        <StageEditor
          stage={editingStage.stage}
          processor={editingStage.processor}
          onupdate={(updatedStage: Stage) => {
            pipeline.stages = pipeline.stages.map(s =>
              s.id === editingStage.stage.id ? updatedStage : s
            )
          }}
          onupdateProcessor={(config: ProcessorConfig) => {
            const ref = editingStage.stage.processor_ref
            pipeline.processors = { ...pipeline.processors, [ref]: config }
          }}
        />
        <button class="btn-close" onclick={() => editingStage = null}>Close</button>
      </div>
    </div>
  {/if}
</main>

<style>
  .app {
    font-family: var(--font-family, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
    padding: var(--space-md, 1rem);
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .execution-section, .results-section {
    margin-top: var(--space-xl, 2rem);
    padding-top: var(--space-md, 1rem);
    border-top: 1px solid var(--color-border, #334155);
  }
  .execution-section h2, .results-section h2 {
    font-size: var(--font-size-lg, 1.1rem);
    font-weight: 600;
    margin: 0 0 0.75rem 0;
    color: var(--color-text, #f1f5f9);
  }
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }
  .modal {
    background: var(--color-surface, #1e293b);
    border-radius: var(--radius-lg, 8px);
    padding: var(--space-lg, 1.5rem);
    max-width: 700px;
    width: 90%;
    max-height: 80vh;
    overflow-y: auto;
  }
  .btn-close {
    margin-top: 0.75rem;
    padding: 0.4rem 0.9rem;
    background: var(--color-surface-hover, #334155);
    color: var(--color-text, #f1f5f9);
    border: none;
    border-radius: var(--radius-md, 6px);
    cursor: pointer;
  }
  .top-nav {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    background: var(--color-surface, #1e293b);
    border-bottom: 1px solid var(--color-border, #334155);
    margin-bottom: var(--space-md, 1rem);
  }
  .app-title {
    font-weight: 600;
    font-size: var(--font-size-lg, 1.1rem);
    color: var(--color-text, #f1f5f9);
  }
  .nav-link {
    color: var(--color-primary, #3b82f6);
    text-decoration: none;
    font-size: var(--font-size-sm, 0.8rem);
  }
  .nav-link:hover {
    text-decoration: underline;
  }
  .tab-bar {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--color-border, #334155);
    margin-bottom: var(--space-md, 1rem);
    padding: 0 var(--space-md, 1rem);
  }
  .tab-button {
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    border: none;
    background: transparent;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    font-size: var(--font-size-sm, 0.8rem);
    border-bottom: 2px solid transparent;
    transition: all var(--transition-fast, 150ms ease);
  }
  .tab-button:hover {
    color: var(--color-text, #f1f5f9);
    background: var(--color-surface-hover, #334155);
  }
  .tab-button.active {
    color: var(--color-primary, #3b82f6);
    border-bottom-color: var(--color-primary, #3b82f6);
  }

  /* ─── Graph tab: full-height layout ─────────────────────────── */
  .graph-fullscreen {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: row;
    overflow: hidden;
  }

  .graph-main {
    display: flex;
    flex-direction: column;
    flex: 1;
    overflow: hidden;
  }

  .canvas-wrapper {
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .canvas-wrapper.maximized {
    flex: 0 0 40px;
  }

  .graph-bottom-panels {
    position: relative;
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--color-border, #334155);
    overflow: hidden;
    background: var(--color-bg, #0f172a);
  }
  .graph-bottom-panels.collapsed {
    flex: 0 0 auto;
    height: 36px !important;
  }

  .panel-bar {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 8px;
    background: var(--color-surface, #1e293b);
    border-bottom: 1px solid var(--color-border, #334155);
    min-height: 32px;
  }

  .panel-toggle {
    background: none;
    border: none;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    font-size: 0.85rem;
    padding: 4px 8px;
    border-radius: 4px;
  }
  .panel-toggle:hover {
    color: var(--color-text, #f1f5f9);
    background: var(--color-surface-hover, #334155);
  }

  .panel-max-btn {
    background: none;
    border: none;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 2px 6px;
    border-radius: 3px;
    line-height: 1;
  }
  .panel-max-btn:hover {
    color: var(--color-text, #f1f5f9);
    background: var(--color-surface-hover, #334155);
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
    background: var(--color-primary, #3b82f6);
    border-radius: 1px;
    opacity: 0.6;
  }

  .panel-content {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-md, 1rem);
  }

  .graph-bottom-panels .execution-section,
  .graph-bottom-panels .results-section {
    margin-top: var(--space-md, 1rem);
    padding-top: var(--space-md, 1rem);
    border-top: 1px solid var(--color-border, #334155);
  }

  .graph-bottom-panels .execution-section h2,
  .graph-bottom-panels .results-section h2 {
    font-size: var(--font-size-lg, 1.1rem);
    font-weight: 600;
    margin: 0 0 0.75rem 0;
    color: var(--color-text, #f1f5f9);
  }
</style>
