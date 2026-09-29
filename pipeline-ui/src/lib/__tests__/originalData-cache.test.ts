import { describe, it, expect } from 'vitest'

/**
 * Pure mirror of App.handleExecuteResult originalData selection (Item 5B):
 * use select-time cache / execute arg; never post-execute Get.
 */
function resolveOriginalData(
  result: unknown | null,
  originalTrace: unknown | undefined,
  selectedRunTraceData: unknown,
): unknown {
  if (!result) return null
  return originalTrace !== undefined ? originalTrace : selectedRunTraceData
}

describe('resolveOriginalData (Item 5B select-time cache)', () => {
  const trace = { resourceSpans: [{ spans: 3 }] }

  it('uses execute-passed cache hit for stage-0 baseline', () => {
    expect(resolveOriginalData({ stages: [] }, trace, null)).toBe(trace)
  })

  it('falls back to App selectedRunTraceData when 4th arg omitted', () => {
    expect(resolveOriginalData({ stages: [] }, undefined, trace)).toBe(trace)
  })

  it('does not invent a Get — null cache stays null', () => {
    expect(resolveOriginalData({ stages: [] }, null, null)).toBe(null)
  })

  it('leaves originalData untouched path when result is null', () => {
    expect(resolveOriginalData(null, trace, trace)).toBe(null)
  })
})
