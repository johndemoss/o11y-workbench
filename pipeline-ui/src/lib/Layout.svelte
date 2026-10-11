<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import CollectorVersionSelect from './CollectorVersionSelect.svelte'
  import { loadCollectorVersions } from './collector-version'

  let { activeTab = 'graph', onTabChange, children }: {
    activeTab: 'quick' | 'pipeline' | 'graph'
    onTabChange?: (tab: 'quick' | 'pipeline' | 'graph') => void
    children?: Snippet
  } = $props()

  let versionsReady = $state(false)

  onMount(async () => {
    try {
      await loadCollectorVersions()
    } catch {
      /* picker stays empty if API unavailable */
    }
    versionsReady = true
  })
</script>

<main class="app">
  <nav class="top-nav">
    <div class="app-brand">
      <img src="/pipeline/logo.svg" alt="o11y workbench" class="app-logo">
      <span class="app-title">Pipeline Prototyper</span>
    </div>
    <div class="nav-right">
      {#if versionsReady}
        <CollectorVersionSelect />
      {/if}
      <a href="/" class="nav-link">← Dashboard</a>
    </div>
  </nav>
  <div class="tab-bar">
    <button
      class="tab-button"
      class:active={activeTab === 'quick'}
      onclick={() => onTabChange?.('quick')}
    >
      Quick
    </button>
    <button
      class="tab-button"
      class:active={activeTab === 'graph'}
      onclick={() => onTabChange?.('graph')}
    >
      Graph
    </button>
    <button
      class="tab-button"
      class:active={activeTab === 'pipeline'}
      onclick={() => onTabChange?.('pipeline')}
    >
      Pipeline
    </button>
  </div>
  {@render children?.()}
</main>

<style>
  .app {
    font-family: var(--font-family, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif);
    padding: var(--space-md, 1rem);
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .top-nav {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    background: var(--color-surface, #1e293b);
    border-bottom: 1px solid var(--color-border, #334155);
    margin-bottom: var(--space-md, 1rem);
  }
  .app-brand {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .app-logo {
    height: 32px;
    width: auto;
  }
  .app-title {
    font-weight: 600;
    font-size: var(--font-size-lg, 1.1rem);
    color: var(--color-text, #f1f5f9);
  }
  .nav-right {
    display: flex;
    align-items: center;
    gap: var(--space-md, 1rem);
  }
  .nav-link {
    color: var(--color-primary, #3b82f6);
    text-decoration: none;
    font-size: var(--font-size-sm, 0.8rem);
  }
  .nav-link:hover {
    text-decoration: underline;
  }
  .tab-bar {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--color-border, #334155);
    margin-bottom: var(--space-md, 1rem);
    padding: 0 var(--space-md, 1rem);
  }
  .tab-button {
    padding: var(--space-sm, 0.5rem) var(--space-md, 1rem);
    border: none;
    background: transparent;
    color: var(--color-text-secondary, #94a3b8);
    cursor: pointer;
    font-size: var(--font-size-sm, 0.8rem);
    border-bottom: 2px solid transparent;
    transition: all var(--transition-fast, 150ms ease);
  }
  .tab-button:hover {
    color: var(--color-text, #f1f5f9);
    background: var(--color-surface-hover, #334155);
  }
  .tab-button.active {
    color: var(--color-primary, #3b82f6);
    border-bottom-color: var(--color-primary, #3b82f6);
  }
</style>
