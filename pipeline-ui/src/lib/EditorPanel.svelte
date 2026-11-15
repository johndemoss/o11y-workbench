<script lang="ts">
  import CodeMirrorEditor from './CodeMirrorEditor.svelte'
  import type { PipelinesBySignal } from './types'
  import { load } from 'js-yaml'
  import { onDestroy, tick } from 'svelte'

  const WIDTH_DEFAULT = 360
  /** Resize may widen the panel, but never shrink below the default. */
  const WIDTH_MIN = WIDTH_DEFAULT
  const WIDTH_MAX = 800

  let {
    pipelines,
    yaml: yamlProp = '',
    graphYaml = '',
    onPipelinesChange,
    onDraftChange,
    selectedNodeId,
    open = $bindable(true),
    onOpenChange,
  }: {
    pipelines: PipelinesBySignal
    /** App-owned pipeline YAML (Model 1 — no EditorPanel POST /export). */
    yaml?: string
    graphYaml?: string
    onPipelinesChange?: (p: PipelinesBySignal) => void
    onDraftChange?: (text: string | null, ready: boolean, error?: string) => void
    selectedNodeId?: string | null
    open?: boolean
    onOpenChange?: (open: boolean) => void
  } = $props()

  let editorRef: CodeMirrorEditor | undefined = $state()
  let yamlContent = $state('')
  let userIsEditing = $state(false)
  let awaitingOwnSync = $state(false)
  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let generation = 0
  let syncError = $state('')
  onDestroy(() => {
    generation++
    if (debounceTimer) clearTimeout(debounceTimer)
  })
  let editorWidth = $state(WIDTH_DEFAULT)
  let isResizing = $state(false)

  let syncStatus = $derived<'synced' | 'editing' | 'syncing'>(
    awaitingOwnSync ? 'syncing' : userIsEditing ? 'editing' : 'synced'
  )

  function setOpen(next: boolean) {
    open = next
    onOpenChange?.(next)
  }

  function handleEditorChange(value: string) {
    const current = ++generation
    userIsEditing = true
    awaitingOwnSync = false
    yamlContent = value
    syncError = ''
    onDraftChange?.(value, false)
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(async () => {
      try {
        load(value)
        const res = await fetch('/api/ottl/pipeline/from-yaml', {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-yaml' },
          body: value,
        })
        if (!res.ok) {
          const message = await res.text()
          throw new Error(message || `Conversion failed: ${res.status}`)
        }
        const parsed = (await res.json()) as PipelinesBySignal
        if (current !== generation) return
        awaitingOwnSync = true
        onPipelinesChange?.(parsed)
        await tick()
        if (current !== generation) return
        awaitingOwnSync = false
        userIsEditing = false
        onDraftChange?.(value, true)
      } catch (e) {
        if (current !== generation) return
        awaitingOwnSync = false
        syncError = e instanceof Error ? e.message : String(e)
        onDraftChange?.(value, false, syncError)
      }
    }, 500)
  }

  function resyncFromGraph() {
    generation++
    if (debounceTimer) clearTimeout(debounceTimer)
    userIsEditing = false
    awaitingOwnSync = false
    syncError = ''
    yamlContent = graphYaml
    onDraftChange?.(null, true)
  }

  function startResize(e: MouseEvent) {
    e.preventDefault()
    isResizing = true
    const startX = e.clientX
    const startWidth = editorWidth

    function onMouseMove(e: MouseEvent) {
      editorWidth = Math.max(WIDTH_MIN, Math.min(WIDTH_MAX, startWidth + (e.clientX - startX)))
    }
    function onMouseUp() {
      isResizing = false
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }

  $effect(() => {
    const y = yamlProp
    if (!userIsEditing && !awaitingOwnSync) {
      yamlContent = y
    }
  })

  $effect(() => {
    if (!selectedNodeId || !editorRef) return
    const yamlText = yamlContent
    const lines = yamlText.split('\n')
    const nodeId = selectedNodeId
    const lineIndex = lines.findIndex((l) => l.includes(nodeId))
    if (lineIndex >= 0) {
      editorRef.scrollToLine(lineIndex + 1)
    }
  })
</script>

{#if open}
  <aside
    class="editor-panel panel yaml"
    class:resizing={isResizing}
    style="width: {editorWidth}px"
    id="yaml-panel"
    data-od-id="pipeline-yaml"
  >
    <div class="panel-head editor-header">
      <h2 class="editor-title">Pipeline YAML</h2>
      <div class="header-actions">
        <div class="sync-indicator">
          <span
            class="sync-dot"
            class:synced={syncStatus === 'synced'}
            class:editing={syncStatus === 'editing'}
            class:syncing={syncStatus === 'syncing'}
          ></span>
          {#if syncStatus === 'editing'}
            <button type="button" class="resync-btn" onclick={resyncFromGraph}>Resync</button>
          {/if}
          {#if syncError}<span role="alert">{syncError}</span>{/if}
        </div>
        <button
          type="button"
          class="ghost toggle-btn"
          disabled={userIsEditing || awaitingOwnSync}
          onclick={() => setOpen(false)}
          aria-label="Hide pipeline YAML"
          aria-expanded="true"
          id="hide-yaml"
          title="Hide YAML"
        >‹</button>
      </div>
    </div>
    <div class="editor-content">
      <CodeMirrorEditor
        bind:this={editorRef}
        value={yamlContent}
        onchange={handleEditorChange}
      />
    </div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="resize-handle" onmousedown={startResize} title="Drag to resize"></div>
  </aside>
{/if}

<style>
  .editor-panel {
    position: relative;
    display: flex;
    flex-direction: column;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    flex-shrink: 0;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  .editor-panel.resizing {
    transition: none;
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border);
    min-height: 58px;
    flex-shrink: 0;
  }

  .editor-title {
    font-size: 14px;
    font-weight: 600;
    margin: 0;
    color: var(--color-text);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
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
    background: var(--color-success);
    transition: background var(--transition-fast);
  }
  .sync-dot.editing { background: var(--color-warning); }
  .sync-dot.syncing { background: var(--color-error); }

  .resync-btn {
    font-size: 12px;
    padding: 8px;
    min-height: 36px;
    background: transparent;
    border: 1px solid var(--color-border);
    color: var(--color-text-secondary);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .resync-btn:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  .toggle-btn {
    min-height: 36px;
    min-width: 36px;
    padding: 0;
    background: transparent;
    border: 1px solid var(--color-border);
    color: var(--color-text);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-size: 16px;
    line-height: 1;
  }

  .toggle-btn:hover {
    background: var(--color-surface-hover);
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
  }
  .resize-handle:hover {
    background: var(--color-border);
  }

  .editor-content {
    flex: 1;
    overflow: hidden;
    padding: 0;
    min-height: 0;
    background: var(--color-bg);
  }
</style>
