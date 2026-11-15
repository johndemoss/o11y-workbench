/**
 * Shared layout constants and helpers for pipeline graph rendering.
 * Extracted from SwimlaneFlowBuilder.svelte for reuse across components.
 */
import type { PipelinesBySignal } from './types'

// ─── Layout constants ────────────────────────────────────────────────

/** Width of a single stage node card in pixels. */
export const STAGE_WIDTH = 164

/** Minimum gap between stage nodes (matches reference edge slot). */
export const MIN_GAP = 48

/** Combined width of a stage + following gap (used for sequential positioning). */
export const STAGE_GAP_TOTAL = STAGE_WIDTH + MIN_GAP // 212

/** Default position of the receiver infra node. */
export const RECEIVER_DEFAULT_X = 20

/** Default width of the receiver infra node. */
export const RECEIVER_DEFAULT_WIDTH = 110

/** Right edge of the default receiver node. */
export const RECEIVER_RIGHT = RECEIVER_DEFAULT_X + RECEIVER_DEFAULT_WIDTH // 130

/** X position of the first stage node. */
export const FIRST_STAGE_X = RECEIVER_RIGHT + MIN_GAP // 178

/** Minimum width for a swimlane container. */
export const SWIMLANE_MIN_WIDTH = 640

/** Fixed card height — fits kind / name / meta without empty bottom padding. */
export const NODE_HEIGHT = 80

/** Midline for Left/Right connection handles (half of NODE_HEIGHT). */
export const HANDLE_TOP = NODE_HEIGHT / 2 // 40

/** Shared vertical origin for receiver/stage/exporter cards inside a swimlane. */
export const NODE_Y = 32

/** Swimlane height for a single row of NODE_HEIGHT cards + lane chrome. */
export const SWIMLANE_HEIGHT = 148

/** Right padding after the exporter card. */
export const SWIMLANE_RIGHT_PAD = 20

/**
 * Legacy constant kept for call sites that still add a fixed overhead.
 * Prefer swimlaneWidthForStageCount() — overhead is receiver + gaps + exporter + pads.
 */
export const SWIMLANE_EXTRA = RECEIVER_DEFAULT_WIDTH + MIN_GAP + RECEIVER_DEFAULT_WIDTH + SWIMLANE_RIGHT_PAD + RECEIVER_DEFAULT_X
// 20 + 110 + 48 + 110 + 20 = 308 when expressed as content without stages

/** X position of the exporter for a packed stage count (fixed 48px edge slots). */
export function exporterXForStageCount(stageCount: number): number {
  return stageCount > 0
    ? FIRST_STAGE_X + (stageCount - 1) * STAGE_GAP_TOTAL + STAGE_WIDTH + MIN_GAP
    : FIRST_STAGE_X
}

/** Swimlane width that packs nodes tightly; min 640 leaves slack after the exporter. */
export function swimlaneWidthForStageCount(stageCount: number): number {
  return Math.max(SWIMLANE_MIN_WIDTH, exporterXForStageCount(stageCount) + RECEIVER_DEFAULT_WIDTH + SWIMLANE_RIGHT_PAD)
}

// ─── Helpers ──────────────────────────────────────────────────────────

/** Fingerprint the pipeline topology that requires rebuilding the flow. */
export function getPipelinesTopologyHash(p: PipelinesBySignal): string {
  return JSON.stringify(
    (['traces', 'logs', 'metrics'] as const).map((signal) => {
      const pipeline = p[signal]
      return pipeline && {
        stageIds: pipeline.stages.map((stage) => stage.id),
        receivers: pipeline.receivers ?? ['otlp'],
        exporters: pipeline.exporters ?? ['debug'],
      }
    }),
  )
}
