<script lang="ts">
  import { formatValue, highlightJSON } from './diff'
  import type { UnifiedDiffLine } from './diff'
  import type { StageDiff } from './types'

  let { stageDiff, unifiedDiff, stageIndex, diffView, onToggleView, onExpandChange }: {
    stageDiff: StageDiff
    unifiedDiff: UnifiedDiffLine[]
    stageIndex: number
    diffView: 'inline' | 'unified'
    onToggleView: () => void
    onExpandChange?: (expanded: boolean) => void
  } = $props()

  let expanded = $state(false)
  const pageSize = 10

  let modifiedPage = $state(0)
  let addedPage = $state(0)
  let removedPage = $state(0)
  let unifiedPage = $state(0)

  function toggleExpanded() {
    expanded = !expanded
    onExpandChange?.(expanded)
    if (expanded) {
      modifiedPage = 0
      addedPage = 0
      removedPage = 0
      unifiedPage = 0
    }
  }

  function totalPages(count: number) {
    return Math.max(1, Math.ceil(count / pageSize))
  }

  function prevPage(setter: (p: number) => void, current: number) {
    if (current > 0) setter(current - 1)
  }

  function nextPage(setter: (p: number) => void, current: number, total: number) {
    if (current < total - 1) setter(current + 1)
  }
</script>

<div class="stage-diff">
  <button class="diff-toggle" onclick={toggleExpanded}>
    <span class="expand-icon">{expanded ? '▾' : '▸'}</span>
    <span>Show Diff</span>
    <span class="diff-counts">
      <span class="count-added">+{stageDiff.added.length}</span>
      <span class="count-removed">-{stageDiff.removed.length}</span>
      <span class="count-modified">~{stageDiff.modified.length}</span>
    </span>
  </button>
  {#if expanded}
    <div class="diff-content">
      <div class="diff-view-toggle">
        <span>{stageDiff.signal}: {stageDiff.input_count} → {stageDiff.output_count} records</span>
        <button class:active={diffView === 'inline'} onclick={onToggleView}>Inline</button>
        <button class:active={diffView === 'unified'} onclick={onToggleView}>Unified Diff</button>
      </div>

      {#if diffView === 'inline'}
        {#each stageDiff.modified.slice(modifiedPage * pageSize, (modifiedPage + 1) * pageSize) as span}
          <div class="diff-item diff-modified">
            <span class="diff-badge">MODIFIED</span>
            <code class="diff-span-id">{span.id}</code>{#if span.name} <span>{span.name}</span>{/if}
            {#if span.changes && span.changes.length > 0}
              <div class="diff-changes">
                {#each span.changes as change}
                  <div class="diff-change diff-change-{change.type}">
                    <span class="change-icon">
                      {#if change.type === 'added'}+{:else if change.type === 'removed'}-{:else}~{/if}
                    </span>
                    <span class="change-field">{change.field}</span>
                    {#if change.type === 'changed'}
                      <span class="change-value-before">{formatValue(change.before)}</span>
                      <span class="change-arrow">→</span>
                      <span class="change-value-after">{formatValue(change.after)}</span>
                    {:else if change.type === 'added'}
                      <span class="change-value-after">{formatValue(change.after)}</span>
                    {:else}
                      <span class="change-value-before">{formatValue(change.before)}</span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
        {#if stageDiff.modified.length > pageSize}
          <div class="pagination">
            <button disabled={modifiedPage === 0} onclick={() => modifiedPage--}>Prev</button>
            <span>{modifiedPage + 1} / {Math.ceil(stageDiff.modified.length / pageSize)}</span>
            <button disabled={modifiedPage >= Math.ceil(stageDiff.modified.length / pageSize) - 1} onclick={() => modifiedPage++}>Next</button>
          </div>
        {/if}
        {#each stageDiff.added.slice(addedPage * pageSize, (addedPage + 1) * pageSize) as span}
          <div class="diff-item diff-added">
            <span class="diff-badge">ADDED</span>
            <code class="diff-span-id">{span.id}</code>{#if span.name} <span>{span.name}</span>{/if}
            {#if span.after !== undefined}<pre>{JSON.stringify(span.after, null, 2)}</pre>{/if}
          </div>
        {/each}
        {#if stageDiff.added.length > pageSize}
          <div class="pagination">
            <button disabled={addedPage === 0} onclick={() => addedPage--}>Prev</button>
            <span>{addedPage + 1} / {Math.ceil(stageDiff.added.length / pageSize)}</span>
            <button disabled={addedPage >= Math.ceil(stageDiff.added.length / pageSize) - 1} onclick={() => addedPage++}>Next</button>
          </div>
        {/if}
        {#each stageDiff.removed.slice(removedPage * pageSize, (removedPage + 1) * pageSize) as span}
          <div class="diff-item diff-removed">
            <span class="diff-badge">REMOVED</span>
            <code class="diff-span-id">{span.id}</code>{#if span.name} <span>{span.name}</span>{/if}
            {#if span.before !== undefined}<pre>{JSON.stringify(span.before, null, 2)}</pre>{/if}
          </div>
        {/each}
        {#if stageDiff.removed.length > pageSize}
          <div class="pagination">
            <button disabled={removedPage === 0} onclick={() => removedPage--}>Prev</button>
            <span>{removedPage + 1} / {Math.ceil(stageDiff.removed.length / pageSize)}</span>
            <button disabled={removedPage >= Math.ceil(stageDiff.removed.length / pageSize) - 1} onclick={() => removedPage++}>Next</button>
          </div>
        {/if}
        {#if stageDiff.added.length === 0 && stageDiff.removed.length === 0 && stageDiff.modified.length === 0}
          <div class="diff-empty">No changes detected</div>
        {/if}
      {:else}
        <div class="unified-diff">
          <p>Large comparisons may show a nonminimal removed/added middle to bound work.</p>
          {#if unifiedDiff && unifiedDiff.length > 0}
            {#each unifiedDiff as line}
              <div class="diff-line diff-line-{line.type}">
                <span class="diff-prefix">{line.type === 'context' ? ' ' : line.type === 'add' ? '+' : '-'}</span>
                <span class="diff-text">{@html highlightJSON(line.text)}</span>
              </div>
            {/each}
          {:else}
            <div class="diff-empty">No changes detected</div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .stage-diff {
    margin-top: 0.5rem;
    border-top: 1px solid var(--color-border, #334155);
    padding-top: 0.5rem;
  }
  .diff-toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.375rem 0.5rem;
    border: none;
    background: var(--color-surface, #1e293b);
    color: var(--color-text, #f1f5f9);
    cursor: pointer;
    font-size: 0.85rem;
    border-radius: var(--radius-sm, 4px);
  }
  .diff-toggle:hover {
    background: var(--color-surface-hover, #334155);
  }
  .expand-icon {
    color: var(--color-text-secondary, #94a3b8);
    font-size: 0.75rem;
  }
  .diff-counts {
    margin-left: auto;
    display: flex;
    gap: 0.5rem;
    font-size: 0.8rem;
  }
  .count-added { color: var(--color-success, #22c55e); }
  .count-removed { color: var(--color-error, #ef4444); }
  .count-modified { color: var(--color-warning, #f59e0b); }
  .diff-content {
    padding: var(--space-sm, 0.5rem);
  }
  .diff-item {
    display: flex;
    align-items: center;
    gap: var(--space-sm, 0.5rem);
    padding: var(--space-xs, 0.25rem) 0;
    font-size: 0.85rem;
  }
  .diff-added { color: var(--color-success, #22c55e); }
  .diff-removed { color: var(--color-error, #ef4444); }
  .diff-modified { color: var(--color-warning, #f59e0b); }
  .diff-badge {
    font-size: 0.7rem;
    font-weight: 600;
    padding: 1px 4px;
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface-hover, #334155);
  }
  .diff-span-id {
    font-family: var(--font-mono, monospace);
    font-size: 0.8rem;
  }
  .diff-span-id.zero-id {
    color: var(--color-error, #ef4444);
    font-weight: bold;
    text-decoration: underline wavy var(--color-error, #ef4444);
  }
  .diff-empty {
    color: var(--color-text-secondary, #94a3b8);
    font-style: italic;
    font-size: 0.85rem;
  }
  .diff-view-toggle {
    display: flex;
    gap: var(--space-xs, 0.25rem);
    margin-bottom: var(--space-sm, 0.5rem);
  }
  .diff-view-toggle button {
    padding: 0.2rem 0.6rem;
    font-size: 0.75rem;
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface, #1e293b);
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
  }
  .diff-view-toggle button.active {
    background: var(--color-primary, #3b82f6);
    color: white;
    border-color: var(--color-primary, #3b82f6);
  }
  .diff-changes {
    margin-top: 0.25rem;
    margin-left: 1.5rem;
    font-size: 0.8rem;
  }
  .diff-change {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0;
    font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace);
  }
  .diff-change-added { color: var(--color-success, #22c55e); }
  .diff-change-removed { color: var(--color-error, #ef4444); }
  .diff-change-changed { color: var(--color-warning, #eab308); }
  .change-icon { font-weight: bold; width: 1rem; text-align: center; }
  .change-field { font-weight: 500; }
  .change-arrow { color: var(--color-text-secondary, #94a3b8); }
  .change-value-before { color: var(--color-error, #ef4444); text-decoration: line-through; }
  .change-value-after { color: var(--color-success, #22c55e); }
  .unified-diff {
    font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace);
    font-size: 0.75rem;
    line-height: 1.5;
    background: var(--color-bg, #0f172a);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    overflow-x: auto;
    max-height: 400px;
    overflow-y: auto;
  }
  .diff-line {
    display: flex;
    padding: 0 0.5rem;
    min-height: 1.25rem;
  }
  .diff-line-context { background: transparent; }
  .diff-line-add { background: rgba(34, 197, 94, 0.1); }
  .diff-line-remove { background: rgba(239, 68, 68, 0.1); }
  .diff-prefix {
    width: 1.2rem;
    flex-shrink: 0;
    color: var(--color-text-secondary, #64748b);
    text-align: center;
    user-select: none;
  }
  .diff-line-add .diff-prefix { color: var(--color-success, #22c55e); }
  .diff-line-remove .diff-prefix { color: var(--color-error, #ef4444); }
  .diff-text { white-space: pre; }

  /* JSON syntax highlighting
     Note: :global() needed because highlightJSON() injects spans via {@html},
     which bypasses Svelte's scoped CSS. */
  :global(.json-key) { color: #93c5fd; }
  :global(.json-string) { color: #86efac; }
  :global(.json-number) { color: #fca5a5; }
  :global(.json-bool) { color: #c084fc; }
  :global(.json-null) { color: #94a3b8; font-style: italic; }

  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.5rem;
    font-size: 0.75rem;
    color: var(--color-text-secondary, #94a3b8);
  }
  .pagination button {
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--color-border, #334155);
    background: var(--color-surface, #1e293b);
    color: var(--color-text, #f1f5f9);
    cursor: pointer;
    border-radius: var(--radius-sm, 4px);
  }
  .pagination button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .pagination button:not(:disabled):hover {
    background: var(--color-border, #334155);
  }
</style>
