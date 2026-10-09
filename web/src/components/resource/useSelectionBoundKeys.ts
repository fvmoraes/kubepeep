import { useSelectionBoundState } from './useSelectionBoundState'

const emptySelection: ReadonlySet<string> = new Set()

/** A row name can reappear in another cluster; selection never crosses that boundary. */
export function useSelectionBoundKeys(identityParts: readonly unknown[]) {
  return useSelectionBoundState(identityParts, emptySelection)
}
