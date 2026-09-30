import { useCallback, useState, type SetStateAction } from 'react'

const emptySelection: ReadonlySet<string> = new Set()

/** A row name can reappear in another cluster; selection never crosses that boundary. */
export function useSelectionBoundKeys(identityParts: readonly unknown[]) {
  const identity = JSON.stringify(identityParts)
  const [state, setState] = useState<{ identity: string; keys: ReadonlySet<string> }>(() => ({ identity, keys: emptySelection }))
  const keys = state.identity === identity ? state.keys : emptySelection
  const setKeys = useCallback((value: SetStateAction<ReadonlySet<string>>) => {
    setState((current) => {
      const previous = current.identity === identity ? current.keys : emptySelection
      return { identity, keys: typeof value === 'function' ? value(previous) : value }
    })
  }, [identity])
  return [keys, setKeys] as const
}
