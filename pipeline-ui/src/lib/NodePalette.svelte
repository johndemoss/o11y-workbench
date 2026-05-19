<script lang="ts">
  let {
    onImport,
  }: {
    onImport?: () => void
  } = $props()

  function onDragStart(event: DragEvent, stageType: string) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/svelteflow', stageType)
      event.dataTransfer.effectAllowed = 'move'
    }
  }

  const stageTypes = [{ type: 'transform', label: 'Transform' }, { type: 'filter', label: 'Filter' }]
</script>

<aside class="palette">
  <h2 class="title">Pipeline Stages</h2>

  {#each stageTypes as st}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="drag-card drag-card--{st.type}"
    draggable={true}
    ondragstart={(e) => onDragStart(e, st.type)}
    onkeydown={(e) => e.key === 'Enter' && onDragStart(e as unknown as DragEvent, st.type)}
  >
    {st.label}
    <div class="hint">drag to add</div>
  </div>
  {/each}

  <button class="import-btn" onclick={() => onImport?.()}>
    Import YAML
  </button>
</aside>

<style>
  .palette {
    width: 180px;
    padding: var(--space-md, 12px);
    background: var(--color-surface, #1e293b);
    border-right: 1px solid var(--color-border, #334155);
    display: flex;
    flex-direction: column;
    gap: var(--space-sm, 8px);
    flex-shrink: 0;
  }

  .title {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text, #f1f5f9);
    margin: 0 0 var(--space-xs, 4px) 0;
    letter-spacing: 0.02em;
  }

  .drag-card {
    padding: var(--space-md, 12px);
    border-radius: var(--radius-md, 6px);
    cursor: grab;
    color: #fff;
    font-weight: 600;
    font-size: 0.85rem;
    text-align: center;
    transition: opacity var(--transition-fast, 150ms), transform var(--transition-fast, 150ms);
    user-select: none;
  }

  .drag-card:hover {
    transform: translateY(-1px);
  }

  .drag-card:active {
    opacity: 0.6;
    cursor: grabbing;
  }

  .drag-card--transform {
    background: var(--color-accent-hover, #7c3aed);
    border-left-color: #7c3aed;
  }

  .drag-card--filter {
    background: var(--color-card-filter, #d97706);
  }

  .hint {
    font-weight: 400;
    font-size: 0.7rem;
    opacity: 0.8;
    margin-top: var(--space-xs, 4px);
  }

  .import-btn {
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-md, 6px);
    background: transparent;
    color: var(--color-text-secondary, #94a3b8);
    font-size: var(--font-size-sm, 0.8rem);
    cursor: pointer;
    transition: background var(--transition-fast, 150ms), color var(--transition-fast, 150ms);
    text-align: center;
    line-height: 1.4;
    margin-top: auto;
  }

  .import-btn:hover {
    background: var(--color-surface-hover, #334155);
    color: var(--color-text, #f1f5f9);
  }

  .import-btn:active {
    background: var(--color-border, #334155);
  }
</style>
