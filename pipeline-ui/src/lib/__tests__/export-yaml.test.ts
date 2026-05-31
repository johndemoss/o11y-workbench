import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { exportPipelinesToYaml, exportPipelinesToYamlLocal } from '../export-yaml'
import type { PipelinesBySignal } from '../types'

describe('exportPipelinesToYamlLocal', () => {
  it('produces valid YAML with empty pipelines', () => {
    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }
    const yaml = exportPipelinesToYamlLocal(pipelines)
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
    const yaml = exportPipelinesToYamlLocal(pipelines)
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
    const yaml = exportPipelinesToYamlLocal(pipelines)
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
    const yaml = exportPipelinesToYamlLocal(pipelines)
    expect(yaml.split('\n').length).toBeGreaterThan(10)
    expect(yaml).toMatch(/^receivers:/m)
    expect(yaml).toMatch(/^service:/m)
    expect(yaml).toMatch(/^  pipelines:/m)
  })
})

describe('exportPipelinesToYaml (API with fallback)', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'fetch')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('calls the API and returns YAML text on success', async () => {
    const expectedYaml = 'receivers:\n  otlp:\n    protocols:\n      grpc:\n'
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      text: async () => expectedYaml,
    } as Response)

    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }

    const result = await exportPipelinesToYaml(pipelines)
    expect(result).toBe(expectedYaml)
    expect(fetch).toHaveBeenCalledWith('/api/ottl/pipeline/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(pipelines),
    })
  })

  it('falls back to local YAML generation when API is unavailable', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('Network error'))

    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }

    const result = await exportPipelinesToYaml(pipelines)
    // Should produce same output as the local version
    const expected = exportPipelinesToYamlLocal(pipelines)
    expect(result).toBe(expected)
  })

  it('falls back when API returns non-ok status', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 500,
    } as Response)

    const pipelines: PipelinesBySignal = {
      traces: { stages: [], processors: {} },
      logs: { stages: [], processors: {} },
      metrics: { stages: [], processors: {} },
    }

    const result = await exportPipelinesToYaml(pipelines)
    const expected = exportPipelinesToYamlLocal(pipelines)
    expect(result).toBe(expected)
  })
})
