export type ErrorMode = 'ignore' | 'silent' | 'propagate'
export type StageType = 'transform' | 'filter' | 'receiver' | 'exporter' | 'connector'

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
  receivers?: string[]
  exporters?: string[]
  receiver_configs?: Record<string, Record<string, unknown>>
  exporter_configs?: Record<string, Record<string, unknown>>
}

export interface RecordChange {
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

export interface DiffRecord {
  id: string
  name?: string
  status: 'added' | 'removed' | 'modified'
  before?: unknown
  after?: unknown
  changes?: RecordChange[]
}

export interface StageDiff {
  stage_index: number
  signal: SignalType
  input_count: number
  output_count: number
  added: DiffRecord[]
  removed: DiffRecord[]
  modified: DiffRecord[]
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

export interface CaptureDroppedCounts {
  traces?: number
  logs?: number
  metrics?: number
}

export function formatCaptureDropped(dropped: CaptureDroppedCounts | undefined): string {
  if (dropped === undefined) return ''
  const parts: string[] = []
  if ((dropped.traces ?? 0) > 0) parts.push(`${dropped.traces} spans`)
  if ((dropped.logs ?? 0) > 0) parts.push(`${dropped.logs} log records`)
  if ((dropped.metrics ?? 0) > 0) parts.push(`${dropped.metrics} data points`)
  return parts.join(', ')
}

export interface Run {
  id: string
  status: 'pending' | 'running' | 'paused' | 'stopping' | 'completed' | 'failed' | 'stopped' | 'streaming'
  span_count: number
  format?: 'json' | 'protobuf'
  capture_truncated?: boolean
  capture_dropped?: CaptureDroppedCounts
  capture_limit_bytes?: number
  has_trace_data?: boolean
  has_log_data?: boolean
  has_metric_data?: boolean
}

export type PipelinesBySignal = Record<SignalType, Pipeline>

export const SIGNALS: SignalType[] = ['traces', 'logs', 'metrics']

export const SIGNAL_COLORS: Record<SignalType, { border: string; bg: string; tag: string; label: string }> = {
  traces:  { border: '#F59E0B', bg: 'rgba(245, 158, 11, 0.05)', tag: '#F59E0B', label: 'Traces' },
  logs:    { border: '#34D399', bg: 'rgba(52, 211, 153, 0.05)', tag: '#34D399', label: 'Logs' },
  metrics: { border: '#38BDF8', bg: 'rgba(56, 189, 248, 0.05)', tag: '#38BDF8', label: 'Metrics' },
}

/** Edge class + stroke matching the lane boundary (--graph-* / SIGNAL_COLORS). */
export function signalEdgeProps(signal: SignalType): { class: string; style: string } {
  return {
    class: `edge-signal-${signal}`,
    style: `stroke: ${SIGNAL_COLORS[signal].border}`,
  }
}

export const STAGE_COLORS: Record<StageType, { badge: string; bg: string; border: string }> = {
  transform: { badge: '#7c3aed', bg: 'rgba(124, 58, 237, 0.08)', border: '#7c3aed' },
  filter:    { badge: '#d97706', bg: 'rgba(217, 119, 6, 0.08)', border: '#d97706' },
  receiver:  { badge: '#059669', bg: 'rgba(5, 150, 105, 0.08)', border: '#059669' },
  exporter:  { badge: '#dc2626', bg: 'rgba(220, 38, 38, 0.08)', border: '#dc2626' },
  connector: { badge: '#6366f1', bg: 'rgba(99, 102, 241, 0.08)', border: '#6366f1' },
}

export interface SnippetTemplate {
  id: string
  name: string
  type: StageType
  /** Signals for which this template can produce a valid processor. */
  signals: SignalType[]
  processor: ProcessorConfig
  description: string
}

export interface CollectorVersionInfo {
  contrib: string
  stable: string
  source: string
}
