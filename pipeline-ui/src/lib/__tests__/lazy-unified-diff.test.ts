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
