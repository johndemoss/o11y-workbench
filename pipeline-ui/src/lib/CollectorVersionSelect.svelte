<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getCollectorVersion,
    getCollectorVersions,
    setCollectorVersion,
    type CollectorVersionInfo,
  } from './collector-version'

  let selected = $state('')
  let versions: CollectorVersionInfo[] = $state([])

  onMount(() => {
    versions = getCollectorVersions()
    selected = getCollectorVersion()
  })

  function onChange(e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value
    selected = v
    setCollectorVersion(v)
  }
</script>

<label class="collector-version">
  Collector
  <select value={selected} onchange={onChange} disabled={versions.length === 0}>
    {#each versions as v (v.contrib)}
      <option value={v.contrib}>{v.contrib}</option>
    {/each}
  </select>
</label>

<style>
  .collector-version {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--color-text-secondary, #94a3b8);
    font-size: var(--font-size-sm, 0.8rem);
  }
  select {
    padding: 0.25rem 0.4rem;
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-surface, #1e293b);
    color: var(--color-text, #f1f5f9);
    font-size: var(--font-size-sm, 0.8rem);
  }
</style>
