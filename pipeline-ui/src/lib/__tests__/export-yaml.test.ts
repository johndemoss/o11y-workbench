import { describe, it, expect } from 'vitest'
import { exportPipelinesToYaml } from '../export-yaml'
import type { PipelinesBySignal } from '../types'

describe('exportPipelinesToYaml', () => {
  it('produces valid YAML with empty pipelines', () => {
    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const yaml = exportPipelinesToYaml(pipelines)
    expect(yaml).toContain('receivers:')
    expect(yaml).toContain('exporters:')
    expect(yaml).toContain('service:')
    expect(yaml).toContain('pipelines:')
  })

  it('includes processor configs from all signals', () => {
    const pipelines: PipelinesBySignal = {
      traces: {
        stages: [{ id: 's1', type: 'transform', processor_ref: 'add_env', error_mode: 'ignore' }],
        processors: { add_env: { error_mode: 'ignore', trace_statements: [{ context: 'span', statements: ['set(attributes["env"], "prod")'] }] } },
      },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const yaml = exportPipelinesToYaml(pipelines)
    expect(yaml).toContain('add_env')
    expect(yaml).toContain('traces')
    expect(yaml).toContain('receivers: [otlp]')
    expect(yaml).toContain('exporters: [debug]')
  })

  it('separates stages per signal pipeline', () => {
    const pipelines: PipelinesBySignal = {
      traces: { stages: [{ id: 't1', type: 'transform', processor_ref: 'p1' }], processors: { p1: { statement: 'x' } } },
      logs: { stages: [{ id: 'l1', type: 'filter', processor_ref: 'p2' }], processors: { p2: { statement: 'y' } } },
      metrics: { stages: [], processors: {} },
    }
    const yaml = exportPipelinesToYaml(pipelines)
    expect(yaml).toContain('traces:')
    expect(yaml).toContain('logs:')
    expect(yaml).toContain('p1')
    expect(yaml).toContain('p2')
  })

  it('output is structurally valid YAML', () => {
    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const yaml = exportPipelinesToYaml(pipelines)
    expect(yaml.split('\n').length).toBeGreaterThan(10)
    expect(yaml).toMatch(/^receivers:/m)
    expect(yaml).toMatch(/^service:/m)
    expect(yaml).toMatch(/^  pipelines:/m)
  })
})
