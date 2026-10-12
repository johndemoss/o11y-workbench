<script lang="ts">
  import { SNIPPETS, extractOttlLines } from './snippets'
  import type { SignalType, SnippetTemplate, StageType } from './types'
  import { SIGNALS } from './types'
  import yaml from 'js-yaml'

  let {
    onImport,
    onExport,
    onSnippetView,
    onAddStage,
    targetSignal = $bindable('traces' as SignalType),
  }: {
    onImport?: () => void
    onExport?: () => void
    onSnippetView?: (snippet: SnippetTemplate) => void
    onAddStage?: (opts: {
      type: StageType
      signal: SignalType
      snippet?: SnippetTemplate
    }) => void
    targetSignal?: SignalType
  } = $props()

  let searchQuery = $state('')
  let typeFilter = $state<'all' | 'transform' | 'filter'>('all')
  let sortOrder = $state<'az' | 'za'>('az')

  const blankStages = [
    { type: 'transform' as const, label: 'Transform' },
    { type: 'filter' as const, label: 'Filter' },
  ]

  let libraryEntries = $derived.by(() => {
    const q = searchQuery.toLowerCase().trim()
    let items = [...SNIPPETS]
    if (typeFilter !== 'all') items = items.filter((s) => s.type === typeFilter)
    if (q) {
      items = items.filter((s) => {
        const stmt = extractOttlLines(s.type, s.processor as Record<string, unknown>)
        return (
          s.name.toLowerCase().includes(q) ||
          s.description.toLowerCase().includes(q) ||
          stmt.toLowerCase().includes(q)
        )
      })
    }
    items.sort((a, b) =>
      sortOrder === 'az' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name),
    )
    return items
  })

  let grouped = $derived.by(() => {
    const groups: { type: 'transform' | 'filter'; label: string; items: SnippetTemplate[] }[] = [
      { type: 'transform', label: 'Transform', items: [] },
      { type: 'filter', label: 'Filter', items: [] },
    ]
    for (const item of libraryEntries) {
      const g = groups.find((x) => x.type === item.type)
      g?.items.push(item)
    }
    return groups.filter((g) => g.items.length > 0)
  })

  function onDragStart(event: DragEvent, stageType: string) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/svelteflow', stageType)
      event.dataTransfer.setData('application/target-signal', targetSignal)
      event.dataTransfer.effectAllowed = 'copy'
    }
  }

  function onDragStartSnippet(event: DragEvent, snippet: SnippetTemplate) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/svelteflow', snippet.type)
      event.dataTransfer.setData('application/snippet', JSON.stringify(snippet))
      event.dataTransfer.setData('application/target-signal', targetSignal)
      event.dataTransfer.effectAllowed = 'copy'
    }
  }

  function previewText(snippet: SnippetTemplate): string {
    try {
      return yaml.dump(snippet.processor, { indent: 2, noRefs: true, sortKeys: false }).trimEnd()
    } catch {
      return JSON.stringify(snippet.processor, null, 2)
    }
  }

  function addBlank(type: StageType) {
    onAddStage?.({ type, signal: targetSignal })
  }

  function addSnippet(snippet: SnippetTemplate) {
    // Saved library entries use span/trace context → Traces only
    const signal: SignalType = 'traces'
    targetSignal = signal
    onAddStage?.({ type: snippet.type, signal, snippet })
  }
</script>

<aside class="palette" data-od-id="stage-palette">
  <div class="palette-heading">
    <h3 data-od-id="palette-heading">Library</h3>
    <div class="heading-actions">
      <button type="button" class="ghost-link" onclick={() => onImport?.()}>Import</button>
      <button type="button" class="ghost-link" onclick={() => onExport?.()}>Export</button>
    </div>
  </div>

  <div class="lane-target">
    <span class="lane-label" id="add-signal-label">Lane</span>
    <div
      class="pill-toggle"
      role="radiogroup"
      aria-labelledby="add-signal-label"
      id="add-signal"
      data-od-id="template-target"
      title="Blank stages use this lane. Favorites always add to Traces."
    >
      {#each SIGNALS as sig}
        <button
          type="button"
          role="radio"
          aria-checked={targetSignal === sig}
          class:active={targetSignal === sig}
          onclick={() => targetSignal = sig}
        >{sig.charAt(0).toUpperCase() + sig.slice(1)}</button>
      {/each}
    </div>
  </div>

  <div class="section-label">Blank</div>
  <div id="stage-palette" class="blank-stages">
    {#each blankStages as st}
      <button
        type="button"
        class="blank-chip"
        data-stage-kind={st.type}
        draggable="true"
        ondragstart={(e) => onDragStart(e, st.type)}
        onclick={() => addBlank(st.type)}
        aria-label="Add {st.label} blank stage"
      >{st.label}</button>
    {/each}
  </div>

  <section class="library-entries" data-od-id="library-entries">
    <div class="library-heading">
      <label for="snippet-search">Favorites</label>
      <span class="library-count" id="library-count" role="status" aria-live="polite">
        {libraryEntries.length}
      </span>
    </div>
    <label class="sr-only" for="snippet-search">Search library</label>
    <input
      id="snippet-search"
      type="search"
      placeholder="Search"
      bind:value={searchQuery}
      data-od-id="library-search"
    />
    <div class="library-controls">
      <select id="library-type" aria-label="Type" bind:value={typeFilter} data-od-id="library-type">
        <option value="all">All types</option>
        <option value="transform">Transform</option>
        <option value="filter">Filter</option>
      </select>
      <select id="library-sort" aria-label="Sort" bind:value={sortOrder} data-od-id="library-sort">
        <option value="az">A–Z</option>
        <option value="za">Z–A</option>
      </select>
    </div>

    <div id="snippet-list">
      {#each grouped as group}
        <div class="library-group">
          <h4>{group.label}</h4>
          <div class="library-cards">
            {#each group.items as snippet}
              <div
                class="palette-item"
                data-stage-kind={snippet.type}
                draggable="true"
                ondragstart={(e) => onDragStartSnippet(e, snippet)}
                role="group"
                aria-label="{snippet.name} library entry"
              >
                <strong>{snippet.name}</strong>
                <small class="card-desc">{snippet.description}</small>
                <div class="toolbar">
                  <button type="button" onclick={() => onSnippetView?.(snippet)}>Preview</button>
                  <button type="button" onclick={() => addSnippet(snippet)}>Add</button>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="palette-empty">
          <p>No matches.</p>
        </div>
      {/each}
    </div>
  </section>
</aside>

<style>
  .palette {
    width: 220px;
    padding: 12px 10px;
    background: var(--color-surface-2, #162032);
    border-right: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex-shrink: 0;
    min-width: 0;
    overflow: auto;
  }

  .palette-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .palette-heading h3 {
    margin: 0;
    font-size: 14px;
  }

  .heading-actions {
    display: inline-flex;
    gap: 4px;
  }

  .ghost-link {
    min-height: 28px;
    padding: 2px 8px;
    border: 0;
    background: transparent;
    color: var(--color-text-secondary);
    font-size: 11px;
  }

  .ghost-link:hover {
    color: var(--color-text);
    background: var(--color-surface-hover);
  }

  .lane-target {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: center;
    gap: 8px;
  }

  .lane-label,
  .section-label,
  label {
    font-size: 11px;
    font-weight: 500;
    color: var(--color-text-secondary);
    margin: 0;
  }

  .pill-toggle {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 2px;
    padding: 3px;
    border-radius: 999px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
  }

  .pill-toggle button {
    min-height: 28px;
    padding: 2px 4px;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--color-text-secondary);
    font-size: 11px;
    font-weight: 500;
  }

  .pill-toggle button.active,
  .pill-toggle button[aria-checked='true'] {
    background: var(--action-bg);
    color: var(--action-fg);
  }

  .blank-stages {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
  }

  .blank-chip {
    min-height: 36px;
    padding: 6px 8px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 12px;
    font-weight: 600;
    cursor: grab;
  }

  .blank-chip[data-stage-kind='transform'] {
    border-top: 2px solid var(--stage-transform, #7c3aed);
  }
  .blank-chip[data-stage-kind='filter'] {
    border-top: 2px solid var(--stage-filter, #d97706);
  }

  .library-entries {
    border-top: 1px solid var(--color-border);
    margin-top: 4px;
    padding-top: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
  }

  .library-heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }

  .library-heading label {
    font-size: 12px;
    font-weight: 600;
    color: var(--color-text);
  }

  .library-count {
    font: 11px var(--font-mono);
    color: var(--color-text-secondary);
  }

  .library-controls {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
  }

  .library-cards {
    display: grid;
    gap: 6px;
  }

  .palette-item {
    border: 1px solid var(--color-border);
    border-radius: 8px;
    padding: 10px;
    background: var(--color-surface);
    box-shadow: var(--shadow-sm);
    cursor: grab;
  }

  .palette-item[data-stage-kind='transform'] {
    border-top: 2px solid var(--stage-transform, #7c3aed);
  }
  .palette-item[data-stage-kind='filter'] {
    border-top: 2px solid var(--stage-filter, #d97706);
  }

  .palette-item strong {
    display: block;
    font-size: 12px;
    font-weight: 600;
  }

  .palette-item small {
    display: block;
    font: 11px/1.4 var(--font-family);
    margin: 4px 0 0;
    color: var(--color-text-secondary);
    overflow-wrap: anywhere;
  }

  .palette-item .toolbar {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
    border-top: 1px solid var(--color-border);
    padding-top: 8px;
    margin-top: 8px;
  }

  .palette-item button,
  select,
  input {
    font-size: 12px;
    min-height: 36px;
  }

  .library-group + .library-group {
    margin-top: 12px;
  }

  .library-group h4 {
    font-size: 11px;
    margin: 0 0 6px;
    font-weight: 600;
    color: var(--color-text-secondary);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  .palette-empty {
    padding: 10px;
    border: 1px dashed var(--color-border);
    border-radius: 6px;
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  .palette-empty p { margin: 0; }

  @media (max-width: 960px) {
    .palette { width: 100%; }
    .library-cards {
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    }
  }
</style>
