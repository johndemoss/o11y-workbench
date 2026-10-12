<script lang="ts">
import type { Stage, ErrorMode, StageType, ProcessorConfig, SignalType } from './types'
import { extractOttlLines, buildProcessorFromOttlLines } from './snippets'
import { withCollectorVersion } from './collector-version'

let {
  stage,
  processor,
  signal = 'traces',
  pipelineYaml = '',
  onupdate,
  onupdateProcessor,
  onvalidate,
}: {
  stage: Stage
  processor: ProcessorConfig
  signal?: SignalType
  /** Full pipeline YAML for Collector validation when available. */
  pipelineYaml?: string
  onupdate: (s: Stage) => void
  onupdateProcessor?: (config: ProcessorConfig) => void
  onvalidate?: () => void
} = $props()

let statementText = $state('')
let diagnostic = $state('Not checked. OTTL validation uses the Collector backend.')
let diagnosticKind = $state<'idle' | 'ok' | 'error' | 'warn'>('idle')
let validating = $state(false)

$effect(() => {
  statementText = extractOttlLines(stage.type, processor as Record<string, unknown>)
})

let statementLabel = $derived(
  stage.type === 'filter' ? 'OTTL drop conditions' : 'OTTL statements',
)

let contextLabel = $derived(
  signal === 'logs' ? 'log' : signal === 'metrics' ? 'datapoint' : 'span',
)

let signalLabel = $derived(signal.charAt(0).toUpperCase() + signal.slice(1))

let statementHelp = $derived(
  stage.type === 'filter'
    ? `True condition drops the ${contextLabel}. Multiple lines are ORed.`
    : `Statements run in order in ${contextLabel} context.`,
)

let docsHref = $derived(
  stage.type === 'filter'
    ? 'https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/filterprocessor'
    : 'https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor',
)

function handleTypeToggle() {
  const newType: StageType = stage.type === 'transform' ? 'filter' : 'transform'
  const next = buildProcessorFromOttlLines(
    newType,
    signal,
    statementText,
    processor as Record<string, unknown>,
  )
  onupdate({ ...stage, type: newType })
  onupdateProcessor?.(next)
  diagnostic = 'Draft changed locally. Re-run Collector validation.'
  diagnosticKind = 'warn'
}

function handleErrorModeChange(e: Event) {
  const val = (e.target as HTMLSelectElement).value as ErrorMode | ''
  const mode = (val || 'ignore') as ErrorMode
  onupdate({ ...stage, error_mode: mode })
  const next = {
    ...buildProcessorFromOttlLines(
      stage.type === 'filter' ? 'filter' : 'transform',
      signal,
      statementText,
      processor as Record<string, unknown>,
    ),
    error_mode: mode,
  }
  onupdateProcessor?.(next)
}

function handleStatementInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  statementText = val
  const next = buildProcessorFromOttlLines(
    stage.type === 'filter' ? 'filter' : 'transform',
    signal,
    val,
    {
      ...(processor as Record<string, unknown>),
      error_mode: stage.error_mode ?? 'ignore',
    },
  )
  onupdateProcessor?.(next)
  diagnostic = 'Draft changed locally. Check with Collector to validate.'
  diagnosticKind = 'warn'
}

async function handleCheck() {
  if (onvalidate) {
    onvalidate()
    return
  }
  if (!statementText.trim()) {
    diagnostic = 'Statement required before Collector validation.'
    diagnosticKind = 'error'
    return
  }
  if (!pipelineYaml.trim()) {
    diagnostic = 'Pipeline YAML unavailable. Collector validation cannot run yet.'
    diagnosticKind = 'error'
    return
  }
  validating = true
  diagnostic = 'Validating with Collector…'
  diagnosticKind = 'idle'
  try {
    const res = await fetch(withCollectorVersion('/api/ottl/pipeline/validate'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-yaml' },
      body: pipelineYaml,
    })
    const data = await res.json()
    if (data.valid) {
      diagnostic = 'Collector validation passed for the current pipeline YAML.'
      diagnosticKind = 'ok'
    } else {
      diagnostic = 'Collector validation errors:\n' + (data.errors || []).join('\n')
      diagnosticKind = 'error'
    }
  } catch (e) {
    diagnostic = 'Collector validation failed: ' + (e instanceof Error ? e.message : String(e))
    diagnosticKind = 'error'
  } finally {
    validating = false
  }
}
</script>

<div class="stage-editor" data-od-id="stage-editor-body">
  <h3 id="stage-name" class="stage-name">{stage.processor_ref || stage.id}</h3>
  <p class="tiny muted" id="stage-context">{signalLabel} / {contextLabel} context</p>

  <div class="meta-row">
    <label for="error-mode">Error mode</label>
    <button
      id="stage-type-toggle"
      type="button"
      class="ghost type-switch"
      onclick={handleTypeToggle}
      title="Switch stage type"
    >{stage.type} ⇄</button>
  </div>
  <select
    id="error-mode"
    data-od-id="stage-error-mode"
    value={stage.error_mode ?? 'ignore'}
    onchange={handleErrorModeChange}
  >
    <option value="ignore">ignore</option>
    <option value="silent">silent</option>
    <option value="propagate">propagate</option>
  </select>

  <label for="statement" id="statement-label">{statementLabel}</label>
  <p id="statement-help" class="tiny muted help" data-od-id="ottl-context-help">{statementHelp}</p>
  <textarea
    id="statement"
    class="yaml-editor"
    rows="5"
    spellcheck="false"
    aria-describedby="statement-help statement-format"
    data-od-id="stage-statement"
    value={statementText}
    oninput={handleStatementInput}
    placeholder={stage.type === 'filter'
      ? 'kind == SPAN_KIND_INTERNAL'
      : 'set(attributes["env"], "prod")'}
  ></textarea>
  <p id="statement-format" class="tiny muted format-note">
    Native Collector OTTL · one expression per line.
  </p>
  <a
    id="ottl-docs"
    class="docs-link"
    href={docsHref}
    target="_blank"
    rel="noopener noreferrer"
    data-od-id="ottl-reference"
  >Collector OTTL reference ↗</a>

  <div class="toolbar">
    <button
      type="button"
      id="check"
      data-od-id="check-stage-draft"
      onclick={handleCheck}
      disabled={validating}
    >{validating ? 'Checking…' : 'Check with Collector'}</button>
  </div>

  <div
    class="status"
    class:ok={diagnosticKind === 'ok'}
    class:error={diagnosticKind === 'error'}
    class:warn={diagnosticKind === 'warn'}
    id="diagnostic"
    role="status"
    aria-live="polite"
  >{diagnostic}</div>
</div>

<style>
  .stage-editor {
    border: 0;
    background: transparent;
    margin: 0;
  }

  .stage-name {
    font: 14px var(--font-mono);
    margin: 0 0 2px;
    overflow-wrap: anywhere;
  }

  .tiny { font-size: 12px; }
  .muted { color: var(--color-text-secondary); }

  #stage-context {
    margin: 0 0 10px;
  }

  label {
    display: block;
    margin-top: 10px;
    margin-bottom: 4px;
    font-size: 12px;
    font-weight: 500;
    color: var(--color-text-secondary);
  }

  .meta-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }

  .meta-row label {
    margin-top: 0;
  }

  .type-switch {
    min-height: 28px;
    padding: 2px 8px;
    font: 11px var(--font-mono);
    color: var(--color-text-secondary);
  }

  .help {
    margin: 0 0 6px;
  }

  .format-note {
    margin: 6px 0 0;
  }

  .yaml-editor {
    font-family: var(--font-mono);
    font-size: 12px;
    min-height: 108px;
    line-height: 1.45;
    background: var(--color-bg);
  }

  .docs-link {
    display: inline-flex;
    align-items: center;
    min-height: 28px;
    margin-top: 2px;
    font-size: 12px;
    color: var(--text-link);
  }

  .toolbar {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 10px;
  }

  .toolbar button {
    min-height: 36px;
    font-size: 12px;
  }

  .status {
    padding: 10px;
    margin-top: 10px;
    font-size: 12px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    white-space: pre-wrap;
  }

  .status.warn {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }

  .status.error {
    background: var(--color-error-bg);
    color: var(--color-error-text);
  }

  .status.ok {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }
</style>
