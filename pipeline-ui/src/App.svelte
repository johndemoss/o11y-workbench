<script lang="ts">
  import type { Pipeline, PipelineResult, Stage, ProcessorConfig } from './lib/types'
  import PipelineBuilder from './lib/PipelineBuilder.svelte'
  import FlowBuilder from './lib/FlowBuilder.svelte'
  import StageEditor from './lib/StageEditor.svelte'
  import RunSelector from './lib/RunSelector.svelte'
  import ResultsView from './lib/ResultsView.svelte'
  import ConfigImport from './lib/ConfigImport.svelte'
  import QuickTab from './lib/QuickTab.svelte'
  import yaml from 'js-yaml'

  let pipeline: Pipeline = $state({ stages: [], processors: {} })
  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let originalData: unknown = $state(null)
  let showImport: boolean = $state(false)
  let pipelineYaml: string = $state('')
  let activeTab: 'quick' | 'pipeline' | 'graph' = $state('pipeline')
  let editingStage: { stage: Stage; processor: ProcessorConfig } | null = $state(null)

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

  function handleImport(p: Pipeline): void {
    pipeline = p
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
    <FlowBuilder
      {pipeline}
      {result}
      onValidate={handleValidate}
      onImport={() => showImport = true}
      onNodeEdit={handleNodeEdit}
      onResultClear={() => { result = null; resultError = undefined }}
    />

    <section class="execution-section">
      <h2>Execute Pipeline</h2>
      <RunSelector
        pipelineYaml={yamlStringify(pipeline)}
        onresult={handleExecuteResult}
      />
    </section>
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

  <section class="results-section">
    <h2>Results</h2>
    <ResultsView {result} error={resultError} {originalData} />
  </section>

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
    max-width: 1100px;
    margin: 0 auto;
    padding: var(--space-md, 1rem);
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
</style>
