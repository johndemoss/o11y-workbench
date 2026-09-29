import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { debounce } from '../debounce'

describe('debounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('invokes once after wait when called 3 times rapidly', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 400)

    debounced()
    debounced()
    debounced()

    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(399)
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('cancel prevents a pending invocation', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 300)

    debounced()
    debounced.cancel()
    vi.advanceTimersByTime(300)
    expect(fn).not.toHaveBeenCalled()
  })

  it('passes the latest args to the deferred call', () => {
    const fn = vi.fn()
    const debounced = debounce(fn as (n: number) => void, 200)

    debounced(1)
    debounced(2)
    debounced(3)
    vi.advanceTimersByTime(200)
    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith(3)
  })
})
