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

<main class="app" data-od-id="integrated-pipeline-workspace">
  <header class="top-nav" data-od-id="pipeline-header">
    <div class="brand">
      <img src="/pipeline/logo.svg" width="420" height="120" alt="o11y workbench" class="app-logo">
      <h1 data-od-id="pipeline-title">Pipeline Prototyper</h1>
    </div>
    <div class="nav-right">
      {#if versionsReady}
        <CollectorVersionSelect />
      {/if}
      <a href="/" class="nav-link" data-od-id="workbench-link">← Dashboard</a>
    </div>
  </header>

  <nav class="tab-bar" aria-label="Pipeline views" data-od-id="pipeline-views">
    <button
      type="button"
      class="tab-button"
      aria-current={activeTab === 'quick' ? 'page' : undefined}
      onclick={() => onTabChange?.('quick')}
    >
      Quick
    </button>
    <button
      type="button"
      class="tab-button"
      aria-current={activeTab === 'graph' ? 'page' : undefined}
      onclick={() => onTabChange?.('graph')}
    >
      Graph
    </button>
    <button
      type="button"
      class="tab-button"
      aria-current={activeTab === 'pipeline' ? 'page' : undefined}
      onclick={() => onTabChange?.('pipeline')}
    >
      Pipeline
    </button>
  </nav>

  <div class="app-body">
    {@render children?.()}
  </div>
</main>

<style>
  .app {
    font-family: var(--font-family);
    padding: 16px;
    max-width: 1800px;
    margin: 0 auto;
    height: 100%;
    display: flex;
    flex-direction: column;
    min-height: 0;
    box-sizing: border-box;
  }

  .top-nav {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    flex-shrink: 0;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 16px;
    min-width: 0;
  }

  .app-logo {
    width: 210px;
    height: 60px;
    object-fit: contain;
  }

  .brand h1 {
    font-size: 18px;
    font-weight: 600;
    margin: 0;
    color: var(--color-text);
  }

  .nav-right {
    display: flex;
    align-items: center;
    gap: 16px;
    font-size: 12px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .nav-link {
    color: var(--text-link);
    text-decoration: none;
    font-size: 12px;
    min-height: 44px;
    display: inline-flex;
    align-items: center;
  }

  .nav-link:hover {
    text-decoration: underline;
  }

  .tab-bar {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--color-border);
    margin: 16px 0;
    flex-shrink: 0;
  }

  .tab-button {
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    min-width: 80px;
    min-height: 44px;
    padding: 8px 16px;
    color: var(--color-text-secondary);
    cursor: pointer;
    font-size: 12px;
  }

  .tab-button:hover {
    color: var(--color-text);
    background: var(--color-surface-hover);
  }

  .tab-button[aria-current='page'] {
    border-bottom-color: var(--focus);
    color: var(--color-info-text);
  }

  .app-body {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  @media (max-width: 960px) {
    .app { padding: 12px; }
    .top-nav, .brand { flex-wrap: wrap; }
  }
</style>
