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

let expanded = $state(false)
let yamlText = $state('')

$effect(() => {
  try {
    yamlText = yaml.dump(processor, { indent: 2, noRefs: true, sortKeys: false })
  } catch {
    yamlText = JSON.stringify(processor, null, 2)
  }
})

let typeLabel = $derived(stage.type === 'transform' ? 'transform' : 'filter')

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

function handleSignalToggle(signal: string) {
  const signals = stage.signals ? [...stage.signals] : []
  const idx = signals.indexOf(signal)
  if (idx >= 0) {
    signals.splice(idx, 1)
  } else {
    signals.push(signal)
  }
  onupdate({ ...stage, signals })
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
  <button class="toggle-btn" onclick={() => expanded = !expanded}>
    <span class="chevron">{expanded ? '▼' : '▶'}</span>
    <span class="stage-id">{stage.id}</span>
    <span class="badge badge--{stage.type}">{typeLabel}</span>
    {#if stage.error_mode}
      <span class="badge badge--mode">{stage.error_mode}</span>
    {/if}
  </button>

  {#if expanded}
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
        <label>Signals</label>
        <div class="signal-options">
          <label class="signal-check">
            <input
              type="checkbox"
              checked={stage.signals?.includes('traces')}
              onchange={() => handleSignalToggle('traces')}
            />
            traces
          </label>
        </div>
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
    </div>
  {/if}
</div>

<style>
  .stage-editor {
    border: 1px solid #d1d5db;
    border-radius: 6px;
    background: #fff;
    margin-bottom: 0.5rem;
  }

  .toggle-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: none;
    background: none;
    cursor: pointer;
    font-size: 0.875rem;
    text-align: left;
  }

  .toggle-btn:hover {
    background: #f9fafb;
  }

  .chevron {
    font-size: 0.75rem;
    width: 1rem;
  }

  .stage-id {
    font-weight: 600;
    font-family: monospace;
  }

  .badge {
    font-size: 0.75rem;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-weight: 500;
  }

  .badge--transform {
    background: #dbeafe;
    color: #1d4ed8;
  }

  .badge--filter {
    background: #fef3c7;
    color: #b45309;
  }

  .badge--mode {
    background: #f3f4f6;
    color: #4b5563;
  }

  .stage-body {
    padding: 0.75rem;
    border-top: 1px solid #e5e7eb;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .field label {
    font-size: 0.75rem;
    font-weight: 600;
    color: #6b7280;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .field input[type="text"],
  .field select {
    padding: 0.375rem 0.5rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 0.875rem;
  }

  .toggle-type {
    padding: 0.25rem 0.5rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    background: #f9fafb;
    cursor: pointer;
    font-size: 0.75rem;
    align-self: flex-start;
  }

  .toggle-type:hover {
    background: #f3f4f6;
  }

  .current-type {
    font-size: 0.75rem;
    color: #6b7280;
    margin-left: 0.25rem;
  }

  .signal-options {
    display: flex;
    gap: 1rem;
  }

  .signal-check {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.875rem;
    cursor: pointer;
  }

  .yaml-editor {
    font-family: 'Fira Code', 'Cascadia Code', 'JetBrains Mono', 'Menlo', 'Consolas', monospace;
    font-size: 0.8125rem;
    padding: 0.5rem;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    resize: vertical;
    line-height: 1.4;
    tab-size: 2;
  }
</style>
