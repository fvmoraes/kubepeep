import { useCallback, useState, type SetStateAction } from 'react'

/** Discard stale UI state before rendering a different resource selection. */
export function useSelectionBoundState<T>(identityParts: readonly unknown[], initial: T) {
  const identity = JSON.stringify(identityParts)
  const [state, setState] = useState(() => ({ identity, value: initial }))
  if (state.identity !== identity) setState({ identity, value: initial })
  const value = state.identity === identity ? state.value : initial
  const update = useCallback((next: SetStateAction<T>) => {
    setState((current) => {
      const previous = current.identity === identity ? current.value : initial
      return { identity, value: typeof next === 'function' ? (next as (previous: T) => T)(previous) : next }
    })
  }, [identity, initial])
  return [value, update] as const
}
