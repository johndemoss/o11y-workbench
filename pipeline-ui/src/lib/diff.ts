import type { StageDiff, DiffSpan, StageResult, SpanChange } from './types'

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
        const changes = computeSpanChanges(beforeSpan, afterSpan)
        modified.push({ span_id: id, status: 'modified', before: beforeSpan, after: afterSpan, changes })
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

export function computeAllStageDiffs(stages: StageResult[], originalData?: unknown): StageDiff[] {
  if (!stages || stages.length === 0) return []

  const diffs: StageDiff[] = []
  for (let i = 0; i < stages.length; i++) {
    const stage = stages[i]
    const before = i === 0 ? (originalData ?? null) : stages[i - 1].intermediate_data
    const after = stage.intermediate_data
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

  // Try resourceSpans / resource_spans structure (OTLP protojson uses snake_case)
  const resourceSpans = (root['resourceSpans'] || root['resource_spans']) as unknown
  if (Array.isArray(resourceSpans)) {
    for (const rs of resourceSpans) {
      const scopeSpans = ((rs as Record<string, unknown>)['scopeSpans'] || (rs as Record<string, unknown>)['scope_spans']) as unknown
      if (Array.isArray(scopeSpans)) {
        for (const ss of scopeSpans) {
          const items = (ss as Record<string, unknown>)['spans']
          if (Array.isArray(items)) {
            for (const span of items) {
              const s = span as Record<string, unknown>
              const spanId = (s['spanId'] || s['span_id']) as string | undefined
              if (spanId) {
                spans.push({ spanId, ...s } as { spanId: string; [key: string]: unknown })
              }
            }
          }
        }
      }
    }
  }

  // Try flat spans array
  if (spans.length === 0) {
    const flatSpans = root['spans'] as unknown
    if (Array.isArray(flatSpans)) {
      for (const span of flatSpans) {
        const s = span as Record<string, unknown>
        const spanId = (s['spanId'] || s['span_id']) as string | undefined
        if (spanId) {
          spans.push({ spanId, ...s } as { spanId: string; [key: string]: unknown })
        }
      }
    }
  }

  return spans
}

function countSpans(data: unknown): number {
  return extractSpans(data).length
}

export function computeSpanChanges(before: unknown, after: unknown): SpanChange[] {
  const changes: SpanChange[] = []
  if (!before || !after || typeof before !== 'object' || typeof after !== 'object') return changes

  const b = before as Record<string, unknown>
  const a = after as Record<string, unknown>

  // Compare attributes (they're arrays of {key, value} objects)
  const bAttrs = normalizeAttrs(b['attributes'])
  const aAttrs = normalizeAttrs(a['attributes'])

  for (const [key, aVal] of Object.entries(aAttrs)) {
    if (!(key in bAttrs)) {
      changes.push({ field: `attributes.${key}`, type: 'added', after: aVal })
    } else if (JSON.stringify(bAttrs[key]) !== JSON.stringify(aVal)) {
      changes.push({ field: `attributes.${key}`, type: 'changed', before: bAttrs[key], after: aVal })
    }
  }
  for (const [key, bVal] of Object.entries(bAttrs)) {
    if (!(key in aAttrs)) {
      changes.push({ field: `attributes.${key}`, type: 'removed', before: bVal })
    }
  }

  const scalarFields = ['name', 'kind', 'trace_id', 'traceId', 'parent_span_id', 'parentSpanId', 'start_time_unix_nano', 'end_time_unix_nano']
  for (const field of scalarFields) {
    if (JSON.stringify(b[field]) !== JSON.stringify(a[field])) {
      changes.push({ field, type: 'changed', before: b[field], after: a[field] })
    }
  }

  for (const arrField of ['events', 'links']) {
    const bArr = JSON.stringify(b[arrField])
    const aArr = JSON.stringify(a[arrField])
    if (bArr !== aArr) {
      changes.push({ field: arrField, type: 'changed', before: b[arrField], after: a[arrField] })
    }
  }

  return changes
}

function normalizeAttrs(attr: unknown): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  if (!attr || !Array.isArray(attr)) return result
  for (const item of attr) {
    const entry = item as Record<string, unknown>
    const key = entry['key'] as string | undefined
    if (key) {
      const val = entry['value'] ?? entry
      result[key] = val
    }
  }
  return result
}
