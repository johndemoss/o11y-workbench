<script lang="ts">
  import CodeMirrorEditor from './CodeMirrorEditor.svelte'
  import { exportPipelinesToYaml } from './export-yaml'
  import type { PipelinesBySignal } from './types'

  let {
    pipelines,
    onPipelinesChange,
    selectedNodeId,
  }: {
    pipelines: PipelinesBySignal
    onPipelinesChange?: (p: PipelinesBySignal) => void
    selectedNodeId?: string | null
  } = $props()

  let open = $state(true)
  let editorRef: CodeMirrorEditor | undefined = $state()
  let yamlContent = $derived(exportPipelinesToYaml(pipelines))
  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let editorWidth = $state(400)
  let isResizing = $state(false)

  function handleEditorChange(_newValue: string) {
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      import('./export-yaml').then(({ importYamlToPipelines }) => {
        const parsed = importYamlToPipelines(_newValue)
        onPipelinesChange?.(parsed)
      }).catch(() => {
        onPipelinesChange?.(pipelines)
      })
    }, 500)
  }

  function startResize(e: MouseEvent) {
    e.preventDefault()
    isResizing = true
    const startX = e.clientX
    const startWidth = editorWidth

    function onMouseMove(e: MouseEvent) {
      editorWidth = Math.max(200, Math.min(800, startWidth + (e.clientX - startX)))
    }
    function onMouseUp() {
      isResizing = false
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }

  // Scroll editor when selectedNodeId changes
  $effect(() => {
    if (!selectedNodeId || !editorRef) return
    const yaml = yamlContent
    const lines = yaml.split('\n')
    // Find the line that contains this node's processor_ref
    const nodeId = selectedNodeId
    const lineIndex = lines.findIndex(l => l.includes(nodeId))
    if (lineIndex >= 0) {
      editorRef.scrollToLine(lineIndex + 1)
    }
  })
</script>

<div class="editor-panel" class:collapsed={!open} style="width: {editorWidth}px">
  {#if open}
    <div class="editor-header">
      <span class="editor-title">Pipeline YAML</span>
      <div class="resize-handle" onmousedown={startResize} title="Drag to resize"></div>
      <button class="toggle-btn" onclick={() => open = false} aria-label="Close editor">◀</button>
    </div>
    <div class="editor-content">
      <CodeMirrorEditor
        bind:this={editorRef}
        value={yamlContent}
        onchange={handleEditorChange}
      />
    </div>
  {:else}
    <button class="toggle-tab" onclick={() => open = true} aria-label="Open editor">
      Editor ▶
    </button>
  {/if}
</div>

<style>
  .editor-panel {
    display: flex;
    flex-direction: column;
    background: var(--color-surface, #1e293b);
    border-right: 1px solid var(--color-border, #334155);
    flex-shrink: 0;
    transition: width 0.2s ease;
  }

  .editor-panel.collapsed {
    width: auto;
  }

  .editor-header {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    border-bottom: 1px solid var(--color-border, #334155);
    flex-shrink: 0;
  }

  .editor-title {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
  }

  .resize-handle {
    cursor: col-resize;
    height: 16px;
    width: 4px;
    background: transparent;
    margin: 0 4px;
    margin-left: auto;
    border-radius: 2px;
    flex-shrink: 0;
  }
  .resize-handle:hover {
    background: var(--color-border, #334155);
  }

  .toggle-btn, .toggle-tab {
    background: none;
    border: none;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    padding: 4px 8px;
    font-size: 0.85rem;
  }

  .toggle-btn:hover, .toggle-tab:hover {
    color: var(--color-text, #f1f5f9);
  }

  .toggle-tab {
    writing-mode: vertical-lr;
    padding: 12px 6px;
    min-height: 100px;
    letter-spacing: 0.05em;
  }

  .editor-content {
    flex: 1;
    overflow: hidden;
    padding: 8px;
  }
</style>
