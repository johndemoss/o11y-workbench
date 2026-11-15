import type { PipelinesBySignal } from './types'
import { dump } from 'js-yaml'

/** Generate a Collector config only when every infrastructure definition is retained. */
export async function exportPipelinesToYaml(pipelines: PipelinesBySignal): Promise<string> {
  return serializePipelines(pipelines, true)
}

/** Preview the graph, including placeholders for definitions not yet supplied. */
export function exportPipelinesToYamlLocal(pipelines: PipelinesBySignal): string {
  return serializePipelines(pipelines, false)
}

function serializePipelines(pipelines: PipelinesBySignal, strict: boolean): string {
  const receivers: Record<string, unknown> = {}
  const exporters: Record<string, unknown> = {}
  const processors: Record<string, unknown> = {}
  const servicePipelines: Record<string, unknown> = {}
  for (const signal of ['traces', 'logs', 'metrics'] as const) {
    const pipeline = pipelines[signal]
    if (!pipeline) continue
    const receiverNames = pipeline.receivers ?? []
    const exporterNames = pipeline.exporters ?? []
    for (const [names, configs, target, kind] of [
      [receiverNames, pipeline.receiver_configs, receivers, 'receiver'],
      [exporterNames, pipeline.exporter_configs, exporters, 'exporter'],
    ] as const) {
      for (const name of names) {
        const config = configs?.[name]
        if (config === undefined && strict) throw new Error(`Missing ${kind} configuration: ${name}`)
        if (strict && name in target && JSON.stringify(target[name]) !== JSON.stringify(config)) {
          throw new Error(`Conflicting ${kind} configurations: ${name}`)
        }
        target[name] = config ?? {}
      }
    }
    Object.assign(processors, pipeline.processors)
    servicePipelines[signal] = {
      receivers: receiverNames,
      processors: pipeline.stages.map(stage => stage.processor_ref),
      exporters: exporterNames,
    }
  }
  return dump({ receivers, processors, exporters, service: { pipelines: servicePipelines } }, { noRefs: true, lineWidth: -1 })
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


