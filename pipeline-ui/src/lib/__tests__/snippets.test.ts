import { describe, it, expect } from 'vitest'
import {
  SNIPPETS,
  extractOttlLines,
  buildProcessorFromOttlLines,
  getSnippetForSignal,
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

  it('converts generic transforms to each lane without changing their statements', () => {
    const ids = ['set-env-attribute', 'rename-attribute', 'filter-production-tags-lambda']
    const lanes = [
      { signal: 'traces', key: 'trace_statements', context: 'span' },
      { signal: 'logs', key: 'log_statements', context: 'log' },
      { signal: 'metrics', key: 'metric_statements', context: 'datapoint' },
    ] as const
    for (const id of ids) {
      const snippet = SNIPPETS.find(s => s.id === id)!
      const originalBlocks = snippet.processor.trace_statements as Array<{ statements: string[] }>
      for (const { signal, key, context } of lanes) {
        const converted = getSnippetForSignal(snippet, signal)!
        const blocks = converted.processor[key] as Array<{ context: string; statements: string[] }>
        expect(blocks.map(block => block.context)).toEqual(originalBlocks.map(() => context))
        expect(blocks.map(block => block.statements)).toEqual(originalBlocks.map(block => block.statements))
        expect(converted.processor.error_mode).toBe(snippet.processor.error_mode)
        for (const other of lanes.filter(lane => lane.key !== key)) {
          expect(converted.processor).not.toHaveProperty(other.key)
        }
      }
    }
  })

  it('retains resource context and error mode in every lane', () => {
    const snippet = SNIPPETS.find(s => s.id === 'add-service-version')!
    for (const key of ['trace_statements', 'log_statements', 'metric_statements'] as const) {
      const signal = key === 'trace_statements' ? 'traces' : key === 'log_statements' ? 'logs' : 'metrics'
      const processor = getSnippetForSignal(snippet, signal)!.processor
      expect(processor.error_mode).toBe('silent')
      expect(processor[key]).toEqual(snippet.processor.trace_statements)
    }
  })

  it('rejects trace-specific filters for logs and metrics', () => {
    for (const id of ['drop-internal-spans', 'drop-by-status']) {
      const snippet = SNIPPETS.find(s => s.id === id)!
      expect(getSnippetForSignal(snippet, 'logs')).toBeNull()
      expect(getSnippetForSignal(snippet, 'metrics')).toBeNull()
      expect(getSnippetForSignal(snippet, 'traces')!.processor).toEqual(snippet.processor)
    }
  })

  it('preserves options and independently copies the template and all statement blocks', () => {
    const snippet = SNIPPETS.find(s => s.id === 'rename-attribute')!
    const original = structuredClone(snippet)
    const extended = structuredClone(snippet)
    extended.processor.custom_option = { value: ['kept'] }
    const blocks = extended.processor.trace_statements as Array<Record<string, unknown>>
    blocks[0].conditions = ['true']
    blocks.push({ context: 'resource', statements: ['set(attributes["x"], "y")'], error_mode: 'propagate' })

    const adapted = getSnippetForSignal(extended, 'logs')!
    const adaptedBlocks = adapted.processor.log_statements as Array<Record<string, unknown>>
    expect(adaptedBlocks).toEqual([
      { ...blocks[0], context: 'log' },
      blocks[1],
    ])
    expect(adapted.processor.custom_option).toEqual({ value: ['kept'] })
    const adaptedStatements = adaptedBlocks[0].statements as string[]
    const adaptedOption = adapted.processor.custom_option as { value: string[] }
    adaptedStatements.push('changed')
    adaptedOption.value.push('changed')
    const canonicalCopy = getSnippetForSignal(snippet, 'metrics')!
    const canonicalBlocks = canonicalCopy.processor.metric_statements as Array<{ statements: string[] }>
    canonicalBlocks[0].statements.push('changed')
    adapted.signals.push('logs')
    expect(extended.processor.trace_statements).toEqual(blocks)
    expect((blocks[0].statements as string[])).not.toContain('changed')
    expect(extended.processor.custom_option).toEqual({ value: ['kept'] })
    expect(SNIPPETS.find(s => s.id === snippet.id)).toEqual(original)
    expect(extended.signals).toEqual(snippet.signals)
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
