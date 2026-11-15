import { describe, it, expect } from 'vitest'
import type { StageDiff } from '../types'

/** Mirrors ResultsView per-stage diff POST body shape. */
export function buildStageDiffRequest(
  stageIndex: number,
  before: unknown,
  after: unknown,
): { stage_index: number; before_data: unknown; after_data: unknown } {
  return { stage_index: stageIndex, before_data: before, after_data: after }
}

describe('per-stage diff transport', () => {
  it('sends only one stage before/after payload', () => {
    const req = buildStageDiffRequest(1, { input: true }, { output: true })
    expect(req).toEqual({
      stage_index: 1,
      before_data: { input: true },
      after_data: { output: true },
    })
    expect(Object.keys(req)).toEqual(['stage_index', 'before_data', 'after_data'])
  })

  it('expects a single StageDiff response object (not an array)', () => {
    const body: StageDiff = {
      stage_index: 0,
      signal: 'traces',
      input_count: 1,
      output_count: 1,
      added: [],
      removed: [],
      modified: [],
    }
    expect(Array.isArray(body)).toBe(false)
    expect(body.stage_index).toBe(0)
  })
})