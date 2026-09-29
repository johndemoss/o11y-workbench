/**
 * Returns a debounced wrapper that delays invoking `fn` until `ms` ms have
 * elapsed since the last call. Exposes `.cancel()` to clear a pending timer.
 */
export function debounce<Args extends unknown[]>(
  fn: (...args: Args) => void,
  ms: number,
): ((...args: Args) => void) & { cancel: () => void } {
  let timer: ReturnType<typeof setTimeout> | undefined

  const debounced = ((...args: Args) => {
    if (timer !== undefined) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = undefined
      fn(...args)
    }, ms)
  }) as ((...args: Args) => void) & { cancel: () => void }

  debounced.cancel = () => {
    if (timer !== undefined) {
      clearTimeout(timer)
      timer = undefined
    }
  }

  return debounced
}
