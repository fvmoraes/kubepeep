import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Star } from 'lucide-react'
import { useState } from 'react'

import { APIError, getPreferences, type FavoriteItem, type FavoriteKind } from '../api/client'
import { mutatePreferences } from '../api/preferences'
import { Button } from './ui'

function favoriteId() {
  const uuid = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`
  return `fav_${uuid.replace(/-/g, '').slice(0, 16)}`
}

function matchesFavorite(item: FavoriteItem, kind: FavoriteKind, namespace: string | undefined, name: string) {
  return item.kind === kind && item.namespace === namespace && item.name === name
}

export interface FavoriteButtonProps {
  kind: FavoriteKind
  /** Undefined for cluster-scoped targets (V6-03). */
  namespace?: string
  name: string
  generation?: string
  label?: string
}

export function FavoriteButton({ kind, namespace, name, generation, label = 'resource' }: FavoriteButtonProps) {
  const queryClient = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const preferences = useQuery({
    queryKey: ['preferences'],
    queryFn: ({ signal }) => getPreferences(signal),
    staleTime: 60_000,
  })
  const items = preferences.data?.favorites?.items ?? []
  const isFavorite = items.some((item) => matchesFavorite(item, kind, namespace, name))

  const toggle = useMutation({
    mutationFn: () => mutatePreferences((current) => {
      const currentItems = current.favorites?.items ?? []
      const currentlyFavorite = currentItems.some((item) => matchesFavorite(item, kind, namespace, name))
      const nextItems = currentlyFavorite
        ? currentItems.filter((item) => !matchesFavorite(item, kind, namespace, name))
        : [...currentItems, { id: favoriteId(), kind, namespace: namespace || undefined, name }].slice(-50)
      return { ...current, favorites: { version: 1, items: nextItems } }
    }, {
      validateSession: (session) => {
        if (generation && session.generation !== generation) {
          throw new APIError(409, { code: 'GENERATION_CHANGED', message: 'The active selection changed before the action.' })
        }
      },
    }),
    onSuccess: (saved) => {
      setError(null)
      queryClient.setQueryData(['preferences'], saved)
      void queryClient.invalidateQueries({ queryKey: ['preferences'] })
    },
    onError: (cause) => {
      setError(cause instanceof APIError ? cause.message : 'The favorite could not be saved.')
    },
  })

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        aria-pressed={isFavorite}
        aria-label={isFavorite ? `Remove ${name} from favorites` : `Add ${name} to favorites`}
        title={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
        disabled={!preferences.data || toggle.isPending}
        onClick={() => toggle.mutate()}
      >
        <Star size={14} aria-hidden="true" fill={isFavorite ? 'currentColor' : 'none'} />
        <span className="sr-only">{isFavorite ? `Remove ${label} from favorites` : `Add ${label} to favorites`}</span>
      </Button>
      {error ? <span className="text-xs text-kp-red" role="status">{error}</span> : null}
    </>
  )
}
