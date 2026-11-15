import { describe, it, expect } from 'vitest'
import type { SignalType, PipelinesBySignal, StageDiff, Run } from '../types'
import { SIGNALS, SIGNAL_COLORS, formatCaptureDropped } from '../types'

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

  it('models generic signal diffs and truncated stopping runs', () => {
    const diff: StageDiff = {
      stage_index: 0, signal: 'logs', input_count: 1, output_count: 1,
      added: [], removed: [],
      modified: [{ id: 'log-1', status: 'modified', changes: [
        { field: 'resource.attributes.environment', type: 'changed', before: 'dev', after: 'prod' },
      ] }],
    }
    const run: Run = {
      id: 'r', status: 'stopping', span_count: 0, format: 'protobuf',
      capture_truncated: true,
      capture_dropped: { traces: 2, logs: 1, metrics: 0 },
      capture_limit_bytes: 4194304,
    }
    expect(diff.modified[0].changes?.[0].field).toContain('resource')
    expect(run.capture_truncated).toBe(true)
    expect(formatCaptureDropped(run.capture_dropped)).toBe('2 spans, 1 log records')
  })

  it('formats per-signal capture drops with native units', () => {
    expect(formatCaptureDropped({ traces: 1, logs: 2, metrics: 3 })).toBe(
      '1 spans, 2 log records, 3 data points',
    )
    expect(formatCaptureDropped({ traces: 0, logs: 0, metrics: 0 })).toBe('')
    expect(formatCaptureDropped(undefined)).toBe('')
  })

})
