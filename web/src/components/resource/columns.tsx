import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'
import { ChevronDown } from 'lucide-react'

import { getPreferences, type ColumnPreferences } from '../../api/client'
import { mutatePreferences } from '../../api/preferences'

// Column visibility (V5-06/V6-01): catalog-driven IDs per collection, applied
// on the shared framework rather than per page. The first (identifier) column
// is always preserved. In-memory state hydrates from the preferences document
// and persists through the merged preferences PUT.
export interface ColumnVisibilityState {
  hidden: string[]
  toggle: (id: string) => void
  reset: () => void
  error?: string
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
          {state.error ? <p className="m-0 max-w-[240px] text-xs text-kp-red" role="alert">{state.error}</p> : null}
        </div>
      ) : null}
    </details>
  )
}

// usePreferenceColumnVisibility binds one collection key to the shared
// preferences document. The global coordinator serializes every writer and
// this mutator replaces only hidden[collectionId], so another collection or
// preference section can never be overwritten by a stale local snapshot.
export function usePreferenceColumnVisibility(collectionId: string): ColumnVisibilityState {
  const queryClient = useQueryClient()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const [localColumns, setLocalColumns] = useState<ColumnPreferences | undefined>(preferences.data?.columns)
  const [saveError, setSaveError] = useState('')

  const onChange = useCallback((next: ColumnPreferences) => {
    const hidden = next.hidden[collectionId] ?? []
    setLocalColumns(next)
    setSaveError('')
    void mutatePreferences((current) => {
      const merged = structuredClone(current)
      merged.columns = {
        hidden: {
          ...(current.columns?.hidden ?? {}),
          [collectionId]: hidden,
        },
      }
      return merged
    }).then((saved) => {
      queryClient.setQueryData(['preferences'], saved)
      setSaveError('')
    }).catch(() => {
      // Keep the optimistic in-memory visibility, but expose that persistence
      // failed so the click is never silently ignored.
      setSaveError('Column visibility changed locally, but preferences could not be saved. Retry or reload to reconcile it.')
    })
  }, [collectionId, queryClient])
  const state = useColumnVisibility(collectionId, localColumns ?? preferences.data?.columns, onChange)
  return { ...state, error: saveError || undefined }
}
