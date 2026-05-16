import { describe, it, expect } from 'vitest'
import type { SignalType, PipelinesBySignal, SnippetTemplate } from '../types'
import { SIGNALS, SIGNAL_COLORS } from '../types'

describe('multi-signal types', () => {
  it('SignalType accepts only traces/logs/metrics', () => {
    const valid: SignalType[] = ['traces', 'logs', 'metrics']
    expect(valid).toHaveLength(3)
  })

  it('SIGNALS has exactly 3 entries', () => {
    expect(SIGNALS).toHaveLength(3)
    expect(SIGNALS).toContain('traces')
    expect(SIGNALS).toContain('logs')
    expect(SIGNALS).toContain('metrics')
  })

  it('SIGNAL_COLORS has entries for all signals', () => {
    for (const s of SIGNALS) {
      expect(SIGNAL_COLORS[s]).toBeDefined()
      expect(SIGNAL_COLORS[s].border).toBeTruthy()
      expect(SIGNAL_COLORS[s].bg).toBeTruthy()
      expect(SIGNAL_COLORS[s].tag).toBeTruthy()
      expect(SIGNAL_COLORS[s].label).toBeTruthy()
    }
  })

  it('PipelinesBySignal is a Record<SignalType, Pipeline>', () => {
    const p: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} }
    }
    expect(p.traces.stages).toEqual([])
  })

  it('SnippetTemplate has required shape', () => {
    const snippet: SnippetTemplate = {
      id: 'test',
      name: 'Test',
      type: 'transform',
      processor: {},
      description: 'A test snippet'
    }
    expect(snippet.id).toBe('test')
    expect(snippet.name).toBe('Test')
    expect(snippet.type).toBe('transform')
  })
})
