export type ErrorMode = 'ignore' | 'silent' | 'propagate'
export type StageType = 'transform' | 'filter'

export interface ProcessorConfig {
  [key: string]: unknown
}

export interface Stage {
  id: string
  type: StageType
  error_mode?: ErrorMode
  processor_ref: string
}

export interface Pipeline {
  stages: Stage[]
  processors: Record<string, ProcessorConfig>
}

export interface SpanChange {
  field: string
  type: 'added' | 'removed' | 'changed'
  before?: unknown
  after?: unknown
}

export interface FullAttributeDiff {
  key: string
  before?: unknown
  after?: unknown
  change_type: 'added' | 'removed' | 'changed' | 'unchanged'
}

export interface DiffSpan {
  span_id: string
  name?: string
  status: 'added' | 'removed' | 'modified'
  before?: unknown
  after?: unknown
  changes?: SpanChange[]
}

export interface StageDiff {
  stage_index: number
  input_span_count: number
  output_span_count: number
  added: DiffSpan[]
  removed: DiffSpan[]
  modified: DiffSpan[]
}

export interface QuickConfig {
  statements: string
  type: 'transform' | 'filter'
  run_id: string
}

export interface StageResult {
  stage_index: number
  stage_id: string
  input_span_count: number
  output_span_count: number
  statements_parsed: number
  errors?: string[]
  intermediate_data?: unknown
  duration_ns: number
}

export interface PipelineResult {
  stages: StageResult[]
  final_data?: unknown
  final_span_count: number
}

// ─── Multi-signal types ──────────────────────────────────────────
export type SignalType = 'traces' | 'logs' | 'metrics'

export type PipelinesBySignal = Record<SignalType, Pipeline>

export const SIGNALS: SignalType[] = ['traces', 'logs', 'metrics']

export const SIGNAL_COLORS: Record<SignalType, { border: string; bg: string; tag: string; label: string }> = {
  traces:  { border: '#F59E0B', bg: 'rgba(245, 158, 11, 0.05)', tag: '#F59E0B', label: 'Traces' },
  logs:    { border: '#34D399', bg: 'rgba(52, 211, 153, 0.05)', tag: '#34D399', label: 'Logs' },
  metrics: { border: '#38BDF8', bg: 'rgba(56, 189, 248, 0.05)', tag: '#38BDF8', label: 'Metrics' },
}

export interface SnippetTemplate {
  id: string
  name: string
  type: StageType
  processor: ProcessorConfig
  description: string
}
