import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import { APIError, createIdempotencyKey, getPreferences, getSession, putPreferences } from '../api/client'
import { Button, Input, Select } from './ui'
import type { Preferences, SavedFilterCollection } from '../api/types'

function errorMessage(error: unknown): string {
  if (error instanceof APIError) return `${error.code}: ${error.message}`
  return error instanceof Error ? error.message : 'The saved filter could not be updated.'
}

function validName(value: string): boolean {
  const length = Array.from(value.trim()).length
  return length >= 1 && length <= 80
}

export function SavedFilterControls({
  collection,
  generation,
  currentQuery,
  onApply,
}: {
  collection: SavedFilterCollection
  generation: string
  currentQuery: Record<string, unknown>
  onApply: (query: Record<string, unknown>) => void
}) {
  const queryClient = useQueryClient()
  const preferences = useQuery({ queryKey: ['preferences'], queryFn: ({ signal }) => getPreferences(signal) })
  const [name, setName] = useState('')
  const [selectedID, setSelectedID] = useState('')
  const [message, setMessage] = useState('')
  const activeRequest = useRef<AbortController | null>(null)
  const filters = preferences.data?.filters?.[collection]?.items ?? []
  const trimmedName = name.trim()
  const canSave = Boolean(preferences.data) && validName(name) && Object.keys(currentQuery).length > 0 && filters.length < 50

  useEffect(() => () => {
    activeRequest.current?.abort()
    activeRequest.current = null
  }, [generation])

  const save = useMutation({
    mutationFn: async () => {
      const current = preferences.data
      if (!current) throw new Error('Preferences are unavailable.')
      const controller = new AbortController()
      activeRequest.current?.abort()
      activeRequest.current = controller
      try {
        const session = await getSession(controller.signal)
        if (session.generation !== generation) {
          throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed before the filter was saved.' })
        }
        const id = createIdempotencyKey()
        const currentItems = current.filters?.[collection]?.items ?? []
        const next: Preferences = {
          ...current,
          filters: {
            ...current.filters,
            [collection]: {
              version: 1,
              items: [...currentItems, { id, name: trimmedName, query: structuredClone(currentQuery) }],
            },
          },
        }
        return { id, preferences: await putPreferences(next, session.csrfToken, controller.signal) }
      } finally {
        if (activeRequest.current === controller) activeRequest.current = null
      }
    },
    onSuccess: ({ id, preferences: saved }) => {
      queryClient.setQueryData(['preferences'], saved)
      setSelectedID(id)
      setName('')
      setMessage('Current bounded filter saved.')
    },
  })

  function apply() {
    const filter = filters.find((item) => item.id === selectedID)
    if (!filter) return
    onApply(structuredClone(filter.query))
    setMessage(`Applied “${filter.name}”.`)
  }

  return (
    <section aria-label={`${collection} saved filters`} className="grid gap-2.5 rounded-xl border border-kp-overlay-0 bg-kp-surface-0/60 px-3 py-2.5 md:grid-cols-2">
      <div className="flex items-end gap-2">
        <label className="grid flex-1 gap-1">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Saved filter</span>
          <Select value={selectedID} disabled={preferences.isPending || filters.length === 0} onChange={(event) => setSelectedID(event.target.value)}>
            <option value="">{preferences.isPending ? 'Loading filters…' : filters.length === 0 ? 'No saved filters' : 'Choose a filter'}</option>
            {filters.map((filter) => <option key={filter.id} value={filter.id}>{filter.name}</option>)}
          </Select>
        </label>
        <Button variant="secondary" size="sm" disabled={selectedID === ''} onClick={apply}>Apply saved filter</Button>
      </div>
      <div className="flex items-end gap-2">
        <label className="grid flex-1 gap-1">
          <span className="text-2xs uppercase tracking-wider text-kp-overlay-text">Save current filter as</span>
          <Input value={name} maxLength={80} placeholder="My bounded view" onChange={(event) => { setName(event.target.value); setMessage('') }} />
        </label>
        <Button variant="secondary" size="sm" disabled={!canSave || save.isPending} onClick={() => save.mutate()}>{save.isPending ? 'Saving…' : 'Save current filter'}</Button>
      </div>
      {preferences.isError ? <p className="text-xs text-kp-red md:col-span-2">Saved filters unavailable: {errorMessage(preferences.error)}</p> : null}
      {!validName(name) && name !== '' ? <p className="text-xs text-kp-red md:col-span-2">Filter name must contain 1–80 characters.</p> : null}
      {filters.length >= 50 ? <p className="text-xs text-kp-red md:col-span-2">This collection already has the maximum 50 saved filters.</p> : null}
      {save.isError ? <p className="text-xs text-kp-red md:col-span-2">{errorMessage(save.error)}</p> : null}
      {message ? <p className="text-xs text-kp-overlay-text md:col-span-2" role="status">{message}</p> : null}
    </section>
  )
}
