<script lang="ts">
  import { SNIPPETS } from './snippets'
  import { STAGE_COLORS } from './types'

  const stageTypes = [{ type: 'transform', label: 'Transform' }, { type: 'filter', label: 'Filter' }] as const

  let { onImport, onExport, onSnippetView }: {
    onImport?: () => void
    onExport?: () => void
    onSnippetView?: (snippet: typeof SNIPPETS[number]) => void
  } = $props()

  let snippetsOpen = $state(true)
  let sectionOpen = $state<Record<string, boolean>>({ transform: true, filter: true })
  let searchQuery = $state('')

  let filteredSnippetsByType = $derived.by(() => {
    const map: Record<string, typeof SNIPPETS> = {}
    const q = searchQuery.toLowerCase().trim()
    for (const st of stageTypes) {
      let items = SNIPPETS.filter(s => s.type === st.type)
      if (q) {
        items = items.filter(s =>
          s.name.toLowerCase().includes(q) ||
          s.description.toLowerCase().includes(q)
        )
      }
      map[st.type] = items
    }
    return map
  })

  let searchActive = $derived(searchQuery.trim().length > 0)

  function onDragStart(event: DragEvent, stageType: string) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/svelteflow', stageType)
      event.dataTransfer.effectAllowed = 'move'
    }
  }

  function onDragStartSnippet(event: DragEvent, snippet: typeof SNIPPETS[number]) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/svelteflow', snippet.type)
      event.dataTransfer.setData('application/snippet', JSON.stringify(snippet))
      event.dataTransfer.effectAllowed = 'move'
    }
  }
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

  <div class="separator" role="separator"></div>

  <button class="section-toggle" onclick={() => snippetsOpen = !snippetsOpen}>
    {snippetsOpen ? '▼' : '▶'} Snippets
  </button>
  {#if snippetsOpen}
    <input class="snippet-search" type="text" placeholder="Search snippets..." bind:value={searchQuery} />

    {#each stageTypes as st}
      {#if (filteredSnippetsByType[st.type]?.length ?? 0) > 0}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <div class="snippet-subheader"
          style="color: {STAGE_COLORS[st.type]?.badge}"
          class:search-active={searchActive}
          onclick={() => { if (!searchActive) sectionOpen[st.type] = !sectionOpen[st.type] }}
          role="button"
          tabindex="0"
          onkeydown={(e) => { if (e.key === 'Enter') sectionOpen[st.type] = !sectionOpen[st.type] }}
        >
          <span class="subheader-dot" style="background: {STAGE_COLORS[st.type]?.badge}"></span>
          <span class="subheader-arrow">{(searchActive || sectionOpen[st.type]) ? '▼' : '▶'}</span>
          {st.label}
        </div>
        {#if searchActive || sectionOpen[st.type]}
          {#each filteredSnippetsByType[st.type] as snippet}
          <div class="snippet-row">
            <div class="snippet-card" style="border-left-color: {STAGE_COLORS[snippet.type]?.border ?? '#334155'}" draggable={true}
              ondragstart={(e) => onDragStartSnippet(e, snippet)}
              onkeydown={(e) => e.key === 'Enter' && onDragStartSnippet(e as unknown as DragEvent, snippet)}>
              <span class="snippet-name">{snippet.name}</span>
              <button class="snippet-eye-btn" onclick={() => onSnippetView?.(snippet)} aria-label="View snippet">👁</button>
            </div>
          </div>
          {/each}
        {/if}
      {/if}
    {/each}
  {/if}

  <button class="import-btn" onclick={() => onImport?.()}>
    Import YAML
  </button>
  <button class="import-btn" onclick={() => onExport?.()}>
    Export YAML
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

  .separator {
    border: none;
    border-top: 1px solid var(--color-border, #334155);
    margin: var(--space-xs, 4px) 0;
  }

  .section-toggle {
    background: none;
    border: none;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    padding: var(--space-xs, 4px) 0;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    width: 100%;
    text-align: left;
  }
  .section-toggle:hover {
    color: var(--color-text, #f1f5f9);
  }

  .snippet-search {
    width: 100%;
    padding: 5px 8px;
    font-size: 0.7rem;
    border: 1px solid var(--color-border, #334155);
    border-radius: 4px;
    background: var(--color-surface, #1e293b);
    color: var(--color-text, #f1f5f9);
    outline: none;
    box-sizing: border-box;
    transition: border-color 150ms ease;
  }
  .snippet-search::placeholder {
    color: var(--color-text-secondary, #64748b);
  }
  .snippet-search:focus {
    border-color: var(--color-accent, #6366f1);
  }
  .snippet-search:not(:placeholder-shown) {
    border-color: var(--color-accent, #6366f1);
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
  }

  .import-btn:hover {
    background: var(--color-surface-hover, #334155);
    color: var(--color-text, #f1f5f9);
  }

  .import-btn:active {
    background: var(--color-border, #334155);
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--color-text-secondary, #94a3b8);
    margin: var(--space-xs, 4px) 0;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .snippet-card {
    flex: 1;
    height: 36px;
    padding: 4px 8px 4px 10px;
    border-left: 3px solid transparent;
    border-radius: 4px;
    cursor: grab;
    background: var(--color-surface-hover, #334155);
    display: flex;
    align-items: center;
    position: relative;
  }

  .snippet-row {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 0;
  }

  .snippet-subheader {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin: 6px 0 2px 0;
    display: flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    user-select: none;
    transition: color 150ms ease;
  }

  .snippet-subheader.search-active {
    cursor: default;
  }

  .subheader-arrow {
    font-size: 0.6rem;
    width: 10px;
    flex-shrink: 0;
  }

  .subheader-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    display: inline-block;
    flex-shrink: 0;
  }

  .snippet-name {
    font-size: 0.65rem;
    font-weight: 500;
    color: var(--color-text, #f1f5f9);
  }

  .snippet-eye-btn {
    position: absolute;
    top: 2px;
    right: 2px;
    z-index: 2;
    background: transparent;
    border: none;
    cursor: pointer;
    font-size: 0.75rem;
    opacity: 0.25;
    filter: grayscale(1) brightness(1.5);
    transition: opacity 150ms ease;
    padding: 2px 3px;
    line-height: 1;
    border-radius: 3px;
    color: #888;
  }
  .snippet-card:hover .snippet-eye-btn {
    opacity: 0.5;
  }
  .snippet-eye-btn:hover {
    background: rgba(0, 0, 0, 0.15);
  }
</style>
