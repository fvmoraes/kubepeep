import { useCallback, useSyncExternalStore } from 'react'

/** Subscribe only to breakpoint changes, not every resize frame. */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback((listener: () => void) => {
    const media = window.matchMedia?.(query)
    media?.addEventListener('change', listener)
    return () => media?.removeEventListener('change', listener)
  }, [query])
  const snapshot = useCallback(() => window.matchMedia?.(query).matches ?? false, [query])
  return useSyncExternalStore(subscribe, snapshot, () => false)
}
