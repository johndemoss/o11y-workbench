<script lang="ts">
import type { Stage, ErrorMode, StageType, ProcessorConfig } from './types'
import yaml from 'js-yaml'

let {
  stage,
  processor,
  onupdate,
  onupdateProcessor
}: {
  stage: Stage
  processor: ProcessorConfig
  onupdate: (s: Stage) => void
  onupdateProcessor?: (config: ProcessorConfig) => void
} = $props()

let yamlText = $state('')

$effect(() => {
  try {
    yamlText = yaml.dump(processor, { indent: 2, noRefs: true, sortKeys: false })
  } catch {
    yamlText = JSON.stringify(processor, null, 2)
  }
})

let typeLabel = $derived(stage.type === 'transform' ? 'transform' : 'filter')

let pipelineYamlPreview = $derived.by(() => {
  const lines: string[] = []
  lines.push('stages:')
  lines.push(`  - id: ${stage.id}`)
  lines.push(`    type: ${stage.type}`)
  lines.push(`    processor_ref: ${stage.processor_ref}`)
  if (stage.error_mode) lines.push(`    error_mode: ${stage.error_mode}`)
  lines.push('processors:')
  lines.push(`  ${stage.processor_ref}:`)
  try {
    const yamlStr = yaml.dump(processor, { indent: 2, noRefs: true, sortKeys: false })
    const procLines = yamlStr.trimEnd().split('\n')
    for (const l of procLines) {
      lines.push(`    ${l}`)
    }
  } catch {
    lines.push('    {}')
  }
  return lines.join('\n')
})

function handleIdChange(e: Event) {
  const target = e.target as HTMLInputElement
  onupdate({ ...stage, id: target.value })
}

function handleTypeToggle() {
  const newType: StageType = stage.type === 'transform' ? 'filter' : 'transform'
  onupdate({ ...stage, type: newType })
}

function handleErrorModeChange(e: Event) {
  const val = (e.target as HTMLSelectElement).value
  onupdate({ ...stage, error_mode: (val || undefined) as ErrorMode | undefined })
}

function handleYamlEdit(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value
  yamlText = val
  try {
    const parsed = yaml.load(val)
    if (parsed && typeof parsed === 'object') {
      onupdateProcessor?.(parsed as ProcessorConfig)
    }
  } catch {
    // Invalid YAML — don't propagate until valid
  }
}
</script>

<div class="stage-editor">
  <div class="stage-header">
    <span class="stage-id">{stage.id}</span>
    <span class="badge badge--{stage.type}">{typeLabel}</span>
    {#if stage.error_mode}
      <span class="badge badge--mode">{stage.error_mode}</span>
    {/if}
  </div>
  <div class="stage-body">
    <div class="field">
      <label for="stage-id">ID</label>
      <input
        id="stage-id"
        type="text"
        value={stage.id}
        oninput={handleIdChange}
      />
    </div>

    <div class="field">
      <label>Type</label>
      <button class="toggle-type" onclick={handleTypeToggle}>
        {stage.type === 'transform' ? 'filter' : 'transform'}
      </button>
      <span class="current-type">{stage.type}</span>
    </div>

    <div class="field">
      <label for="error-mode">Error Mode</label>
      <select
        id="error-mode"
        value={stage.error_mode ?? ''}
        onchange={handleErrorModeChange}
      >
        <option value="">default</option>
        <option value="ignore">ignore</option>
        <option value="silent">silent</option>
        <option value="propagate">propagate</option>
      </select>
    </div>

    <div class="field">
      <label for="yaml-editor">Processor Config</label>
      <textarea
        id="yaml-editor"
        class="yaml-editor"
        rows="6"
        value={yamlText}
        oninput={handleYamlEdit}
      ></textarea>
    </div>

    <div class="field">
      <label>Pipeline YAML Preview</label>
      <pre class="yaml-preview">{pipelineYamlPreview}</pre>
    </div>
  </div>
</div>

<style>
  .stage-editor {
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-md, 6px);
    background: var(--color-surface, #1e293b);
    margin-bottom: var(--space-sm, 0.5rem);
  }

  .stage-header {
    display: flex;
    align-items: center;
    gap: var(--space-sm, 0.5rem);
    padding: var(--space-sm, 0.5rem) 0.75rem;
    font-size: 0.875rem;
  }

  .stage-id {
    font-weight: 600;
    font-family: var(--font-mono, monospace);
  }

  .badge {
    font-size: 0.75rem;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-weight: 500;
  }

  .badge--transform {
    background: var(--color-primary, #3b82f6);
    color: var(--color-text, #f1f5f9);
  }

  .badge--filter {
    background: var(--color-warning, #f59e0b);
    color: var(--color-text, #f1f5f9);
  }

  .badge--mode {
    background: var(--color-surface-hover, #334155);
    color: var(--color-text-secondary, #94a3b8);
  }

  .stage-body {
    padding: 0.75rem;
    border-top: 1px solid var(--color-border, #334155);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-xs, 0.25rem);
  }

  .field label {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--color-text-secondary, #94a3b8);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .field input[type="text"],
  .field select {
    padding: 0.375rem var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    font-size: 0.875rem;
  }

  .toggle-type {
    padding: var(--space-xs, 0.25rem) var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface, #1e293b);
    cursor: pointer;
    font-size: 0.75rem;
    align-self: flex-start;
  }

  .toggle-type:hover {
    background: var(--color-surface-hover, #334155);
  }

  .current-type {
    font-size: 0.75rem;
    color: var(--color-text-secondary, #94a3b8);
    margin-left: var(--space-xs, 0.25rem);
  }

  .yaml-editor {
    font-family: var(--font-mono, monospace);
    font-size: 0.8125rem;
    padding: var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    resize: vertical;
    line-height: 1.4;
    tab-size: 2;
  }

  .yaml-preview {
    font-family: var(--font-mono, monospace);
    font-size: 0.8125rem;
    padding: var(--space-sm, 0.5rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface, #1e293b);
    line-height: 1.4;
    tab-size: 2;
    overflow-x: auto;
    white-space: pre;
    margin: 0;
  }
</style>
