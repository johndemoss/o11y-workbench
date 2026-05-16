import type { SnippetTemplate } from './types'

export const SNIPPETS: SnippetTemplate[] = [
  {
    id: 'drop-internal-spans',
    name: 'Drop internal spans',
    type: 'filter',
    description: 'Drops spans with SPAN_KIND_INTERNAL',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'kind != SPAN_KIND_INTERNAL',
          ],
        },
      ],
    },
  },
  {
    id: 'set-env-attribute',
    name: 'Set environment attribute',
    type: 'transform',
    description: 'Sets attributes["env"] to "prod"',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'set(attributes["env"], "prod")',
          ],
        },
      ],
    },
  },
  {
    id: 'drop-by-status',
    name: 'Drop by status code',
    type: 'filter',
    description: 'Filters spans where status_code is not OK',
    processor: {
      error_mode: 'propagate',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'status_code != OK',
          ],
        },
      ],
    },
  },
  {
    id: 'add-service-version',
    name: 'Add service version',
    type: 'transform',
    description: 'Sets attributes["service.version"] to "1.0.0"',
    processor: {
      error_mode: 'silent',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'set(attributes["service.version"], "1.0.0")',
          ],
        },
      ],
    },
  },
  {
    id: 'rename-attribute',
    name: 'Rename attribute',
    type: 'transform',
    description: 'Copies old attribute to new name, then deletes old',
    processor: {
      error_mode: 'ignore',
      trace_statements: [
        {
          context: 'span',
          statements: [
            'set(attributes["new_name"], attributes["old_name"])',
            'delete_key(attributes, "old_name")',
          ],
        },
      ],
    },
  },
]
