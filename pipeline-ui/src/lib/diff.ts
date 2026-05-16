import type { StageDiff, DiffSpan, StageResult } from './types'

export function computeStageDiff(before: unknown, after: unknown, stageIndex: number): StageDiff {
  if (!before || !after) {
    const inputSpanCount = before ? countSpans(before) : 0
    const outputSpanCount = after ? countSpans(after) : 0
    return {
      stage_index: stageIndex,
      input_span_count: inputSpanCount,
      output_span_count: outputSpanCount,
      added: [],
      removed: [],
      modified: []
    }
  }

  const beforeSpans = extractSpans(before)
  const afterSpans = extractSpans(after)

  const beforeMap = new Map(beforeSpans.map(s => [s.spanId, s]))
  const afterMap = new Map(afterSpans.map(s => [s.spanId, s]))

  const added: DiffSpan[] = []
  const removed: DiffSpan[] = []
  const modified: DiffSpan[] = []

  for (const [id, afterSpan] of afterMap) {
    const beforeSpan = beforeMap.get(id)
    if (!beforeSpan) {
      added.push({ span_id: id, status: 'added', after: afterSpan })
    } else {
      if (JSON.stringify(beforeSpan) !== JSON.stringify(afterSpan)) {
        modified.push({ span_id: id, status: 'modified', before: beforeSpan, after: afterSpan })
      }
    }
  }

  for (const [id, beforeSpan] of beforeMap) {
    if (!afterMap.has(id)) {
      removed.push({ span_id: id, status: 'removed', before: beforeSpan })
    }
  }

  return {
    stage_index: stageIndex,
    input_span_count: beforeSpans.length,
    output_span_count: afterSpans.length,
    added,
    removed,
    modified
  }
}

export function computeAllStageDiffs(stages: StageResult[]): StageDiff[] {
  if (!stages || stages.length === 0) return []

  const diffs: StageDiff[] = []
  for (let i = 0; i < stages.length; i++) {
    const stage = stages[i]
    const before = stage.intermediate_data
    const after = i < stages.length - 1 ? stages[i + 1].intermediate_data : stages[i]
    const diff = computeStageDiff(before, after, stage.stage_index)
    diffs.push(diff)
  }
  return diffs
}

function extractSpans(data: unknown): Array<{ spanId: string; [key: string]: unknown }> {
  // Handle scope-spans (nested in resourceSpans → scopeSpans → spans)
  const spans: Array<{ spanId: string; [key: string]: unknown }> = []

  if (!data || typeof data !== 'object') return spans

  const root = data as Record<string, unknown>

  // Try resourceSpans structure
  const resourceSpans = root['resourceSpans']
  if (Array.isArray(resourceSpans)) {
    for (const rs of resourceSpans) {
      const scopeSpans = (rs as Record<string, unknown>)['scopeSpans']
      if (Array.isArray(scopeSpans)) {
        for (const ss of scopeSpans) {
          const items = (ss as Record<string, unknown>)['spans']
          if (Array.isArray(items)) {
            for (const span of items) {
              const s = span as Record<string, unknown>
              if (s['spanId']) {
                spans.push(s as { spanId: string; [key: string]: unknown })
              }
            }
          }
        }
      }
    }
  }

  // Try flat spans array
  if (spans.length === 0 && Array.isArray(root['spans'])) {
    for (const span of root['spans']) {
      const s = span as Record<string, unknown>
      if (s['spanId']) {
        spans.push(s as { spanId: string; [key: string]: unknown })
      }
    }
  }

  return spans
}

function countSpans(data: unknown): number {
  return extractSpans(data).length
}
