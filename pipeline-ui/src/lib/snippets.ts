import type { SignalType, SnippetTemplate } from './types'

/** Canonical trace-origin templates; signals lists the lanes each template supports. */
export const SNIPPETS: SnippetTemplate[] = [
  {
    id: 'drop-internal-spans',
    name: 'Drop internal spans',
    type: 'filter',
    signals: ['traces'],
    description: 'Drops spans where kind is SPAN_KIND_INTERNAL.',
    processor: {
      error_mode: 'ignore',
      traces: {
        span: ['kind == SPAN_KIND_INTERNAL'],
      },
    },
  },
  {
    id: 'set-env-attribute',
    name: 'Set environment attribute',
    type: 'transform',
    signals: ['traces', 'logs', 'metrics'],
    description: 'Sets the env attribute to prod.',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: ['set(attributes["env"], "prod")'],
        },
      ],
    },
  },
  {
    id: 'drop-by-status',
    name: 'Drop spans with OK status',
    type: 'filter',
    signals: ['traces'],
    description: 'Drops spans with STATUS_CODE_OK.',
    processor: {
      error_mode: 'ignore',
      traces: {
        span: ['status.code == STATUS_CODE_OK'],
      },
    },
  },
  {
    id: 'add-service-version',
    name: 'Add service version',
    type: 'transform',
    signals: ['traces', 'logs', 'metrics'],
    description: 'Sets resource attribute service.version.',
    processor: {
      error_mode: 'silent',
      trace_statements: [
        {
          context: 'resource',
          statements: ['set(attributes["service.version"], "1.0.0")'],
        },
      ],
    },
  },
  {
    id: 'rename-attribute',
    name: 'Rename attribute',
    type: 'transform',
    signals: ['traces', 'logs', 'metrics'],
    description: 'Renames an attribute (copy + delete).',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'set(attributes["new_name"], attributes["old_name"]) where attributes["old_name"] != nil',
            'delete_key(attributes, "old_name")',
          ],
        },
      ],
    },
  },
  {
    id: 'filter-production-tags-lambda',
    name: 'Keep production tags (lambda)',
    type: 'transform',
    signals: ['traces', 'logs', 'metrics'],
    description: 'Copies only prod values from a tags attribute array into prod_tags using an experimental OTTL lambda.',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'set(attributes["prod_tags"], Filter(attributes["tags"], (_, value) => value == "prod")) where attributes["tags"] != nil',
          ],
        },
      ],
    },
  },
]

/** Return an independent, lane-specific copy, or null when the template is unsupported. */
export function getSnippetForSignal(snippet: SnippetTemplate, signal: SignalType): SnippetTemplate | null {
  if (!snippet.signals.includes(signal)) return null

  const copy = structuredClone(snippet)
  if (copy.type !== 'transform') return copy

  const processor = copy.processor
  const blocks = processor.trace_statements ?? processor.log_statements ?? processor.metric_statements
  delete processor.trace_statements
  delete processor.log_statements
  delete processor.metric_statements

  const key = signal === 'logs' ? 'log_statements' : signal === 'metrics' ? 'metric_statements' : 'trace_statements'
  const context = signal === 'logs' ? 'log' : signal === 'metrics' ? 'datapoint' : 'span'
  if (Array.isArray(blocks)) {
    processor[key] = blocks.map((block: Record<string, unknown>) =>
      block.context === 'span' || block.context === 'log' || block.context === 'datapoint'
        ? { ...block, context }
        : block,
    )
  }
  return copy
}

/** Extract native OTTL lines from a processor config for the inspector textarea. */
export function extractOttlLines(type: string, processor: Record<string, unknown>): string {
  if (type === 'filter') {
    const traces = processor.traces as { span?: unknown[] } | undefined
    const logs = processor.logs as { log_record?: unknown[] } | undefined
    const metrics = processor.metrics as { datapoint?: unknown[] } | undefined
    const lines =
      traces?.span ??
      logs?.log_record ??
      metrics?.datapoint ??
      []
    return lines.map(String).join('\n')
  }

  const blocks =
    (processor.trace_statements as Array<{ statements?: unknown[] }> | undefined) ??
    (processor.log_statements as Array<{ statements?: unknown[] }> | undefined) ??
    (processor.metric_statements as Array<{ statements?: unknown[] }> | undefined) ??
    []
  const lines: string[] = []
  for (const block of blocks) {
    for (const s of block.statements ?? []) lines.push(String(s))
  }
  if (lines.length === 0 && Array.isArray(processor.statements)) {
    return (processor.statements as unknown[]).map(String).join('\n')
  }
  return lines.join('\n')
}

/** Build processor config from native OTTL lines, preserving unrelated keys when possible. */
export function buildProcessorFromOttlLines(
  type: 'transform' | 'filter',
  signal: 'traces' | 'logs' | 'metrics',
  linesText: string,
  previous: Record<string, unknown> = {},
): Record<string, unknown> {
  const lines = linesText.split('\n').map((l) => l.trim()).filter(Boolean)
  const error_mode = (previous.error_mode as string | undefined) ?? 'ignore'

  if (type === 'filter') {
    if (signal === 'logs') {
      return { error_mode, logs: { log_record: lines } }
    }
    if (signal === 'metrics') {
      return { error_mode, metrics: { datapoint: lines } }
    }
    return { error_mode, traces: { span: lines } }
  }

  const context = signal === 'logs' ? 'log' : signal === 'metrics' ? 'datapoint' : 'span'
  const key =
    signal === 'logs' ? 'log_statements' : signal === 'metrics' ? 'metric_statements' : 'trace_statements'
  return {
    error_mode,
    [key]: [{ context, statements: lines }],
  }
}

export function previewProcessorYaml(processor: Record<string, unknown>): string {
  try {
    // Lightweight preview without importing js-yaml in every call site
    return JSON.stringify(processor, null, 2)
  } catch {
    return '{}'
  }
}
