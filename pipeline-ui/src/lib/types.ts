export type ErrorMode = 'ignore' | 'silent' | 'propagate'
export type StageType = 'transform' | 'filter'

export interface ProcessorConfig {
  [key: string]: unknown
}

export interface Stage {
  id: string
  type: StageType
  error_mode?: ErrorMode
  signals?: string[]
  processor_ref: string
}

export interface Pipeline {
  stages: Stage[]
  processors: Record<string, ProcessorConfig>
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
