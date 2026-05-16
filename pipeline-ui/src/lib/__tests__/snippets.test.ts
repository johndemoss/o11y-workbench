import { describe, it, expect } from 'vitest'
import { SNIPPETS } from '../snippets'

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

  it('each snippet has trace_statements in processor', () => {
    for (const s of SNIPPETS) {
      expect(s.processor.trace_statements).toBeDefined()
      expect(Array.isArray(s.processor.trace_statements)).toBe(true)
    }
  })
})
