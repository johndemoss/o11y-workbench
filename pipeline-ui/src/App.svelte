<script lang="ts">
  import type { Pipeline, PipelineResult } from './lib/types'
  import PipelineBuilder from './lib/PipelineBuilder.svelte'
  import RunSelector from './lib/RunSelector.svelte'
  import ResultsView from './lib/ResultsView.svelte'
  import ConfigImport from './lib/ConfigImport.svelte'

  let pipeline: Pipeline = $state({ stages: [], processors: {} })
  let result: PipelineResult | null = $state(null)
  let resultError: string | undefined = $state(undefined)
  let showImport: boolean = $state(false)
  let pipelineYaml: string = $state('')

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

  function handleExecute(p: Pipeline): void {
    pipelineYaml = yamlStringify(p)
  }

  function handleExecuteResult(r: PipelineResult | null, err?: string): void {
    result = r
    resultError = err
  }

  function handleImport(p: Pipeline): void {
    pipeline = p
    showImport = false
  }

  function yamlStringify(p: Pipeline): string {
    let yaml = 'stages:\n'
    for (const s of p.stages) {
      yaml += `  - id: ${s.id}\n`
      yaml += `    type: ${s.type}\n`
      yaml += `    processor_ref: ${s.processor_ref}\n`
      if (s.error_mode) yaml += `    error_mode: ${s.error_mode}\n`
    }
    yaml += 'processors:\n'
    for (const [key, val] of Object.entries(p.processors)) {
      yaml += `  ${key}: {}\n`
    }
    return yaml
  }
</script>

<main class="app">
  <PipelineBuilder
    {pipeline}
    onValidate={handleValidate}
    onExecute={handleExecute}
    onImport={() => showImport = true}
  />

  <section class="execution-section">
    <h2>Execute Pipeline</h2>
    <RunSelector
      pipelineYaml={yamlStringify(pipeline)}
      onresult={handleExecuteResult}
    />
  </section>

  <section class="results-section">
    <h2>Results</h2>
    <ResultsView {result} error={resultError} />
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
</main>

<style>
  .app {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    max-width: 1100px;
    margin: 0 auto;
    padding: 1rem;
  }
  .execution-section, .results-section {
    margin-top: 2rem;
    padding-top: 1rem;
    border-top: 1px solid #e5e7eb;
  }
  .execution-section h2, .results-section h2 {
    font-size: 1.1rem;
    font-weight: 600;
    margin: 0 0 0.75rem 0;
    color: #374151;
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
    background: white;
    border-radius: 0.5rem;
    padding: 1.5rem;
    max-width: 700px;
    width: 90%;
    max-height: 80vh;
    overflow-y: auto;
  }
  .btn-close {
    margin-top: 0.75rem;
    padding: 0.4rem 0.9rem;
    background: #6b7280;
    color: white;
    border: none;
    border-radius: 0.375rem;
    cursor: pointer;
  }
</style>
