<script lang="ts">
  import type { Pipeline, Stage, StageType, ProcessorConfig } from './types'
  import StageEditor from './StageEditor.svelte'

  let { pipeline = { stages: [], processors: {} }, onValidate, onExecute, onImport }: {
    pipeline: Pipeline
    onValidate?: (p: Pipeline) => void
    onExecute?: (p: Pipeline) => void
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
      <button onclick={() => onExecute?.(pipeline)} class="btn btn-accent">Execute</button>
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
  .pipeline-builder { padding: 1rem; max-width: 900px; margin: 0 auto; }
  .builder-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
  .builder-header h1 { font-size: 1.5rem; font-weight: 600; margin: 0; }
  .badge { background: #9333ea; color: white; padding: 0.15rem 0.5rem; border-radius: 999px; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; }
  .actions { margin-left: auto; display: flex; gap: 0.5rem; }
  .stage-list { display: flex; flex-direction: column; gap: 0.75rem; margin-bottom: 1rem; }
  .stage-card-wrapper { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem; border: 1px solid #e5e7eb; border-radius: 0.5rem; background: #f9fafb; cursor: move; transition: border-color 0.15s; }
  .stage-card-wrapper.drag-over { border-color: #6366f1; background: #eef2ff; }
  .drag-handle { cursor: grab; color: #9ca3af; font-size: 1.25rem; user-select: none; }
  .empty-state { text-align: center; padding: 3rem 1rem; color: #9ca3af; border: 2px dashed #e5e7eb; border-radius: 0.5rem; }
  .btn { padding: 0.4rem 0.9rem; border: none; border-radius: 0.375rem; cursor: pointer; font-size: 0.875rem; font-weight: 500; }
  .btn-primary { background: #6366f1; color: white; }
  .btn-accent { background: #059669; color: white; }
  .btn-secondary { background: #6b7280; color: white; }
  .btn-add-stage { width: 100%; padding: 0.6rem; background: #f3f4f6; border: 2px dashed #d1d5db; border-radius: 0.5rem; cursor: pointer; font-size: 0.9rem; color: #374151; }
  .btn-add-stage:hover { background: #e5e7eb; }
  .btn-delete { background: none; border: none; cursor: pointer; color: #ef4444; font-size: 0.9rem; padding: 0.25rem; }
  .delete-confirm { display: flex; align-items: center; gap: 0.3rem; font-size: 0.8rem; }
  .btn-danger-small { background: #ef4444; color: white; border: none; border-radius: 0.25rem; padding: 0.15rem 0.4rem; cursor: pointer; font-size: 0.75rem; }
  .btn-small { background: #d1d5db; border: none; border-radius: 0.25rem; padding: 0.15rem 0.4rem; cursor: pointer; font-size: 0.75rem; }
</style>
