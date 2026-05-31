<script lang="ts">
  import CodeMirrorEditor from './CodeMirrorEditor.svelte'
  import { exportPipelinesToYaml } from './export-yaml'
  import type { PipelinesBySignal } from './types'
  import yaml from 'js-yaml'
  import { tick } from 'svelte'

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
  let yamlContent = $state('')
  let userIsEditing = $state(false)
  let awaitingOwnSync = $state(false)
  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let editorWidth = $state(400)
  let isResizing = $state(false)

  let syncStatus = $derived<'synced' | 'editing' | 'syncing'>(
    awaitingOwnSync ? 'syncing' : userIsEditing ? 'editing' : 'synced'
  )

  function handleEditorChange(_newValue: string) {
    userIsEditing = true
    yamlContent = _newValue  // immediate local feedback

    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(async () => {
      try {
        yaml.load(_newValue)  // pre-validate syntax before hitting the API
        const res = await fetch('/api/ottl/pipeline/from-yaml', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-yaml' },
          body: _newValue,
        })
        if (!res.ok) throw new Error(`from-yaml failed: ${res.status}`)
        const parsed = (await res.json()) as PipelinesBySignal
        const isSame = JSON.stringify(parsed) === JSON.stringify(pipelines)
        if (!isSame) {
          awaitingOwnSync = true
          onPipelinesChange?.(parsed)
          await tick()         // drain effect queue before re-enabling sync
          awaitingOwnSync = false
        }
        userIsEditing = false  // edit cycle complete
      } catch {
        // YAML parse error — linter handles visual feedback
        // Keep userIsEditing = true so controlled sync doesn't overwrite editor content
      }
    }, 500)
  }

  async function resyncFromGraph() {
    userIsEditing = false
    awaitingOwnSync = false
    yamlContent = await exportPipelinesToYaml(pipelines)
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

  // Sync pipelines to yamlContent when not being edited
  $effect(() => {
    // Track pipelines prop changes
    const _ = JSON.stringify(pipelines)
    if (!userIsEditing && !awaitingOwnSync) {
      exportPipelinesToYaml(pipelines).then(y => yamlContent = y)
    }
  })

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
      <div class="sync-indicator">
        <span class="sync-dot" class:synced={syncStatus === 'synced'} class:editing={syncStatus === 'editing'} class:syncing={syncStatus === 'syncing'}></span>
        {#if syncStatus === 'editing'}
          <button class="resync-btn" onclick={resyncFromGraph}>Resync</button>
        {/if}
      </div>
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
      Editor
    </button>
  {/if}
  <div class="resize-handle" onmousedown={startResize} title="Drag to resize"></div>
</div>

<style>
  .editor-panel {
    position: relative;
    display: flex;
    flex-direction: column;
    background: var(--color-surface, #1e293b);
    border-right: 1px solid var(--color-border, #334155);
    flex-shrink: 0;
    transition: width 0.2s ease;
  }

  .editor-panel.collapsed {
    width: 32px !important;
    min-width: 32px;
    overflow: hidden;
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    border-bottom: 1px solid var(--color-border, #334155);
    flex-shrink: 0;
  }

  .editor-title {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
  }

  .sync-indicator {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .sync-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #22c55e; /* green — synced */
    transition: background 0.2s;
  }
  .sync-dot.editing {
    background: #eab308; /* yellow — editing */
  }
  .sync-dot.syncing {
    background: #ef4444; /* red — syncing */
  }

  .resync-btn {
    font-size: 0.75rem;
    padding: 2px 8px;
    background: transparent;
    border: 1px solid var(--color-border, #334155);
    color: var(--color-text-secondary, #94a3b8);
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
  }
  .resync-btn:hover {
    background: var(--color-border, #334155);
    color: var(--color-text, #f1f5f9);
  }

  .resize-handle {
    position: absolute;
    right: 0;
    top: 0;
    bottom: 0;
    width: 6px;
    cursor: col-resize;
    background: transparent;
    z-index: 10;
    transition: background 0.15s;
  }
  .resize-handle:hover {
    background: var(--color-border, #334155);
  }
  .editor-panel.collapsed .resize-handle {
    display: none;
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
    width: 100%;
    min-height: 120px;
    padding: 12px 4px;
    letter-spacing: 0.05em;
  }

  .editor-content {
    flex: 1;
    overflow: hidden;
    padding: 8px;
  }
</style>
