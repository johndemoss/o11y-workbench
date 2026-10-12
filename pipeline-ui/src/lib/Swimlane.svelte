<script lang="ts">
  import type { SignalType } from './types'
  import { SIGNAL_COLORS } from './types'

  let {
    signal,
    label,
    children,
  }: {
    signal: SignalType
    label: string
    children?: import('svelte').Snippet
  } = $props()

  const colors = $derived(SIGNAL_COLORS[signal])
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="swimlane swimlane--{signal}"
  role="region"
  aria-label="{label} pipeline"
  style="background: {colors.bg}; border: 1px dashed {colors.border};"
>
  <div class="swimlane-tag" style="background: {colors.tag}; color: #000;">
    {label}
  </div>
  {#if children}
    {@render children()}
  {/if}
</div>

<style>
  .swimlane {
    position: relative;
    border-radius: 8px;
    padding: 28px 16px 16px;
    min-height: 148px;
    width: 100%;
    box-sizing: border-box;
  }

  .swimlane-tag {
    position: absolute;
    top: 0;
    left: 12px;
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 2px 8px;
    border-radius: 0 0 4px 4px;
    z-index: 1;
  }
</style>
