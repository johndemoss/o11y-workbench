import yaml from 'js-yaml'
import type { PipelinesBySignal } from './types'

/**
 * Generates a full OTel Collector config YAML string from PipelinesBySignal.
 * Uses the Go backend API with fallback to client-side generation for export.
 * Each signal type becomes a pipeline under service.pipelines.
 * Processor configs are merged into the top-level processors section.
 */
export async function exportPipelinesToYaml(pipelines: PipelinesBySignal): Promise<string> {
  try {
    const res = await fetch('/api/ottl/pipeline/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(pipelines),
    })
    if (!res.ok) throw new Error(`API error: ${res.status}`)
    return await res.text()
  } catch {
    // Fallback to client-side YAML generation
    return exportPipelinesToYamlLocal(pipelines)
  }
}

/**
 * Client-side YAML generation (fallback for when API is unavailable).
 * Used directly by tests and when the backend is offline.
 */
export function exportPipelinesToYamlLocal(pipelines: PipelinesBySignal): string {
  const lines: string[] = []

  // Receivers section
  lines.push('receivers:')
  lines.push('  otlp:')
  lines.push('    protocols:')
  lines.push('      grpc:')
  lines.push('')

  // Processors section — merge all processor configs from all signals
  lines.push('processors:')
  const allProcessorKeys = new Set<string>()
  for (const signal of ['traces', 'logs', 'metrics'] as const) {
    const p = pipelines[signal]
    if (p) {
      for (const key of Object.keys(p.processors)) {
        allProcessorKeys.add(key)
      }
    }
  }
  if (allProcessorKeys.size === 0) {
    lines.push('  {}')
  } else {
    for (const key of allProcessorKeys) {
      for (const signal of ['traces', 'logs', 'metrics'] as const) {
        const p = pipelines[signal]
        if (p && p.processors[key]) {
          lines.push(`  ${key}:`)
          const config = p.processors[key]
          lines.push(...yamlDump(config, 4))
          break
        }
      }
    }
  }
  lines.push('')

  // Exporters section
  lines.push('exporters:')
  lines.push('  debug:')
  lines.push('')

  // Service pipelines section
  lines.push('service:')
  lines.push('  pipelines:')
  for (const signal of ['traces', 'logs', 'metrics'] as const) {
    const p = pipelines[signal]
    if (!p) continue
    lines.push(`    ${signal}:`)
    lines.push('      receivers: [otlp]')
    if (p.stages.length > 0) {
      lines.push(`      processors: [${p.stages.map(s => s.processor_ref).join(', ')}]`)
    } else {
      lines.push('      processors: []')
    }
    lines.push('      exporters: [debug]')
  }

  return lines.join('\n')
}

function yamlDump(obj: unknown, indent: number): string[] {
  const lines: string[] = []
  function recurse(value: unknown, depth: number): void {
    const prefix = ' '.repeat(depth)
    if (Array.isArray(value)) {
      for (const item of value) {
        if (typeof item === 'object' && item !== null) {
          lines.push(`${prefix}-`)
          recurse(item, depth + 2)
        } else {
          lines.push(`${prefix}- ${JSON.stringify(item)}`)
        }
      }
    } else if (typeof value === 'object' && value !== null) {
      for (const [k, v] of Object.entries(value)) {
        if (typeof v === 'object' && v !== null && !Array.isArray(v)) {
          lines.push(`${prefix}${k}:`)
          recurse(v, depth + 2)
        } else if (Array.isArray(v)) {
          lines.push(`${prefix}${k}:`)
          recurse(v, depth + 2)
        } else {
          lines.push(`${prefix}${k}: ${JSON.stringify(v)}`)
        }
      }
    } else {
      lines.push(`${prefix}${JSON.stringify(value)}`)
    }
  }
  recurse(obj, indent)
  return lines
}

/**
 * Triggers a browser download of a YAML file.
 */
export function downloadYaml(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'application/x-yaml' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

function inferStageType(processorRef: string): 'transform' | 'filter' {
  const prefix = processorRef.split('/')[0]
  if (prefix === 'filter') return 'filter'
  return 'transform'
}

/**
 * Parses a YAML string into PipelinesBySignal.
 * This reverses exportPipelinesToYaml (YAML → data).
 */
export function importYamlToPipelines(yamlText: string): PipelinesBySignal {
  const result: PipelinesBySignal = {
    traces: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    logs: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
    metrics: { stages: [], processors: {}, receivers: ['otlp'], exporters: ['debug'] },
  }

  try {
    const config = yaml.load(yamlText) as Record<string, unknown> | null
    if (!config) return result

    // Extract processors
    const processorsSection = config['processors'] as Record<string, unknown> | undefined
    const processorConfigs: Record<string, Record<string, unknown>> = {}
    if (processorsSection) {
      for (const [key, val] of Object.entries(processorsSection)) {
        if (val && typeof val === 'object') {
          processorConfigs[key] = val as Record<string, unknown>
        }
      }
    }

    // Extract receivers
    const receiversSection = config['receivers'] as Record<string, unknown> | undefined
    if (receiversSection) {
      const recvNames = Object.keys(receiversSection)
      for (const signal of ['traces', 'logs', 'metrics'] as const) {
        result[signal].receivers = [...recvNames]
      }
    }

    // Extract exporters
    const exportersSection = config['exporters'] as Record<string, unknown> | undefined
    if (exportersSection) {
      const expNames = Object.keys(exportersSection)
      for (const signal of ['traces', 'logs', 'metrics'] as const) {
        result[signal].exporters = [...expNames]
      }
    }

    // Extract service.pipelines
    const service = config['service'] as Record<string, unknown> | undefined
    const pipelines = service?.['pipelines'] as Record<string, unknown> | undefined
    if (pipelines) {
      for (const [signal, pipelineDef] of Object.entries(pipelines)) {
        if (signal === 'traces' || signal === 'logs' || signal === 'metrics') {
          const p = pipelineDef as Record<string, unknown>
          const procRefs = (p['processors'] as string[] | undefined) || []

          const stages = procRefs.map((ref) => ({
            id: ref,
            type: inferStageType(ref),
            processor_ref: ref,
          }))

          result[signal as keyof PipelinesBySignal] = {
            stages,
            processors: { ...processorConfigs },
            receivers: result[signal as keyof PipelinesBySignal].receivers,
            exporters: result[signal as keyof PipelinesBySignal].exporters,
          }
        }
      }
    }
  } catch {
    // Fall back to defaults on parse error
  }

  return result
}
