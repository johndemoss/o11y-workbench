<script lang="ts">
  let { added = 0, removed = 0, modified = 0 }: {
    added: number
    removed: number
    modified: number
  } = $props()
</script>

{#if added > 0 || removed > 0 || modified > 0}
  <div class="badge-row">
    {#if added > 0}
      <span class="badge added">+{added}</span>
    {/if}
    {#if removed > 0}
      <span class="badge removed">−{removed}</span>
    {/if}
    {#if modified > 0}
      <span class="badge modified">~{modified}</span>
    {/if}
  </div>
{/if}

<style>
  .badge-row {
    display: flex;
    gap: 4px;
    margin-top: 6px;
    flex-wrap: wrap;
  }
  .badge {
    font-size: 0.65rem;
    font-weight: 700;
    padding: 1px 6px;
    border-radius: 4px;
    font-family: monospace;
  }
  .added {
    background: rgba(34, 197, 94, 0.2);
    color: #22c55e;
    border: 1px solid rgba(34, 197, 94, 0.3);
  }
  .removed {
    background: rgba(239, 68, 68, 0.2);
    color: #ef4444;
    border: 1px solid rgba(239, 68, 68, 0.3);
  }
  .modified {
    background: rgba(245, 158, 11, 0.2);
    color: #f59e0b;
    border: 1px solid rgba(245, 158, 11, 0.3);
  }
</style>
