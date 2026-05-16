<script lang="ts">
  import type { Pipeline } from './types'

  let { onimport }: {
    onimport?: (pipeline: Pipeline) => void
  } = $props()

  let yamlText = $state(`receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/add_env:
    error_mode: ignore
    trace_statements:
      - context: span
        statements:
          - set(attributes["env"], "prod")
  filter/drop_internal:
    error_mode: propagate
    traces:
      span:
        - kind != SPAN_KIND_INTERNAL
exporters:
  debug:
service:
  pipelines:
    traces:
      processors: [transform/add_env, filter/drop_internal]`)

  let loading = $state(false)
  let error = $state('')

  let fileInput: HTMLInputElement | undefined = $state()

  function handleDragOver(e: DragEvent) {
    e.preventDefault()
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    const file = e.dataTransfer?.files?.[0]
    if (file) readFile(file)
  }

  function handleFileSelect() {
    fileInput?.click()
  }

  function onFilePicked(e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    if (file) readFile(file)
  }

  function readFile(file: File) {
    const reader = new FileReader()
    reader.onload = () => {
      yamlText = reader.result as string
    }
    reader.readAsText(file)
  }

  async function importConfig(): Promise<void> {
    if (!yamlText.trim()) return
    loading = true
    error = ''
    try {
      const res = await fetch('/api/ottl/pipeline/import-config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-yaml' },
        body: yamlText
      })
      const data = await res.json()
      if (!res.ok) {
        error = data.error || 'Import failed'
      } else {
        onimport?.(data)
      }
    } catch (e: any) {
      error = e.message || 'Network error'
    } finally {
      loading = false
    }
  }
</script>

<div class="config-import">
  <div class="header">
    <h2>Import Collector Config</h2>
    <button class="close-btn" onclick={() => onimport?.(undefined as any)} aria-label="Close">✕</button>
  </div>

  <div
    class="drop-zone"
    ondragover={handleDragOver}
    ondrop={handleDrop}
    role="button"
    tabindex="0"
    onclick={handleFileSelect}
    onkeydown={(e) => e.key === 'Enter' && handleFileSelect()}
  >
    <p>Drop a YAML file here, or click to browse</p>
    <input
      type="file"
      accept=".yaml,.yml"
      bind:this={fileInput}
      onchange={onFilePicked}
      class="hidden"
    />
  </div>

  <textarea
    bind:value={yamlText}
    rows="10"
    class="yaml-input"
    placeholder="Paste Collector YAML config here..."
  ></textarea>

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <div class="actions">
    <button class="cancel-btn" onclick={() => onimport?.(undefined as any)}>Cancel</button>
    <button class="import-btn" onclick={importConfig} disabled={loading}>
      {#if loading}
        Importing...
      {:else}
        Import
      {/if}
    </button>
  </div>
</div>

<style>
  .config-import {
    border: 1px solid #d1d5db;
    border-radius: 0.5rem;
    padding: 1.25rem;
    background: #fff;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .header h2 {
    margin: 0;
    font-size: 1.125rem;
    font-weight: 600;
  }

  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 1rem;
    color: #6b7280;
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
  }

  .close-btn:hover {
    background: #f3f4f6;
  }

  .drop-zone {
    border: 2px dashed #d1d5db;
    border-radius: 0.375rem;
    padding: 1rem;
    text-align: center;
    cursor: pointer;
    color: #6b7280;
    font-size: 0.875rem;
    transition: border-color 0.15s;
  }

  .drop-zone:hover {
    border-color: #3b82f6;
  }

  .hidden {
    display: none;
  }

  .yaml-input {
    width: 100%;
    font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 0.8125rem;
    padding: 0.625rem;
    border: 1px solid #d1d5db;
    border-radius: 0.375rem;
    resize: vertical;
    box-sizing: border-box;
  }

  .yaml-input:focus {
    outline: none;
    border-color: #3b82f6;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
  }

  .error {
    color: #dc2626;
    font-size: 0.875rem;
    margin: 0;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
  }

  .import-btn {
    padding: 0.5rem 1rem;
    background: #3b82f6;
    color: #fff;
    border: none;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    cursor: pointer;
  }

  .import-btn:hover:not(:disabled) {
    background: #2563eb;
  }

  .import-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .cancel-btn {
    padding: 0.5rem 1rem;
    background: #f3f4f6;
    color: #374151;
    border: 1px solid #d1d5db;
    border-radius: 0.375rem;
    font-size: 0.875rem;
    cursor: pointer;
  }

  .cancel-btn:hover {
    background: #e5e7eb;
  }
</style>
