import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'

import {
  getNamespaceScopes,
  getSession,
  selectNamespaceScope,
  type SelectionSummary,
} from '../api/client'
import { Button } from './ui'
import { StatePanel } from './StatePanel'

export function DefaultScopeGate({ selection, selectionPending = false, children }: { selection: SelectionSummary | null; selectionPending?: boolean; children: ReactNode }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const attemptedGeneration = useRef<string | null>(null)
  const needsSavedScope = Boolean(selection && selection.scopeSource !== 'cli' && selection.scopeId === null)
  const scopes = useQuery({
    queryKey: ['namespace-scopes', selection?.generation],
    queryFn: ({ signal }) => getNamespaceScopes({ limit: 100 }, signal),
    enabled: needsSavedScope,
    retry: false,
  })
  const defaultScope = scopes.data?.find((scope) => scope.isDefault) ?? null
  const session = useQuery({
    queryKey: ['session'],
    queryFn: ({ signal }) => getSession(signal),
    enabled: needsSavedScope && defaultScope !== null,
    staleTime: 5 * 60_000,
    retry: false,
  })
  const activate = useMutation({
    mutationFn: async () => {
      if (!selection || !defaultScope || !session.data) throw new Error('Default scope activation is not ready.')
      return selectNamespaceScope(defaultScope.id, { expectedGeneration: selection.generation }, session.data.csrfToken)
    },
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: ['session'] })
      await queryClient.invalidateQueries({ queryKey: ['local-status'] })
    },
  })

  useEffect(() => {
    if (!selection || !defaultScope || !session.data || activate.isPending) return
    if (attemptedGeneration.current === selection.generation) return
    attemptedGeneration.current = selection.generation
    activate.mutate()
  }, [activate, defaultScope, selection, session.data])

  if (selectionPending) {
    return <StatePanel kind="loading" title="Loading the active context">KubePeep is resolving the required namespace universe before resources are requested.</StatePanel>
  }
  if (!needsSavedScope || location.pathname === '/namespaces') return <>{children}</>
  if (scopes.isPending || defaultScope && (session.isPending || activate.isPending)) {
    return <StatePanel kind="loading" title="Activating the default scope">KubePeep is restoring the required namespace universe for this context.</StatePanel>
  }
  if (scopes.isError || session.isError || activate.isError) {
    return (
      <StatePanel
        kind="error"
        title="Default scope could not be activated"
        action={<Button onClick={() => { attemptedGeneration.current = null; void scopes.refetch() }}>Retry activation</Button>}
      >Open Namespace Scopes to verify the saved default and retry without broadening cluster access.</StatePanel>
    )
  }
  if (!defaultScope) {
    const hasScopes = (scopes.data?.length ?? 0) > 0
    return (
      <StatePanel
        kind="empty"
        title={hasScopes ? 'Choose a default namespace scope' : 'Create a default namespace scope'}
        action={<Button onClick={() => navigate('/namespaces')}>{hasScopes ? 'Choose Default Scope' : 'Create Namespace Scope'}</Button>}
      >KubePeep requires an explicit default for this context and will not assume access to all namespaces.</StatePanel>
    )
  }
  return <>{children}</>
}
