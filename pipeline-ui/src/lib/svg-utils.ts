/**
 * Shared layout constants and helpers for pipeline graph rendering.
 * Extracted from SwimlaneFlowBuilder.svelte for reuse across components.
 */
import type { PipelinesBySignal } from './types'

// ─── Layout constants ────────────────────────────────────────────────

/** Width of a single stage node card in pixels. */
export const STAGE_WIDTH = 160

/** Minimum gap between stage nodes. */
export const MIN_GAP = 50

/** Combined width of a stage + following gap (used for sequential positioning). */
export const STAGE_GAP_TOTAL = STAGE_WIDTH + MIN_GAP // 210

/** Default position of the receiver infra node. */
export const RECEIVER_DEFAULT_X = 20

/** Default width of the receiver infra node. */
export const RECEIVER_DEFAULT_WIDTH = 160

/** Right edge of the default receiver node. */
export const RECEIVER_RIGHT = RECEIVER_DEFAULT_X + RECEIVER_DEFAULT_WIDTH // 180

/** X position of the first stage node. */
export const FIRST_STAGE_X = RECEIVER_RIGHT + MIN_GAP // 230

/** Minimum width for a swimlane container. */
export const SWIMLANE_MIN_WIDTH = 640

/** Extra padding for swimlane width calculation (receiver + exporter space + gaps). */
export const SWIMLANE_EXTRA = 430

// ─── Helpers ──────────────────────────────────────────────────────────

/**
 * Compute a deterministic hash of pipeline structure for change detection.
 * Compares stage IDs and processor keys rather than full deep equality.
 */
export function getPipelinesHash(p: PipelinesBySignal): string {
  return JSON.stringify({
    traces: {
      stageIds: p.traces?.stages?.map((s) => s.id) ?? [],
      processorKeys: Object.keys(p.traces?.processors ?? {}),
    },
    logs: {
      stageIds: p.logs?.stages?.map((s) => s.id) ?? [],
      processorKeys: Object.keys(p.logs?.processors ?? {}),
    },
    metrics: {
      stageIds: p.metrics?.stages?.map((s) => s.id) ?? [],
      processorKeys: Object.keys(p.metrics?.processors ?? {}),
    },
  })
}
