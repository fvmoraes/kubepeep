import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'

import { getPreferences, type ColumnPreferences } from '../../api/client'
import { mutatePreferences } from '../../api/preferences'

// Column visibility (V5-06/V6-01): catalog-driven IDs per collection, applied
// on the shared table rather than per page. The identifier column stays visible. In-memory state hydrates from the preferences document
// and persists through the merged preferences PUT.
export interface ColumnVisibilityState {
  useInitialVisibility?: boolean
  setHidden?: (ids: string[]) => void
  hidden: string[]
  toggle: (id: string) => void
  reset: () => void
  order?: string[]
  reorder?: (ids: string[]) => void
  resetOrder?: () => void
  error?: string
}

export function useColumnVisibility(collectionId: string, preferences: ColumnPreferences | undefined, onChange: (next: ColumnPreferences) => void): ColumnVisibilityState {
  const stored = useMemo(() => preferences?.hidden?.[collectionId] ?? [], [collectionId, preferences])
  const hidden = stored
  return useMemo(() => ({
    hidden,
    useInitialVisibility: !Object.hasOwn(preferences?.hidden ?? {}, collectionId),
    setHidden: (ids: string[]) => onChange({ ...preferences, hidden: { ...preferences?.hidden, [collectionId]: ids } }),
    order: preferences?.order?.[collectionId] ?? [],
    toggle: (id: string) => {
      const next = hidden.includes(id) ? hidden.filter((value) => value !== id) : [...hidden, id]
      onChange({ ...preferences, hidden: { ...(preferences?.hidden ?? {}), [collectionId]: next } })
    },
    reset: () => {
      const hidden = { ...preferences?.hidden }; delete hidden[collectionId]
      onChange({ ...preferences, hidden })
    },
    reorder: (ids: string[]) => onChange({ hidden: preferences?.hidden ?? {}, ...preferences, order: { ...preferences?.order, [collectionId]: ids } }),
    resetOrder: () => onChange({ hidden: preferences?.hidden ?? {}, ...preferences, order: { ...preferences?.order, [collectionId]: [] } }),
  }), [collectionId, hidden, onChange, preferences])
}

// usePreferenceColumnVisibility binds one collection key to the shared
// preferences document. The global coordinator serializes every writer and
// this mutator replaces only hidden[collectionId], so another collection or
// preference section can never be overwritten by a stale local snapshot.
export function usePreferenceColumnVisibility(collectionId: string, legacyCollectionId?: string): ColumnVisibilityState {
  const queryClient = useQueryClient()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal), staleTime: 60_000 })
  const [localColumns, setLocalColumns] = useState<{ id: string; value: ColumnPreferences }>()
  const [saveError, setSaveError] = useState('')

  const onChange = useCallback((next: ColumnPreferences) => {
    const hidden = next.hidden[collectionId]
    const order = next.order?.[collectionId]
    setLocalColumns({ id: collectionId, value: next })
    setSaveError('')
    void mutatePreferences((current) => {
      const merged = structuredClone(current)
      merged.columns = {
        ...current.columns,
        hidden: {
          ...(current.columns?.hidden ?? {}),
        },
        order: { ...current.columns?.order },
      }
      if (hidden === undefined) delete merged.columns.hidden[collectionId]
      else merged.columns.hidden[collectionId] = hidden
      if (order === undefined) delete merged.columns.order![collectionId]
      else merged.columns.order![collectionId] = order
      return merged
    }).then((saved) => {
      queryClient.setQueryData(['preferences'], saved)
      setSaveError('')
    }).catch(() => {
      // Keep the optimistic in-memory visibility, but expose that persistence
      // failed so the click is never silently ignored.
      setSaveError('Columns changed locally, but preferences could not be saved. Retry or reload to reconcile it.')
    })
  }, [collectionId, queryClient])
  const storedColumns = localColumns?.id === collectionId ? localColumns.value : preferences.data?.columns
  const effectiveColumns = useMemo(() => {
    if (!legacyCollectionId || !storedColumns) return storedColumns
    const next = { ...storedColumns, hidden: { ...storedColumns.hidden }, order: { ...storedColumns.order } }
    if (!Object.hasOwn(next.hidden, collectionId) && Object.hasOwn(next.hidden, legacyCollectionId)) next.hidden[collectionId] = next.hidden[legacyCollectionId]
    if (!Object.hasOwn(next.order, collectionId) && Object.hasOwn(next.order, legacyCollectionId)) next.order[collectionId] = next.order[legacyCollectionId]
    return next
  }, [collectionId, legacyCollectionId, storedColumns])
  const state = useColumnVisibility(collectionId, effectiveColumns, onChange)
  return { ...state, error: saveError || undefined }
}
