import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'
import { ChevronDown } from 'lucide-react'

import { getPreferences, getSession, putPreferences, type ColumnPreferences, type Preferences } from '../../api/client'

// Column visibility (V5-06/V6-01): catalog-driven IDs per collection, applied
// on the shared framework rather than per page. The first (identifier) column
// is always preserved. In-memory state hydrates from the preferences document
// and persists through the merged preferences PUT.
export interface ColumnVisibilityState {
  hidden: string[]
  toggle: (id: string) => void
  reset: () => void
}

export function useColumnVisibility(collectionId: string, preferences: ColumnPreferences | undefined, onChange: (next: ColumnPreferences) => void): ColumnVisibilityState {
  const stored = useMemo(() => preferences?.hidden?.[collectionId] ?? [], [collectionId, preferences])
  const hidden = stored
  return useMemo(() => ({
    hidden,
    toggle: (id: string) => {
      const next = hidden.includes(id) ? hidden.filter((value) => value !== id) : [...hidden, id]
      onChange({ hidden: { ...(preferences?.hidden ?? {}), [collectionId]: next } })
    },
    reset: () => {
      onChange({ hidden: { ...(preferences?.hidden ?? {}), [collectionId]: [] } })
    },
  }), [collectionId, hidden, onChange, preferences])
}

export function applyColumnVisibility<T extends { key: string }>(columns: readonly T[], state: ColumnVisibilityState): T[] {
  // The leading identifier column always stays visible.
  const [first, ...rest] = columns
  return [first, ...rest.filter((column) => !state.hidden.includes(column.key))]
}

export function ColumnVisibilityControl({ state, columns }: { state: ColumnVisibilityState; columns: readonly { key: string }[] }) {
  const [open, setOpen] = useState(false)
  const hideable = columns.slice(1)
  if (hideable.length === 0) return null
  return (
    <details className="relative justify-self-end" open={open}>
      <summary
        role="button"
        aria-label="Choose visible columns"
        onClick={(event) => { event.preventDefault(); setOpen((current: boolean) => !current) }}
        className="flex h-7 cursor-pointer list-none items-center gap-1 rounded-md border border-kp-overlay-0 bg-kp-surface-1 px-2 text-xs text-kp-subtext hover:text-kp-text"
      >
        Columns <ChevronDown size={12} aria-hidden="true" className={open ? 'rotate-180 transition-transform' : 'transition-transform'} />
      </summary>
      {open ? (
        <div className="absolute right-0 z-20 mt-1 grid min-w-[180px] gap-1 rounded-lg border border-kp-overlay-1 bg-kp-surface-2 p-2 shadow-lg">
          {hideable.map((column) => (
            <label key={column.key} className="flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-xs text-kp-subtext hover:bg-kp-surface-3">
              <input
                type="checkbox"
                checked={!state.hidden.includes(column.key)}
                onChange={() => state.toggle(column.key)}
                className="accent-kp-mauve"
              />
              {column.key}
            </label>
          ))}
          <button type="button" onClick={state.reset} className="mt-1 rounded px-1.5 py-1 text-left text-xs text-kp-sky hover:bg-kp-surface-3">
            Reset columns
          </button>
        </div>
      ) : null}
    </details>
  )
}


// usePreferenceColumnVisibility binds column visibility for one collection to
// the allowlisted preferences document: hydrated on read, persisted on change
// by merging into the current document (never clobbering other sections).
export function usePreferenceColumnVisibility(collectionId: string): ColumnVisibilityState {
  const queryClient = useQueryClient()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const onChange = useCallback((next: ColumnPreferences) => {
    void (async () => {
      try {
        const session = await getSession()
        const base = await getPreferences()
        const merged: Preferences = structuredClone(base)
        merged.columns = next
        const saved = await putPreferences(merged, session.csrfToken)
        queryClient.setQueryData(['preferences'], saved)
      } catch {
        // A failed save keeps the in-memory control usable; the failure is
        // recoverable by retrying or reloading (V6-05).
      }
    })()
  }, [queryClient])
  return useColumnVisibility(collectionId, preferences.data?.columns, onChange)
}
