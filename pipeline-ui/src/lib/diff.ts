import type { StageDiff, DiffSpan, StageResult, SpanChange, FullAttributeDiff } from './types'

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
      added.push({ span_id: id, status: 'added', name: afterSpan['name'] as string | undefined, after: afterSpan })
    } else {
      if (JSON.stringify(beforeSpan) !== JSON.stringify(afterSpan)) {
        const changes = computeSpanChanges(beforeSpan, afterSpan)
        modified.push({ span_id: id, status: 'modified', name: (afterSpan['name'] || beforeSpan['name']) as string | undefined, before: beforeSpan, after: afterSpan, changes })
      }
    }
  }

  for (const [id, beforeSpan] of beforeMap) {
    if (!afterMap.has(id)) {
      removed.push({ span_id: id, status: 'removed', name: beforeSpan['name'] as string | undefined, before: beforeSpan })
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

export function base64ToHex(b64: string): string {
  try {
    const bytes = atob(b64)
    return Array.from(bytes, c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
  } catch {
    return b64
  }
}

export function isZeroId(id: string): boolean {
  try {
    const bytes = atob(id)
    return bytes.length > 0 && Array.from(bytes).every(c => c === '\0')
  } catch {
    return false
  }
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

export function computeFullAttributeDiff(before: unknown, after: unknown): FullAttributeDiff[] {
  const beforeAttrs = normalizeAttrs(
    before && typeof before === 'object' ? (before as Record<string, unknown>)['attributes'] : undefined
  )
  const afterAttrs = normalizeAttrs(
    after && typeof after === 'object' ? (after as Record<string, unknown>)['attributes'] : undefined
  )

  const allKeys = new Set([...Object.keys(beforeAttrs), ...Object.keys(afterAttrs)])

  const result: FullAttributeDiff[] = []
  for (const key of allKeys) {
    const hasBefore = key in beforeAttrs
    const hasAfter = key in afterAttrs

    if (!hasAfter) {
      result.push({ key, before: beforeAttrs[key], after: undefined, change_type: 'removed' })
    } else if (!hasBefore) {
      result.push({ key, before: undefined, after: afterAttrs[key], change_type: 'added' })
    } else if (JSON.stringify(beforeAttrs[key]) !== JSON.stringify(afterAttrs[key])) {
      result.push({ key, before: beforeAttrs[key], after: afterAttrs[key], change_type: 'changed' })
    } else {
      result.push({ key, before: beforeAttrs[key], after: afterAttrs[key], change_type: 'unchanged' })
    }
  }

  return result
}

export interface UnifiedDiffLine {
  type: 'context' | 'add' | 'remove'
  text: string
}

export function computeUnifiedDiff(before: unknown, after: unknown): UnifiedDiffLine[] {
  const beforeStr = JSON.stringify(before, null, 2)
  const afterStr = JSON.stringify(after, null, 2)

  if (beforeStr === afterStr) {
    return []
  }

  const beforeLines = beforeStr.split('\n')
  const afterLines = afterStr.split('\n')
  const m = beforeLines.length
  const n = afterLines.length

  // LCS table
  const dp: number[][] = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] = beforeLines[i - 1] === afterLines[j - 1]
        ? dp[i - 1][j - 1] + 1
        : Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }

  // Backtrack
  const reversed: UnifiedDiffLine[] = []
  let i = m, j = n
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && beforeLines[i - 1] === afterLines[j - 1]) {
      reversed.push({ type: 'context', text: beforeLines[i - 1] })
      i--; j--
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      reversed.push({ type: 'add', text: afterLines[j - 1] })
      j--
    } else {
      reversed.push({ type: 'remove', text: beforeLines[i - 1] })
      i--
    }
  }

  return reversed.reverse()
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
