import { describe, it, expect } from 'vitest'
import { defaultPipeline, normalizePipelines } from '../pipeline-defaults'
import type { Pipeline, PipelinesBySignal, SignalType } from '../types'

const SIGNALS: SignalType[] = ['traces', 'logs', 'metrics']

const tracesLane: Pipeline = {
  stages: [{ id: 't1', type: 'transform', processor_ref: 'transform' }],
  processors: { transform: { error_mode: 'ignore' } },
  receivers: ['otlp'],
  exporters: ['debug'],
  receiver_configs: { otlp: { protocols: { grpc: {} } } },
  exporter_configs: { debug: {} },
}

const logsLane: Pipeline = {
  stages: [{ id: 'l1', type: 'filter', processor_ref: 'filter' }],
  processors: { filter: { error_mode: 'silent' } },
  receivers: ['filelog'],
  exporters: ['otlphttp'],
  receiver_configs: { filelog: { include: ['/var/log/app.log'] } },
  exporter_configs: { otlphttp: { endpoint: 'http://collector:4318' } },
}

const prev: PipelinesBySignal = {
  traces: tracesLane,
  logs: logsLane,
  metrics: defaultPipeline(),
}

describe('defaultPipeline', () => {
  it('carries the product default infrastructure', () => {
    expect(defaultPipeline()).toEqual({
      stages: [],
      processors: {},
      receivers: ['otlp'],
      exporters: ['debug'],
      receiver_configs: { otlp: { protocols: { grpc: {} } } },
      exporter_configs: { debug: {} },
    })
  })

  it('returns independent instances', () => {
    defaultPipeline().receivers?.push('custom')
    expect(defaultPipeline().receivers).toEqual(['otlp'])
  })
})

describe('normalizePipelines', () => {
  it('keeps prior lanes for signals missing from the parsed result', () => {
    const parsed = { traces: tracesLane } as Partial<PipelinesBySignal>
    const out = normalizePipelines(parsed, prev)

    expect(out.traces.stages).toEqual(tracesLane.stages)
    expect(out.logs).toEqual(prev.logs)
    expect(out.metrics).toEqual(prev.metrics)
    expect(out.logs.receivers).toEqual(['filelog'])
    expect(out.logs.receiver_configs).toEqual(logsLane.receiver_configs)
  })

  it('uses default infrastructure when a missing lane has no prior value', () => {
    const out = normalizePipelines({})

    for (const signal of SIGNALS) {
      expect(out[signal].receivers).toEqual(['otlp'])
      expect(out[signal].exporters).toEqual(['debug'])
      expect(out[signal].receiver_configs).toEqual({ otlp: { protocols: { grpc: {} } } })
      expect(out[signal].exporter_configs).toEqual({ debug: {} })
    }
  })

  it('resets explicitly empty lanes to default infrastructure', () => {
    const parsed: Partial<PipelinesBySignal> = {
      logs: {
        stages: [],
        processors: {},
        receivers: [],
        exporters: [],
        receiver_configs: {},
        exporter_configs: {},
      },
    }
    const out = normalizePipelines(parsed, prev)

    expect(out.logs.receivers).toEqual(['otlp'])
    expect(out.logs.exporters).toEqual(['debug'])
    expect(out.logs.receiver_configs).toEqual({ otlp: { protocols: { grpc: {} } } })
    expect(out.logs.exporter_configs).toEqual({ debug: {} })
  })

  it('honors a genuine replacement lane', () => {
    const replacement: Pipeline = {
      stages: [{ id: 'l2', type: 'transform', processor_ref: 'transform2' }],
      processors: { transform2: { error_mode: 'propagate' } },
      receivers: ['otlp'],
      exporters: ['debug'],
      receiver_configs: { otlp: { protocols: { http: {} } } },
      exporter_configs: { debug: { verbosity: 'detailed' } },
    }
    const out = normalizePipelines({ logs: replacement }, prev)

    expect(out.logs.stages).toEqual(replacement.stages)
    expect(out.logs.processors).toEqual(replacement.processors)
    expect(out.logs.receiver_configs).toEqual({ otlp: { protocols: { http: {} } } })
    expect(out.logs.exporter_configs).toEqual({ debug: { verbosity: 'detailed' } })
  })
})
