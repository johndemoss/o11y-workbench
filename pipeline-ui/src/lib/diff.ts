import type { FullAttributeDiff } from './types'

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

  // Bound quadratic work. For large inputs retain exact common edges and
  // represent the whole middle as removals/additions rather than guessing.
  if ((m + 1) * (n + 1) > 1_000_000) {
    let prefix = 0
    while (prefix < m && prefix < n && beforeLines[prefix] === afterLines[prefix]) prefix++
    let suffix = 0
    while (suffix < m - prefix && suffix < n - prefix &&
      beforeLines[m - 1 - suffix] === afterLines[n - 1 - suffix]) suffix++
    return [
      ...beforeLines.slice(0, prefix).map(text => ({ type: 'context' as const, text })),
      ...beforeLines.slice(prefix, m - suffix).map(text => ({ type: 'remove' as const, text })),
      ...afterLines.slice(prefix, n - suffix).map(text => ({ type: 'add' as const, text })),
      ...beforeLines.slice(m - suffix).map(text => ({ type: 'context' as const, text })),
    ]
  }

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

/** Lazy unified-diff cache: LCS runs only when expanded + unified. */
export function buildStageUnifiedDiffsLazy(
  compute: (before: unknown, after: unknown) => UnifiedDiffLine[] = computeUnifiedDiff,
) {
  const cache = new Map<number, { before: unknown; after: unknown; lines: UnifiedDiffLine[] }>()
  return {
    clear() {
      cache.clear()
    },
    get(
      stageIndex: number,
      before: unknown,
      after: unknown,
      opts: { expanded: boolean; unified: boolean },
    ): UnifiedDiffLine[] {
      if (!opts.expanded || !opts.unified) return []
      const hit = cache.get(stageIndex)
      if (hit !== undefined && hit.before === before && hit.after === after) return hit.lines
      const lines = compute(before, after)
      cache.set(stageIndex, { before, after, lines })
      return lines
    },
  }
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

export function formatValue(val: unknown): string {
  if (val === null || val === undefined) return 'null'
  if (typeof val === 'string') return `"${val}"`
  if (typeof val === 'object') return JSON.stringify(val)
  return String(val)
}

export function highlightJSON(str: string): string {
  const escaped = str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.replace(
    /("(?:[^"\\]|\\.)*")\s*:|("(?:[^"\\]|\\.)*")|(\btrue\b|\bfalse\b)|(\bnull\b)|(-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b)/g,
    (_m, key, str, bool, nullVal, num) => {
      if (key) return `<span class="json-key">${key}</span>:`
      if (str) return `<span class="json-string">${str}</span>`
      if (bool) return `<span class="json-bool">${bool}</span>`
      if (nullVal) return `<span class="json-null">${nullVal}</span>`
      if (num) return `<span class="json-number">${num}</span>`
      return _m
    }
  )
}

export function formatJSON(val: unknown): string {
  return JSON.stringify(val, null, 2)
}
