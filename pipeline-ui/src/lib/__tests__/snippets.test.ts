import { describe, it, expect } from 'vitest'
import {
  SNIPPETS,
  extractOttlLines,
  buildProcessorFromOttlLines,
} from '../snippets'

describe('snippet templates', () => {
  it('has at least 5 snippets', () => {
    expect(SNIPPETS.length).toBeGreaterThanOrEqual(5)
  })

  it('each snippet has required fields', () => {
    for (const s of SNIPPETS) {
      expect(s.id).toBeTruthy()
      expect(s.name).toBeTruthy()
      expect(s.type).toMatch(/^(transform|filter)$/)
      expect(s.processor).toBeDefined()
      expect(s.description).toBeTruthy()
    }
  })

  it('each snippet has unique id', () => {
    const ids = SNIPPETS.map(s => s.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('transform snippets use trace_statements; filter snippets use drop conditions', () => {
    for (const s of SNIPPETS) {
      if (s.type === 'transform') {
        expect(s.processor.trace_statements).toBeDefined()
        expect(Array.isArray(s.processor.trace_statements)).toBe(true)
      } else {
        const traces = s.processor.traces as { span?: unknown[] }
        expect(Array.isArray(traces?.span)).toBe(true)
        expect((traces.span ?? []).length).toBeGreaterThan(0)
      }
    }
  })

  it('extract/build round-trip OTTL lines for transform and filter', () => {
    const transformLines = 'set(attributes["env"], "prod")'
    const filterLines = 'kind == SPAN_KIND_INTERNAL'
    const t = buildProcessorFromOttlLines('transform', 'traces', transformLines)
    const f = buildProcessorFromOttlLines('filter', 'traces', filterLines)
    expect(extractOttlLines('transform', t)).toBe(transformLines)
    expect(extractOttlLines('filter', f)).toBe(filterLines)
  })
})
