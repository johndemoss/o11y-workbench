import { describe, it, expect } from 'vitest'
import { dump, load, YAMLException } from 'js-yaml'

// The pipeline UI formats processor bodies with these exact dump options and
// relies on YAMLException.mark for lint diagnostics. Guard the js-yaml surface
// the app uses so a major upgrade cannot silently change it.
const DUMP_OPTIONS = { indent: 2, noRefs: true, sortKeys: false }

describe('js-yaml integration', () => {
  it('dumps nested processor bodies with two-space indentation and no references', () => {
    const shared = { enabled: true }
    const processor = {
      transform: { trace_statements: [{ set: { 'attributes["env"]': 'prod' } }] },
      first: shared,
      second: shared,
    }

    const yamlText = dump(processor, DUMP_OPTIONS).trimEnd()
    const lines = yamlText.split('\n')

    expect(lines[0]).toBe('transform:')
    expect(lines).toContain('  trace_statements:')
    expect(lines).toContain('    - set:')
    expect(lines).toContain('        attributes["env"]: prod')
    // noRefs inlines duplicated objects instead of emitting anchors or aliases.
    expect(yamlText).not.toMatch(/&\w+|\*\w+/)
  })

  it('keeps mapping keys in insertion order when sortKeys is disabled', () => {
    const yamlText = dump({ zeta: 1, alpha: 2 }, DUMP_OPTIONS)
    expect(yamlText.indexOf('zeta')).toBeLessThan(yamlText.indexOf('alpha'))
  })

  it('loads a round-tripped document unchanged', () => {
    const original = { pipelines: { traces: { processors: ['transform/set-env'] } } }
    expect(load(dump(original, DUMP_OPTIONS))).toEqual(original)
  })

  it('reports an invalid document through YAMLException with a positioned mark', () => {
    let caught: unknown
    try {
      load('stages:\n  - id: one\n   bad indent: true\n')
    } catch (error) {
      caught = error
    }

    expect(caught).toBeInstanceOf(YAMLException)
    const mark = (caught as YAMLException).mark
    if (!mark) {
      throw new Error('expected YAMLException.mark for an invalid document')
    }
    expect(mark.line).toBeGreaterThanOrEqual(0)
    expect(mark.column).toBeGreaterThanOrEqual(0)
  })
})
