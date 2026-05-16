<script lang="ts">
  import type { NodeProps, Node } from '@xyflow/svelte'
  import Swimlane from './Swimlane.svelte'
  import type { SignalType } from './types'

  let { data }: NodeProps<Node<{ signal: SignalType; label: string }>> = $props()
</script>

<Swimlane signal={data.signal} label={data.label} />

<style>
  :global(.swimlane) {
    width: 100%;
    height: 100%;
    box-sizing: border-box;
  }
</style>
