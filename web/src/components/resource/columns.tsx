import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'

import { getPreferences, type ColumnPreferences } from '../../api/client'
import { mutatePreferences } from '../../api/preferences'

// Column visibility (V5-06/V6-01): catalog-driven IDs per collection, applied
// on the shared table rather than per page. The identifier column stays visible. In-memory state hydrates from the preferences document
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
