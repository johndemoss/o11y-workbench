import { describe, it, expect } from 'vitest'
import { load } from 'js-yaml'
import { exportPipelinesToYaml, exportPipelinesToYamlLocal } from '../export-yaml'
import type { PipelinesBySignal } from '../types'

describe('pipeline YAML', () => {
  const pipelines: PipelinesBySignal = {
    traces: {
      stages: [{ id: 'one', type: 'transform', processor_ref: 'transform/one' }],
      processors: { 'transform/one': { empty: {}, list: [], nested: [{ conditions: [], statements: ['set(name, "a: # b")'] }] } },
      receivers: ['otlp/custom'], exporters: ['otlphttp/custom'],
    },
    logs: { stages: [], processors: {} },
    metrics: { stages: [], processors: {} },
  }

  it('retains references and empty collections in the model preview', () => {
    expect(load(exportPipelinesToYamlLocal(pipelines))).toEqual({
      receivers: { 'otlp/custom': {} },
      exporters: { 'otlphttp/custom': {} },
      processors: pipelines.traces.processors,
      service: { pipelines: {
        traces: { receivers: ['otlp/custom'], processors: ['transform/one'], exporters: ['otlphttp/custom'] },
        logs: { receivers: [], processors: [], exporters: [] },
        metrics: { receivers: [], processors: [], exporters: [] },
      } },
    })
  })

  it('rejects a lossy Collector export from references alone', async () => {
    await expect(exportPipelinesToYaml(pipelines)).rejects.toThrow('Missing receiver configuration')
  })
})
