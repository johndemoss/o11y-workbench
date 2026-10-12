<script lang="ts">
  import { BaseEdge, getStraightPath, type EdgeProps } from '@xyflow/svelte'

  /**
   * Straight edge that forces a horizontal midline.
   * Swimlane cards share a fixed height; any handle-measurement drift would
   * otherwise draw a diagonal "crooked" line between equal-rank nodes.
   */
  let {
    id,
    sourceX,
    sourceY,
    targetX,
    targetY,
    markerEnd,
    markerStart,
    style,
    interactionWidth,
  }: EdgeProps = $props()

  let path = $derived.by(() => {
    const y = Math.round((sourceY + targetY) / 2)
    const [d] = getStraightPath({
      sourceX,
      sourceY: y,
      targetX,
      targetY: y,
    })
    return d
  })
</script>

<BaseEdge
  {id}
  {path}
  {markerEnd}
  {markerStart}
  {style}
  {interactionWidth}
/>
