import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { debounce } from '../debounce'

/**
 * Mirrors App.svelte Model-1 ownership: immediate local yaml + debounced
 * POST /export with yamlVersion race guard.
 */
function createExportScheduler(opts: {
  ms: number
  exportFn: (ver: number, payload: unknown) => Promise<void>
}) {
  let yamlVersion = 0
  let pipelineYaml = ''
  const debouncedExport = debounce((ver: number, payload: unknown) => {
    void opts.exportFn(ver, payload)
  }, opts.ms)

  return {
    get yamlVersion() {
      return yamlVersion
    },
    get pipelineYaml() {
      return pipelineYaml
    },
    onPipelinesChange(localYaml: string, payload: unknown) {
      const ver = ++yamlVersion
      pipelineYaml = localYaml
      debouncedExport(ver, payload)
    },
    applyExportResult(ver: number, text: string) {
      if (ver === yamlVersion) {
        pipelineYaml = text
      }
    },
    cancel() {
      debouncedExport.cancel()
    },
  }
}

describe('export debounce (App ownership model)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('coalesces 5 rapid pipelines changes into a single POST /export', async () => {
    const posts: unknown[] = []
    const scheduler = createExportScheduler({
      ms: 400,
      exportFn: async (ver, payload) => {
        posts.push({ ver, payload })
      },
    })

    for (let i = 0; i < 5; i++) {
      scheduler.onPipelinesChange(`local-${i}`, { n: i })
    }

    expect(posts).toHaveLength(0)
    expect(scheduler.pipelineYaml).toBe('local-4')

    vi.advanceTimersByTime(400)
    await Promise.resolve()

    expect(posts).toHaveLength(1)
    expect(posts[0]).toEqual({ ver: 5, payload: { n: 4 } })
  })

  it('yamlVersion race guard drops stale export responses', async () => {
    const scheduler = createExportScheduler({
      ms: 400,
      exportFn: async () => {},
    })

    scheduler.onPipelinesChange('local-a', { a: 1 })
    const staleVer = scheduler.yamlVersion
    scheduler.onPipelinesChange('local-b', { b: 2 })
    const freshVer = scheduler.yamlVersion

    scheduler.applyExportResult(staleVer, 'stale-api-yaml')
    expect(scheduler.pipelineYaml).toBe('local-b')

    scheduler.applyExportResult(freshVer, 'fresh-api-yaml')
    expect(scheduler.pipelineYaml).toBe('fresh-api-yaml')
  })
})
