<script lang="ts">
  import type { Pipeline, Stage, StageType, ProcessorConfig } from './types'
  import StageEditor from './StageEditor.svelte'

  let { pipeline = { stages: [], processors: {} }, onValidate, onImport }: {
    pipeline: Pipeline
    onValidate?: (p: Pipeline) => void
    onImport?: () => void
  } = $props()

  let dragIdx: number | null = $state(null)
  let dropIdx: number | null = $state(null)
  let deleteConfirmIdx: number | null = $state(null)

  function addStage(): void {
    const id = `stage-${pipeline.stages.length + 1}`
    const procName = `proc-${id}`
    pipeline.stages = [...pipeline.stages, {
      id,
      type: 'transform' as StageType,
      processor_ref: procName,
    }]
    pipeline.processors = { ...pipeline.processors, [procName]: {} }
  }

  function removeStage(idx: number): void {
    const removed = pipeline.stages[idx]
    const { [removed.processor_ref]: _, ...rest } = pipeline.processors
    pipeline.stages = pipeline.stages.filter((_, i) => i !== idx)
    pipeline.processors = rest
    deleteConfirmIdx = null
  }

  function updateStage(idx: number, stage: Stage): void {
    pipeline.stages = pipeline.stages.map((s, i) => i === idx ? stage : s)
  }

  function updateProcessor(idx: number, config: ProcessorConfig): void {
    const stage = pipeline.stages[idx]
    pipeline.processors = { ...pipeline.processors, [stage.processor_ref]: config }
  }

  function moveUp(idx: number): void {
    if (idx === 0) return
    const stages = [...pipeline.stages]
    const tmp = stages[idx]
    stages[idx] = stages[idx - 1]
    stages[idx - 1] = tmp
    pipeline.stages = stages
  }

  function moveDown(idx: number): void {
    if (idx === pipeline.stages.length - 1) return
    const stages = [...pipeline.stages]
    const tmp = stages[idx]
    stages[idx] = stages[idx + 1]
    stages[idx + 1] = tmp
    pipeline.stages = stages
  }

  function onDragStart(e: DragEvent, idx: number): void {
    dragIdx = idx
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(e: DragEvent, idx: number): void {
    e.preventDefault()
    dropIdx = idx
  }

  function onDrop(): void {
    if (dragIdx === null || dropIdx === null || dragIdx === dropIdx) {
      dragIdx = null; dropIdx = null; return
    }
    const stages = [...pipeline.stages]
    const [moved] = stages.splice(dragIdx, 1)
    stages.splice(dropIdx, 0, moved)
    pipeline.stages = stages
    dragIdx = null; dropIdx = null
  }
</script>

<div class="pipeline-builder">
  <header class="builder-header">
    <h1>Pipeline Prototyper</h1>
    <span class="badge">Experimental</span>
    <div class="actions">
      <button onclick={() => onValidate?.(pipeline)} class="btn btn-primary">Validate</button>
      <button onclick={() => onImport?.()} class="btn btn-secondary">Import Config</button>
    </div>
  </header>

  <div class="stage-list">
    {#if pipeline.stages.length === 0}
      <div class="empty-state">
        <p>No stages defined. Click "Add Stage" to begin.</p>
      </div>
    {:else}
      {#each pipeline.stages as stage, idx (stage.id)}
        <div
          class="stage-card-wrapper"
          draggable="true"
          ondragstart={(e) => onDragStart(e, idx)}
          ondragover={(e) => onDragOver(e, idx)}
          ondrop={onDrop}
          ondragend={() => { dragIdx = null; dropIdx = null }}
          class:drag-over={dropIdx === idx && dragIdx !== idx}
        >
          <div class="drag-handle">⠿</div>
          <div class="move-buttons">
            <button onclick={() => moveUp(idx)} disabled={idx === 0} class="move-btn" title="Move up" aria-label="Move stage up">↑</button>
            <button onclick={() => moveDown(idx)} disabled={idx === pipeline.stages.length - 1} class="move-btn" title="Move down" aria-label="Move stage down">↓</button>
          </div>
          <StageEditor {stage} processor={pipeline.processors[stage.processor_ref] || {}} onupdate={(s) => updateStage(idx, s)} onupdateProcessor={(c) => updateProcessor(idx, c)} />
          {#if deleteConfirmIdx === idx}
            <div class="delete-confirm">
              <span>Delete?</span>
              <button onclick={() => removeStage(idx)} class="btn-danger-small">Yes</button>
              <button onclick={() => { deleteConfirmIdx = null }} class="btn-small">No</button>
            </div>
          {:else}
            <button onclick={() => { deleteConfirmIdx = idx }} class="btn-delete" title="Delete stage">✕</button>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  <button onclick={addStage} class="btn btn-add-stage">+ Add Stage</button>
</div>

<style>
  .pipeline-builder { padding: var(--space-md, 1rem); max-width: 900px; margin: 0 auto; }
  .builder-header { display: flex; align-items: center; gap: var(--space-md, 1rem); margin-bottom: var(--space-lg, 1.5rem); flex-wrap: wrap; }
  .builder-header h1 { font-size: 1.5rem; font-weight: 600; margin: 0; }
  .badge { background: var(--color-accent, #8b5cf6); color: var(--color-text, #f1f5f9); padding: 0.15rem 0.5rem; border-radius: 999px; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; }
  .actions { margin-left: auto; display: flex; gap: var(--space-sm, 0.5rem); }
  .stage-list { display: flex; flex-direction: column; gap: 0.75rem; margin-bottom: var(--space-md, 1rem); }
  .stage-card-wrapper { display: flex; align-items: center; gap: var(--space-sm, 0.5rem); padding: var(--space-sm, 0.5rem); border: 1px solid var(--color-border, #334155); border-radius: var(--radius-lg, 8px); background: var(--color-surface, #1e293b); cursor: move; transition: border-color 0.15s; }
  .stage-card-wrapper.drag-over { border-color: var(--color-primary, #3b82f6); background: var(--color-surface-hover, #334155); }
  .drag-handle { cursor: grab; color: var(--color-text-secondary, #94a3b8); font-size: 1.25rem; user-select: none; }
  .empty-state { text-align: center; padding: 3rem var(--space-md, 1rem); color: var(--color-text-secondary, #94a3b8); border: 2px dashed var(--color-border, #334155); border-radius: var(--radius-lg, 8px); }
  .btn { padding: 0.4rem 0.9rem; border: none; border-radius: var(--radius-md, 6px); cursor: pointer; font-size: 0.875rem; font-weight: 500; }
  .btn-primary { background: var(--color-primary, #3b82f6); color: var(--color-text, #f1f5f9); }
  .btn-accent { background: var(--color-success, #22c55e); color: var(--color-text, #f1f5f9); }
  .btn-secondary { background: var(--color-surface-hover, #334155); color: var(--color-text, #f1f5f9); }
  .btn-add-stage { width: 100%; padding: 0.6rem; background: var(--color-surface, #1e293b); border: 2px dashed var(--color-border, #334155); border-radius: var(--radius-lg, 8px); cursor: pointer; font-size: 0.9rem; color: var(--color-text, #f1f5f9); }
  .btn-add-stage:hover { background: var(--color-surface-hover, #334155); }
  .btn-delete { background: none; border: none; cursor: pointer; color: var(--color-error, #ef4444); font-size: 0.9rem; padding: 0.25rem; }
  .delete-confirm { display: flex; align-items: center; gap: 0.3rem; font-size: 0.8rem; }
  .btn-danger-small { background: var(--color-error, #ef4444); color: var(--color-text, #f1f5f9); border: none; border-radius: var(--radius-sm, 4px); padding: 0.15rem 0.4rem; cursor: pointer; font-size: 0.75rem; }
  .btn-small { background: var(--color-border, #334155); border: none; border-radius: var(--radius-sm, 4px); padding: 0.15rem 0.4rem; cursor: pointer; font-size: 0.75rem; }
  .move-buttons {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex-shrink: 0;
  }

  .move-btn {
    background: var(--color-surface-hover, #334155);
    border: 1px solid var(--color-border, #334155);
    color: var(--color-text, #f1f5f9);
    cursor: pointer;
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 0.75rem;
    line-height: 1.2;
    transition: background 0.15s;
  }

  .move-btn:hover:not(:disabled) {
    background: var(--color-primary, #3b82f6);
  }

  .move-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }
</style>
