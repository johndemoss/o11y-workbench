import type { Pipeline, PipelinesBySignal, SignalType } from './types'

/** Product default lane: OTLP receiver in, debug exporter out. */
export function defaultPipeline(): Pipeline {
  return {
    stages: [],
    processors: {},
    receivers: ['otlp'],
    exporters: ['debug'],
    receiver_configs: { otlp: { protocols: { grpc: {} } } },
    exporter_configs: { debug: {} },
  }
}

/**
 * Normalize a parsed PipelinesBySignal.
 *
 * Signals absent from the parsed result keep the previously held lane, so an
 * import that defines only some service.pipelines does not wipe other lanes.
 * Present lanes (including explicitly empty ones) replace the previous lane,
 * with product default infrastructure filled in where omitted or empty.
 */
export function normalizePipelines(
  p: Partial<PipelinesBySignal>,
  prev?: Partial<PipelinesBySignal> | null,
): PipelinesBySignal {
  const defaults = defaultPipeline()
  const lane = (signal: SignalType): Pipeline => {
    const base = p[signal] ?? prev?.[signal] ?? defaults
    return {
      ...base,
      receivers: base.receivers?.length ? base.receivers : defaults.receivers,
      exporters: base.exporters?.length ? base.exporters : defaults.exporters,
      receiver_configs: base.receiver_configs && Object.keys(base.receiver_configs).length
        ? base.receiver_configs
        : defaults.receiver_configs,
      exporter_configs: base.exporter_configs && Object.keys(base.exporter_configs).length
        ? base.exporter_configs
        : defaults.exporter_configs,
    }
  }
  return { traces: lane('traces'), logs: lane('logs'), metrics: lane('metrics') }
}
