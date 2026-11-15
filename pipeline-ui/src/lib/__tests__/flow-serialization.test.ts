import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { preserveInfraConfigs } from '../flow-serialization'
import type { Pipeline } from '../types'

describe('preserveInfraConfigs (A9)', () => {
  it('keeps custom receiver and exporter definitions unchanged', () => {
    const pipeline: Pipeline = {
      stages: [],
      processors: {},
      receivers: ['otlp/custom', 'other'],
      exporters: ['debug', 'otlphttp/out'],
      receiver_configs: {
        'otlp/custom': { endpoint: 'https://example.test' },
        other: { secret: true },
      },
      exporter_configs: {
        debug: { verbosity: 'detailed' },
        'otlphttp/out': { endpoint: 'http://127.0.0.1:4318' },
      },
    }
    const infra = preserveInfraConfigs(pipeline)
    expect(infra.receiver_configs).toEqual(pipeline.receiver_configs)
    expect(infra.exporter_configs).toEqual(pipeline.exporter_configs)
    expect(infra.receivers).toEqual(['otlp/custom', 'other'])
    expect(infra.exporters).toEqual(['debug', 'otlphttp/out'])
  })

  it('fills only built-in otlp and debug defaults when absent', () => {
    const pipeline: Pipeline = {
      stages: [],
      processors: {},
      receivers: ['otlp', 'custom-rcv'],
      exporters: ['debug', 'other-exp'],
    }
    const infra = preserveInfraConfigs(pipeline)
    expect(infra.receiver_configs).toEqual({
      otlp: { protocols: { grpc: {} } },
    })
    expect(infra.exporter_configs).toEqual({ debug: {} })
    expect(infra.receiver_configs['custom-rcv']).toBeUndefined()
    expect(infra.exporter_configs['other-exp']).toBeUndefined()
  })

  it('does not invent settings for custom names without stored configs', () => {
    const pipeline: Pipeline = {
      stages: [],
      processors: {},
      receivers: ['kafka'],
      exporters: ['prometheus'],
    }
    const infra = preserveInfraConfigs(pipeline)
    expect(infra.receiver_configs).toEqual({})
    expect(infra.exporter_configs).toEqual({})
  })
})