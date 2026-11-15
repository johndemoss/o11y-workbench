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

/** Largest LCS table (in cells) the exact walk is allowed to build. */
const LCS_CELL_BOUND = 1_000_000

interface DiffRange {
  a0: number
  a1: number
  b0: number
  b1: number
}

interface DiffAnchor {
  a: number
  b: number
}

type DiffTask = { kind: 'range'; range: DiffRange } | { kind: 'context'; text: string }

/**
 * Unified line diff. Ranges small enough for the exact LCS table use it
 * directly, so small payloads keep their historical output byte for byte.
 * Larger ranges split on patience anchors (lines unique to both sides, in
 * order) and every gap between anchors is diffed the same way down to the LCS
 * leaves. A range with no usable anchors — or one that outlives the scan
 * budget for adversarial reorderings — degrades to common-edge trim + whole
 * range replace, which is the old fallback applied per range instead of to the
 * whole payload.
 */
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

  if ((m + 1) * (n + 1) <= LCS_CELL_BOUND) {
    return lcsDiff(beforeLines, 0, m, afterLines, 0, n)
  }

  const out: UnifiedDiffLine[] = []
  const stack: DiffTask[] = [{ kind: 'range', range: { a0: 0, a1: m, b0: 0, b1: n } }]
  // Anchor scanning costs O(range), so cap the total scanned lines. Only
  // pathological orderings ever reach the cap; earlier ranges already split.
  let budget = 16 * (m + n) + 20_000

  while (stack.length > 0) {
    const task = stack.pop() as DiffTask
    if (task.kind === 'context') {
      out.push({ type: 'context', text: task.text })
      continue
    }

    const { a0, a1, b0, b1 } = task.range
    const aLen = a1 - a0
    const bLen = b1 - b0
    if (aLen === 0) {
      for (let j = b0; j < b1; j++) out.push({ type: 'add', text: afterLines[j] })
      continue
    }
    if (bLen === 0) {
      for (let i = a0; i < a1; i++) out.push({ type: 'remove', text: beforeLines[i] })
      continue
    }
    if ((aLen + 1) * (bLen + 1) <= LCS_CELL_BOUND) {
      for (const line of lcsDiff(beforeLines, a0, a1, afterLines, b0, b1)) out.push(line)
      continue
    }

    let anchors: DiffAnchor[] = []
    if (budget >= aLen + bLen) {
      budget -= aLen + bLen
      anchors = findAnchors(beforeLines, a0, a1, afterLines, b0, b1)
    }

    if (anchors.length === 0) {
      // No anchors: retain exact common edges, replace the middle.
      let aStart = a0
      let bStart = b0
      let aEnd = a1
      let bEnd = b1
      while (aStart < aEnd && bStart < bEnd && beforeLines[aStart] === afterLines[bStart]) {
        out.push({ type: 'context', text: beforeLines[aStart] })
        aStart++
        bStart++
      }
      while (aEnd > aStart && bEnd > bStart && beforeLines[aEnd - 1] === afterLines[bEnd - 1]) {
        aEnd--
        bEnd--
      }
      for (let i = aStart; i < aEnd; i++) out.push({ type: 'remove', text: beforeLines[i] })
      for (let j = bStart; j < bEnd; j++) out.push({ type: 'add', text: afterLines[j] })
      for (let i = aEnd; i < a1; i++) out.push({ type: 'context', text: beforeLines[i] })
      continue
    }

    const middle: DiffTask[] = []
    let prevA = a0 - 1
    let prevB = b0 - 1
    for (const anchor of anchors) {
      if (anchor.a > prevA + 1 || anchor.b > prevB + 1) {
        middle.push({
          kind: 'range',
          range: { a0: prevA + 1, a1: anchor.a, b0: prevB + 1, b1: anchor.b },
        })
      }
      middle.push({ kind: 'context', text: beforeLines[anchor.a] })
      prevA = anchor.a
      prevB = anchor.b
    }
    if (prevA + 1 < a1 || prevB + 1 < b1) {
      middle.push({ kind: 'range', range: { a0: prevA + 1, a1, b0: prevB + 1, b1 } })
    }
    for (let t = middle.length - 1; t >= 0; t--) stack.push(middle[t])
  }

  return out
}

/** Exact LCS walk over one line range; the terminal handler for small ranges. */
function lcsDiff(
  beforeLines: string[],
  a0: number,
  a1: number,
  afterLines: string[],
  b0: number,
  b1: number,
): UnifiedDiffLine[] {
  const m = a1 - a0
  const n = b1 - b0

  // LCS table
  const dp: number[][] = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] = beforeLines[a0 + i - 1] === afterLines[b0 + j - 1]
        ? dp[i - 1][j - 1] + 1
        : Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }

  // Backtrack
  const reversed: UnifiedDiffLine[] = []
  let i = m, j = n
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && beforeLines[a0 + i - 1] === afterLines[b0 + j - 1]) {
      reversed.push({ type: 'context', text: beforeLines[a0 + i - 1] })
      i--; j--
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      reversed.push({ type: 'add', text: afterLines[b0 + j - 1] })
      j--
    } else {
      reversed.push({ type: 'remove', text: beforeLines[a0 + i - 1] })
      i--
    }
  }

  return reversed.reverse()
}

/**
 * Patience anchors for one range: lines occurring exactly once on each side,
 * reduced to the longest strictly increasing run of after-positions. The line
 * text is the hash key; Map lookups hash it.
 */
function findAnchors(
  beforeLines: string[],
  a0: number,
  a1: number,
  afterLines: string[],
  b0: number,
  b1: number,
): DiffAnchor[] {
  const countBefore = new Map<string, number>()
  for (let i = a0; i < a1; i++) {
    const line = beforeLines[i]
    countBefore.set(line, (countBefore.get(line) ?? 0) + 1)
  }

  const countAfter = new Map<string, number>()
  const afterIndex = new Map<string, number>()
  for (let j = b0; j < b1; j++) {
    const line = afterLines[j]
    countAfter.set(line, (countAfter.get(line) ?? 0) + 1)
    afterIndex.set(line, j)
  }

  const pairs: DiffAnchor[] = []
  for (let i = a0; i < a1; i++) {
    const line = beforeLines[i]
    if (countBefore.get(line) !== 1 || countAfter.get(line) !== 1) continue
    pairs.push({ a: i, b: afterIndex.get(line) as number })
  }
  if (pairs.length === 0) return []

  // Longest strictly increasing subsequence of after-positions (patience
  // sorting: binary-search each pair into the tail of the best run).
  const tails: number[] = []
  const previous: number[] = new Array(pairs.length).fill(-1)
  for (let k = 0; k < pairs.length; k++) {
    let lo = 0
    let hi = tails.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (pairs[tails[mid]].b < pairs[k].b) lo = mid + 1
      else hi = mid
    }
    if (lo > 0) previous[k] = tails[lo - 1]
    tails[lo] = k
  }

  const anchors: DiffAnchor[] = []
  let k = tails[tails.length - 1]
  while (k >= 0) {
    anchors.push(pairs[k])
    k = previous[k]
  }
  return anchors.reverse()
}

/** Lazy unified-diff cache: the diff runs only when expanded + unified. */
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
