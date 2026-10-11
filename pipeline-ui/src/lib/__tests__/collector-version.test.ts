import { describe, it, expect } from 'vitest'
import { withCollectorVersion } from '../collector-version'

describe('withCollectorVersion', () => {
  it('preserves existing query keys and appends collector_version', () => {
    expect(withCollectorVersion('/api/ottl/pipeline/execute?run_id=1', '0.162.0')).toBe(
      '/api/ottl/pipeline/execute?run_id=1&collector_version=0.162.0',
    )
  })
})
