import { describe, it, expect, vi } from 'vitest'
import { buildStageUnifiedDiffsLazy, computeUnifiedDiff, type UnifiedDiffLine } from '../diff'

const stages = [
  { intermediate_data: { a: 1 } },
  { intermediate_data: { a: 2 } },
  { intermediate_data: { a: 3 } },
]
const originalData = { a: 0 }

function payloads(stageIndex: number) {
  const before = stageIndex === 0 ? originalData : stages[stageIndex - 1].intermediate_data
  const after = stages[stageIndex].intermediate_data
  return { before, after }
}

describe('buildStageUnifiedDiffsLazy', () => {
  it('does not call compute on mount-style access (collapsed / inline)', () => {
    const compute = vi.fn((): UnifiedDiffLine[] => [{ type: 'add', text: 'x' }])
    const cache = buildStageUnifiedDiffsLazy(compute)

    for (let i = 0; i < stages.length; i++) {
      const { before, after } = payloads(i)
      expect(cache.get(i, before, after, { expanded: false, unified: false })).toEqual([])
      expect(cache.get(i, before, after, { expanded: true, unified: false })).toEqual([])
      expect(cache.get(i, before, after, { expanded: false, unified: true })).toEqual([])
    }

    expect(compute).toHaveBeenCalledTimes(0)
  })

  it('computes only the expanded unified stage; caches same ref', () => {
    const compute = vi.fn((_b: unknown, _a: unknown): UnifiedDiffLine[] => [
      { type: 'add', text: 'line' },
    ])
    const cache = buildStageUnifiedDiffsLazy(compute)

    // simulate mount over all stages (no expand / not unified)
    for (let i = 0; i < stages.length; i++) {
      const { before, after } = payloads(i)
      cache.get(i, before, after, { expanded: false, unified: false })
    }
    expect(compute).toHaveBeenCalledTimes(0)

    const { before, after } = payloads(1)
    const first = cache.get(1, before, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(1)
    expect(compute).toHaveBeenCalledWith(before, after)

    const second = cache.get(1, before, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(1)
    expect(second).toBe(first)

    // other stages still untouched
    const p0 = payloads(0)
    cache.get(0, p0.before, p0.after, { expanded: false, unified: true })
    expect(compute).toHaveBeenCalledTimes(1)
  })

  it('clears cache when result identity changes', () => {
    const compute = vi.fn((): UnifiedDiffLine[] => [{ type: 'remove', text: 'y' }])
    const cache = buildStageUnifiedDiffsLazy(compute)
    const { before, after } = payloads(0)

    cache.get(0, before, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(1)

    cache.clear()
    cache.get(0, before, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(2)
  })

  it('DiffPanel Show Diff expand + unified triggers compute without stage-header expand', () => {
    // ResultsView maps DiffPanel onExpandChange → opts.expanded (not expandedStages)
    const compute = vi.fn((): UnifiedDiffLine[] => [{ type: 'add', text: 'diff' }])
    const cache = buildStageUnifiedDiffsLazy(compute)

    // mount / result present: DiffPanels collapsed, even if global view is already unified
    for (let i = 0; i < stages.length; i++) {
      const { before, after } = payloads(i)
      cache.get(i, before, after, { expanded: false, unified: true })
    }
    expect(compute).toHaveBeenCalledTimes(0)

    // user opens Show Diff on stage 0 while unified → LCS for that stage only
    const { before, after } = payloads(0)
    cache.get(0, before, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(1)
    expect(compute).toHaveBeenCalledWith(before, after)
  })

  it('invalidates a stage entry when either payload changes', () => {
    const compute = vi.fn((): UnifiedDiffLine[] => [])
    const cache = buildStageUnifiedDiffsLazy(compute)
    const before = { a: 1 }
    const after = { a: 2 }
    cache.get(0, before, after, { expanded: true, unified: true })
    cache.get(0, before, { a: 3 }, { expanded: true, unified: true })
    cache.get(0, { a: 0 }, after, { expanded: true, unified: true })
    expect(compute).toHaveBeenCalledTimes(3)
  })

  it('bounds large comparisons without hiding changed lines', () => {
    const before = Array.from({ length: 1100 }, (_, i) => `old-${i}`)
    const after = Array.from({ length: 1100 }, (_, i) => `new-${i}`)
    const lines = computeUnifiedDiff(before, after)
    expect(lines.some(line => line.type === 'remove' && line.text.includes('old-500'))).toBe(true)
    expect(lines.some(line => line.type === 'add' && line.text.includes('new-500'))).toBe(true)
    expect(lines.some(line => line.type === 'context' && line.text.includes('old-500'))).toBe(false)
  })
})

/** Sizes of the maximal runs of consecutive non-context lines. */
function changedHunks(lines: UnifiedDiffLine[]): number[] {
  const sizes: number[] = []
  let current = 0
  for (const line of lines) {
    if (line.type === 'context') {
      if (current > 0) sizes.push(current)
      current = 0
    } else {
      current++
    }
  }
  if (current > 0) sizes.push(current)
  return sizes
}

/** A diff is faithful when remove+context replays before, add+context replays after. */
function expectDiffFaithful(before: unknown, after: unknown, lines: UnifiedDiffLine[]) {
  const beforeLines = JSON.stringify(before, null, 2).split('\n')
  const afterLines = JSON.stringify(after, null, 2).split('\n')
  expect(lines.filter(line => line.type !== 'add').map(line => line.text)).toEqual(beforeLines)
  expect(lines.filter(line => line.type !== 'remove').map(line => line.text)).toEqual(afterLines)
}

function structuredRecord(index: number, extraAttribute = false) {
  return {
    resource: {
      attributes: {
        'service.name': `svc-${index % 7}`,
        'host.name': `host-${String(index).padStart(4, '0')}`,
        'telemetry.sdk.language': 'nodejs',
      },
    },
    spans: [
      {
        traceId: `trace-${String(index).padStart(4, '0')}`,
        spanId: `span-${String(index).padStart(4, '0')}`,
        parentSpanId: `parent-${String(index - 1).padStart(4, '0')}`,
        name: `GET /api/v1/orders/${index % 13}`,
        kind: 'SERVER',
        startTimeUnixNano: 1700000000000000000 + index,
        endTimeUnixNano: 1700000000001000000 + index,
        attributes: {
          'http.method': 'GET',
          'http.status_code': 200,
          'net.peer.ip': `10.0.${index % 255}.${(index * 7) % 255}`,
          ...(extraAttribute ? { 'http.route': '/api/v1/orders' } : {}),
        },
        events: [
          { name: 'cache-miss', time: 1700000000000500000 + index },
          { name: 'db-query', time: 1700000000000600000 + index },
        ],
      },
    ],
  }
}

/** Fan-out shaped payload: repeated records whose ids make the anchors unique. */
function structuredPayload(
  count: number,
  opts: { extraAttributeAt?: number[]; skipAt?: number } = {},
) {
  const records: unknown[] = []
  for (let index = 0; index < count; index++) {
    if (opts.skipAt === index) continue
    records.push(structuredRecord(index, opts.extraAttributeAt?.includes(index) ?? false))
  }
  return records
}

describe('computeUnifiedDiff on multi-record payloads', () => {
  it('keeps a single attribute insert in a >1500-line payload surgical', () => {
    const before = structuredPayload(120)
    const after = structuredPayload(120, { extraAttributeAt: [73] })
    expect(JSON.stringify(before, null, 2).split('\n').length).toBeGreaterThan(1500)
    expect(JSON.stringify(after, null, 2).split('\n').length).toBeGreaterThan(1500)

    const lines = computeUnifiedDiff(before, after)
    expectDiffFaithful(before, after, lines)

    const hunks = changedHunks(lines)
    const changed = lines.filter(line => line.type !== 'context')
    expect(changed.length).toBeLessThan(40)
    expect(hunks.length).toBeGreaterThan(0)
    expect(Math.max(...hunks)).toBeLessThanOrEqual(12)
    expect(lines.some(line => line.type === 'add' && line.text.includes('"http.route"'))).toBe(true)
    // Neighbouring records stay untouched context.
    expect(lines.some(line => line.type === 'context' && line.text.includes('host-0074'))).toBe(true)
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0072'))).toBe(true)
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0075'))).toBe(true)
  })

  it('localizes scattered attribute inserts through the anchor path', () => {
    // Real fan-out captures differ in several places at once, so common-edge
    // trimming cannot isolate them. The pre-anchor fallback replaced the whole
    // middle (~2800 lines, one enormous hunk); anchors must split it.
    const before = structuredPayload(120)
    const after = structuredPayload(120, { extraAttributeAt: [20, 60, 100] })
    expect(JSON.stringify(before, null, 2).split('\n').length).toBeGreaterThan(1500)

    const lines = computeUnifiedDiff(before, after)
    expectDiffFaithful(before, after, lines)

    const hunks = changedHunks(lines)
    const changed = lines.filter(line => line.type !== 'context')
    const adds = lines.filter(line => line.type === 'add' && line.text.includes('"http.route"'))
    expect(adds.length).toBe(3)
    expect(changed.length).toBeLessThan(40)
    expect(Math.max(...hunks)).toBeLessThanOrEqual(12)
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0073'))).toBe(true)
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0021'))).toBe(true)
  })

  it('limits a deleted record block to that block', () => {
    const before = structuredPayload(120)
    const after = structuredPayload(120, { skipAt: 60 })
    expect(JSON.stringify(before, null, 2).split('\n').length).toBeGreaterThan(1500)

    const lines = computeUnifiedDiff(before, after)
    expectDiffFaithful(before, after, lines)

    const removed = lines.filter(line => line.type === 'remove')
    const added = lines.filter(line => line.type === 'add')
    expect(added.length).toBe(0)
    expect(removed.length).toBeGreaterThan(15)
    expect(removed.length).toBeLessThan(45)

    const removedText = removed.map(line => line.text).join('\n')
    expect(removedText).toContain('trace-0060')
    expect(removedText).not.toContain('trace-0059')
    expect(removedText).not.toContain('trace-0061')
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0061'))).toBe(true)
    expect(lines.some(line => line.type === 'context' && line.text.includes('trace-0059'))).toBe(true)
  })

  it('keeps exact LCS ordering for small inputs', () => {
    const before = { a: 'one', b: 2 }
    const after = { a: 'two', b: 2 }
    expect(computeUnifiedDiff(before, after)).toEqual([
      { type: 'context', text: '{' },
      { type: 'remove', text: '  "a": "one",' },
      { type: 'add', text: '  "a": "two",' },
      { type: 'context', text: '  "b": 2' },
      { type: 'context', text: '}' },
    ])
  })
})

describe('computeUnifiedDiff on pathological large inputs', () => {
  it('returns no diff for identical 5000-line payloads', () => {
    const identical = Array.from({ length: 5000 }, () => '  "dup",')
    expect(computeUnifiedDiff(identical, [...identical])).toEqual([])
  })

  it('does not blow up on 5000 identical lines with one changed line', () => {
    const before = Array.from({ length: 5000 }, () => '  "dup",')
    const after = [...before]
    after[2500] = '  "other",'

    const started = performance.now()
    const lines = computeUnifiedDiff(before, after)
    const elapsed = performance.now() - started

    expect(elapsed).toBeLessThan(4000)
    expectDiffFaithful(before, after, lines)
    const changed = lines.filter(line => line.type !== 'context')
    expect(changed.length).toBe(2)
    expect(changedHunks(lines)).toEqual([2])
    expect(lines.filter(line => line.type === 'context').length).toBe(
      JSON.stringify(before, null, 2).split('\n').length - 1,
    )
  })

  it('completes on thousands of uniquely ordered but reversed lines', () => {
    const before = Array.from({ length: 3000 }, (_, index) => `line-${index}`)
    const after = [...before].reverse()

    const started = performance.now()
    const lines = computeUnifiedDiff(before, after)
    const elapsed = performance.now() - started

    expect(elapsed).toBeLessThan(4000)
    expectDiffFaithful(before, after, lines)
  })
})
